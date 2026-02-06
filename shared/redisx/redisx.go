package redisx

import (
	"context"
	"time"

	"esppd.local/shared/config"
	"github.com/redis/go-redis/v9"
)

// NewClient builds a Redis client from configuration. Call Ping() to verify connectivity.
func NewClient(cfg config.Redis) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func Ping(ctx context.Context, rdb *redis.Client) error {
	if rdb == nil {
		return nil
	}
	ctxPing, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return rdb.Ping(ctxPing).Err()
}
