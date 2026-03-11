# PostgreSQL Store Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a PostgreSQL store in `internal/infra/postgres/` that mirrors the SQLite store exactly — same `domain.Store` interface, same file layout, goose migrations adapted for PostgreSQL syntax. Wire DSN auto-detection in `internal/infra/open.go` so a `postgres://` or `postgresql://` DSN routes to the new store.

**Architecture:** The PostgreSQL store is a mechanical port of the SQLite store. The only differences are: `$1`-style placeholders instead of `?`, `NOW()` instead of `datetime('now')`, PostgreSQL-specific `ON CONFLICT DO NOTHING` instead of `INSERT OR IGNORE`, pgx error code detection instead of string matching for unique violations, and PostgreSQL DDL syntax in migrations. All Go structure (`Store` struct, `Open`, `Ping`, `Close`, per-domain files) is identical to the SQLite package.

**Tech Stack:** Go, `database/sql`, `github.com/jackc/pgx/v5/stdlib`, `github.com/pressly/goose/v3`

**Depends on:** Plan 002 (SQLite store) must be complete.

---

## Chunk 1: Dependencies and package skeleton

### Task 1: Add pgx dependency

**Files:**
- Modify: `go.mod` (via `go get`)

- [ ] **Step 1: Add pgx/v5 to go.mod**

Run:

```
go get github.com/jackc/pgx/v5
```

This adds `github.com/jackc/pgx/v5` (and its transitive dependencies) to `go.mod` and `go.sum`. The stdlib adapter (`github.com/jackc/pgx/v5/stdlib`) is included in the same module — no separate `go get` needed.

**Commit:** `chore: add pgx/v5 dependency`

---

### Task 2: PostgreSQL store skeleton

**Files:**
- Create: `internal/infra/postgres/store.go`

- [ ] **Step 1: Create store.go with Open, Ping, Close**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Store implements domain.Store using PostgreSQL.
type Store struct {
	db *sql.DB
}

// Open creates a new PostgreSQL store, running migrations.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Ping verifies that the PostgreSQL connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
```

**Commit:** `feat: add postgres store skeleton with Open/Ping/Close`

---

### Task 3: PostgreSQL error helper

**Files:**
- Create: `internal/infra/postgres/errors.go`

- [ ] **Step 1: Create errors.go for unique violation detection**

pgx wraps PostgreSQL error codes via `pgconn.PgError`. The unique violation code is `23505`.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation returns true if the error is a PostgreSQL unique constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

**Commit:** `feat: add postgres unique violation error helper`

---

## Chunk 2: Migration

### Task 4: PostgreSQL migration file

**Files:**
- Create: `internal/infra/postgres/migrations/001_init.sql`

- [ ] **Step 1: Write PostgreSQL DDL mirroring the SQLite schema**

Key syntax differences from SQLite:
- `TIMESTAMPTZ` instead of `DATETIME`
- `DEFAULT NOW()` instead of `DEFAULT (datetime('now'))`
- `CHECK (kind IN ('role', 'memory'))` — same syntax, but PostgreSQL enforces it natively
- The `BOOLEAN` type is available but not needed here
- The null-exclusion partial unique index for `agent_permissions` uses standard SQL `WHERE` — supported in PostgreSQL
- `ON CONFLICT DO NOTHING` is used in the `SeedPermissions` Go code (not in DDL)
- Drop order respects foreign key dependencies

```sql
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
```

**Commit:** `feat: add postgres goose migration (001_init.sql)`

---

## Chunk 3: Domain store files

### Task 5: Teams

**Files:**
- Create: `internal/infra/postgres/teams.go`

- [ ] **Step 1: Port teams.go from SQLite to PostgreSQL**

Differences from SQLite:
- `?` placeholders become `$1`, `$2`, etc.
- `datetime('now')` becomes `NOW()`

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateTeam(ctx context.Context, t *domain.Team) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO teams (id, name) VALUES ($1, $2)",
		t.ID, t.Name)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: team %q", domain.ErrAlreadyExists, t.Name)
		}
		return fmt.Errorf("insert team: %w", err)
	}
	return nil
}

func (s *Store) GetTeam(ctx context.Context, id string) (*domain.Team, error) {
	var t domain.Team
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, created_at, updated_at FROM teams WHERE id = $1", id,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return &t, nil
}

func (s *Store) ListTeams(ctx context.Context) ([]*domain.Team, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, created_at, updated_at FROM teams ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []*domain.Team
	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, &t)
	}
	return teams, rows.Err()
}

func (s *Store) UpdateTeam(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE teams SET name = $1, updated_at = NOW() WHERE id = $2",
		name, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: team %q", domain.ErrAlreadyExists, name)
		}
		return fmt.Errorf("update team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agents WHERE team_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count agents for team: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: team has %d agents", domain.ErrHasDependencies, count)
	}

	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE team_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count tasks for team: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: team has %d tasks", domain.ErrHasDependencies, count)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	return tx.Commit()
}
```

