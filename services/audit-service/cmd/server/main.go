package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esppd.local/audit-service/internal/http"
	"esppd.local/audit-service/internal/repo"
	"esppd.local/shared/config"
	"esppd.local/shared/db"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"esppd.local/shared/logging"
	"esppd.local/shared/metrics"
	"esppd.local/shared/redisx"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

func main() {
	common := config.LoadCommon("audit-service", "AUDIT_SERVICE_PORT", 8008)
	pg := config.LoadPostgres()
	redisCfg := config.LoadRedis()
	jwtCfg := config.LoadJWT()
	natsCfg := config.LoadNATS()

	_ = logging.New(common.ServiceName, common.LogLevel, common.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, pg.DSN(), pg.MaxConns)
	if err != nil {
		log.Fatal().Err(err).Msg("connect postgres")
	}
	defer pool.Close()

	rdb := redisx.NewClient(redisCfg)
	if err := redisx.Ping(ctx, rdb); err != nil {
		log.Warn().Err(err).Msg("redis unavailable; falling back to in-memory rate limit")
		rdb = nil
	} else {
		defer func() { _ = rdb.Close() }()
	}

	verifier, err := jwtx.NewVerifierFromFile(jwtCfg.PublicKey, jwtCfg.Issuer, jwtCfg.Audience)
	if err != nil {
		log.Fatal().Err(err).Msg("load JWT public key")
	}

	nc, err := nats.Connect(natsCfg.URL, nats.Timeout(3*time.Second))
	if err != nil {
		log.Fatal().Err(err).Msg("connect nats")
	}
	defer nc.Drain()

	repository := repo.New(pool)
	h := http.NewHandler(repository, verifier, nc)
	h.StartNATSConsumers(ctx)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return httpx.WriteError(c, err)
		},
	})
	app.Use(requestid.New())
	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(httpx.RateLimit(httpx.RateLimitConfig{
		Service: common.ServiceName,
		Redis:   rdb,
		Max:     common.RateLimitPerMin,
		Window:  time.Minute,
	}))

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
