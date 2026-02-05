package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"esppd.local/audit-service/internal/repo"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	repo     *repo.Repo
	verifier *jwtx.Verifier
	nc       *nats.Conn
}

func NewHandler(r *repo.Repo, v *jwtx.Verifier, nc *nats.Conn) *Handler {
	return &Handler{repo: r, verifier: v, nc: nc}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/audit", httpx.JWTAuth(h.verifier, true))
	g.Get("/logs", h.QueryLogs)
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

func (h *Handler) StartNATSConsumers(ctx context.Context) {
	sub, err := h.nc.Subscribe("audit.log", func(m *nats.Msg) {
		var evt auditEvent
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			log.Warn().Err(err).Msg("invalid audit.log")
			return
		}

		var rid *int64
		if evt.ResourceID != 0 {
			rid = &evt.ResourceID
		}
		ip := evt.IPAddress
		ua := evt.UserAgent

		meta := json.RawMessage(m.Data)
		in := repo.InsertInput{
			Action:       evt.Action,
			ResourceType: evt.ResourceType,
			ResourceID:   rid,
			UserID:       evt.UserID,
			Role:         evt.Role,
			IPAddress:    &ip,
			UserAgent:    &ua,
			Metadata:     meta,
			Timestamp:    evt.Timestamp,
		}
		if err := h.repo.Insert(context.Background(), 0, "SYSTEM", in); err != nil {
			log.Error().Err(err).Msg("insert audit log")
		}
	})
	if err != nil {
		log.Warn().Err(err).Msg("subscribe audit.log")
		return
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()
}

func (h *Handler) QueryLogs(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if !canReadAudit(a.Role) {
		return fiber.NewError(http.StatusForbidden, "forbidden")
	}

	limit := clampInt(parseInt(c.Query("limit"), 100), 1, 500)
	offset := clampInt(parseInt(c.Query("offset"), 0), 0, 100_000)

	var userID *int64
	if u := c.Query("user_id"); u != "" {
		if v, err := strconv.ParseInt(u, 10, 64); err == nil {
			userID = &v
		}
	}

	var from *time.Time
	if s := c.Query("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			from = &t
		}
	}
	var to *time.Time
	if s := c.Query("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			to = &t
		}
	}

	q := repo.QueryInput{
		Action:   c.Query("action"),
		Resource: c.Query("resource_type"),
		UserID:   userID,
		From:     from,
		To:       to,
		Limit:    limit,
		Offset:   offset,
	}

	items, err := h.repo.Query(c.Context(), a.UserID, a.Role, q)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items, "limit": limit, "offset": offset})
}

func canReadAudit(role string) bool {
	switch role {
	case "SUPER_ADMIN", "AUDITOR":
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
