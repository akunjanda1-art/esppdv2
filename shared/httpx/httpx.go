package httpx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/redis/go-redis/v9"
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

type RateLimitConfig struct {
	Service string
	Redis   *redis.Client
	Max     int
	Window  time.Duration

	// Key defaults to client IP address.
	Key func(*fiber.Ctx) string
	// Skip defaults to allowing /healthz and /metrics without rate limiting.
	Skip func(*fiber.Ctx) bool
}

var rateLimitLua = redis.NewScript(`
local key = KEYS[1]
local ttl_ms = tonumber(ARGV[1])
local max = tonumber(ARGV[2])

local current = redis.call("INCR", key)
if current == 1 then
  redis.call("PEXPIRE", key, ttl_ms)
end

local pttl = redis.call("PTTL", key)
local allowed = 1
if current > max then
  allowed = 0
end
return {current, allowed, pttl}
`)

// RateLimit provides a distributed, Redis-backed rate limiter with an in-memory fallback.
// It is safe to apply globally; by default it skips /healthz and /metrics.
func RateLimit(cfg RateLimitConfig) fiber.Handler {
	if cfg.Max <= 0 {
		cfg.Max = 120
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.Key == nil {
		cfg.Key = func(c *fiber.Ctx) string { return c.IP() }
	}
	if cfg.Skip == nil {
		cfg.Skip = func(c *fiber.Ctx) bool {
			switch c.Path() {
			case "/healthz", "/metrics":
				return true
			default:
				return false
			}
		}
	}

	// Fallback to process-local limiter if Redis is not configured.
	if cfg.Redis == nil {
		return limiter.New(limiter.Config{
			Max:        cfg.Max,
			Expiration: cfg.Window,
			KeyGenerator: func(c *fiber.Ctx) string {
				return cfg.Service + ":" + cfg.Key(c)
			},
			Next: cfg.Skip,
		})
	}

	return func(c *fiber.Ctx) error {
		if cfg.Skip != nil && cfg.Skip(c) {
			return c.Next()
		}

		identity := cfg.Key(c)
		if identity == "" {
			identity = "unknown"
		}
		key := "rl:" + cfg.Service + ":" + identity

		ctxRL, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		res, err := rateLimitLua.Run(ctxRL, cfg.Redis, []string{key}, cfg.Window.Milliseconds(), cfg.Max).Result()
		if err != nil {
			// Fail-open: if Redis is down, don't take the whole system down.
			return c.Next()
		}

		arr, ok := res.([]any)
		if !ok || len(arr) < 3 {
			return c.Next()
		}

		cur, _ := toInt64(arr[0])
		allowed, _ := toInt64(arr[1])
		pttl, _ := toInt64(arr[2])

		remaining := int64(cfg.Max) - cur
		if remaining < 0 {
			remaining = 0
		}
		c.Set("X-RateLimit-Limit", strconv.Itoa(cfg.Max))
		c.Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if allowed == 0 {
			retryAfter := int64(math.Ceil(float64(pttl) / 1000.0))
			if retryAfter < 0 {
				retryAfter = 0
			}
			c.Set("Retry-After", strconv.FormatInt(retryAfter, 10))
			return fiber.NewError(http.StatusTooManyRequests, fmt.Sprintf("rate limit exceeded (retry in %ds)", retryAfter))
		}

		return c.Next()
	}
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case uint64:
		return int64(t), true
	case string:
		i, err := strconv.ParseInt(t, 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}
