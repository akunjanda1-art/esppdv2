package http

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"esppd.local/auth-service/internal/repo"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	repo       *repo.Repo
	signer     *jwtx.Signer
	verifier   *jwtx.Verifier
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewHandler(r *repo.Repo, signer *jwtx.Signer, verifier *jwtx.Verifier, accessTTL, refreshTTL time.Duration) *Handler {
	return &Handler{repo: r, signer: signer, verifier: verifier, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	auth := r.Group("/auth")
	auth.Post("/login", h.Login)
	auth.Post("/refresh", h.Refresh)
	auth.Post("/logout", httpx.JWTAuth(h.verifier, true), h.Logout)
	auth.Get("/me", httpx.JWTAuth(h.verifier, true), h.Me)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid json")
	}
	if req.Username == "" || req.Password == "" {
		return fiber.NewError(http.StatusBadRequest, "username/password required")
	}

	user, err := h.repo.GetUserByUsername(c.Context(), req.Username)
	if err != nil {
		if h.repo.IsNoRows(err) {
			return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	accessJTI, err := newTokenID(16)
	if err != nil {
		return err
	}
	refreshJTI, err := newTokenID(32)
	if err != nil {
		return err
	}

	access, err := h.signer.NewAccessToken(user.ID, user.Role, accessJTI)
	if err != nil {
		return err
	}
	refresh, err := h.signer.NewRefreshToken(user.ID, refreshJTI)
	if err != nil {
		return err
	}

	if err := h.repo.InsertRefreshToken(c.Context(), refreshJTI, user.ID, time.Now().Add(h.refreshTTL)); err != nil {
		return err
	}

	return c.JSON(tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.accessTTL.Seconds()),
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid json")
	}
	if req.RefreshToken == "" {
		return fiber.NewError(http.StatusBadRequest, "refresh_token required")
	}

	claims, err := h.verifier.ParseAndValidate(req.RefreshToken)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "invalid refresh token")
	}
	if claims.TokenType != jwtx.TokenTypeRefresh {
		return fiber.NewError(http.StatusUnauthorized, "invalid refresh token")
	}

	rec, err := h.repo.GetRefreshToken(c.Context(), claims.ID)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "refresh token revoked")
	}
	if rec.RevokedAt != nil || time.Now().After(rec.ExpiresAt) {
		return fiber.NewError(http.StatusUnauthorized, "refresh token revoked")
	}

	user, err := h.repo.GetUserByID(c.Context(), rec.UserID)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "invalid user")
	}

	newAccessJTI, err := newTokenID(16)
	if err != nil {
		return err
	}
	newRefreshJTI, err := newTokenID(32)
	if err != nil {
		return err
	}

	access, err := h.signer.NewAccessToken(rec.UserID, user.Role, newAccessJTI)
	if err != nil {
		return err
	}
	refresh, err := h.signer.NewRefreshToken(rec.UserID, newRefreshJTI)
	if err != nil {
		return err
	}

	// Rotate refresh token.
	if err := h.repo.RevokeRefreshToken(c.Context(), rec.ID, &newRefreshJTI); err != nil {
		return err
	}
	if err := h.repo.InsertRefreshToken(c.Context(), newRefreshJTI, rec.UserID, time.Now().Add(h.refreshTTL)); err != nil {
		return err
	}

	return c.JSON(tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.accessTTL.Seconds()),
	})
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	// For now: revoke refresh token provided in body.
	var req refreshRequest
	_ = c.BodyParser(&req)
	if req.RefreshToken != "" {
		claims, err := h.verifier.ParseAndValidate(req.RefreshToken)
		if err == nil {
			_ = h.repo.RevokeRefreshToken(c.Context(), claims.ID, nil)
		}
	}
	return c.SendStatus(http.StatusNoContent)
}

func (h *Handler) Me(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	return c.JSON(fiber.Map{
		"user_id": a.UserID,
		"role":    a.Role,
	})
}

func newTokenID(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
