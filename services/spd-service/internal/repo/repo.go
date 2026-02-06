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

type SPDRow struct {
	ID           int64
	NomorSurat   string
	EmployeeID   *int64
	UnitID       int64
	PurposeEnc   []byte
	TotalCostEnc []byte
	Status       string
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (r *Repo) CreateSPD(ctx context.Context, userID int64, role string, nomorSurat string, unitID int64, employeeID *int64, purposeEnc, totalCostEnc []byte) (*SPDRow, error) {
	var out *SPDRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO spds (nomor_surat, employee_id, unit_id, purpose_enc, total_cost_enc, status, created_by)
			VALUES ($1, $2, $3, $4, $5, 'DRAFT', $6)
			RETURNING id, nomor_surat, employee_id, unit_id, purpose_enc, total_cost_enc, status, created_by, created_at, updated_at
		`, nomorSurat, employeeID, unitID, purposeEnc, totalCostEnc, userID)

		var r SPDRow
		if err := row.Scan(&r.ID, &r.NomorSurat, &r.EmployeeID, &r.UnitID, &r.PurposeEnc, &r.TotalCostEnc, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return err
		}
		out = &r
		return nil
	})
	return out, err
}

func (r *Repo) GetSPD(ctx context.Context, userID int64, role string, id int64) (*SPDRow, error) {
	var out *SPDRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			SELECT id, nomor_surat, employee_id, unit_id, purpose_enc, total_cost_enc, status, created_by, created_at, updated_at
			FROM spds
			WHERE id = $1 AND deleted_at IS NULL
		`, id)
		var r SPDRow
		if err := row.Scan(&r.ID, &r.NomorSurat, &r.EmployeeID, &r.UnitID, &r.PurposeEnc, &r.TotalCostEnc, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return err
		}
		out = &r
		return nil
	})
	return out, err
}

func (r *Repo) ListSPDs(ctx context.Context, userID int64, role string, limit, offset int) ([]SPDRow, error) {
	out := make([]SPDRow, 0, limit)
	err := r.pool.WithTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, nomor_surat, employee_id, unit_id, purpose_enc, total_cost_enc, status, created_by, created_at, updated_at
			FROM spds
			WHERE deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r SPDRow
			if err := rows.Scan(&r.ID, &r.NomorSurat, &r.EmployeeID, &r.UnitID, &r.PurposeEnc, &r.TotalCostEnc, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

func (r *Repo) SubmitSPD(ctx context.Context, userID int64, role string, id int64) (*SPDRow, error) {
	var out *SPDRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}

		row := tx.QueryRow(ctx, `
			UPDATE spds
			SET status = 'SUBMITTED', updated_at = NOW()
			WHERE id = $1 AND status = 'DRAFT' AND deleted_at IS NULL
			RETURNING id, nomor_surat, employee_id, unit_id, purpose_enc, total_cost_enc, status, created_by, created_at, updated_at
		`, id)
		var r SPDRow
		if err := row.Scan(&r.ID, &r.NomorSurat, &r.EmployeeID, &r.UnitID, &r.PurposeEnc, &r.TotalCostEnc, &r.Status, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return err
		}
		out = &r
		return nil
	})
	return out, err
}
