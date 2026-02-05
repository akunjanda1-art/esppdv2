package repo

import (
	"context"
	"time"

	"esppd.local/shared/db"
	"github.com/jackc/pgx/v5"
)

type Repo struct {
	pool *db.Pool
}

func New(pool *db.Pool) *Repo {
	return &Repo{pool: pool}
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type MonthlyStatusCount struct {
	Month  time.Time `json:"month"`
	Status string    `json:"status"`
	Count  int64     `json:"count"`
}

func (r *Repo) SPDCountsByStatus(ctx context.Context, userID int64, role string) ([]StatusCount, error) {
	out := make([]StatusCount, 0, 8)
	err := r.pool.WithTx(ctx, pgx.TxOptions{ReadOnly: true}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT status, COUNT(*)
			FROM spds
			WHERE deleted_at IS NULL
			GROUP BY status
			ORDER BY status
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s StatusCount
			if err := rows.Scan(&s.Status, &s.Count); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}

func (r *Repo) SPDMonthlyCounts(ctx context.Context, userID int64, role string, months int) ([]MonthlyStatusCount, error) {
	out := make([]MonthlyStatusCount, 0, months*8)
	err := r.pool.WithTx(ctx, pgx.TxOptions{ReadOnly: true}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT date_trunc('month', created_at) AS month, status, COUNT(*)
			FROM spds
			WHERE deleted_at IS NULL
				AND created_at >= (NOW() - ($1::text || ' months')::interval)
			GROUP BY 1, 2
			ORDER BY 1 DESC, 2
		`, months)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r MonthlyStatusCount
			if err := rows.Scan(&r.Month, &r.Status, &r.Count); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
