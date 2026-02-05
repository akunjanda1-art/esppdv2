package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"esppd.local/approval-service/internal/repo"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	repo     *repo.Repo
	verifier *jwtx.Verifier
}

func NewHandler(r *repo.Repo, v *jwtx.Verifier) *Handler {
	return &Handler{repo: r, verifier: v}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/approvals", httpx.JWTAuth(h.verifier, true))
	g.Get("/pending", h.ListPending)
	g.Post("/:spd_id/approve", h.Approve)
	g.Post("/:spd_id/reject", h.Reject)
}

type decisionRequest struct {
	Comment *string `json:"comment"`
}

func (h *Handler) Approve(c *fiber.Ctx) error {
	spdID, err := httpx.ParseIDParam(c, "spd_id")
	if err != nil {
		return err
	}
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if !canApprove(a.Role) {
		return fiber.NewError(http.StatusForbidden, "forbidden")
	}

	var req decisionRequest
	_ = c.BodyParser(&req)

	row, err := h.repo.Approve(c.Context(), a.UserID, a.Role, spdID, req.Comment)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(http.StatusBadRequest, "no pending approval")
		}
		return err
	}
	return c.JSON(row)
}

func (h *Handler) Reject(c *fiber.Ctx) error {
	spdID, err := httpx.ParseIDParam(c, "spd_id")
	if err != nil {
		return err
	}
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if !canApprove(a.Role) {
		return fiber.NewError(http.StatusForbidden, "forbidden")
	}

	var req decisionRequest
	_ = c.BodyParser(&req)

	row, err := h.repo.Reject(c.Context(), a.UserID, a.Role, spdID, req.Comment)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fiber.NewError(http.StatusBadRequest, "no pending approval")
		}
		return err
	}
	return c.JSON(row)
}

func (h *Handler) ListPending(c *fiber.Ctx) error {
	limit := clampInt(parseInt(c.Query("limit"), 50), 1, 200)
	offset := clampInt(parseInt(c.Query("offset"), 0), 0, 10_000)

	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	items, err := h.repo.ListPending(c.Context(), a.UserID, a.Role, limit, offset)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items, "limit": limit, "offset": offset})
}

// StartNATSConsumers wires async workflows.
func (h *Handler) StartNATSConsumers(ctx context.Context, nc *nats.Conn) {
	sub, err := nc.Subscribe("spd.submitted", func(m *nats.Msg) {
		var evt struct {
			SPDID int64 `json:"spd_id"`
		}
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			log.Warn().Err(err).Msg("invalid spd.submitted event")
			return
		}
		if evt.SPDID == 0 {
			return
		}
		if err := h.repo.CreatePendingApproval(context.Background(), evt.SPDID, 1); err != nil {
			log.Error().Err(err).Int64("spd_id", evt.SPDID).Msg("create approval")
		}
	})
	if err != nil {
		log.Warn().Err(err).Msg("subscribe spd.submitted")
		return
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()
}

func canApprove(role string) bool {
	switch role {
	case "SUPER_ADMIN", "APPROVER", "FINANCE", "BUDGET_ADMIN":
		return true
	default:
		return false
	}
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
