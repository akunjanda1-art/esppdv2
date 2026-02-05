package httpx

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
)

const (
	localUserID    = "user_id"
	localUserRole  = "user_role"
	localTokenType = "token_type"
	localTokenID   = "token_id"
)

type AuthInfo struct {
	UserID    int64
	Role      string
	TokenType string
	TokenID   string
}

func AuthFromLocals(c *fiber.Ctx) (AuthInfo, bool) {
	uidV := c.Locals(localUserID)
	roleV := c.Locals(localUserRole)
	ttV := c.Locals(localTokenType)
	jtiV := c.Locals(localTokenID)

	uid, ok := uidV.(int64)
	if !ok {
		return AuthInfo{}, false
	}
	role, _ := roleV.(string)
	tt, _ := ttV.(string)
	jti, _ := jtiV.(string)

	return AuthInfo{UserID: uid, Role: role, TokenType: tt, TokenID: jti}, true
}

func JWTAuth(verifier *jwtx.Verifier, requireAccess bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		h := c.Get("Authorization")
		if h == "" {
			return fiber.NewError(http.StatusUnauthorized, "missing Authorization header")
		}
		parts := strings.SplitN(h, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return fiber.NewError(http.StatusUnauthorized, "invalid Authorization header")
		}

		claims, err := verifier.ParseAndValidate(parts[1])
		if err != nil {
			return fiber.NewError(http.StatusUnauthorized, "invalid token")
		}
		if requireAccess && claims.TokenType != jwtx.TokenTypeAccess {
			return fiber.NewError(http.StatusUnauthorized, "access token required")
		}

		c.Locals(localUserID, claims.UserID)
		c.Locals(localUserRole, claims.Role)
		c.Locals(localTokenType, claims.TokenType)
		c.Locals(localTokenID, claims.ID)

		return c.Next()
	}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func WriteError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return c.Status(fe.Code).JSON(ErrorResponse{Error: fe.Message})
	}
	return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{Error: "internal error"})
}

func ParseIDParam(c *fiber.Ctx, name string) (int64, error) {
	idStr := c.Params(name)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fiber.NewError(http.StatusBadRequest, "invalid id")
	}
	return id, nil
}
