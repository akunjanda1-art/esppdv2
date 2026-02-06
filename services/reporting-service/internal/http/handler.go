package http

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"esppd.local/reporting-service/internal/repo"
	"esppd.local/shared/cachex"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	repo     *repo.Repo
	verifier *jwtx.Verifier
	cache    *cachex.Cache
}

func NewHandler(r *repo.Repo, v *jwtx.Verifier, cache *cachex.Cache) *Handler {
	return &Handler{repo: r, verifier: v, cache: cache}
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

	cacheKey := fmt.Sprintf("spds:summary:uid:%d:role:%s:months:%d", a.UserID, a.Role, months)
	if h.cache != nil {
		var cached fiber.Map
		if ok, err := h.cache.GetJSON(c.Context(), cacheKey, &cached); err == nil && ok {
			return c.JSON(cached)
		}
	}

	byStatus, err := h.repo.SPDCountsByStatus(c.Context(), a.UserID, a.Role)
	if err != nil {
		return err
	}
	byMonth, err := h.repo.SPDMonthlyCounts(c.Context(), a.UserID, a.Role, months)
	if err != nil {
		return err
	}

	out := fiber.Map{"by_status": byStatus, "by_month": byMonth}
	if h.cache != nil {
		_ = h.cache.SetJSON(c.Context(), cacheKey, out, 15*time.Second)
	}
	return c.JSON(out)
}

func (h *Handler) SummaryCSV(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	months := clampInt(parseInt(c.Query("months"), 6), 1, 36)

	cacheKey := fmt.Sprintf("spds:summarycsv:uid:%d:role:%s:months:%d", a.UserID, a.Role, months)
	if h.cache != nil {
		if b, ok, err := h.cache.Get(c.Context(), cacheKey); err == nil && ok {
			c.Set("Content-Type", "text/csv; charset=utf-8")
			c.Set("Content-Disposition", "attachment; filename=spd_summary.csv")
			return c.Send(b)
		}
	}

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
	b := buf.Bytes()
	if h.cache != nil {
		_ = h.cache.Set(c.Context(), cacheKey, b, 15*time.Second)
	}
	return c.Send(b)
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
