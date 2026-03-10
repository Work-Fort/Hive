# Domain Model + SQLite Store Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Define Hive's domain types, port interfaces, and a complete SQLite store implementation with goose migrations covering all tables from the design spec.

**Architecture:** Domain types and store interfaces in `internal/domain/` with zero infrastructure dependencies. SQLite adapter in `internal/infra/sqlite/` using `database/sql` with modernc.org/sqlite driver and goose for migrations. DSN router in `internal/infra/open.go`.

**Tech Stack:** Go, modernc.org/sqlite, pressly/goose/v3, database/sql

**Depends on:** Plan 001 (project skeleton) must be complete.

---

## Chunk 1: Domain Types and Port Interfaces

### Task 1: Domain types

**Files:**
- Create: `internal/domain/types.go`

- [ ] **Step 1: Create domain types**

Create `internal/domain/types.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later

// Package domain defines the core types and port interfaces for Hive.
// This package has zero dependencies on infrastructure — it defines
// what the system does, not how.
package domain

import "time"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
)

// DocumentKind identifies whether a document belongs to a role or an agent.
type DocumentKind string

const (
	DocumentKindRole   DocumentKind = "role"
	DocumentKindMemory DocumentKind = "memory"
)

// Team is an organizational unit. Agents belong to exactly one team.
type Team struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Role is a reusable capability definition with optional single-parent
// inheritance. Roles are composable — an agent can have multiple roles.
type Role struct {
	ID        string
	Name      string
	ParentID  string // empty if no parent
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Agent is a provisioned identity that belongs to one team and can be
// assigned multiple roles with priority ordering.
type Agent struct {
	ID        string
	Name      string
	TeamID    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentRole links an agent to a role with a priority. Lower priority
// number = higher precedence in document resolution.
type AgentRole struct {
	AgentID  string
	RoleID   string
	Priority int
}

// Document holds markdown content attached to either a role or an agent.
// Exactly one of RoleID or AgentID must be set.
type Document struct {
	ID        string
	Kind      DocumentKind
	Title     string
	Content   string
	RoleID    string // set when Kind == DocumentKindRole
	AgentID   string // set when Kind == DocumentKindMemory
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Task is a work item belonging to a team, optionally assigned to an agent.
type Task struct {
	ID          string
	TeamID      string
	AgentID     string // empty if unassigned
	Title       string
	Description string
	Status      TaskStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Permission is a named capability that can be granted to agents.
type Permission struct {
	ID   string
	Name string
}

// AgentPermission grants a permission to an agent, optionally scoped to a
// specific team. If ScopeTeamID is empty, the permission is global.
type AgentPermission struct {
	AgentID      string
	PermissionID string
	ScopeTeamID  string // empty = global scope
}

// ProvisioningResponse is the hierarchical response returned when an agent
// requests its provisioning data.
type ProvisioningResponse struct {
	Roles  []ProvisioningRoleGroup `json:"roles"`
	Memory []Document              `json:"memory"`
}

// ProvisioningRoleGroup is a single role assignment with its full
// inheritance chain of documents.
type ProvisioningRoleGroup struct {
	Priority int                      `json:"priority"`
	Chain    []ProvisioningChainEntry `json:"chain"`
}

// ProvisioningChainEntry is one link in the role inheritance chain,
// ordered from leaf (index 0) to root (last element).
type ProvisioningChainEntry struct {
	Role      string     `json:"role"`
	Documents []Document `json:"documents"`
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/domain/...
```

Expected: compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/types.go
git commit -m "feat: add domain types for teams, roles, agents, documents, tasks, permissions"
```

### Task 2: Domain error types

**Files:**
- Create: `internal/domain/errors.go`

- [ ] **Step 1: Create domain errors**

Create `internal/domain/errors.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package domain

import "errors"

