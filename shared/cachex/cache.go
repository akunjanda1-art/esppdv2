package cachex

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb    *redis.Client
	prefix string
}

func New(rdb *redis.Client, prefix string) *Cache {
	if rdb == nil {
		return nil
	}
	return &Cache{rdb: rdb, prefix: prefix}
}

func (c *Cache) key(k string) string {
	if c.prefix == "" {
		return k
	}
	return c.prefix + ":" + k
}

func (c *Cache) Get(ctx context.Context, k string) ([]byte, bool, error) {
	if c == nil || c.rdb == nil {
		return nil, false, nil
	}
	b, err := c.rdb.Get(ctx, c.key(k)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return b, true, nil
}

func (c *Cache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Set(ctx, c.key(k), v, ttl).Err()
}

func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if c == nil || c.rdb == nil || len(keys) == 0 {
		return nil
	}
	ks := make([]string, 0, len(keys))
	for _, k := range keys {
		ks = append(ks, c.key(k))
	}
	return c.rdb.Del(ctx, ks...).Err()
}

func (c *Cache) GetJSON(ctx context.Context, k string, dst any) (bool, error) {
	b, ok, err := c.Get(ctx, k)
	if err != nil || !ok {
		return ok, err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		// Corrupt cache entry: best-effort delete.
		_ = c.Del(ctx, k)
		return false, nil
	}
	return true, nil
}

func (c *Cache) SetJSON(ctx context.Context, k string, v any, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Set(ctx, k, b, ttl)
}
