-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN assigned_role TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN current_project TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN current_workflow_id TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN lease_expires_at TIMESTAMPTZ;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents ADD CONSTRAINT agents_current_assignment_ck CHECK (
    (assigned_role IS NULL
        AND current_project IS NULL
        AND current_workflow_id IS NULL
        AND lease_expires_at IS NULL)
    OR
    (assigned_role IS NOT NULL
        AND current_project IS NOT NULL
        AND current_workflow_id IS NOT NULL
        AND lease_expires_at IS NOT NULL)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_agents_free ON agents(id) WHERE current_workflow_id IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_agents_lease_expires ON agents(lease_expires_at) WHERE lease_expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agents_lease_expires;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agents_free;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_current_assignment_ck;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents DROP COLUMN IF EXISTS lease_expires_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents DROP COLUMN IF EXISTS current_workflow_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents DROP COLUMN IF EXISTS current_project;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agents DROP COLUMN IF EXISTS assigned_role;
-- +goose StatementEnd