var (
	// ErrNotFound is returned when an entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is returned when creating a duplicate entity.
	ErrAlreadyExists = errors.New("already exists")

	// ErrHasDependencies is returned when deleting an entity that has
	// dependent entities (e.g., deleting a team with agents).
	ErrHasDependencies = errors.New("has dependencies")

	// ErrDepthExceeded is returned when a role inheritance chain would
	// exceed the configured max depth.
	ErrDepthExceeded = errors.New("role depth exceeded")

	// ErrCycleDetected is returned when a role parent assignment would
	// create a cycle in the inheritance chain.
	ErrCycleDetected = errors.New("role inheritance cycle detected")
)
```

- [ ] **Step 2: Commit**

```bash
git add internal/domain/errors.go
git commit -m "feat: add domain error types"
```

### Task 3: Port interfaces (store)

**Files:**
- Create: `internal/domain/ports.go`

- [ ] **Step 1: Create store interfaces**

Create `internal/domain/ports.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package domain

import (
	"context"
	"io"
)

// TeamStore persists team metadata.
type TeamStore interface {
	CreateTeam(ctx context.Context, t *Team) error
	GetTeam(ctx context.Context, id string) (*Team, error)
	ListTeams(ctx context.Context) ([]*Team, error)
	UpdateTeam(ctx context.Context, id, name string) error
	DeleteTeam(ctx context.Context, id string) error
}

// RoleStore persists role metadata and handles inheritance queries.
type RoleStore interface {
	CreateRole(ctx context.Context, r *Role) error
	GetRole(ctx context.Context, id string) (*Role, error)
	ListRoles(ctx context.Context, parentID string) ([]*Role, error)
	UpdateRole(ctx context.Context, id, name, parentID string) error
	DeleteRole(ctx context.Context, id string) error

	// GetRoleChain returns the inheritance chain from the given role
	// to the root, up to maxDepth levels. The result is ordered
	// leaf-to-root (index 0 = the given role).
	GetRoleChain(ctx context.Context, roleID string, maxDepth int) ([]*Role, error)
}

// AgentStore persists agent metadata and role assignments.
type AgentStore interface {
	CreateAgent(ctx context.Context, a *Agent) error
	GetAgent(ctx context.Context, id string) (*Agent, error)
	ListAgents(ctx context.Context, teamID string) ([]*Agent, error)
	UpdateAgent(ctx context.Context, id, name, teamID string) error
	DeleteAgent(ctx context.Context, id string) error

	SetAgentRoles(ctx context.Context, agentID string, roles []AgentRole) error
	GetAgentRoles(ctx context.Context, agentID string) ([]AgentRole, error)
}

// DocumentStore persists markdown documents for roles and agents.
type DocumentStore interface {
	CreateDocument(ctx context.Context, d *Document) error
	GetDocument(ctx context.Context, id string) (*Document, error)
	UpdateDocument(ctx context.Context, id, title, content string) error
	DeleteDocument(ctx context.Context, id string) error

	ListRoleDocuments(ctx context.Context, roleID string) ([]*Document, error)
	ListAgentMemory(ctx context.Context, agentID string) ([]*Document, error)
}

