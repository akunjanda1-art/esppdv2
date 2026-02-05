BEGIN;

-- Minimal seed data for local development.

INSERT INTO units (id, code, name)
VALUES (1, 'ADM', 'Administrasi')
ON CONFLICT (id) DO NOTHING;

INSERT INTO employees (id, nip, name, unit_id)
VALUES (1, '000000', 'Admin', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, username, password_hash, role, employee_id)
VALUES (
  1,
  'admin',
  crypt('admin123', gen_salt('bf', 10)),
  'SUPER_ADMIN',
  1
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_units (user_id, unit_id)
VALUES (1, 1)
ON CONFLICT DO NOTHING;

-- Example approver
INSERT INTO users (id, username, password_hash, role)
VALUES (
  2,
  'approver',
  crypt('approver123', gen_salt('bf', 10)),
  'APPROVER'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_units (user_id, unit_id)
VALUES (2, 1)
ON CONFLICT DO NOTHING;

COMMIT;
