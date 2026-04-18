-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN current_role TEXT;
ALTER TABLE agents ADD COLUMN current_project TEXT;
ALTER TABLE agents ADD COLUMN current_workflow_id TEXT;
ALTER TABLE agents ADD COLUMN lease_expires_at TIMESTAMPTZ;
ALTER TABLE agents ADD CONSTRAINT agents_current_assignment_ck CHECK (
    (current_role IS NULL
        AND current_project IS NULL
        AND current_workflow_id IS NULL
        AND lease_expires_at IS NULL)
    OR
    (current_role IS NOT NULL
        AND current_project IS NOT NULL
        AND current_workflow_id IS NOT NULL
        AND lease_expires_at IS NOT NULL)
);
CREATE INDEX idx_agents_free ON agents(id) WHERE current_workflow_id IS NULL;
CREATE INDEX idx_agents_lease_expires ON agents(lease_expires_at) WHERE lease_expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agents_lease_expires;
DROP INDEX IF EXISTS idx_agents_free;
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_current_assignment_ck;
ALTER TABLE agents DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE agents DROP COLUMN IF EXISTS current_workflow_id;
ALTER TABLE agents DROP COLUMN IF EXISTS current_project;
ALTER TABLE agents DROP COLUMN IF EXISTS current_role;
-- +goose StatementEnd