// TaskStore persists tasks scoped to teams.
type TaskStore interface {
	CreateTask(ctx context.Context, t *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	ListTeamTasks(ctx context.Context, teamID string) ([]*Task, error)
	UpdateTask(ctx context.Context, id string, t *Task) error
	DeleteTask(ctx context.Context, id string) error
}

// PermissionStore manages RBAC permissions.
type PermissionStore interface {
	// SeedPermissions ensures all named permissions exist in the database.
	SeedPermissions(ctx context.Context, names []string) error

	GetAgentPermissions(ctx context.Context, agentID string) ([]AgentPermission, error)
	SetAgentPermissions(ctx context.Context, agentID string, perms []AgentPermission) error

	// HasPermission checks if an agent has a specific permission,
	// either globally or scoped to the given team.
	HasPermission(ctx context.Context, agentID, permName, scopeTeamID string) (bool, error)
}

// Store combines all storage interfaces.
type Store interface {
	TeamStore
	RoleStore
	AgentStore
	DocumentStore
	TaskStore
	PermissionStore
	io.Closer
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/domain/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/ports.go
git commit -m "feat: add store port interfaces (hexagonal ports)"
```

## Chunk 2: SQLite Store — Schema and Core Tables

### Task 4: SQLite store scaffolding and migrations

**Files:**
- Create: `internal/infra/sqlite/store.go`
- Create: `internal/infra/sqlite/migrations/001_init.sql`

- [ ] **Step 1: Create store with migration runner**

Create `internal/infra/sqlite/store.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Store implements domain.Store using SQLite.
type Store struct {
	db *sql.DB
}

// Open creates a new SQLite store, running migrations.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = ":memory:"
	}

	db, err := sql.Open("sqlite", dsn+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 2: Create initial migration**

Create `internal/infra/sqlite/migrations/001_init.sql`:

```sql
-- +goose Up

CREATE TABLE teams (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE roles (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    parent_id  TEXT REFERENCES roles(id),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE agents (
    id         TEXT PRIMARY KEY,
    name       TEXT UNIQUE NOT NULL,
    team_id    TEXT NOT NULL REFERENCES teams(id),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
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
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    CHECK ((role_id IS NULL) != (agent_id IS NULL))
);

CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES teams(id),
    agent_id    TEXT REFERENCES agents(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed')),
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
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

- [ ] **Step 3: Verify compilation**

```bash
go mod tidy
go build ./internal/infra/sqlite/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/infra/sqlite/ go.mod go.sum
git commit -m "feat: add SQLite store scaffolding with initial migration"
```

### Task 5: DSN router

**Files:**
- Create: `internal/infra/open.go`

- [ ] **Step 1: Create DSN router**

Create `internal/infra/open.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package infra

import (
	"strings"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// Open auto-detects the database backend from the DSN and returns a Store.
//
// DSN formats:
//   - postgres://... or postgresql://... → PostgreSQL (not yet implemented)
//   - Any file path or empty string      → SQLite (empty = :memory:)
func Open(dsn string) (domain.Store, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return nil, fmt.Errorf("postgres not yet implemented")
	}
	return sqlite.Open(dsn)
}
```

Note: Add `"fmt"` to imports. The postgres path will be implemented in plan 008.

- [ ] **Step 2: Commit**

```bash
git add internal/infra/open.go
git commit -m "feat: add DSN auto-detection router"
```

### Task 6: Teams store implementation

**Files:**
- Create: `internal/infra/sqlite/teams.go`

- [ ] **Step 1: Write team store test**

Create `internal/infra/sqlite/teams_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

func newTestStore(t *testing.T) domain.Store {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGetTeam(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t_001", Name: "alpha"}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	got, err := store.GetTeam(ctx, "t_001")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("got name %q, want %q", got.Name, "alpha")
	}
}

func TestCreateTeamDuplicate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t_001", Name: "alpha"}
	store.CreateTeam(ctx, team)

	dup := &domain.Team{ID: "t_002", Name: "alpha"}
	err := store.CreateTeam(ctx, dup)
	if err == nil {
		t.Fatal("expected error for duplicate team name")
	}
}

func TestListTeams(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTeam(ctx, &domain.Team{ID: "t_002", Name: "beta"})

	teams, err := store.ListTeams(ctx)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 2 {
		t.Errorf("got %d teams, want 2", len(teams))
	}
}

func TestDeleteTeamWithAgents(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	err := store.DeleteTeam(ctx, "t_001")
	if err == nil {
		t.Fatal("expected error when deleting team with agents")
	}
}

func TestDeleteTeamEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	if err := store.DeleteTeam(ctx, "t_001"); err != nil {
		t.Fatalf("delete team: %v", err)
	}

	_, err := store.GetTeam(ctx, "t_001")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/infra/sqlite/... -v -run TestCreateAndGetTeam
```

Expected: FAIL (methods not implemented).

- [ ] **Step 3: Implement teams store**

Create `internal/infra/sqlite/teams.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateTeam(ctx context.Context, t *domain.Team) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO teams (id, name) VALUES (?, ?)",
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
		"SELECT id, name, created_at, updated_at FROM teams WHERE id = ?", id,
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
		"UPDATE teams SET name = ?, updated_at = datetime('now') WHERE id = ?",
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
	// Check for dependent agents
	var count int
	s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agents WHERE team_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("%w: team has %d agents", domain.ErrHasDependencies, count)
	}

	// Check for dependent tasks
	s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE team_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("%w: team has %d tasks", domain.ErrHasDependencies, count)
	}

	res, err := s.db.ExecContext(ctx, "DELETE FROM teams WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	return nil
}
```

- [ ] **Step 4: Create helper for unique constraint detection**

Create `internal/infra/sqlite/errors.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import "strings"

