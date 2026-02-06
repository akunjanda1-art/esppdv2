package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func New(serviceName, level, env string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var out io.Writer = os.Stdout
	if strings.EqualFold(env, "development") {
		out = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05.000"}
	}

	lvl := zerolog.InfoLevel
	if parsed, err := zerolog.ParseLevel(level); err == nil {
		lvl = parsed
	}

	logger := zerolog.New(out).
		With().
		Timestamp().
		Str("service", serviceName).
		Logger().
		Level(lvl)

	// Set global default too.
	log.Logger = logger

	return logger
}
