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

type ApprovalRow struct {
	ID         int64
	SPDID      int64
	Step       int
	ApproverID *int64
	Status     string
	Comment    *string
	DecidedBy  *int64
	DecidedAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (r *Repo) CreatePendingApproval(ctx context.Context, spdID int64, step int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO approvals (spd_id, step, status)
		VALUES ($1, $2, 'PENDING')
		ON CONFLICT DO NOTHING
	`, spdID, step)
	return err
}

func (r *Repo) Approve(ctx context.Context, userID int64, role string, spdID int64, comment *string) (*ApprovalRow, error) {
	var out *ApprovalRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}

		row := tx.QueryRow(ctx, `
			UPDATE approvals
			SET status = 'APPROVED', comment = $3, decided_by = $2, decided_at = NOW(), updated_at = NOW()
			WHERE spd_id = $1 AND status = 'PENDING' AND (approver_id IS NULL OR approver_id = $2)
			RETURNING id, spd_id, step, approver_id, status, comment, decided_by, decided_at, created_at, updated_at
		`, spdID, userID, comment)

		var a ApprovalRow
		if err := row.Scan(&a.ID, &a.SPDID, &a.Step, &a.ApproverID, &a.Status, &a.Comment, &a.DecidedBy, &a.DecidedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return err
		}

		// Simplified: single-step approval.
		_, err := tx.Exec(ctx, `
			UPDATE spds SET status = 'APPROVED', updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL
		`, spdID)
		if err != nil {
			return err
		}

		out = &a
		return nil
	})
	return out, err
}

func (r *Repo) Reject(ctx context.Context, userID int64, role string, spdID int64, comment *string) (*ApprovalRow, error) {
	var out *ApprovalRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}

		row := tx.QueryRow(ctx, `
			UPDATE approvals
			SET status = 'REJECTED', comment = $3, decided_by = $2, decided_at = NOW(), updated_at = NOW()
			WHERE spd_id = $1 AND status = 'PENDING' AND (approver_id IS NULL OR approver_id = $2)
			RETURNING id, spd_id, step, approver_id, status, comment, decided_by, decided_at, created_at, updated_at
		`, spdID, userID, comment)

		var a ApprovalRow
		if err := row.Scan(&a.ID, &a.SPDID, &a.Step, &a.ApproverID, &a.Status, &a.Comment, &a.DecidedBy, &a.DecidedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `
			UPDATE spds SET status = 'REJECTED', updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL
		`, spdID)
		if err != nil {
			return err
		}

		out = &a
		return nil
	})
	return out, err
}

func (r *Repo) ListPending(ctx context.Context, userID int64, role string, limit, offset int) ([]ApprovalRow, error) {
	out := make([]ApprovalRow, 0, limit)
	err := r.pool.WithTx(ctx, pgx.TxOptions{ReadOnly: true}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, spd_id, step, approver_id, status, comment, decided_by, decided_at, created_at, updated_at
			FROM approvals
			WHERE status = 'PENDING' AND (approver_id IS NULL OR approver_id = $1)
			ORDER BY created_at ASC
			LIMIT $2 OFFSET $3
		`, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a ApprovalRow
			if err := rows.Scan(&a.ID, &a.SPDID, &a.Step, &a.ApproverID, &a.Status, &a.Comment, &a.DecidedBy, &a.DecidedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}