// isUniqueViolation returns true if the error is a SQLite unique constraint
// violation. modernc.org/sqlite wraps errors as strings.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go mod tidy
go test ./internal/infra/sqlite/... -v
```

Expected: all TestCreateAndGetTeam, TestCreateTeamDuplicate, TestListTeams,
TestDeleteTeamWithAgents, TestDeleteTeamEmpty pass.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/sqlite/teams.go internal/infra/sqlite/teams_test.go internal/infra/sqlite/errors.go go.mod go.sum
git commit -m "feat: add SQLite teams store with tests"
```

### Task 7: Roles store implementation

**Files:**
- Create: `internal/infra/sqlite/roles.go`
- Create: `internal/infra/sqlite/roles_test.go`

- [ ] **Step 1: Write role store tests**

Create `internal/infra/sqlite/roles_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateAndGetRole(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	role := &domain.Role{ID: "r_001", Name: "developer"}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}

	got, err := store.GetRole(ctx, "r_001")
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if got.Name != "developer" {
		t.Errorf("got name %q, want %q", got.Name, "developer")
	}
}

func TestRoleInheritanceChain(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "base"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "developer", ParentID: "r_001"})
	store.CreateRole(ctx, &domain.Role{ID: "r_003", Name: "frontend-dev", ParentID: "r_002"})

	chain, err := store.GetRoleChain(ctx, "r_003", 10)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	if len(chain) != 3 {
		t.Fatalf("got %d roles in chain, want 3", len(chain))
	}
	if chain[0].Name != "frontend-dev" {
		t.Errorf("chain[0] = %q, want %q", chain[0].Name, "frontend-dev")
	}
	if chain[1].Name != "developer" {
		t.Errorf("chain[1] = %q, want %q", chain[1].Name, "developer")
	}
	if chain[2].Name != "base" {
		t.Errorf("chain[2] = %q, want %q", chain[2].Name, "base")
	}
}

func TestRoleChainDepthLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "base"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "mid", ParentID: "r_001"})
	store.CreateRole(ctx, &domain.Role{ID: "r_003", Name: "leaf", ParentID: "r_002"})

	chain, err := store.GetRoleChain(ctx, "r_003", 2)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	// Should return only 2 levels (leaf + mid), truncated at depth limit
	if len(chain) != 2 {
		t.Errorf("got %d roles, want 2 (depth limited)", len(chain))
	}
}

func TestDeleteRoleWithChildren(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "base"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "child", ParentID: "r_001"})

	err := store.DeleteRole(ctx, "r_001")
	if !errors.Is(err, domain.ErrHasDependencies) {
		t.Fatalf("expected ErrHasDependencies, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/infra/sqlite/... -v -run TestRole
```

Expected: FAIL.

- [ ] **Step 3: Implement roles store**

