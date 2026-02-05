package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"esppd.local/document-service/internal/http"
	"esppd.local/document-service/internal/repo"
	"esppd.local/shared/config"
	"esppd.local/shared/cryptox"
	"esppd.local/shared/db"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"esppd.local/shared/logging"
	"esppd.local/shared/metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

func main() {
	common := config.LoadCommon("document-service", "DOCUMENT_SERVICE_PORT", 8004)
	pg := config.LoadPostgres()
	jwtCfg := config.LoadJWT()
	cryptoCfg := config.LoadCrypto()
	natsCfg := config.LoadNATS()
	minioCfg := config.LoadMinIO()

	_ = logging.New(common.ServiceName, common.LogLevel, common.Env)

	if cryptoCfg.DataKeyBase64 == "" || strings.HasPrefix(cryptoCfg.DataKeyBase64, "REPLACE_") {
		log.Fatal().Msg("DATA_ENC_KEY_BASE64 is required; run scripts/gen-secrets.ps1")
	}
	aesgcm, err := cryptox.NewAESGCMFromBase64(cryptoCfg.DataKeyBase64)
	if err != nil {
		log.Fatal().Err(err).Msg("init encryption")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, pg.DSN(), pg.MaxConns)
	if err != nil {
		log.Fatal().Err(err).Msg("connect postgres")
	}
	defer pool.Close()

	verifier, err := jwtx.NewVerifierFromFile(jwtCfg.PublicKey, jwtCfg.Issuer, jwtCfg.Audience)
	if err != nil {
		log.Fatal().Err(err).Msg("load JWT public key")
	}

	var nc *nats.Conn
	nc, err = nats.Connect(natsCfg.URL, nats.Timeout(3*time.Second))
	if err != nil {
		log.Warn().Err(err).Msg("NATS not available; async jobs disabled")
	}
	if nc != nil {
		defer nc.Drain()
	}

	mc, err := minio.New(minioCfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioCfg.AccessKey, minioCfg.SecretKey, ""),
		Secure: minioCfg.UseSSL,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("connect minio")
	}

	repository := repo.New(pool)
	h := http.NewHandler(repository, verifier, aesgcm, nc, mc, minioCfg.Bucket)
	if nc != nil {
		h.StartNATSConsumers(ctx)
	}

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
