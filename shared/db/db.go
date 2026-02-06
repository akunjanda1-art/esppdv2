package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	*pgxpool.Pool
}

func NewPool(ctx context.Context, dsn string, maxConns int32) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Ping(ctxPing); err != nil {
		p.Close()
		return nil, err
	}

	return &Pool{Pool: p}, nil
}

// WithTx runs fn inside a transaction.
func (p *Pool) WithTx(ctx context.Context, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := p.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetRLSContext sets per-transaction settings used by Postgres RLS policies.
func SetRLSContext(ctx context.Context, tx pgx.Tx, userID int64, role string) error {
	uid := strconv.FormatInt(userID, 10)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", uid); err != nil {
		return fmt.Errorf("set app.current_user_id: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.user_role', $1, true)", role); err != nil {
		return fmt.Errorf("set app.user_role: %w", err)
	}
	return nil
}