Create `internal/infra/sqlite/roles.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

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
		"INSERT INTO roles (id, name, parent_id) VALUES (?, ?, ?)",
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
		"SELECT id, name, parent_id, created_at, updated_at FROM roles WHERE id = ?", id,
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
			"SELECT id, name, parent_id, created_at, updated_at FROM roles WHERE parent_id = ? ORDER BY name",
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
		"UPDATE roles SET name = ?, parent_id = ?, updated_at = datetime('now') WHERE id = ?",
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
	// Check for child roles
	var count int
	s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM roles WHERE parent_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("%w: role has %d child roles", domain.ErrHasDependencies, count)
	}

	// Check for agent assignments
	s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agent_roles WHERE role_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("%w: role assigned to %d agents", domain.ErrHasDependencies, count)
	}

	res, err := s.db.ExecContext(ctx, "DELETE FROM roles WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	return nil
}

// GetRoleChain returns the inheritance chain from the given role to the root,
// up to maxDepth levels. Uses a recursive CTE. Ordered leaf-to-root.
func (s *Store) GetRoleChain(ctx context.Context, roleID string, maxDepth int) ([]*domain.Role, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE chain(id, name, parent_id, created_at, updated_at, depth) AS (
			SELECT id, name, parent_id, created_at, updated_at, 1
			FROM roles WHERE id = ?
			UNION ALL
			SELECT r.id, r.name, r.parent_id, r.created_at, r.updated_at, c.depth + 1
			FROM roles r
			JOIN chain c ON r.id = c.parent_id
			WHERE c.depth < ?
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

- [ ] **Step 4: Run tests**

```bash
go test ./internal/infra/sqlite/... -v -run TestRole
```

Expected: all role tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/sqlite/roles.go internal/infra/sqlite/roles_test.go
git commit -m "feat: add SQLite roles store with recursive CTE chain query"
```

### Task 8: Agents store implementation

**Files:**
- Create: `internal/infra/sqlite/agents.go`
- Create: `internal/infra/sqlite/agents_test.go`

- [ ] **Step 1: Write agent store tests**

Create `internal/infra/sqlite/agents_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateAndGetAgent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	agent := &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := store.GetAgent(ctx, "a_001")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Name != "alice" || got.TeamID != "t_001" {
		t.Errorf("got %+v", got)
	}
}

func TestSetAndGetAgentRoles(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "reviewer"})

	roles := []domain.AgentRole{
		{AgentID: "a_001", RoleID: "r_001", Priority: 1},
		{AgentID: "a_001", RoleID: "r_002", Priority: 2},
	}
	if err := store.SetAgentRoles(ctx, "a_001", roles); err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	got, err := store.GetAgentRoles(ctx, "a_001")
	if err != nil {
		t.Fatalf("get agent roles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d roles, want 2", len(got))
	}
	if got[0].Priority != 1 || got[1].Priority != 2 {
		t.Errorf("roles not ordered by priority: %+v", got)
	}
}

func TestListAgentsByTeam(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTeam(ctx, &domain.Team{ID: "t_002", Name: "beta"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_002", Name: "bob", TeamID: "t_002"})

	agents, err := store.ListAgents(ctx, "t_001")
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "alice" {
		t.Errorf("expected only alice, got %+v", agents)
	}
}
```

- [ ] **Step 2: Implement agents store**

Create `internal/infra/sqlite/agents.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO agents (id, name, team_id) VALUES (?, ?, ?)",
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
		"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE id = ?", id,
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
			"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE team_id = ? ORDER BY name",
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
		"UPDATE agents SET name = ?, team_id = ?, updated_at = datetime('now') WHERE id = ?",
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
	// Check for assigned tasks
	var count int
	s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE agent_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("%w: agent has %d tasks", domain.ErrHasDependencies, count)
	}

	// agent_roles and agent_permissions cascade on delete
	res, err := s.db.ExecContext(ctx, "DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_roles WHERE agent_id = ?", agentID)
	if err != nil {
		return fmt.Errorf("clear agent roles: %w", err)
	}

	for _, r := range roles {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_roles (agent_id, role_id, priority) VALUES (?, ?, ?)",
			agentID, r.RoleID, r.Priority)
		if err != nil {
			return fmt.Errorf("insert agent role: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT agent_id, role_id, priority FROM agent_roles WHERE agent_id = ? ORDER BY priority",
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
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/infra/sqlite/... -v -run TestAgent
```

