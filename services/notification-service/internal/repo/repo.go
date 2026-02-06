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

type NotificationRow struct {
	ID          int64
	Channel     string
	Recipient   string
	Subject     string
	Body        string
	Status      string
	RequestedBy int64
	CreatedAt   time.Time
	SentAt      *time.Time
}

func (r *Repo) InsertNotification(ctx context.Context, userID int64, role string, channel, recipient, subject, body, status string, requestedBy int64) (*NotificationRow, error) {
	var out *NotificationRow
	err := r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO notifications (channel, recipient, subject, body, status, requested_by, sent_at)
			VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $5 = 'SENT' THEN NOW() ELSE NULL END)
			RETURNING id, channel, recipient, subject, body, status, requested_by, created_at, sent_at
		`, channel, recipient, subject, body, status, requestedBy)
		var n NotificationRow
		if err := row.Scan(&n.ID, &n.Channel, &n.Recipient, &n.Subject, &n.Body, &n.Status, &n.RequestedBy, &n.CreatedAt, &n.SentAt); err != nil {
			return err
		}
		out = &n
		return nil
	})
	return out, err
}
