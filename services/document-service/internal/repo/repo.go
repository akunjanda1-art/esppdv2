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

type SPDForDoc struct {
	ID           int64
	NomorSurat   string
	UnitID       int64
	PurposeEnc   []byte
	TotalCostEnc []byte
	Status       string
	CreatedAt    time.Time
}

func (r *Repo) GetSPDForDoc(ctx context.Context, userID int64, role string, spdID int64) (*SPDForDoc, error) {
	var out *SPDForDoc
	err := r.pool.WithTx(ctx, pgx.TxOptions{ReadOnly: true}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			SELECT id, nomor_surat, unit_id, purpose_enc, total_cost_enc, status, created_at
			FROM spds
			WHERE id = $1 AND deleted_at IS NULL
		`, spdID)
		var s SPDForDoc
		if err := row.Scan(&s.ID, &s.NomorSurat, &s.UnitID, &s.PurposeEnc, &s.TotalCostEnc, &s.Status, &s.CreatedAt); err != nil {
			return err
		}
		out = &s
		return nil
	})
	return out, err
}

type DocumentRow struct {
	ID          int64
	SPDID       int64
	Format      string
	ObjectKey   string
	RequestedBy int64
	CreatedAt   time.Time
}

func (r *Repo) InsertDocument(ctx context.Context, userID int64, role string, spdID int64, format, objectKey string, requestedBy int64) (*DocumentRow, error) {
	var out *DocumentRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO documents (spd_id, format, object_key, requested_by)
			VALUES ($1, $2, $3, $4)
			RETURNING id, spd_id, format, object_key, requested_by, created_at
		`, spdID, format, objectKey, requestedBy)
		var d DocumentRow
		if err := row.Scan(&d.ID, &d.SPDID, &d.Format, &d.ObjectKey, &d.RequestedBy, &d.CreatedAt); err != nil {
			return err
		}
		out = &d
		return nil
	})
	return out, err
}