Expected: all agent tests pass (including the team dependency test from Task 6).

- [ ] **Step 4: Commit**

```bash
git add internal/infra/sqlite/agents.go internal/infra/sqlite/agents_test.go
git commit -m "feat: add SQLite agents store with role assignments"
```

## Chunk 3: Remaining Store Implementations

### Task 9: Documents store

**Files:**
- Create: `internal/infra/sqlite/documents.go`
- Create: `internal/infra/sqlite/documents_test.go`

- [ ] **Step 1: Write document store tests**

Create `internal/infra/sqlite/documents_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateRoleDocument(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})

	doc := &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindRole,
		Title: "Dev Guide", Content: "# Developer Guide\n...",
		RoleID: "r_001",
	}
	if err := store.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	got, err := store.GetDocument(ctx, "d_001")
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if got.Title != "Dev Guide" || got.RoleID != "r_001" {
		t.Errorf("got %+v", got)
	}
}

func TestCreateMemoryDocument(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	doc := &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindMemory,
		Title: "Patterns", Content: "# Learned Patterns\n...",
		AgentID: "a_001",
	}
	if err := store.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	docs, err := store.ListAgentMemory(ctx, "a_001")
	if err != nil {
		t.Fatalf("list agent memory: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "Patterns" {
		t.Errorf("got %+v", docs)
	}
}

func TestListRoleDocuments(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})
	store.CreateDocument(ctx, &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindRole,
		Title: "Guide A", RoleID: "r_001",
	})
	store.CreateDocument(ctx, &domain.Document{
		ID: "d_002", Kind: domain.DocumentKindRole,
		Title: "Guide B", RoleID: "r_001",
	})

	docs, err := store.ListRoleDocuments(ctx, "r_001")
	if err != nil {
		t.Fatalf("list role documents: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("got %d documents, want 2", len(docs))
	}
}
```

- [ ] **Step 2: Implement documents store**

Create `internal/infra/sqlite/documents.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

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
		"INSERT INTO documents (id, kind, title, content, role_id, agent_id) VALUES (?, ?, ?, ?, ?, ?)",
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
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE id = ?", id,
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
		"UPDATE documents SET title = ?, content = ?, updated_at = datetime('now') WHERE id = ?",
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
	res, err := s.db.ExecContext(ctx, "DELETE FROM documents WHERE id = ?", id)
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
	return s.listDocuments(ctx, "SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE role_id = ? ORDER BY title", roleID)
}

func (s *Store) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	return s.listDocuments(ctx, "SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE agent_id = ? ORDER BY title", agentID)
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

- [ ] **Step 3: Run tests**

```bash
go test ./internal/infra/sqlite/... -v -run TestDocument -run TestMemory -run TestListRole
```

Expected: all document tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/sqlite/documents.go internal/infra/sqlite/documents_test.go
git commit -m "feat: add SQLite documents store for role docs and agent memory"
```

### Task 10: Tasks store

**Files:**
- Create: `internal/infra/sqlite/tasks.go`
- Create: `internal/infra/sqlite/tasks_test.go`

- [ ] **Step 1: Write task store tests**

