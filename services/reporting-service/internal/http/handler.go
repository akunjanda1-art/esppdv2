package http

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"

	"esppd.local/reporting-service/internal/repo"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	repo     *repo.Repo
	verifier *jwtx.Verifier
}

func NewHandler(r *repo.Repo, v *jwtx.Verifier) *Handler {
	return &Handler{repo: r, verifier: v}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/reports", httpx.JWTAuth(h.verifier, true))
	g.Get("/spds/summary", h.Summary)
	g.Get("/spds/summary.csv", h.SummaryCSV)
}

func (h *Handler) Summary(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	months := clampInt(parseInt(c.Query("months"), 6), 1, 36)

	byStatus, err := h.repo.SPDCountsByStatus(c.Context(), a.UserID, a.Role)
	if err != nil {
		return err
	}
	byMonth, err := h.repo.SPDMonthlyCounts(c.Context(), a.UserID, a.Role, months)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"by_status": byStatus, "by_month": byMonth})
}

func (h *Handler) SummaryCSV(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	months := clampInt(parseInt(c.Query("months"), 6), 1, 36)

	rows, err := h.repo.SPDMonthlyCounts(c.Context(), a.UserID, a.Role, months)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"month", "status", "count"})
	for _, r := range rows {
		_ = w.Write([]string{r.Month.UTC().Format("2006-01"), r.Status, strconv.FormatInt(r.Count, 10)})
	}
	w.Flush()

	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", "attachment; filename=spd_summary.csv")
	return c.Send(buf.Bytes())
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
