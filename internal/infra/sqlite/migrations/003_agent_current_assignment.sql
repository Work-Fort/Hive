-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN current_role TEXT;
ALTER TABLE agents ADD COLUMN current_project TEXT;
ALTER TABLE agents ADD COLUMN current_workflow_id TEXT;
ALTER TABLE agents ADD COLUMN lease_expires_at DATETIME;
-- +goose StatementEnd

-- +goose StatementBegin
-- All-or-nothing invariant: either the agent is free (all NULL) or fully
-- claimed (all four NOT NULL). SQLite cannot add a table-level CHECK after
-- the fact, so we enforce it with a trigger pair on INSERT and UPDATE.
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

-- +goose StatementBegin
CREATE INDEX idx_agents_free
    ON agents(id)
    WHERE current_workflow_id IS NULL;
CREATE INDEX idx_agents_lease_expires
    ON agents(lease_expires_at)
    WHERE lease_expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agents_lease_expires;
DROP INDEX IF EXISTS idx_agents_free;
DROP TRIGGER IF EXISTS agents_current_assignment_ck_update;
DROP TRIGGER IF EXISTS agents_current_assignment_ck_insert;
-- +goose StatementEnd
-- +goose StatementBegin
-- SQLite prior to 3.35 cannot DROP COLUMN; we no-op here. If you need to
-- revert the columns, recreate the agents table from a prior snapshot.
-- +goose StatementEnd