Create `internal/infra/sqlite/tasks_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateAndGetTask(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	task := &domain.Task{
		ID: "tk_001", TeamID: "t_001",
		Title: "Fix bug", Description: "Fix the login bug",
		Status: domain.TaskStatusPending,
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Title != "Fix bug" || got.Status != domain.TaskStatusPending {
		t.Errorf("got %+v", got)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTask(ctx, &domain.Task{
		ID: "tk_001", TeamID: "t_001", Title: "Fix bug",
		Status: domain.TaskStatusPending,
	})

	updated := &domain.Task{
		ID: "tk_001", TeamID: "t_001", Title: "Fix bug",
		Status: domain.TaskStatusInProgress,
	}
	if err := store.UpdateTask(ctx, "tk_001", updated); err != nil {
		t.Fatalf("update task: %v", err)
	}

	got, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.TaskStatusInProgress {
		t.Errorf("got status %q, want %q", got.Status, domain.TaskStatusInProgress)
	}
}

func TestListTeamTasks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTeam(ctx, &domain.Team{ID: "t_002", Name: "beta"})
	store.CreateTask(ctx, &domain.Task{ID: "tk_001", TeamID: "t_001", Title: "Task A", Status: domain.TaskStatusPending})
	store.CreateTask(ctx, &domain.Task{ID: "tk_002", TeamID: "t_002", Title: "Task B", Status: domain.TaskStatusPending})

	tasks, err := store.ListTeamTasks(ctx, "t_001")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Task A" {
		t.Errorf("expected only Task A, got %+v", tasks)
	}
}
```

- [ ] **Step 2: Implement tasks store**

Create `internal/infra/sqlite/tasks.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

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
		"INSERT INTO tasks (id, team_id, agent_id, title, description, status) VALUES (?, ?, ?, ?, ?, ?)",
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
		"SELECT id, team_id, agent_id, title, description, status, created_at, updated_at FROM tasks WHERE id = ?", id,
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
		"SELECT id, team_id, agent_id, title, description, status, created_at, updated_at FROM tasks WHERE team_id = ? ORDER BY created_at",
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
		"UPDATE tasks SET title = ?, description = ?, status = ?, agent_id = ?, updated_at = datetime('now') WHERE id = ?",
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
	res, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
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

- [ ] **Step 3: Run tests**

```bash
go test ./internal/infra/sqlite/... -v -run TestTask
```

Expected: all task tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/sqlite/tasks.go internal/infra/sqlite/tasks_test.go
git commit -m "feat: add SQLite tasks store"
```

### Task 11: Permissions store

**Files:**
- Create: `internal/infra/sqlite/permissions.go`
- Create: `internal/infra/sqlite/permissions_test.go`

- [ ] **Step 1: Write permission store tests**

Create `internal/infra/sqlite/permissions_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestSeedPermissions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	names := []string{"role:read", "role:write", "memory:read"}
	if err := store.SeedPermissions(ctx, names); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	// Seed again (idempotent)
	if err := store.SeedPermissions(ctx, names); err != nil {
		t.Fatalf("re-seed permissions: %v", err)
	}
}

func TestAgentPermissions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
	store.SeedPermissions(ctx, []string{"role:read", "memory:write", "task:read"})

	perms := []domain.AgentPermission{
		{AgentID: "a_001", PermissionID: "", ScopeTeamID: ""},
	}
	// We need permission IDs — get them from the store
	// For simplicity, test HasPermission directly

	// Set permissions using the name-based approach
	store.SetAgentPermissions(ctx, "a_001", []domain.AgentPermission{
		{AgentID: "a_001", PermissionID: "role:read", ScopeTeamID: ""},
		{AgentID: "a_001", PermissionID: "task:read", ScopeTeamID: "t_001"},
	})

	// Global permission check
	has, err := store.HasPermission(ctx, "a_001", "role:read", "")
	if err != nil {
		t.Fatalf("has permission: %v", err)
	}
	if !has {
		t.Error("expected alice to have global role:read")
	}

	// Scoped permission check
	has, err = store.HasPermission(ctx, "a_001", "task:read", "t_001")
	if err != nil {
		t.Fatalf("has permission: %v", err)
	}
	if !has {
		t.Error("expected alice to have task:read scoped to t_001")
	}

	// Permission not granted
	has, err = store.HasPermission(ctx, "a_001", "memory:write", "")
	if err != nil {
		t.Fatalf("has permission: %v", err)
	}
	if has {
		t.Error("expected alice NOT to have memory:write")
	}
}
```

