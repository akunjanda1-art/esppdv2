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

type User struct {
	ID           int64
	Username     string
	Role         string
	PasswordHash string
}

func (r *Repo) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, username, role, password_hash
		FROM users
		WHERE username = $1 AND deleted_at IS NULL
	`, username)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) GetUserByID(ctx context.Context, id int64) (*User, error) {
\trow := r.pool.QueryRow(ctx, `
\t\tSELECT id, username, role, password_hash
\t\tFROM users
\t\tWHERE id = $1 AND deleted_at IS NULL
\t`, id)

\tvar u User
\tif err := row.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash); err != nil {
\t\treturn nil, err
\t}
\treturn &u, nil
}

type RefreshToken struct {
	ID         string
	UserID     int64
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
}

func (r *Repo) InsertRefreshToken(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, id, userID, expiresAt)
	return err
}

func (r *Repo) GetRefreshToken(ctx context.Context, id string) (*RefreshToken, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at, replaced_by
		FROM refresh_tokens
		WHERE id = $1
	`, id)

	var t RefreshToken
	if err := row.Scan(&t.ID, &t.UserID, &t.ExpiresAt, &t.RevokedAt, &t.ReplacedBy); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repo) RevokeRefreshToken(ctx context.Context, id string, replacedBy *string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW(), replaced_by = $2
		WHERE id = $1 AND revoked_at IS NULL
	`, id, replacedBy)
	return err
}

func (r *Repo) IsNoRows(err error) bool {
	return err == pgx.ErrNoRows
}
