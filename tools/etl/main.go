package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"esppd.local/shared/config"
	"esppd.local/shared/cryptox"
	"esppd.local/shared/db"
	"github.com/jackc/pgx/v5"
)

func main() {
	var (
		inputDir  = flag.String("dir", "etl/input", "Directory containing legacy CSV exports")
		targetDSN = flag.String("target-dsn", "", "Target Postgres DSN (overrides POSTGRES_* env)")
		dryRun    = flag.Bool("dry-run", false, "Parse only; do not write to target DB")
	)
	flag.Parse()

	pg := config.LoadPostgres()
	if *targetDSN == "" {
		*targetDSN = pg.DSN()
	}
	cryptoCfg := config.LoadCrypto()
	if cryptoCfg.DataKeyBase64 == "" || strings.HasPrefix(cryptoCfg.DataKeyBase64, "REPLACE_") {
		fatalf("DATA_ENC_KEY_BASE64 is required (run scripts/gen-secrets.ps1)")
	}
	aesgcm, err := cryptox.NewAESGCMFromBase64(cryptoCfg.DataKeyBase64)
	if err != nil {
		fatalf("init encryption: %v", err)
	}

	ctx := context.Background()

	if *dryRun {
		fmt.Println("DRY RUN: no writes will be performed")
	}

	pool, err := db.NewPool(ctx, *targetDSN, 10)
	if err != nil {
		fatalf("connect target postgres: %v", err)
	}
	defer pool.Close()

	base := filepath.Clean(*inputDir)
	fmt.Println("ETL input dir:", base)

	must(migrateUnits(ctx, pool, filepath.Join(base, "units.csv"), *dryRun))
	must(migrateEmployees(ctx, pool, filepath.Join(base, "employees.csv"), *dryRun))
	must(migrateUsers(ctx, pool, filepath.Join(base, "users.csv"), *dryRun))
	must(migrateUserUnits(ctx, pool, filepath.Join(base, "user_units.csv"), *dryRun))
	must(migrateBudgets(ctx, pool, aesgcm, filepath.Join(base, "budgets.csv"), *dryRun))
	must(migrateSPDs(ctx, pool, aesgcm, filepath.Join(base, "spds.csv"), *dryRun))
	must(migrateApprovals(ctx, pool, filepath.Join(base, "approvals.csv"), *dryRun))
	must(migrateDocuments(ctx, pool, filepath.Join(base, "documents.csv"), *dryRun))
	must(migrateNotifications(ctx, pool, filepath.Join(base, "notifications.csv"), *dryRun))

	if !*dryRun {
		must(bumpSequences(ctx, pool))
	}

	fmt.Println("ETL completed.")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("skip (missing):", path)
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}

	for i := range header {
		header[i] = strings.ToLower(strings.TrimSpace(header[i]))
	}

	out := make([]map[string]string, 0, 1024)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				row[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, row)
	}
	fmt.Printf("loaded %d rows: %s\n", len(out), path)
	return out, nil
}

func str(row map[string]string, key string) string { return row[strings.ToLower(key)] }

