package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"esppd.local/budget-service/internal/repo"
	"esppd.local/shared/cryptox"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
)

type Handler struct {
	repo     *repo.Repo
	verifier *jwtx.Verifier
	aesgcm   *cryptox.AESGCM
	nc       *nats.Conn
}

func NewHandler(r *repo.Repo, v *jwtx.Verifier, aes *cryptox.AESGCM, nc *nats.Conn) *Handler {
	return &Handler{repo: r, verifier: v, aesgcm: aes, nc: nc}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/budgets", httpx.JWTAuth(h.verifier, true))
	g.Post("/", h.Create)
	g.Get("/:id", h.Get)
}

type createRequest struct {
	UnitID      int64  `json:"unit_id"`
	Amount      string `json:"amount"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

type budgetResponse struct {
	ID          int64  `json:"id"`
	UnitID      int64  `json:"unit_id"`
	Amount      string `json:"amount"`
	Source      string `json:"source"`
	Description string `json:"description"`
	CreatedBy   int64  `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid json")
	}
	if req.UnitID == 0 {
		return fiber.NewError(http.StatusBadRequest, "unit_id required")
	}
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if !canManageBudget(a.Role) {
		return fiber.NewError(http.StatusForbidden, "forbidden")
	}

	amountEnc, err := h.aesgcm.EncryptString(req.Amount, []byte("budgets:amount"))
	if err != nil {
		return err
	}
	sourceEnc, err := h.aesgcm.EncryptString(req.Source, []byte("budgets:source"))
	if err != nil {
		return err
	}

	row, err := h.repo.CreateBudget(c.Context(), a.UserID, a.Role, req.UnitID, amountEnc, sourceEnc, req.Description)
	if err != nil {
		return err
	}

	h.publishAudit("CREATE_BUDGET", "budget", row.ID, a, c)

	return c.Status(http.StatusCreated).JSON(h.toResponse(row, a.Role))
}

func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := httpx.ParseIDParam(c, "id")
	if err != nil {
		return err
	}
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	row, err := h.repo.GetBudget(c.Context(), a.UserID, a.Role, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(http.StatusNotFound, "not found")
		}
		return err
	}

	h.publishAudit("VIEW_BUDGET", "budget", row.ID, a, c)

	return c.JSON(h.toResponse(row, a.Role))
}

func (h *Handler) toResponse(r *repo.BudgetRow, role string) budgetResponse {
	amount := "***"
	source := "***"
	if canViewBudgetAmounts(role) {
		if v, err := h.aesgcm.DecryptString(r.AmountEnc, []byte("budgets:amount")); err == nil {
			amount = v
		}
		if v, err := h.aesgcm.DecryptString(r.SourceEnc, []byte("budgets:source")); err == nil {
			source = v
		}
	}

	return budgetResponse{
		ID:          r.ID,
		UnitID:      r.UnitID,
		Amount:      amount,
		Source:      source,
		Description: r.Description,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func canManageBudget(role string) bool {
	switch strings.ToUpper(role) {
	case "SUPER_ADMIN", "BUDGET_ADMIN", "FINANCE":
		return true
	default:
		return false
	}
}

func canViewBudgetAmounts(role string) bool {
	return canManageBudget(role)
}

type auditEvent struct {
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   int64     `json:"resource_id"`
	UserID       int64     `json:"user_id"`
	Role         string    `json:"role"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	Timestamp    time.Time `json:"timestamp"`
}

func (h *Handler) publishAudit(action, resourceType string, resourceID int64, a httpx.AuthInfo, c *fiber.Ctx) {
	if h.nc == nil {
		return
	}
	evt := auditEvent{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		UserID:       a.UserID,
		Role:         a.Role,
		IPAddress:    c.IP(),
		UserAgent:    c.Get("User-Agent"),
		Timestamp:    time.Now().UTC(),
	}
	if b, err := json.Marshal(evt); err == nil {
		_ = h.nc.Publish("audit.log", b)
	}
}