**Commit:** `feat: add postgres teams store`

---

### Task 6: Roles

**Files:**
- Create: `internal/infra/postgres/roles.go`

- [ ] **Step 1: Port roles.go from SQLite to PostgreSQL**

The recursive CTE is valid PostgreSQL syntax — no changes needed to the CTE structure itself, only placeholder style.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateRole(ctx context.Context, r *domain.Role) error {
	var parentID *string
	if r.ParentID != "" {
		parentID = &r.ParentID
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO roles (id, name, parent_id) VALUES ($1, $2, $3)",
		r.ID, r.Name, parentID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: role %q", domain.ErrAlreadyExists, r.Name)
		}
		return fmt.Errorf("insert role: %w", err)
	}
	return nil
}

func (s *Store) GetRole(ctx context.Context, id string) (*domain.Role, error) {
	var r domain.Role
	var parentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, parent_id, created_at, updated_at FROM roles WHERE id = $1", id,
	).Scan(&r.ID, &r.Name, &parentID, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get role: %w", err)
	}
	if parentID.Valid {
		r.ParentID = parentID.String
	}
	return &r, nil
}

func (s *Store) ListRoles(ctx context.Context, parentID string) ([]*domain.Role, error) {
	var rows *sql.Rows
	var err error

	if parentID != "" {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, parent_id, created_at, updated_at FROM roles WHERE parent_id = $1 ORDER BY name",
			parentID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, parent_id, created_at, updated_at FROM roles ORDER BY name")
	}
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []*domain.Role
	for rows.Next() {
		var r domain.Role
		var pid sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &pid, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		if pid.Valid {
			r.ParentID = pid.String
		}
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}

func (s *Store) UpdateRole(ctx context.Context, id, name, parentID string) error {
	var pid *string
	if parentID != "" {
		pid = &parentID
	}

	res, err := s.db.ExecContext(ctx,
		"UPDATE roles SET name = $1, parent_id = $2, updated_at = NOW() WHERE id = $3",
		name, pid, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: role %q", domain.ErrAlreadyExists, name)
		}
		return fmt.Errorf("update role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM roles WHERE parent_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count child roles: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: role has %d child roles", domain.ErrHasDependencies, count)
	}

	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agent_roles WHERE role_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count agent assignments for role: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: role assigned to %d agents", domain.ErrHasDependencies, count)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM roles WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	return tx.Commit()
}

// GetRoleChain returns the inheritance chain from the given role to the root,
// up to maxDepth levels. Uses a recursive CTE. Ordered leaf-to-root.
func (s *Store) GetRoleChain(ctx context.Context, roleID string, maxDepth int) ([]*domain.Role, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE chain(id, name, parent_id, created_at, updated_at, depth) AS (
			SELECT id, name, parent_id, created_at, updated_at, 1
			FROM roles WHERE id = $1
			UNION ALL
			SELECT r.id, r.name, r.parent_id, r.created_at, r.updated_at, c.depth + 1
			FROM roles r
			JOIN chain c ON r.id = c.parent_id
			WHERE c.depth < $2
		)
		SELECT id, name, parent_id, created_at, updated_at FROM chain ORDER BY depth
	`, roleID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("get role chain: %w", err)
	}
	defer rows.Close()

	var chain []*domain.Role
	for rows.Next() {
		var r domain.Role
		var pid sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &pid, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role chain: %w", err)
		}
		if pid.Valid {
			r.ParentID = pid.String
		}
		chain = append(chain, &r)
	}
	return chain, rows.Err()
}
```

**Commit:** `feat: add postgres roles store`

---

### Task 7: Agents

**Files:**
- Create: `internal/infra/postgres/agents.go`

- [ ] **Step 1: Port agents.go from SQLite to PostgreSQL**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO agents (id, name, team_id) VALUES ($1, $2, $3)",
		a.ID, a.Name, a.TeamID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent %q", domain.ErrAlreadyExists, a.Name)
		}
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	var a domain.Agent
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE id = $1", id,
	).Scan(&a.ID, &a.Name, &a.TeamID, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return &a, nil
}

