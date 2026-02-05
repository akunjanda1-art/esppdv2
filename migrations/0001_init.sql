BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Core org tables
CREATE TABLE IF NOT EXISTS units (
  id BIGSERIAL PRIMARY KEY,
  code TEXT UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS employees (
  id BIGSERIAL PRIMARY KEY,
  nip TEXT UNIQUE,
  name TEXT NOT NULL,
  unit_id BIGINT REFERENCES units(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

-- Auth tables
CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'USER',
  employee_id BIGINT REFERENCES employees(id),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS user_units (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  unit_id BIGINT NOT NULL REFERENCES units(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, unit_id)
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
  id TEXT PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  replaced_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

-- Budget
CREATE TABLE IF NOT EXISTS budgets (
  id BIGSERIAL PRIMARY KEY,
  unit_id BIGINT NOT NULL REFERENCES units(id),
  amount_enc BYTEA,
  source_enc BYTEA,
  description TEXT,
  created_by BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_budgets_unit ON budgets(unit_id) WHERE deleted_at IS NULL;

-- SPDs
CREATE TABLE IF NOT EXISTS spds (
  id BIGSERIAL PRIMARY KEY,
  nomor_surat VARCHAR(50) UNIQUE NOT NULL,
  employee_id BIGINT REFERENCES employees(id),
  unit_id BIGINT NOT NULL REFERENCES units(id),
  budget_id BIGINT REFERENCES budgets(id),

  purpose_enc BYTEA,
  total_cost_enc BYTEA,

  status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
  current_approver_id BIGINT,

  created_by BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,

  search_vector tsvector,

  CONSTRAINT spds_valid_status CHECK (status IN ('DRAFT','SUBMITTED','APPROVED','REJECTED','CANCELLED'))
);

CREATE INDEX IF NOT EXISTS idx_spds_employee ON spds(employee_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_spds_unit ON spds(unit_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_spds_status ON spds(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_spds_created_at ON spds(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_spds_search ON spds USING GIN (search_vector);

-- Approval workflow
CREATE TABLE IF NOT EXISTS approvals (
  id BIGSERIAL PRIMARY KEY,
  spd_id BIGINT NOT NULL REFERENCES spds(id) ON DELETE CASCADE,
  step INT NOT NULL DEFAULT 1,
  approver_id BIGINT REFERENCES users(id),
  status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
  comment TEXT,
  decided_by BIGINT REFERENCES users(id),
  decided_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  CONSTRAINT approvals_valid_status CHECK (status IN ('PENDING','APPROVED','REJECTED'))
);

CREATE INDEX IF NOT EXISTS idx_approvals_spd ON approvals(spd_id);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);

-- Documents
CREATE TABLE IF NOT EXISTS documents (
  id BIGSERIAL PRIMARY KEY,
  spd_id BIGINT NOT NULL REFERENCES spds(id) ON DELETE CASCADE,
  format VARCHAR(10) NOT NULL,
  object_key TEXT NOT NULL,
  requested_by BIGINT NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_documents_spd ON documents(spd_id);

-- Notifications
CREATE TABLE IF NOT EXISTS notifications (
  id BIGSERIAL PRIMARY KEY,
  channel TEXT NOT NULL,
  recipient TEXT NOT NULL,
  subject TEXT,
  body TEXT,
  status TEXT NOT NULL DEFAULT 'QUEUED',
  requested_by BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  sent_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);

-- Audit logs (partitioned, append-only)
CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGSERIAL NOT NULL,
  action VARCHAR(50) NOT NULL,
  resource_type VARCHAR(50) NOT NULL,
  resource_id BIGINT,
  user_id BIGINT NOT NULL,
  role TEXT NOT NULL DEFAULT '',
  ip_address INET,
  user_agent TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (timestamp);

CREATE TABLE IF NOT EXISTS audit_logs_2026_02 PARTITION OF audit_logs
  FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE IF NOT EXISTS audit_logs_default PARTITION OF audit_logs DEFAULT;

CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user_ts ON audit_logs(user_id, timestamp DESC);

-- Utility triggers
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION spds_search_vector_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.search_vector = to_tsvector('simple', COALESCE(NEW.nomor_surat, ''));
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_units_updated_at BEFORE UPDATE ON units
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_employees_updated_at BEFORE UPDATE ON employees
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_budgets_updated_at BEFORE UPDATE ON budgets
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_spds_updated_at BEFORE UPDATE ON spds
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_approvals_updated_at BEFORE UPDATE ON approvals
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_spds_search_vector BEFORE INSERT OR UPDATE OF nomor_surat ON spds
  FOR EACH ROW EXECUTE FUNCTION spds_search_vector_update();

-- Prevent updates/deletes on audit logs (immutable)
CREATE OR REPLACE FUNCTION prevent_audit_mutations() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit logs are immutable';
END;
$$;

CREATE TRIGGER trg_audit_immutable BEFORE UPDATE OR DELETE ON audit_logs
  FOR EACH ROW EXECUTE FUNCTION prevent_audit_mutations();

COMMIT;
