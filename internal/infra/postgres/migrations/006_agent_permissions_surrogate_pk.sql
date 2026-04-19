-- +goose Up
-- Fresh PG installs (running 001 after this fix) already have the
-- surrogate PK. This migration handles any PG instance that ran the
-- old 001 where agent_permissions had a composite PK including the
-- nullable scope_team_id column. We detect by checking whether the
-- 'id' column already exists.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'agent_permissions' AND column_name = 'id'
    ) THEN
        -- Recreate with surrogate PK; preserve existing rows.
        CREATE TABLE agent_permissions_new (
            id            BIGSERIAL PRIMARY KEY,
            agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
            permission_id TEXT NOT NULL REFERENCES permissions(id),
            scope_team_id TEXT REFERENCES teams(id)
        );
        INSERT INTO agent_permissions_new (agent_id, permission_id, scope_team_id)
        SELECT agent_id, permission_id, scope_team_id FROM agent_permissions;
        DROP TABLE agent_permissions;
        ALTER TABLE agent_permissions_new RENAME TO agent_permissions;
        CREATE UNIQUE INDEX uq_agent_perm_global
            ON agent_permissions(agent_id, permission_id)
            WHERE scope_team_id IS NULL;
        CREATE UNIQUE INDEX uq_agent_perm_scoped
            ON agent_permissions(agent_id, permission_id, scope_team_id)
            WHERE scope_team_id IS NOT NULL;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- No-op: reversing this migration would require knowing whether the
-- table was modified by this migration or was already correct. Since
-- PG is unreleased, simply leave the schema as-is.
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
