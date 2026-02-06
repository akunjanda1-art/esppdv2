# ETL Tool (Legacy -> eSPPD v2)

Tool ini mengimpor **export CSV** dari database lama ke schema baru eSPPD v2, termasuk:
- `budgets.amount` + `budgets.source` -> **AES-256-GCM field encryption** (`amount_enc`, `source_enc`)
- `spds.purpose` + `spds.total_cost` -> **AES-256-GCM field encryption** (`purpose_enc`, `total_cost_enc`)

## Input
Letakkan file CSV di folder `etl/input/` (atau set flag `--dir`).

Nama file yang didukung (boleh sebagian, yang tidak ada akan di-skip):
- `units.csv`
- `employees.csv`
- `users.csv`
- `user_units.csv`
- `budgets.csv`
- `spds.csv`
- `approvals.csv`
- `documents.csv`
- `notifications.csv`

CSV harus punya header (case-insensitive). Contoh header minimal:
- `units.csv`: `id,code,name,created_at,updated_at,deleted_at`
- `employees.csv`: `id,nip,name,unit_id,created_at,updated_at,deleted_at`
- `users.csv`: `id,username,password_hash,role,employee_id,is_active,mfa_enabled,last_login_at,created_at,updated_at,deleted_at`
- `budgets.csv`: `id,unit_id,amount,source,description,created_by,created_at,updated_at,deleted_at`
- `spds.csv`: `id,nomor_surat,employee_id,unit_id,budget_id,purpose,total_cost,status,current_approver_id,created_by,created_at,updated_at,deleted_at`

Format waktu yang didukung:
- RFC3339 / RFC3339Nano (mis. `2026-02-05T12:34:56Z`)
- `YYYY-MM-DD HH:MM:SS` (mis. `2026-02-05 12:34:56`)
- `YYYY-MM-DD`

## Menjalankan
Menggunakan env yang sama dengan services (paling penting: `POSTGRES_*` dan `DATA_ENC_KEY_BASE64`).

1. Pastikan `.env` sudah benar (gunakan `scripts/gen-secrets.ps1` untuk generate `DATA_ENC_KEY_BASE64`).
2. Jalankan tool:

```powershell
docker run --rm -v ${PWD}:/work -w /work/tools/etl golang:1.22 `
  bash -c "go mod tidy && go run . --dir /work/etl/input"
```

Dry-run (hanya parse CSV, tidak insert):

```powershell
docker run --rm -v ${PWD}:/work -w /work/tools/etl golang:1.22 `
  bash -c "go run . --dir /work/etl/input --dry-run"
```

