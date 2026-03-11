-- +goose Up

CREATE TABLE teams (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE roles (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    parent_id  TEXT REFERENCES roles(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agents (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    team_id    TEXT NOT NULL REFERENCES teams(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_roles (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    role_id  TEXT NOT NULL REFERENCES roles(id),
    priority INTEGER NOT NULL,
    PRIMARY KEY (agent_id, role_id),
    UNIQUE (agent_id, priority)
);

CREATE TABLE documents (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('role', 'memory')),
    title      TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    role_id    TEXT REFERENCES roles(id) ON DELETE CASCADE,
    agent_id   TEXT REFERENCES agents(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((role_id IS NULL) != (agent_id IS NULL))
);

CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES teams(id),
    agent_id    TEXT REFERENCES agents(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE permissions (
    id   TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE agent_permissions (
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id),
    scope_team_id TEXT REFERENCES teams(id),
    PRIMARY KEY (agent_id, permission_id, scope_team_id)
);

CREATE UNIQUE INDEX uq_agent_perm_global
    ON agent_permissions(agent_id, permission_id)
    WHERE scope_team_id IS NULL;

-- +goose Down

DROP TABLE agent_permissions;
DROP TABLE permissions;
DROP TABLE tasks;
DROP TABLE documents;
DROP TABLE agent_roles;
DROP TABLE agents;
DROP TABLE roles;
DROP TABLE teams;
