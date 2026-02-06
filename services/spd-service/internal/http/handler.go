package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"esppd.local/shared/cryptox"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"esppd.local/spd-service/internal/repo"
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
	g := r.Group("/spds", httpx.JWTAuth(h.verifier, true))
	g.Post("/", h.Create)
	g.Get("/", h.List)
	g.Get("/:id", h.Get)
	g.Post("/:id/submit", h.Submit)
}

type createRequest struct {
	NomorSurat string `json:"nomor_surat"`
	EmployeeID *int64 `json:"employee_id"`
	UnitID     int64  `json:"unit_id"`
	Purpose    string `json:"purpose"`
	TotalCost  string `json:"total_cost"`
}

type spdResponse struct {
	ID         int64  `json:"id"`
	NomorSurat string `json:"nomor_surat"`
	EmployeeID *int64 `json:"employee_id"`
	UnitID     int64  `json:"unit_id"`
	Purpose    string `json:"purpose"`
	TotalCost  string `json:"total_cost"`
	Status     string `json:"status"`
	CreatedBy  int64  `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid json")
	}
	if req.NomorSurat == "" || req.UnitID == 0 {
		return fiber.NewError(http.StatusBadRequest, "nomor_surat and unit_id required")
	}

	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	purposeEnc, err := h.aesgcm.EncryptString(req.Purpose, []byte("spds:purpose"))
	if err != nil {
		return err
	}
	totalEnc, err := h.aesgcm.EncryptString(req.TotalCost, []byte("spds:total_cost"))
	if err != nil {
		return err
	}

	row, err := h.repo.CreateSPD(c.Context(), a.UserID, a.Role, req.NomorSurat, req.UnitID, req.EmployeeID, purposeEnc, totalEnc)
	if err != nil {
		return err
	}
	resp, err := h.toResponse(row)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(resp)
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

	row, err := h.repo.GetSPD(c.Context(), a.UserID, a.Role, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(http.StatusNotFound, "not found")
		}
		return err
	}
	resp, err := h.toResponse(row)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

func (h *Handler) List(c *fiber.Ctx) error {
	limit := clampInt(parseInt(c.Query("limit"), 20), 1, 100)
	offset := clampInt(parseInt(c.Query("offset"), 0), 0, 10_000)

	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	rows, err := h.repo.ListSPDs(c.Context(), a.UserID, a.Role, limit, offset)
	if err != nil {
		return err
	}
	out := make([]spdResponse, 0, len(rows))
	for i := range rows {
		r := rows[i]
		resp, err := h.toResponse(&r)
		if err != nil {
			return err
		}
		out = append(out, resp)
	}
	return c.JSON(fiber.Map{
		"items":  out,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) Submit(c *fiber.Ctx) error {
	id, err := httpx.ParseIDParam(c, "id")
	if err != nil {
		return err
	}
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	row, err := h.repo.SubmitSPD(c.Context(), a.UserID, a.Role, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(http.StatusBadRequest, "cannot submit")
		}
		return err
	}

	if h.nc != nil {
		evt := SPDSubmittedEvent{SPDID: row.ID, UnitID: row.UnitID, UserID: a.UserID, SubmittedAt: time.Now().UTC()}
		if b, err := json.Marshal(evt); err == nil {
			_ = h.nc.Publish("spd.submitted", b)
		}
	}

	resp, err := h.toResponse(row)
	if err != nil {
		return err
	}
	return c.JSON(resp)
}

type SPDSubmittedEvent struct {
	SPDID       int64     `json:"spd_id"`
	UnitID      int64     `json:"unit_id"`
	UserID      int64     `json:"user_id"`
	SubmittedAt time.Time `json:"submitted_at"`
}

func (h *Handler) toResponse(r *repo.SPDRow) (spdResponse, error) {
	purpose, err := h.aesgcm.DecryptString(r.PurposeEnc, []byte("spds:purpose"))
	if err != nil {
		return spdResponse{}, err
	}
	total, err := h.aesgcm.DecryptString(r.TotalCostEnc, []byte("spds:total_cost"))
	if err != nil {
		return spdResponse{}, err
	}

	return spdResponse{
		ID:         r.ID,
		NomorSurat: r.NomorSurat,
		EmployeeID: r.EmployeeID,
		UnitID:     r.UnitID,
		Purpose:    purpose,
		TotalCost:  total,
		Status:     r.Status,
		CreatedBy:  r.CreatedBy,
		CreatedAt:  r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  r.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return i
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