func (s *Store) ListAgents(ctx context.Context, teamID string) ([]*domain.Agent, error) {
	var rows *sql.Rows
	var err error

	if teamID != "" {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE team_id = $1 ORDER BY name",
			teamID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, team_id, created_at, updated_at FROM agents ORDER BY name")
	}
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*domain.Agent
	for rows.Next() {
		var a domain.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.TeamID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

func (s *Store) UpdateAgent(ctx context.Context, id, name, teamID string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE agents SET name = $1, team_id = $2, updated_at = NOW() WHERE id = $3",
		name, teamID, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent %q", domain.ErrAlreadyExists, name)
		}
		return fmt.Errorf("update agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE agent_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count tasks for agent: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: agent has %d tasks", domain.ErrHasDependencies, count)
	}

	// agent_roles and agent_permissions cascade on delete
	res, err := tx.ExecContext(ctx, "DELETE FROM agents WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	return tx.Commit()
}

func (s *Store) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_roles WHERE agent_id = $1", agentID)
	if err != nil {
		return fmt.Errorf("clear agent roles: %w", err)
	}

	for _, r := range roles {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_roles (agent_id, role_id, priority) VALUES ($1, $2, $3)",
			agentID, r.RoleID, r.Priority)
		if err != nil {
			return fmt.Errorf("insert agent role: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT agent_id, role_id, priority FROM agent_roles WHERE agent_id = $1 ORDER BY priority",
		agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.AgentRole
	for rows.Next() {
		var r domain.AgentRole
		if err := rows.Scan(&r.AgentID, &r.RoleID, &r.Priority); err != nil {
			return nil, fmt.Errorf("scan agent role: %w", err)
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}
```

**Commit:** `feat: add postgres agents store`

---

### Task 8: Documents

**Files:**
- Create: `internal/infra/postgres/documents.go`

- [ ] **Step 1: Port documents.go from SQLite to PostgreSQL**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateDocument(ctx context.Context, d *domain.Document) error {
	var roleID, agentID *string
	if d.RoleID != "" {
		roleID = &d.RoleID
	}
	if d.AgentID != "" {
		agentID = &d.AgentID
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO documents (id, kind, title, content, role_id, agent_id) VALUES ($1, $2, $3, $4, $5, $6)",
		d.ID, d.Kind, d.Title, d.Content, roleID, agentID)
	if err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

func (s *Store) GetDocument(ctx context.Context, id string) (*domain.Document, error) {
	var d domain.Document
	var roleID, agentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE id = $1", id,
	).Scan(&d.ID, &d.Kind, &d.Title, &d.Content, &roleID, &agentID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: document %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if roleID.Valid {
		d.RoleID = roleID.String
	}
	if agentID.Valid {
		d.AgentID = agentID.String
	}
	return &d, nil
}

func (s *Store) UpdateDocument(ctx context.Context, id, title, content string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE documents SET title = $1, content = $2, updated_at = NOW() WHERE id = $3",
		title, content, id)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteDocument(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM documents WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error) {
	return s.listDocuments(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE role_id = $1 ORDER BY title",
		roleID)
}

func (s *Store) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	return s.listDocuments(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE agent_id = $1 ORDER BY title",
		agentID)
}

func (s *Store) listDocuments(ctx context.Context, query string, arg string) ([]*domain.Document, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []*domain.Document
	for rows.Next() {
		var d domain.Document
		var roleID, agentID sql.NullString
		if err := rows.Scan(&d.ID, &d.Kind, &d.Title, &d.Content, &roleID, &agentID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		if roleID.Valid {
			d.RoleID = roleID.String
		}
		if agentID.Valid {
			d.AgentID = agentID.String
		}
		docs = append(docs, &d)
	}
	return docs, rows.Err()
}
```

**Commit:** `feat: add postgres documents store`

---

### Task 9: Tasks

**Files:**
- Create: `internal/infra/postgres/tasks.go`

- [ ] **Step 1: Port tasks.go from SQLite to PostgreSQL**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	var agentID *string
	if t.AgentID != "" {
		agentID = &t.AgentID
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO tasks (id, team_id, agent_id, title, description, status) VALUES ($1, $2, $3, $4, $5, $6)",
		t.ID, t.TeamID, agentID, t.Title, t.Description, t.Status)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (s *Store) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	var t domain.Task
	var agentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, created_at, updated_at FROM tasks WHERE id = $1", id,
	).Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if agentID.Valid {
		t.AgentID = agentID.String
	}
	return &t, nil
}

func (s *Store) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, created_at, updated_at FROM tasks WHERE team_id = $1 ORDER BY created_at",
		teamID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		var t domain.Task
		var agentID sql.NullString
		if err := rows.Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if agentID.Valid {
			t.AgentID = agentID.String
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func (s *Store) UpdateTask(ctx context.Context, id string, t *domain.Task) error {
	var agentID *string
	if t.AgentID != "" {
		agentID = &t.AgentID
	}

	res, err := s.db.ExecContext(ctx,
		"UPDATE tasks SET title = $1, description = $2, status = $3, agent_id = $4, updated_at = NOW() WHERE id = $5",
		t.Title, t.Description, t.Status, agentID, id)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteTask(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	return nil
}
```

**Commit:** `feat: add postgres tasks store`

---

### Task 10: Permissions

**Files:**
- Create: `internal/infra/postgres/permissions.go`

- [ ] **Step 1: Port permissions.go from SQLite to PostgreSQL**

The key difference is `SeedPermissions`: SQLite uses `INSERT OR IGNORE`; PostgreSQL uses `INSERT ... ON CONFLICT DO NOTHING`.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) SeedPermissions(ctx context.Context, names []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, name := range names {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO permissions (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			"perm_"+name, name)
		if err != nil {
			return fmt.Errorf("seed permission %q: %w", name, err)
		}
	}
	return tx.Commit()
}

func (s *Store) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ap.agent_id, p.name, COALESCE(ap.scope_team_id, '')
		FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = $1
		ORDER BY p.name
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent permissions: %w", err)
	}
	defer rows.Close()

	var perms []domain.AgentPermission
	for rows.Next() {
		var p domain.AgentPermission
		if err := rows.Scan(&p.AgentID, &p.PermissionID, &p.ScopeTeamID); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (s *Store) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_permissions WHERE agent_id = $1", agentID)
	if err != nil {
		return fmt.Errorf("clear permissions: %w", err)
	}

	for _, p := range perms {
		var scopeTeamID *string
		if p.ScopeTeamID != "" {
			scopeTeamID = &p.ScopeTeamID
		}

		// Resolve permission name to ID
		var permID string
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM permissions WHERE name = $1", p.PermissionID,
		).Scan(&permID)
		if err != nil {
			return fmt.Errorf("resolve permission %q: %w", p.PermissionID, err)
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_permissions (agent_id, permission_id, scope_team_id) VALUES ($1, $2, $3)",
			agentID, permID, scopeTeamID)
		if err != nil {
			return fmt.Errorf("insert permission: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) HasPermission(ctx context.Context, agentID, permName, scopeTeamID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = $1 AND p.name = $2
		AND (ap.scope_team_id IS NULL OR ap.scope_team_id = $3)
	`, agentID, permName, scopeTeamID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return count > 0, nil
}
```

**Commit:** `feat: add postgres permissions store`

---

## Chunk 4: DSN routing and compile check

### Task 11: Wire open.go

**Files:**
- Modify: `internal/infra/open.go`

- [ ] **Step 1: Replace the "not yet implemented" stub with the real postgres.Open call**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package infra

import (
	"fmt"
	"strings"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/postgres"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// Open auto-detects the database backend from the DSN and returns a Store.
//
// DSN formats:
//   - postgres://... or postgresql://... -> PostgreSQL
//   - Any file path or empty string      -> SQLite (empty = :memory:)
func Open(dsn string) (domain.Store, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		s, err := postgres.Open(dsn)
		if err != nil {
			return nil, fmt.Errorf("open postgres store: %w", err)
		}
		return s, nil
	}
	return sqlite.Open(dsn)
}
```

**Commit:** `feat: wire postgres store into DSN auto-detection`

---

### Task 12: Build verification

**Files:**
- No file changes — compile and vet only.

- [ ] **Step 1: Verify the package compiles**

Run:

```
go build ./...
go vet ./...
```

Both must pass with zero errors. If `go build` fails due to missing `pgx` in `go.sum` (e.g., if Task 1 was skipped), run `go get github.com/jackc/pgx/v5` first.

**Commit:** *(no commit — this is a verification step only)*

---

## Syntax translation reference

For quick reference during implementation, here are all the SQLite-to-PostgreSQL substitutions applied uniformly across every file:

| SQLite | PostgreSQL |
|---|---|
| `?` (first arg) | `$1` |
| `?` (nth arg) | `$n` |
| `datetime('now')` | `NOW()` |
| `INSERT OR IGNORE INTO` | `INSERT INTO ... ON CONFLICT DO NOTHING` |
| `DATETIME NOT NULL DEFAULT (datetime('now'))` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` |
| `strings.Contains(err.Error(), "UNIQUE constraint failed")` | `errors.As(err, &pgErr) && pgErr.Code == "23505"` |
| `_ "modernc.org/sqlite"` | `_ "github.com/jackc/pgx/v5/stdlib"` |
| `sql.Open("sqlite", ...)` | `sql.Open("pgx", ...)` |
| `goose.SetDialect("sqlite3")` | `goose.SetDialect("postgres")` |
