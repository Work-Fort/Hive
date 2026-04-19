-- +goose Up
-- Replace composite PK (agent_id, permission_id, scope_team_id) with a
-- surrogate integer PK so scope_team_id can remain NULL for global grants.
-- SQLite cannot ALTER TABLE DROP CONSTRAINT, so we recreate the table.
-- +goose StatementBegin
CREATE TABLE agent_permissions_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id),
    scope_team_id TEXT REFERENCES teams(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO agent_permissions_new (agent_id, permission_id, scope_team_id)
SELECT agent_id, permission_id, scope_team_id FROM agent_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE agent_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agent_permissions_new RENAME TO agent_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX uq_agent_perm_global
    ON agent_permissions(agent_id, permission_id)
    WHERE scope_team_id IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX uq_agent_perm_scoped
    ON agent_permissions(agent_id, permission_id, scope_team_id)
    WHERE scope_team_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- Recreate the original composite-PK table (data preserved).
-- +goose StatementBegin
CREATE TABLE agent_permissions_old (
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id),
    scope_team_id TEXT REFERENCES teams(id),
    PRIMARY KEY (agent_id, permission_id, scope_team_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO agent_permissions_old (agent_id, permission_id, scope_team_id)
SELECT agent_id, permission_id, scope_team_id FROM agent_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS uq_agent_perm_scoped;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS uq_agent_perm_global;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE agent_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE agent_permissions_old RENAME TO agent_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX uq_agent_perm_global
    ON agent_permissions(agent_id, permission_id)
    WHERE scope_team_id IS NULL;
-- +goose StatementEnd
