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

type BudgetRow struct {
	ID          int64
	UnitID      int64
	AmountEnc   []byte
	SourceEnc   []byte
	Description string
	CreatedBy   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Repo) CreateBudget(ctx context.Context, userID int64, role string, unitID int64, amountEnc, sourceEnc []byte, description string) (*BudgetRow, error) {
	var out *BudgetRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO budgets (unit_id, amount_enc, source_enc, description, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, unit_id, amount_enc, source_enc, description, created_by, created_at, updated_at
		`, unitID, amountEnc, sourceEnc, description, userID)
		var b BudgetRow
		if err := row.Scan(&b.ID, &b.UnitID, &b.AmountEnc, &b.SourceEnc, &b.Description, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return err
		}
		out = &b
		return nil
	})
	return out, err
}

func (r *Repo) GetBudget(ctx context.Context, userID int64, role string, id int64) (*BudgetRow, error) {
	var out *BudgetRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{ReadOnly: true}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			SELECT id, unit_id, amount_enc, source_enc, description, created_by, created_at, updated_at
			FROM budgets
			WHERE id = $1 AND deleted_at IS NULL
		`, id)
		var b BudgetRow
		if err := row.Scan(&b.ID, &b.UnitID, &b.AmountEnc, &b.SourceEnc, &b.Description, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return err
		}
		out = &b
		return nil
	})
	return out, err
}