func int64Ptr(row map[string]string, key string) (*int64, error) {
	v := str(row, key)
	if v == "" {
		return nil, nil
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &i, nil
}

func int64Val(row map[string]string, key string) (int64, error) {
	v := str(row, key)
	if v == "" {
		return 0, fmt.Errorf("%s: required", key)
	}
	return strconv.ParseInt(v, 10, 64)
}

func boolVal(row map[string]string, key string, def bool) bool {
	v := strings.ToLower(str(row, key))
	if v == "" {
		return def
	}
	switch v {
	case "1", "t", "true", "y", "yes":
		return true
	case "0", "f", "false", "n", "no":
		return false
	default:
		return def
	}
}

func timePtr(row map[string]string, key string) (*time.Time, error) {
	v := str(row, key)
	if v == "" {
		return nil, nil
	}
	tt, err := parseTime(v)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &tt, nil
}

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

func migrateUnits(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			updatedAt, err := timePtr(row, "updated_at")
			if err != nil {
				return err
			}
			deletedAt, err := timePtr(row, "deleted_at")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO units (id, code, name, created_at, updated_at, deleted_at)
				VALUES ($1, $2, $3, COALESCE($4, NOW()), COALESCE($5, NOW()), $6)
				ON CONFLICT (id) DO NOTHING
			`, id, str(row, "code"), str(row, "name"), createdAt, updatedAt, deletedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateEmployees(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			unitID, err := int64Ptr(row, "unit_id")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			updatedAt, err := timePtr(row, "updated_at")
			if err != nil {
				return err
			}
			deletedAt, err := timePtr(row, "deleted_at")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO employees (id, nip, name, unit_id, created_at, updated_at, deleted_at)
				VALUES ($1, $2, $3, $4, COALESCE($5, NOW()), COALESCE($6, NOW()), $7)
				ON CONFLICT (id) DO NOTHING
			`, id, str(row, "nip"), str(row, "name"), unitID, createdAt, updatedAt, deletedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateUsers(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			employeeID, err := int64Ptr(row, "employee_id")
			if err != nil {
				return err
			}
			lastLoginAt, err := timePtr(row, "last_login_at")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			updatedAt, err := timePtr(row, "updated_at")
			if err != nil {
				return err
			}
			deletedAt, err := timePtr(row, "deleted_at")
			if err != nil {
				return err
			}
			pw := str(row, "password_hash")
			if pw == "" {
				return fmt.Errorf("users.id=%d: password_hash required", id)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO users (id, username, password_hash, role, employee_id, is_active, mfa_enabled, last_login_at, created_at, updated_at, deleted_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, NOW()), COALESCE($10, NOW()), $11)
				ON CONFLICT (id) DO NOTHING
			`, id, str(row, "username"), pw, str(row, "role"), employeeID, boolVal(row, "is_active", true), boolVal(row, "mfa_enabled", false), lastLoginAt, createdAt, updatedAt, deletedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateUserUnits(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			userID, err := int64Val(row, "user_id")
			if err != nil {
				return err
			}
			unitID, err := int64Val(row, "unit_id")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO user_units (user_id, unit_id, created_at)
				VALUES ($1, $2, COALESCE($3, NOW()))
				ON CONFLICT DO NOTHING
			`, userID, unitID, createdAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateBudgets(ctx context.Context, pool *db.Pool, aesgcm *cryptox.AESGCM, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			unitID, err := int64Val(row, "unit_id")
			if err != nil {
				return err
			}
			createdBy, err := int64Val(row, "created_by")
			if err != nil {
				return err
			}
			if err := db.SetRLSContext(ctx, tx, createdBy, "SUPER_ADMIN"); err != nil {
				return err
			}

			var amountEnc []byte
			if v := str(row, "amount"); v != "" {
				amountEnc, err = aesgcm.EncryptString(v, []byte("budgets:amount"))
				if err != nil {
					return err
				}
			}
			var sourceEnc []byte
			if v := str(row, "source"); v != "" {
				sourceEnc, err = aesgcm.EncryptString(v, []byte("budgets:source"))
				if err != nil {
					return err
				}
			}

			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			updatedAt, err := timePtr(row, "updated_at")
			if err != nil {
				return err
			}
			deletedAt, err := timePtr(row, "deleted_at")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO budgets (id, unit_id, amount_enc, source_enc, description, created_by, created_at, updated_at, deleted_at)
				VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, NOW()), COALESCE($8, NOW()), $9)
				ON CONFLICT (id) DO NOTHING
			`, id, unitID, amountEnc, sourceEnc, str(row, "description"), createdBy, createdAt, updatedAt, deletedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateSPDs(ctx context.Context, pool *db.Pool, aesgcm *cryptox.AESGCM, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			unitID, err := int64Val(row, "unit_id")
			if err != nil {
				return err
			}
			employeeID, err := int64Ptr(row, "employee_id")
			if err != nil {
				return err
			}
			budgetID, err := int64Ptr(row, "budget_id")
			if err != nil {
				return err
			}
			currentApproverID, err := int64Ptr(row, "current_approver_id")
			if err != nil {
				return err
			}
			createdBy, err := int64Val(row, "created_by")
			if err != nil {
				return err
			}

			if err := db.SetRLSContext(ctx, tx, createdBy, "SUPER_ADMIN"); err != nil {
				return err
			}

			var purposeEnc []byte
			if v := str(row, "purpose"); v != "" {
				purposeEnc, err = aesgcm.EncryptString(v, []byte("spds:purpose"))
				if err != nil {
					return err
				}
			}
			var totalEnc []byte
			if v := str(row, "total_cost"); v != "" {
				totalEnc, err = aesgcm.EncryptString(v, []byte("spds:total_cost"))
				if err != nil {
					return err
				}
			}

			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			updatedAt, err := timePtr(row, "updated_at")
			if err != nil {
				return err
			}
			deletedAt, err := timePtr(row, "deleted_at")
			if err != nil {
				return err
			}

			status := str(row, "status")
			if status == "" {
				status = "DRAFT"
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO spds (
					id, nomor_surat, employee_id, unit_id, budget_id,
					purpose_enc, total_cost_enc, status, current_approver_id,
					created_by, created_at, updated_at, deleted_at
				)
				VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9,
					$10, COALESCE($11, NOW()), COALESCE($12, NOW()), $13
				)
				ON CONFLICT (id) DO NOTHING
			`, id, str(row, "nomor_surat"), employeeID, unitID, budgetID, purposeEnc, totalEnc, status, currentApproverID, createdBy, createdAt, updatedAt, deletedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateApprovals(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			spdID, err := int64Val(row, "spd_id")
			if err != nil {
				return err
			}
			step, _ := strconv.Atoi(str(row, "step"))
			approverID, err := int64Ptr(row, "approver_id")
			if err != nil {
				return err
			}
			decidedBy, err := int64Ptr(row, "decided_by")
			if err != nil {
				return err
			}
			decidedAt, err := timePtr(row, "decided_at")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			updatedAt, err := timePtr(row, "updated_at")
			if err != nil {
				return err
			}

			status := str(row, "status")
			if status == "" {
				status = "PENDING"
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO approvals (id, spd_id, step, approver_id, status, comment, decided_by, decided_at, created_at, updated_at)
				VALUES ($1, $2, COALESCE($3, 1), $4, $5, $6, $7, $8, COALESCE($9, NOW()), COALESCE($10, NOW()))
				ON CONFLICT (id) DO NOTHING
			`, id, spdID, step, approverID, status, str(row, "comment"), decidedBy, decidedAt, createdAt, updatedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateDocuments(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			spdID, err := int64Val(row, "spd_id")
			if err != nil {
				return err
			}
			requestedBy, err := int64Val(row, "requested_by")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO documents (id, spd_id, format, object_key, requested_by, created_at)
				VALUES ($1, $2, $3, $4, $5, COALESCE($6, NOW()))
				ON CONFLICT (id) DO NOTHING
			`, id, spdID, str(row, "format"), str(row, "object_key"), requestedBy, createdAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateNotifications(ctx context.Context, pool *db.Pool, path string, dry bool) error {
	rows, err := readCSV(path)
	if err != nil || len(rows) == 0 || dry {
		return err
	}
	return pool.WithTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, row := range rows {
			id, err := int64Val(row, "id")
			if err != nil {
				return err
			}
			requestedBy, err := int64Ptr(row, "requested_by")
			if err != nil {
				return err
			}
			createdAt, err := timePtr(row, "created_at")
			if err != nil {
				return err
			}
			sentAt, err := timePtr(row, "sent_at")
			if err != nil {
				return err
			}
			deletedAt, err := timePtr(row, "deleted_at")
			if err != nil {
				return err
			}
			status := str(row, "status")
			if status == "" {
				status = "QUEUED"
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO notifications (id, channel, recipient, subject, body, status, requested_by, created_at, sent_at, deleted_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, NOW()), $9, $10)
				ON CONFLICT (id) DO NOTHING
			`, id, str(row, "channel"), str(row, "recipient"), str(row, "subject"), str(row, "body"), status, requestedBy, createdAt, sentAt, deletedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func bumpSequences(ctx context.Context, pool *db.Pool) error {
	tables := []string{"units", "employees", "users", "budgets", "spds", "approvals", "documents", "notifications"}
	for _, t := range tables {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			SELECT setval(pg_get_serial_sequence('%s','id'), COALESCE((SELECT MAX(id) FROM %s), 1), true)
		`, t, t))
		if err != nil {
			return err
		}
	}
	return nil
}

