-- +goose Up
-- Rename current_role → assigned_role. current_role is a PostgreSQL
-- reserved keyword; renaming keeps both backends consistent and avoids
-- quoting requirements in every query. SQLite RENAME COLUMN requires
-- 3.25.0+; the minimum supported version is 3.35.0.
--
-- The triggers from 003 reference the old column name; we drop and
-- recreate them here so they reference assigned_role.
-- +goose StatementBegin
DROP TRIGGER IF EXISTS agents_current_assignment_ck_insert;
DROP TRIGGER IF EXISTS agents_current_assignment_ck_update;
ALTER TABLE agents RENAME COLUMN current_role TO assigned_role;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER agents_current_assignment_ck_insert
BEFORE INSERT ON agents
FOR EACH ROW
WHEN NOT (
    (NEW.assigned_role IS NULL
        AND NEW.current_project IS NULL
        AND NEW.current_workflow_id IS NULL
        AND NEW.lease_expires_at IS NULL)
    OR
    (NEW.assigned_role IS NOT NULL
        AND NEW.current_project IS NOT NULL
        AND NEW.current_workflow_id IS NOT NULL
        AND NEW.lease_expires_at IS NOT NULL)
)
BEGIN
    SELECT RAISE(ABORT, 'agents.current_* must be all NULL or all set');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER agents_current_assignment_ck_update
BEFORE UPDATE ON agents
FOR EACH ROW
WHEN NOT (
    (NEW.assigned_role IS NULL
        AND NEW.current_project IS NULL
        AND NEW.current_workflow_id IS NULL
        AND NEW.lease_expires_at IS NULL)
    OR
    (NEW.assigned_role IS NOT NULL
        AND NEW.current_project IS NOT NULL
        AND NEW.current_workflow_id IS NOT NULL
        AND NEW.lease_expires_at IS NOT NULL)
)
BEGIN
    SELECT RAISE(ABORT, 'agents.current_* must be all NULL or all set');
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS agents_current_assignment_ck_insert;
DROP TRIGGER IF EXISTS agents_current_assignment_ck_update;
ALTER TABLE agents RENAME COLUMN assigned_role TO current_role;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER agents_current_assignment_ck_insert
BEFORE INSERT ON agents
FOR EACH ROW
WHEN NOT (
    (NEW.current_role IS NULL
        AND NEW.current_project IS NULL
        AND NEW.current_workflow_id IS NULL
        AND NEW.lease_expires_at IS NULL)
    OR
    (NEW.current_role IS NOT NULL
        AND NEW.current_project IS NOT NULL
        AND NEW.current_workflow_id IS NOT NULL
        AND NEW.lease_expires_at IS NOT NULL)
)
BEGIN
    SELECT RAISE(ABORT, 'agents.current_* must be all NULL or all set');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER agents_current_assignment_ck_update
BEFORE UPDATE ON agents
FOR EACH ROW
WHEN NOT (
    (NEW.current_role IS NULL
        AND NEW.current_project IS NULL
        AND NEW.current_workflow_id IS NULL
        AND NEW.lease_expires_at IS NULL)
    OR
    (NEW.current_role IS NOT NULL
        AND NEW.current_project IS NOT NULL
        AND NEW.current_workflow_id IS NOT NULL
        AND NEW.lease_expires_at IS NOT NULL)
)
BEGIN
    SELECT RAISE(ABORT, 'agents.current_* must be all NULL or all set');
END;
-- +goose StatementEnd
