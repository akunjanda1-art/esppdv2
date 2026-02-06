package http

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"esppd.local/auth-service/internal/ldap"
	"esppd.local/auth-service/internal/repo"
	"esppd.local/shared/cryptox"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"esppd.local/shared/mfax"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	repo       *repo.Repo
	signer     *jwtx.Signer
	verifier   *jwtx.Verifier
	aesgcm     *cryptox.AESGCM
	ldap       *ldapauth.Client
	mfaIssuer  string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewHandler(r *repo.Repo, signer *jwtx.Signer, verifier *jwtx.Verifier, aesgcm *cryptox.AESGCM, ldap *ldapauth.Client, mfaIssuer string, accessTTL, refreshTTL time.Duration) *Handler {
	if mfaIssuer == "" {
		mfaIssuer = "esppd"
	}
	return &Handler{
		repo:       r,
		signer:     signer,
		verifier:   verifier,
		aesgcm:     aesgcm,
		ldap:       ldap,
		mfaIssuer:  mfaIssuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	auth := r.Group("/auth")
	auth.Post("/login", h.Login)
	auth.Post("/refresh", h.Refresh)
	auth.Post("/logout", httpx.JWTAuth(h.verifier, true), h.Logout)
	auth.Get("/me", httpx.JWTAuth(h.verifier, true), h.Me)
	auth.Post("/mfa/enroll", httpx.JWTAuth(h.verifier, true), h.MFAEnroll)
	auth.Post("/mfa/verify", httpx.JWTAuth(h.verifier, true), h.MFAVerify)
	auth.Delete("/mfa", httpx.JWTAuth(h.verifier, true), h.MFADisable)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	OTP      string `json:"otp,omitempty"`
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

	if !user.IsActive {
		return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	authed := false
	if h.ldap != nil && h.ldap.Enabled() && (h.ldap.Mode() == "prefer" || h.ldap.Mode() == "required") {
		ok, err := h.ldap.Authenticate(c.Context(), req.Username, req.Password)
		if err != nil {
			if h.ldap.Mode() == "required" {
				return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
			}
			log.Warn().Err(err).Msg("ldap auth error; falling back to local auth")
		} else if ok {
			authed = true
		} else if h.ldap.Mode() == "required" {
			return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
		}
	}

	if !authed {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
		}
	}

	if user.MFAEnabled {
		if len(user.MFASecretEnc) == 0 {
			return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
		}
		secret, err := h.aesgcm.DecryptString(user.MFASecretEnc, []byte("users:mfa_secret"))
		if err != nil {
			return err
		}
		if req.OTP == "" {
			return fiber.NewError(http.StatusUnauthorized, "mfa_required")
		}
		if !mfax.VerifyTOTP(secret, req.OTP, time.Now().UTC(), 6, 30*time.Second, 1) {
			return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
		}
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
	if rec.UserID != claims.UserID {
		return fiber.NewError(http.StatusUnauthorized, "refresh token revoked")
	}
	if rec.RevokedAt != nil || time.Now().After(rec.ExpiresAt) {
		return fiber.NewError(http.StatusUnauthorized, "refresh token revoked")
	}

	user, err := h.repo.GetUserByID(c.Context(), rec.UserID)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "invalid user")
	}
	if !user.IsActive {
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
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	var req refreshRequest
	_ = c.BodyParser(&req)
	if req.RefreshToken != "" {
		claims, err := h.verifier.ParseAndValidate(req.RefreshToken)
		if err == nil {
			if claims.UserID != a.UserID {
				return fiber.NewError(http.StatusForbidden, "forbidden")
			}
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

type mfaEnrollResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

func (h *Handler) MFAEnroll(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	user, err := h.repo.GetUserByID(c.Context(), a.UserID)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if user.MFAEnabled {
		return fiber.NewError(http.StatusBadRequest, "mfa already enabled")
	}

	secret, err := mfax.GenerateSecretBase32(20)
	if err != nil {
		return err
	}
	secretEnc, err := h.aesgcm.EncryptString(secret, []byte("users:mfa_secret"))
	if err != nil {
		return err
	}
	if err := h.repo.SetMFASecret(c.Context(), a.UserID, secretEnc); err != nil {
		return err
	}

	return c.JSON(mfaEnrollResponse{
		Secret:     secret,
		OTPAuthURL: mfax.OTPAuthURL(h.mfaIssuer, user.Username, secret, 6, 30*time.Second),
	})
}

type mfaVerifyRequest struct {
	Code string `json:"code"`
}

func (h *Handler) MFAVerify(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	var req mfaVerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid json")
	}
	if req.Code == "" {
		return fiber.NewError(http.StatusBadRequest, "code required")
	}

	user, err := h.repo.GetUserByID(c.Context(), a.UserID)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if len(user.MFASecretEnc) == 0 {
		return fiber.NewError(http.StatusBadRequest, "mfa not enrolled")
	}

	secret, err := h.aesgcm.DecryptString(user.MFASecretEnc, []byte("users:mfa_secret"))
	if err != nil {
		return err
	}
	if !mfax.VerifyTOTP(secret, req.Code, time.Now().UTC(), 6, 30*time.Second, 1) {
		return fiber.NewError(http.StatusUnauthorized, "invalid code")
	}

	if err := h.repo.EnableMFA(c.Context(), a.UserID); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func (h *Handler) MFADisable(c *fiber.Ctx) error {
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	user, err := h.repo.GetUserByID(c.Context(), a.UserID)
	if err != nil {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}
	if !user.MFAEnabled {
		return c.SendStatus(http.StatusNoContent)
	}

	type mfaDisableRequest struct {
		Code string `json:"code"`
	}
	var req mfaDisableRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid json")
		}
	}
	if req.Code == "" {
		return fiber.NewError(http.StatusBadRequest, "code required")
	}
	if len(user.MFASecretEnc) == 0 {
		return fiber.NewError(http.StatusBadRequest, "mfa not enrolled")
	}
	secret, err := h.aesgcm.DecryptString(user.MFASecretEnc, []byte("users:mfa_secret"))
	if err != nil {
		return err
	}
	if !mfax.VerifyTOTP(secret, req.Code, time.Now().UTC(), 6, 30*time.Second, 1) {
		return fiber.NewError(http.StatusUnauthorized, "invalid code")
	}
	if err := h.repo.DisableMFA(c.Context(), a.UserID); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func newTokenID(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
