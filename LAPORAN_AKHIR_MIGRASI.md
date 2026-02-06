# Laporan Akhir Migrasi eSPPD (Go Microservices Rewrite)

**Tanggal:** 2026-02-05  
**Lokasi Repo:** `c:\laragon\www\esppdv2\eSPPD`

## Ringkasan
Proyek eSPPD telah diubah **total** dari monolith menjadi **arsitektur microservices berbasis Go** (in-place). Fokus utama: keamanan data anggaran, auditability, dan kesiapan high-throughput.

> Catatan: sesuai permintaan terakhir, berkas/struktur lama yang tidak terpakai sudah dihapus (folder `legacy/` tidak ada lagi).

## Deliverables (Yang Sudah Tersedia di Repo)
- **8 microservices** (Fiber) di `services/`:
  - `auth-service` (JWT RS256 + refresh rotation)
  - `spd-service` (SPD draft/list/get/submit + field encryption)
  - `approval-service` (approve/reject + consumer event submit)
  - `document-service` (generate dokumen via NATS + simpan ke MinIO)
  - `notification-service` (queue consumer + stub sender)
  - `reporting-service` (summary JSON + export CSV)
  - `budget-service` (field encryption + masking + publish audit event)
  - `audit-service` (consumer audit.log + query audit logs)
- **Shared module**: `shared/` (config, logging, AES-256-GCM, JWT, DB helper RLS, HTTP utils, metrics Prometheus)
- **Database migrations**: `migrations/`
  - `0001_init.sql` (schema inti)
  - `0002_rls.sql` (PostgreSQL Row-Level Security policies)
  - `0003_seed.sql` (seed akun dev)
  - `0004_mfa.sql` (MFA secret + enrollment)
- **Docker Compose full stack**: `docker-compose.yml`
  - PostgreSQL, Redis, NATS JetStream, MinIO, Kong Gateway, Prometheus, Grafana, 8 services
- **Kong config (DB-less)**: `docker/kong/kong.yml`
- **Kubernetes skeleton**: `k8s/` (namespace/configmap/secret/ingress + manifest per service)
- **Monitoring**: `monitoring/` (Prometheus + Grafana provisioning + dashboard)
- **Load testing**: `load-test/spd_load.js` (k6)
- **API Specs (OpenAPI)**: `api/*.yaml`
- **Dev scripts**:
  - `scripts/gen-secrets.ps1` (generate `.env`, AES key, dan JWT keypair dev)
  - `scripts/migrate.ps1` (apply SQL migrations ke container postgres)
- **ETL tool**:
  - `tools/etl/` (CSV importer untuk data legacy ke schema baru)
- **CI**: `.github/workflows/ci.yml`
- **Dokumen migrasi**: `MIGRATION_GUIDE.md`

## Keamanan (Implemented)
- **JWT RS256** (auth-service) dengan refresh token rotation.
- **MFA TOTP** (RFC6238) dengan endpoint enroll/verify/disable.
- **LDAP Authentication** (mode: disabled/prefer/required).
- **AES-256-GCM field-level encryption**:
  - `spds.purpose_enc`, `spds.total_cost_enc`
  - `budgets.amount_enc`, `budgets.source_enc`
- **Data masking** untuk field budget pada role non-authorized.
- **PostgreSQL RLS**: kebijakan isolasi data (unit/user) di `migrations/0002_rls.sql`.
- **Audit trail**:
  - `budget-service` publish event `audit.log`
  - `audit-service` consume dan simpan ke `audit_logs` (immutable via trigger)
- **Rate limiting terdistribusi** via Redis (fallback ke in-memory bila Redis down).

## Event/Queue Topics (NATS)
- `spd.submitted` (dari spd-service)
- `document.generate` (job dokumen)
- `document.generated` (notifikasi selesai; minimal)
- `notification.send` (job notifikasi)
- `audit.log` (audit event)

## Cara Menjalankan (Local)
1. Buka folder repo: `c:\laragon\www\esppdv2\eSPPD`
2. Generate secrets + env:
   - `./scripts/gen-secrets.ps1`
3. Jalankan stack:
   - `docker compose up -d --build`
4. Jalankan migrasi:
   - `./scripts/migrate.ps1`
5. Cek health:
   - `curl http://localhost:8001/healthz`

**Akun dev (seed):**
- `admin` / `admin123` (role `SUPER_ADMIN`)
- `approver` / `approver123` (role `APPROVER`)

## Verifikasi yang Sudah Dilakukan Saat Pengerjaan
- Struktur folder microservices + file utama ada untuk semua service.
- `docker compose config` berhasil (compose valid).
- `scripts/gen-secrets.ps1` berhasil membuat `.env` + `secrets/jwt_private.pem` + `secrets/jwt_public.pem`.
- Unit test untuk generator DOCX/XLSX (OOXML) berjalan di modul document-service.
 - `docker compose build` dicoba pada **2026-02-06**, namun **gagal** karena error engine Docker:
   - `request returned 500 Internal Server Error ... /_ping`
   - Pada percobaan lain: `rpc error: code = Unavailable desc = error reading from server: EOF`

## Batasan / Yang Belum (Penting untuk “Full Migration” sebenarnya)
- **ETL legacy belum dijalankan** karena membutuhkan akses database lama + mapping data nyata (tool `tools/etl` sudah ada).
- **Run-time e2e** belum diverifikasi penuh di mesin ini (butuh Docker runtime aktif + data contoh).

## Rekomendasi Next Steps
1. Pastikan Docker daemon berjalan dan user punya permission akses engine.
2. Jalankan `docker compose up -d --build` dan `./scripts/migrate.ps1`.
3. Jalankan **ETL migrasi data** (users/units/employees/spds/budgets) dari DB lama menggunakan `tools/etl`.
4. Jalankan E2E test (login → create SPD → submit → approve → generate doc → cek reporting & audit).
5. Jika butuh output DOCX/XLSX yang lebih advanced (template kompleks), pertimbangkan library enterprise seperti unioffice (opsional).

---

Jika Anda ingin, saya bisa lanjutkan dengan: skrip ETL migrasi data dari database lama + uji integrasi end-to-end.
