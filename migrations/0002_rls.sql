BEGIN;

-- SPDs RLS
ALTER TABLE spds ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS spds_select ON spds;
DROP POLICY IF EXISTS spds_insert ON spds;
DROP POLICY IF EXISTS spds_update ON spds;

CREATE POLICY spds_select ON spds
  FOR SELECT
  USING (
    created_by = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
    OR unit_id IN (
      SELECT unit_id FROM user_units
      WHERE user_id = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
    )
    OR COALESCE(current_setting('app.user_role', true), '') = 'SUPER_ADMIN'
  );

CREATE POLICY spds_insert ON spds
  FOR INSERT
  WITH CHECK (
    (
      created_by = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
      AND unit_id IN (
        SELECT unit_id FROM user_units
        WHERE user_id = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
      )
    )
    OR COALESCE(current_setting('app.user_role', true), '') = 'SUPER_ADMIN'
  );

CREATE POLICY spds_update ON spds
  FOR UPDATE
  USING (
    created_by = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
    OR COALESCE(current_setting('app.user_role', true), '') = 'SUPER_ADMIN'
  )
  WITH CHECK (
    created_by = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
    OR COALESCE(current_setting('app.user_role', true), '') = 'SUPER_ADMIN'
  );

-- Budgets RLS
ALTER TABLE budgets ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS budgets_select ON budgets;
DROP POLICY IF EXISTS budgets_insert ON budgets;
DROP POLICY IF EXISTS budgets_update ON budgets;

CREATE POLICY budgets_select ON budgets
  FOR SELECT
  USING (
    COALESCE(current_setting('app.user_role', true), '') IN ('SUPER_ADMIN','FINANCE','BUDGET_ADMIN')
    OR unit_id IN (
      SELECT unit_id FROM user_units
      WHERE user_id = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
    )
  );

CREATE POLICY budgets_insert ON budgets
  FOR INSERT
  WITH CHECK (
    COALESCE(current_setting('app.user_role', true), '') IN ('SUPER_ADMIN','FINANCE','BUDGET_ADMIN')
    AND created_by = COALESCE(current_setting('app.current_user_id', true), '0')::bigint
  );

CREATE POLICY budgets_update ON budgets
  FOR UPDATE
  USING (
    COALESCE(current_setting('app.user_role', true), '') IN ('SUPER_ADMIN','FINANCE','BUDGET_ADMIN')
  )
  WITH CHECK (
    COALESCE(current_setting('app.user_role', true), '') IN ('SUPER_ADMIN','FINANCE','BUDGET_ADMIN')
  );

COMMIT;
