# eSPPD Microservices (Go)

Production-ready microservices architecture for a campus/government-style eSPPD system with strong security, auditability, and high throughput. This repository is a **full rewrite** from the previous monolith.

## Architecture (High Level)
```
Internet
  |
  v
[WAF / Cloudflare]
  |
  v
[Load Balancer / Ingress]
  |
  v
[API Gateway (Kong)]
  |
  +--> auth-service (8001)
  +--> spd-service (8002)
  +--> approval-service (8003)
  +--> document-service (8004)
  +--> notification-service (8005)
  +--> reporting-service (8006)
  +--> budget-service (8007)
  +--> audit-service (8008)

Data Layer:
- PostgreSQL 16 (RLS + partitioning)
- Redis (cache/session/rate limit)
- NATS JetStream (async jobs)
- MinIO (encrypted documents)
- Prometheus + Grafana (observability)
```

## Services
- `auth-service`: JWT RS256, refresh tokens, MFA stub, LDAP stub
- `spd-service`: SPD CRUD, encrypted fields, full-text search
- `approval-service`: Workflow approvals, concurrency-safe
- `document-service`: PDF/DOCX/Excel generation queue
- `notification-service`: Email + SMS/WhatsApp stubs
- `budget-service`: Field-level encryption + ABAC/RBAC
- `reporting-service`: Aggregation + export
- `audit-service`: Immutable audit logs

## Quick Start (Docker Compose)
1. Generate dev secrets + `.env`:
   ```powershell
   ./scripts/gen-secrets.ps1
   ```
2. Start stack:
   ```bash
   docker compose up -d --build
   ```
3. Run migrations:
   ```bash
   ./scripts/migrate.ps1
   ```
4. Health checks:
   ```bash
   curl http://localhost:8001/healthz
   ```

## Security Notes
- JWT RS256 keys are mounted at `/secrets` (dev helper: `./scripts/gen-secrets.ps1`)
- AES-256-GCM for field-level encryption
- PostgreSQL Row-Level Security (RLS) policies enabled
- Audit logging on all sensitive actions
- TLS 1.3 expected at ingress / gateway layer

## Kubernetes
Manifests are in `k8s/` with deployments, services, ingress, secrets, and configmaps.

## Adding a New Service
1. Copy an existing service folder under `services/`
2. Update module name & service port
3. Register routes in gateway config

## Legacy Code
The previous monolith has been moved to `legacy/` for reference only.

---

### Directory Structure
```
services/
api/
migrations/
k8s/
monitoring/
load-test/
scripts/
```
