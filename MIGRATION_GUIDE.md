# Migration Guide (Laravel/Livewire -> Go Microservices)

This repo is a full rewrite. The safest migration strategy is **strangler-fig**: run the new services alongside the legacy monolith and migrate capabilities incrementally.

## Phase 0 — Prepare
- Freeze schema changes in legacy (or introduce a migration window).
- Identify the system-of-record per domain: Auth, SPD, Approvals, Budgets, Documents, Audit.
- Create a mapping table of legacy IDs to new IDs (especially employees, units, users).

## Phase 1 — Auth First
- Stand up `auth-service`.
- Mirror legacy users into `users` (bcrypt hashes are compatible).
- Switch clients to obtain tokens from `auth-service`.
- Keep legacy app behind the gateway but require JWT for new APIs.

## Phase 2 — SPD (Read -> Write)
- Start with read-only endpoints: list/get SPD from the new DB.
- Implement write path: create draft + submit.
- During cutover, dual-write (legacy + new) until confidence is high.

## Phase 3 — Approvals
- Make approvals event-driven:
  - `spd-service` publishes `spd.submitted`
  - `approval-service` creates approval records
- Replace legacy approval UI to call `approval-service`.

## Phase 4 — Documents
- Route all heavy generation through `document-service` + queue.
- Store outputs in MinIO, keep only metadata in Postgres.

## Phase 5 — Budget & Audit
- Put **all** budget reads/writes behind `budget-service`.
- Enforce encryption and publish audit events to `audit-service`.
- Run access reviews using audit logs.

## Phase 6 — Decommission Legacy
- Disable legacy write endpoints.
- Keep legacy DB read-only for a defined retention window.
- Remove legacy code only after production stability period.

## Data Migration Notes
- Use batch migration (ETL) with idempotent scripts.
- Prefer incremental sync using timestamps until full cutover.
- Validate:
  - Record counts per table
  - Random sampling comparisons
  - Approval state consistency

## Rollback Plan (Always)
- Maintain the ability to route traffic back to legacy.
- Keep old DB intact until sign-off.
