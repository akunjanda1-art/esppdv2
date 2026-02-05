package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esppd.local/shared/config"
	"esppd.local/shared/db"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"esppd.local/shared/logging"
	"esppd.local/shared/metrics"
	"esppd.local/auth-service/internal/http"
	"esppd.local/auth-service/internal/repo"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/rs/zerolog/log"
)

func main() {
	common := config.LoadCommon("auth-service", "AUTH_SERVICE_PORT", 8001)
	pg := config.LoadPostgres()
	jwtCfg := config.LoadJWT()

	_ = logging.New(common.ServiceName, common.LogLevel, common.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, pg.DSN(), pg.MaxConns)
	if err != nil {
		log.Fatal().Err(err).Msg("connect postgres")
	}
	defer pool.Close()

	signer, err := jwtx.NewSignerFromFileWithPassphrase(jwtCfg.PrivateKey, jwtCfg.PrivateKeyPassphrase, jwtCfg.Issuer, jwtCfg.Audience, jwtCfg.AccessTTL, jwtCfg.RefreshTTL)
	if err != nil {
		log.Fatal().Err(err).Msg("load JWT private key")
	}
	verifier, err := jwtx.NewVerifierFromFile(jwtCfg.PublicKey, jwtCfg.Issuer, jwtCfg.Audience)
	if err != nil {
		log.Fatal().Err(err).Msg("load JWT public key")
	}

	repository := repo.New(pool)
	h := http.NewHandler(repository, signer, verifier, jwtCfg.AccessTTL, jwtCfg.RefreshTTL)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return httpx.WriteError(c, err)
		},
	})
	app.Use(requestid.New())
	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(limiter.New(limiter.Config{Max: common.RateLimitPerMin, Expiration: time.Minute}))

	m := metrics.NewHTTPMetrics(common.ServiceName)
	app.Use(m.Middleware())

	app.Get("/healthz", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	app.Get("/metrics", metrics.Handler())

	api := app.Group("/v1")
	h.RegisterRoutes(api)

	go func() {
		addr := fmt.Sprintf(":%d", common.Port)
		log.Info().Str("addr", addr).Msg("listening")
		if err := app.Listen(addr); err != nil {
			log.Error().Err(err).Msg("server stopped")
		}
	}()

	<-ctx.Done()
	_, cancel := context.WithTimeout(context.Background(), common.ShutdownTimeout)
	defer cancel()
	_ = app.Shutdown()
	log.Info().Msg("shutdown")
}