- [ ] **Step 2: Implement permissions store**

Create `internal/infra/sqlite/permissions.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) SeedPermissions(ctx context.Context, names []string) error {
	for _, name := range names {
		_, err := s.db.ExecContext(ctx,
			"INSERT OR IGNORE INTO permissions (id, name) VALUES (?, ?)",
			"perm_"+name, name)
		if err != nil {
			return fmt.Errorf("seed permission %q: %w", name, err)
		}
	}
	return nil
}

func (s *Store) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ap.agent_id, p.name, COALESCE(ap.scope_team_id, '')
		FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = ?
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
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *Store) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_permissions WHERE agent_id = ?", agentID)
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
			"SELECT id FROM permissions WHERE name = ?", p.PermissionID,
		).Scan(&permID)
		if err != nil {
			return fmt.Errorf("resolve permission %q: %w", p.PermissionID, err)
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_permissions (agent_id, permission_id, scope_team_id) VALUES (?, ?, ?)",
			agentID, permID, scopeTeamID)
		if err != nil {
			return fmt.Errorf("insert permission: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) HasPermission(ctx context.Context, agentID, permName, scopeTeamID string) (bool, error) {
	// Check for global permission (scope_team_id IS NULL) or scoped permission
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = ? AND p.name = ?
		AND (ap.scope_team_id IS NULL OR ap.scope_team_id = ?)
	`, agentID, permName, scopeTeamID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return count > 0, nil
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/infra/sqlite/... -v -run TestPermission -run TestSeed -run TestAgent
```

Expected: all permission tests pass.

- [ ] **Step 4: Run full test suite**

```bash
go test ./internal/infra/sqlite/... -v
```

Expected: ALL tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/sqlite/permissions.go internal/infra/sqlite/permissions_test.go
git commit -m "feat: add SQLite permissions store with RBAC support"
```

### Task 12: Wire database into daemon

**Files:**
- Modify: `cmd/daemon.go`
- Modify: `internal/daemon/server.go`

- [ ] **Step 1: Update daemon to open database and seed permissions**

In `cmd/daemon.go`, add database initialization before creating the server:

After the health service creation, add:

```go
// Database
dsn := viper.GetString("db")
if dsn == "" {
	dsn = filepath.Join(config.GlobalPaths.StateDir, "hive.db")
}

store, err := infra.Open(dsn)
if err != nil {
	return fmt.Errorf("open database: %w", err)
}
defer store.Close()

// Seed permissions
permNames := []string{
	"role:read", "role:write",
	"memory:read", "memory:write",
	"task:read", "task:write",
	"agent:manage", "team:manage",
}
if err := store.SeedPermissions(context.Background(), permNames); err != nil {
	return fmt.Errorf("seed permissions: %w", err)
}
```

Add these imports: `"path/filepath"`, `"github.com/Work-Fort/Hive/internal/config"`,
`"github.com/Work-Fort/Hive/internal/infra"`.

Update `ServerConfig` to include `Store domain.Store` and pass it through.

- [ ] **Step 2: Verify build and run**

```bash
go mod tidy
go build -o build/hive .
./build/hive daemon --port 17099 &
sleep 1
curl -s http://127.0.0.1:17099/v1/health | jq .
kill %1
ls ~/.local/state/hive/hive.db
```

Expected: health returns healthy, database file exists.

- [ ] **Step 3: Commit**

```bash
git add cmd/daemon.go internal/daemon/server.go internal/infra/open.go go.mod go.sum
git commit -m "feat: wire SQLite database into daemon with permission seeding"
```
