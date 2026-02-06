package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"esppd.local/notification-service/internal/repo"
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
	g := r.Group("/notifications", httpx.JWTAuth(h.verifier, true))
	g.Post("/test", h.SendTest)
}

type sendRequest struct {
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type notificationEvent struct {
	ID          string    `json:"id"`
	Channel     string    `json:"channel"`
	Recipient   string    `json:"recipient"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	RequestedBy int64     `json:"requested_by"`
	Role        string    `json:"role"`
	RequestedAt time.Time `json:"requested_at"`
}

func (h *Handler) SendTest(c *fiber.Ctx) error {
	var req sendRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid json")
	}
	if req.Channel == "" {
		req.Channel = "email"
	}
	req.Channel = strings.ToLower(strings.TrimSpace(req.Channel))
	if req.Recipient == "" {
		return fiber.NewError(http.StatusBadRequest, "recipient required")
	}

	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	id, err := newID(12)
	if err != nil {
		return err
	}
	evt := notificationEvent{
		ID:          id,
		Channel:     req.Channel,
		Recipient:   req.Recipient,
		Subject:     req.Subject,
		Body:        req.Body,
		RequestedBy: a.UserID,
		Role:        a.Role,
		RequestedAt: time.Now().UTC(),
	}

	b, _ := json.Marshal(evt)
	if err := h.nc.Publish("notification.send", b); err != nil {
		return err
	}
	return c.Status(http.StatusAccepted).JSON(fiber.Map{"id": id, "status": "queued"})
}

func (h *Handler) StartNATSConsumers(ctx context.Context) {
	sub, err := h.nc.Subscribe("notification.send", func(m *nats.Msg) {
		var evt notificationEvent
		if err := json.Unmarshal(m.Data, &evt); err != nil {
			log.Warn().Err(err).Msg("invalid notification.send")
			return
		}
		// Stub sender: record as SENT.
		_, err := h.repo.InsertNotification(context.Background(), evt.RequestedBy, evt.Role, evt.Channel, evt.Recipient, evt.Subject, evt.Body, "SENT", evt.RequestedBy)
		if err != nil {
			log.Error().Err(err).Str("id", evt.ID).Msg("insert notification")
			return
		}
		log.Info().Str("channel", evt.Channel).Str("recipient", evt.Recipient).Msg("notification sent (stub)")
	})
	if err != nil {
		log.Warn().Err(err).Msg("subscribe notification.send")
		return
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()
}

func newID(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
