package repo

import (
	"context"
	"encoding/json"
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

type AuditRow struct {
	ID           int64           `json:"id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *int64          `json:"resource_id"`
	UserID       int64           `json:"user_id"`
	Role         string          `json:"role"`
	IPAddress    *string         `json:"ip_address"`
	UserAgent    *string         `json:"user_agent"`
	Metadata     json.RawMessage `json:"metadata"`
	Timestamp    time.Time       `json:"timestamp"`
}

type InsertInput struct {
	Action       string
	ResourceType string
	ResourceID   *int64
	UserID       int64
	Role         string
	IPAddress    *string
	UserAgent    *string
	Metadata     json.RawMessage
	Timestamp    time.Time
}

func (r *Repo) Insert(ctx context.Context, userID int64, role string, in InsertInput) error {
	return r.pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_logs (action, resource_type, resource_id, user_id, role, ip_address, user_agent, metadata, timestamp)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, NOW()))
		`, in.Action, in.ResourceType, in.ResourceID, in.UserID, in.Role, in.IPAddress, in.UserAgent, in.Metadata, in.Timestamp)
		return err
	})
}

type QueryInput struct {
	Action   string
	UserID   *int64
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
	Resource string
}

func (r *Repo) Query(ctx context.Context, userID int64, role string, q QueryInput) ([]AuditRow, error) {
	out := make([]AuditRow, 0, q.Limit)
	err := r.pool.WithTx(ctx, pgx.TxOptions{ReadOnly: true}, func(tx pgx.Tx) error {
		if err := db.SetRLSContext(ctx, tx, userID, role); err != nil {
			return err
		}

		// Very simple filter builder.
		args := make([]any, 0, 8)
		where := "WHERE 1=1"
		if q.Action != "" {
			args = append(args, q.Action)
			where += " AND action = $" + itoa(len(args))
		}
		if q.Resource != "" {
			args = append(args, q.Resource)
			where += " AND resource_type = $" + itoa(len(args))
		}
		if q.UserID != nil {
			args = append(args, *q.UserID)
			where += " AND user_id = $" + itoa(len(args))
		}
		if q.From != nil {
			args = append(args, *q.From)
			where += " AND timestamp >= $" + itoa(len(args))
		}
		if q.To != nil {
			args = append(args, *q.To)
			where += " AND timestamp <= $" + itoa(len(args))
		}

		args = append(args, q.Limit)
		limitPos := itoa(len(args))
		args = append(args, q.Offset)
		offsetPos := itoa(len(args))

		rows, err := tx.Query(ctx, `
			SELECT id, action, resource_type, resource_id, user_id, role, ip_address::text, user_agent, metadata, timestamp
			FROM audit_logs
			`+where+`
			ORDER BY timestamp DESC
			LIMIT $`+limitPos+` OFFSET $`+offsetPos+`
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r AuditRow
			if err := rows.Scan(&r.ID, &r.Action, &r.ResourceType, &r.ResourceID, &r.UserID, &r.Role, &r.IPAddress, &r.UserAgent, &r.Metadata, &r.Timestamp); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

func itoa(i int) string {
	// tiny helper to avoid strconv import in hot path
	if i == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	n := i
	for n > 0 {
		pos--
		buf[pos] = byte('0' + (n % 10))
		n /= 10
	}
	return string(buf[pos:])
}
