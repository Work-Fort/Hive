# Export/Import Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `hive export` and `hive import` CLI commands for full-state transfer between filesystems and database backends.

**Architecture:** A `DataSource` interface abstracts REST vs direct-DB access. Export fetches entities in dependency order and writes YAML/front-matter files. Import parses files, validates references, and creates entities in dependency order. Timestamp preservation requires modifying store Create methods to accept timestamps from the struct.

**Tech Stack:** Go, gopkg.in/yaml.v3, Cobra CLI, existing domain.Store + client.Client

**Spec:** `docs/superpowers/specs/2026-03-11-export-import-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/transfer/datasource.go` | `DataSource` interface definition |
| `internal/transfer/db_source.go` | `dbDataSource` — wraps `domain.Store` for direct DB access |
| `internal/transfer/rest_source.go` | `restDataSource` — wraps `client.Client` for REST access |
| `internal/transfer/exporter.go` | Export logic: fetch entities, serialize, write files |
| `internal/transfer/importer.go` | Import logic: parse files, validate refs, create entities |
| `internal/transfer/yaml.go` | YAML struct types + front-matter marshal/unmarshal |
| `internal/transfer/sanitize.go` | Filename sanitization helper |
| `internal/transfer/exporter_test.go` | Unit tests for export logic |
| `internal/transfer/importer_test.go` | Unit tests for import logic |
| `internal/transfer/yaml_test.go` | Unit tests for YAML serialization |
| `internal/transfer/sanitize_test.go` | Unit tests for filename sanitization |
| `cmd/export/export.go` | `hive export` CLI subcommand |
| `cmd/importcmd/importcmd.go` | `hive import` CLI subcommand (package `importcmd` to avoid Go keyword) |
| `client/permissions_list.go` | `ListPermissions` + `CreatePermission` client methods |
| `tests/e2e/export_import_test.go` | E2E tests for export/import round-trip |

### Modified Files

| File | Change |
|------|--------|
| `internal/domain/ports.go` | Add lookup-by-name methods + `ListPermissions` + `LookupPermissionByName` to store interfaces |
| `internal/infra/sqlite/teams.go` | Add `LookupTeamByName` + modify `CreateTeam` to include timestamps |
| `internal/infra/sqlite/roles.go` | Add `LookupRoleByName` + modify `CreateRole` to include timestamps |
| `internal/infra/sqlite/agents.go` | Add `LookupAgentByName` + modify `CreateAgent` to include timestamps |
| `internal/infra/sqlite/documents.go` | Add `LookupDocumentByOwnerAndTitle` + modify `CreateDocument` to include timestamps |
| `internal/infra/sqlite/tasks.go` | Add `LookupTaskByTeamAndTitle` + modify `CreateTask` to include timestamps |
| `internal/infra/sqlite/permissions.go` | Add `ListPermissions` + `LookupPermissionByName` |
| `internal/daemon/rest_huma.go` | Add `GET /v1/permissions` and `POST /v1/permissions` routes |
| `internal/daemon/rest_types.go` | Add permission list/create Huma types |
| `cmd/root.go` | Register export + import subcommands |
| `go.mod` | Add direct `gopkg.in/yaml.v3` dependency |

---

## Chunk 1: Store Foundation

### Task 1: Add name-lookup methods to domain ports

**Files:**
- Modify: `internal/domain/ports.go`

- [ ] **Step 1: Add lookup methods to store interfaces**

Add these methods to the appropriate sub-interfaces in `internal/domain/ports.go`:

```go
// In TeamStore (after DeleteTeam):
LookupTeamByName(ctx context.Context, name string) (*Team, error)

// In RoleStore (after GetRoleChain):
LookupRoleByName(ctx context.Context, name string) (*Role, error)

// In AgentStore (after GetAgentRoles):
LookupAgentByName(ctx context.Context, name string) (*Agent, error)

// In DocumentStore (after ListAgentMemory):
LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*Document, error)

// In TaskStore (after DeleteTask):
LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*Task, error)

// In PermissionStore (after HasPermission):
ListPermissions(ctx context.Context) ([]*Permission, error)
LookupPermissionByName(ctx context.Context, name string) (*Permission, error)
```

- [ ] **Step 2: Verify compilation fails**

Run: `go build ./...`
Expected: FAIL — SQLite store (and Postgres if present) don't implement new methods

- [ ] **Step 3: Commit**

```bash
git add internal/domain/ports.go
git commit -m "feat: add name-lookup methods to store interfaces"
```

### Task 2: Implement name-lookup methods in SQLite store

**Files:**
- Modify: `internal/infra/sqlite/teams.go`
- Modify: `internal/infra/sqlite/roles.go`
- Modify: `internal/infra/sqlite/agents.go`
- Modify: `internal/infra/sqlite/documents.go`
- Modify: `internal/infra/sqlite/tasks.go`
- Modify: `internal/infra/sqlite/permissions.go`

- [ ] **Step 1: Write tests for all lookup methods**

Create `internal/infra/sqlite/lookup_test.go`:

```go
package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLookupTeamByName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t1", Name: "engineering"}
	if err := s.CreateTeam(ctx, team); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupTeamByName(ctx, "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "t1" {
		t.Errorf("got ID %q, want t1", got.ID)
	}

	_, err = s.LookupTeamByName(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestLookupRoleByName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	role := &domain.Role{ID: "r1", Name: "developer"}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupRoleByName(ctx, "developer")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "r1" {
		t.Errorf("got ID %q, want r1", got.ID)
	}
}

func TestLookupAgentByName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t1", Name: "eng"}
	if err := s.CreateTeam(ctx, team); err != nil {
		t.Fatal(err)
	}

	agent := &domain.Agent{ID: "a1", Name: "claude", TeamID: "t1"}
	if err := s.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupAgentByName(ctx, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "a1" {
		t.Errorf("got ID %q, want a1", got.ID)
	}
}

func TestLookupDocumentByOwnerAndTitle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	role := &domain.Role{ID: "r1", Name: "dev"}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatal(err)
	}

	doc := &domain.Document{ID: "d1", Kind: domain.DocumentKindRole, Title: "Standards", RoleID: "r1"}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupDocumentByOwnerAndTitle(ctx, "r1", "Standards")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "d1" {
		t.Errorf("got ID %q, want d1", got.ID)
	}
}

func TestLookupTaskByTeamAndTitle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t1", Name: "eng"}
	if err := s.CreateTeam(ctx, team); err != nil {
		t.Fatal(err)
	}

	task := &domain.Task{ID: "tk1", TeamID: "t1", Title: "Fix bug", Status: domain.TaskStatusPending}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupTaskByTeamAndTitle(ctx, "t1", "Fix bug")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "tk1" {
		t.Errorf("got ID %q, want tk1", got.ID)
	}
}

func TestListPermissions(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.SeedPermissions(ctx, []string{"read", "write"}); err != nil {
		t.Fatal(err)
	}

	perms, err := s.ListPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(perms) != 2 {
		t.Fatalf("got %d perms, want 2", len(perms))
	}
}

func TestLookupPermissionByName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.SeedPermissions(ctx, []string{"read"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupPermissionByName(ctx, "read")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "read" {
		t.Errorf("got name %q, want read", got.Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/infra/sqlite/ -run TestLookup -v`
Expected: FAIL — methods not implemented

- [ ] **Step 3: Implement lookup methods**

Add to `internal/infra/sqlite/teams.go`:
```go
func (s *Store) LookupTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	var t domain.Team
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, created_at, updated_at FROM teams WHERE name = ?", name,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: team named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup team by name: %w", err)
	}
	return &t, nil
}
```

Add to `internal/infra/sqlite/roles.go`:
```go
func (s *Store) LookupRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	var r domain.Role
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, COALESCE(parent_id, ''), created_at, updated_at FROM roles WHERE name = ?", name,
	).Scan(&r.ID, &r.Name, &r.ParentID, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: role named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup role by name: %w", err)
	}
	return &r, nil
}
```

Add to `internal/infra/sqlite/agents.go`:
```go
func (s *Store) LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error) {
	var a domain.Agent
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE name = ?", name,
	).Scan(&a.ID, &a.Name, &a.TeamID, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: agent named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup agent by name: %w", err)
	}
	return &a, nil
}
```

Add to `internal/infra/sqlite/documents.go`:
```go
func (s *Store) LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error) {
	var d domain.Document
	err := s.db.QueryRowContext(ctx, `
		SELECT id, kind, title, content, COALESCE(role_id, ''), COALESCE(agent_id, ''), created_at, updated_at
		FROM documents
		WHERE (role_id = ? OR agent_id = ?) AND title = ?`,
		ownerID, ownerID, title,
	).Scan(&d.ID, &d.Kind, &d.Title, &d.Content, &d.RoleID, &d.AgentID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: document %q for owner %q", domain.ErrNotFound, title, ownerID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup document by owner and title: %w", err)
	}
	return &d, nil
}
```

Add to `internal/infra/sqlite/tasks.go`:
```go
func (s *Store) LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error) {
	var t domain.Task
	err := s.db.QueryRowContext(ctx, `
		SELECT id, team_id, COALESCE(agent_id, ''), title, description, status, created_at, updated_at
		FROM tasks
		WHERE team_id = ? AND title = ?`,
		teamID, title,
	).Scan(&t.ID, &t.TeamID, &t.AgentID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: task %q in team %q", domain.ErrNotFound, title, teamID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup task by team and title: %w", err)
	}
	return &t, nil
}
```

Add to `internal/infra/sqlite/permissions.go`:
```go
func (s *Store) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name FROM permissions ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var perms []*domain.Permission
	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *Store) LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	var p domain.Permission
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name FROM permissions WHERE name = ?", name,
	).Scan(&p.ID, &p.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: permission named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup permission by name: %w", err)
	}
	return &p, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/infra/sqlite/ -run "TestLookup|TestListPermissions" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/sqlite/
git commit -m "feat: add name-lookup and permission-list methods to SQLite store"
```

### Task 3: Modify store Create methods to accept timestamps

The current Create methods (e.g., `CreateTeam`) insert only `id` and `name`, relying on SQLite `DEFAULT (datetime('now'))` for timestamps. For import with timestamp preservation, modify them to include timestamps from the struct when non-zero, falling back to `time.Now()` when zero.

**Files:**
- Modify: `internal/infra/sqlite/teams.go`
- Modify: `internal/infra/sqlite/roles.go`
- Modify: `internal/infra/sqlite/agents.go`
- Modify: `internal/infra/sqlite/documents.go`
- Modify: `internal/infra/sqlite/tasks.go`

- [ ] **Step 1: Write test for timestamp preservation**

Add to `internal/infra/sqlite/lookup_test.go`:

```go
func TestCreateTeamPreservesTimestamps(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	team := &domain.Team{ID: "t1", Name: "eng", CreatedAt: ts, UpdatedAt: ts}
	if err := s.CreateTeam(ctx, team); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTeam(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, ts)
	}
	if !got.UpdatedAt.Equal(ts) {
		t.Errorf("updated_at = %v, want %v", got.UpdatedAt, ts)
	}
}

func TestCreateTeamDefaultsTimestamps(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t1", Name: "eng"}
	if err := s.CreateTeam(ctx, team); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTeam(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}
```

- [ ] **Step 2: Run test to verify timestamp preservation fails**

Run: `go test ./internal/infra/sqlite/ -run TestCreateTeamPreservesTimestamps -v`
Expected: FAIL — current INSERT doesn't include timestamps, so DB default overwrites them

- [ ] **Step 3: Modify Create methods**

Pattern for each entity (shown for teams, apply same pattern to roles, agents, documents, tasks):

In `internal/infra/sqlite/teams.go`, replace `CreateTeam`:

```go
func (s *Store) CreateTeam(ctx context.Context, t *domain.Team) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO teams (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		t.ID, t.Name, t.CreatedAt.UTC(), t.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: team %q", domain.ErrAlreadyExists, t.Name)
		}
		return fmt.Errorf("insert team: %w", err)
	}
	return nil
}
```

Apply same pattern to:
- `CreateRole` — add `created_at, updated_at` to INSERT (also includes `parent_id`)
- `CreateAgent` — add `created_at, updated_at` to INSERT (also includes `team_id`)
- `CreateDocument` — add `created_at, updated_at` to INSERT (already has many columns)
- `CreateTask` — add `created_at, updated_at` to INSERT (already has many columns)

Each needs `import "time"` added if not already present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/infra/sqlite/ -run "TestCreateTeam" -v`
Expected: PASS — both timestamp preservation and default behavior work

- [ ] **Step 5: Run full store test suite to check for regressions**

Run: `go test ./internal/infra/sqlite/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/sqlite/
git commit -m "feat: store Create methods accept timestamps for import support"
```

### Task 4: Add permissions REST endpoints and client methods

**Files:**
- Modify: `internal/daemon/rest_types.go`
- Modify: `internal/daemon/rest_huma.go`
- Create: `client/permissions_list.go`

- [ ] **Step 1: Add Huma types for permissions**

Add to `internal/daemon/rest_types.go`:

```go
// -- Permissions --

type listPermissionsOutput struct {
	Body []permissionEntityResponse `json:"body"`
}

type permissionEntityResponse struct {
	ID   string `json:"ID" doc:"Permission ID"`
	Name string `json:"Name" doc:"Permission name"`
}

type createPermissionInput struct {
	Body struct {
		Name string `json:"name" doc:"Permission name" minLength:"1"`
	}
}

type createPermissionOutput struct {
	Body permissionEntityResponse `json:"body"`
}
```

- [ ] **Step 2: Register permission routes**

Add to `internal/daemon/rest_huma.go`:

```go
func registerPermissionListRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-permissions",
		Method:      http.MethodGet,
		Path:        "/v1/permissions",
		Summary:     "List all permissions",
	}, func(ctx context.Context, input *struct{}) (*listPermissionsOutput, error) {
		perms, err := store.ListPermissions(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		out := &listPermissionsOutput{}
		for _, p := range perms {
			out.Body = append(out.Body, permissionEntityResponse{ID: p.ID, Name: p.Name})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-permission",
		Method:        http.MethodPost,
		Path:          "/v1/permissions",
		Summary:       "Create a permission",
		DefaultStatus: 201,
	}, func(ctx context.Context, input *createPermissionInput) (*createPermissionOutput, error) {
		if err := store.SeedPermissions(ctx, []string{input.Body.Name}); err != nil {
			return nil, mapDomainErr(err)
		}
		p, err := store.LookupPermissionByName(ctx, input.Body.Name)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &createPermissionOutput{Body: permissionEntityResponse{ID: p.ID, Name: p.Name}}, nil
	})
}
```

Wire into `server.go` — add `registerPermissionListRoutes(api, cfg.Store)` alongside the other `register*Routes` calls.

- [ ] **Step 3: Add client methods**

Create `client/permissions_list.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// Permission represents a named capability.
type Permission struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
}

// ListPermissions returns all registered permissions.
func (c *Client) ListPermissions(ctx context.Context) ([]Permission, error) {
	var out []Permission
	return out, c.do(ctx, http.MethodGet, "/v1/permissions", nil, &out)
}

// CreatePermission creates a permission by name (idempotent).
func (c *Client) CreatePermission(ctx context.Context, name string) (*Permission, error) {
	body := map[string]string{"name": name}
	var out Permission
	return &out, c.do(ctx, http.MethodPost, "/v1/permissions", body, &out)
}
```

- [ ] **Step 4: Run build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Run E2E tests to check for regressions**

Run: `cd tests/e2e && go test -v -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/rest_types.go internal/daemon/rest_huma.go internal/daemon/server.go client/permissions_list.go
git commit -m "feat: add GET/POST /v1/permissions endpoints and client methods"
```

---

## Chunk 2: Transfer Package — Interface, Serialization, DataSource Implementations

### Task 5: YAML serialization types and helpers

**Files:**
- Create: `internal/transfer/yaml.go`
- Create: `internal/transfer/yaml_test.go`
- Create: `internal/transfer/sanitize.go`
- Create: `internal/transfer/sanitize_test.go`

- [ ] **Step 1: Add yaml.v3 as direct dependency**

Run: `go get gopkg.in/yaml.v3`

- [ ] **Step 2: Write sanitize tests**

Create `internal/transfer/sanitize_test.go`:

```go
package transfer

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Engineering", "engineering"},
		{"My Role!", "my-role"},
		{"hello world", "hello-world"},
		{"a--b", "a--b"},
		{"  spaces  ", "spaces"},
		{"café", "caf"},
	}
	for _, tt := range tests {
		got := SanitizeName(tt.in)
		if got != tt.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: Implement SanitizeName**

Create `internal/transfer/sanitize.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"regexp"
	"strings"
)

var nonAlphanumHyphen = regexp.MustCompile(`[^a-z0-9-]+`)
var multiHyphen = regexp.MustCompile(`-{3,}`)

// SanitizeName converts an entity name to a safe filename component.
// Lowercase, spaces→hyphens, non-alphanumeric stripped.
func SanitizeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumHyphen.ReplaceAllString(s, "")
	s = multiHyphen.ReplaceAllString(s, "--")
	s = strings.Trim(s, "-")
	return s
}
```

- [ ] **Step 4: Run sanitize tests**

Run: `go test ./internal/transfer/ -run TestSanitize -v`
Expected: PASS

- [ ] **Step 5: Write YAML serialization types and front-matter helpers**

Create `internal/transfer/yaml.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// YAML types matching the spec file formats.

type TeamFile struct {
	Name      string    `yaml:"name"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

type RoleFile struct {
	Name      string    `yaml:"name"`
	Parent    string    `yaml:"parent"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

type PermissionFile struct {
	Name string `yaml:"name"`
}

type AgentRoleEntry struct {
	Role     string `yaml:"role"`
	Priority int    `yaml:"priority"`
}

type AgentPermissionEntry struct {
	Permission string `yaml:"permission"`
	ScopeTeam  string `yaml:"scope_team,omitempty"`
}

type AgentFile struct {
	Name        string                 `yaml:"name"`
	Team        string                 `yaml:"team"`
	CreatedAt   time.Time              `yaml:"created_at"`
	UpdatedAt   time.Time              `yaml:"updated_at"`
	Roles       []AgentRoleEntry       `yaml:"roles,omitempty"`
	Permissions []AgentPermissionEntry `yaml:"permissions,omitempty"`
}

type DocumentFrontMatter struct {
	Title     string    `yaml:"title"`
	Kind      string    `yaml:"kind"`
	Role      string    `yaml:"role,omitempty"`
	Agent     string    `yaml:"agent,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

type TaskFile struct {
	Title       string    `yaml:"title"`
	Team        string    `yaml:"team"`
	Agent       string    `yaml:"agent,omitempty"`
	Status      string    `yaml:"status"`
	CreatedAt   time.Time `yaml:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at"`
	Description string    `yaml:"description,omitempty"`
}

// MarshalFrontMatter writes YAML front-matter + body content.
func MarshalFrontMatter(fm *DocumentFrontMatter, body string) ([]byte, error) {
	header, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal front matter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(header)
	buf.WriteString("---\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}

// UnmarshalFrontMatter parses YAML front-matter delimited by "---" and returns
// the front matter and remaining body content.
func UnmarshalFrontMatter(r io.Reader) (*DocumentFrontMatter, string, error) {
	scanner := bufio.NewScanner(r)

	// Expect first line to be "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, "", fmt.Errorf("expected front matter delimiter '---'")
	}

	var fmLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}

	var fm DocumentFrontMatter
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
		return nil, "", fmt.Errorf("unmarshal front matter: %w", err)
	}

	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	return &fm, strings.Join(bodyLines, "\n"), nil
}
```

- [ ] **Step 6: Write YAML tests**

Create `internal/transfer/yaml_test.go`:

```go
package transfer

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestTeamFileRoundTrip(t *testing.T) {
	ts := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	tf := TeamFile{Name: "engineering", CreatedAt: ts, UpdatedAt: ts}

	data, err := yaml.Marshal(tf)
	if err != nil {
		t.Fatal(err)
	}

	var got TeamFile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "engineering" {
		t.Errorf("name = %q, want engineering", got.Name)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, ts)
	}
}

func TestFrontMatterRoundTrip(t *testing.T) {
	ts := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	fm := &DocumentFrontMatter{
		Title:     "Coding Standards",
		Kind:      "role",
		Role:      "developer",
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	body := "Some markdown content\nWith multiple lines"

	data, err := MarshalFrontMatter(fm, body)
	if err != nil {
		t.Fatal(err)
	}

	gotFM, gotBody, err := UnmarshalFrontMatter(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	if gotFM.Title != "Coding Standards" {
		t.Errorf("title = %q, want Coding Standards", gotFM.Title)
	}
	if gotFM.Kind != "role" {
		t.Errorf("kind = %q, want role", gotFM.Kind)
	}
	if gotFM.Role != "developer" {
		t.Errorf("role = %q, want developer", gotFM.Role)
	}
	if gotBody != body {
		t.Errorf("body = %q, want %q", gotBody, body)
	}
}

func TestAgentFileWithPermissions(t *testing.T) {
	af := AgentFile{
		Name: "claude",
		Team: "engineering",
		Roles: []AgentRoleEntry{
			{Role: "developer", Priority: 1},
		},
		Permissions: []AgentPermissionEntry{
			{Permission: "read-docs", ScopeTeam: "engineering"},
			{Permission: "write-docs"},
		},
	}

	data, err := yaml.Marshal(af)
	if err != nil {
		t.Fatal(err)
	}

	var got AgentFile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if len(got.Permissions) != 2 {
		t.Fatalf("got %d permissions, want 2", len(got.Permissions))
	}
	if got.Permissions[0].ScopeTeam != "engineering" {
		t.Errorf("scope_team = %q, want engineering", got.Permissions[0].ScopeTeam)
	}
	if got.Permissions[1].ScopeTeam != "" {
		t.Errorf("global perm scope_team = %q, want empty", got.Permissions[1].ScopeTeam)
	}
}
```

- [ ] **Step 7: Run YAML tests**

Run: `go test ./internal/transfer/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/transfer/ go.mod go.sum
git commit -m "feat: add transfer package with YAML types, front-matter helpers, and filename sanitization"
```

### Task 6: DataSource interface and dbDataSource implementation

**Files:**
- Create: `internal/transfer/datasource.go`
- Create: `internal/transfer/db_source.go`

- [ ] **Step 1: Create DataSource interface**

Create `internal/transfer/datasource.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"

	"github.com/Work-Fort/Hive/internal/domain"
)

// DataSource abstracts read/write access to Hive entities, allowing
// export/import logic to work against either a REST API or a direct database.
type DataSource interface {
	// Teams
	ListTeams(ctx context.Context) ([]*domain.Team, error)
	CreateTeam(ctx context.Context, t *domain.Team) error
	UpdateTeam(ctx context.Context, id, name string) error
	LookupTeamByName(ctx context.Context, name string) (*domain.Team, error)

	// Roles
	ListAllRoles(ctx context.Context) ([]*domain.Role, error)
	CreateRole(ctx context.Context, r *domain.Role) error
	UpdateRole(ctx context.Context, id, name, parentID string) error
	LookupRoleByName(ctx context.Context, name string) (*domain.Role, error)

	// Permissions
	ListPermissions(ctx context.Context) ([]*domain.Permission, error)
	EnsurePermission(ctx context.Context, name string) error
	LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error)

	// Agents
	ListAllAgents(ctx context.Context) ([]*domain.Agent, error)
	CreateAgent(ctx context.Context, a *domain.Agent) error
	UpdateAgent(ctx context.Context, id, name, teamID string) error
	LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error)
	GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error)
	SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error
	GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error)
	SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error

	// Documents
	ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error)
	ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error)
	CreateDocument(ctx context.Context, d *domain.Document) error
	UpdateDocument(ctx context.Context, id, title, content string) error
	LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error)

	// Tasks
	ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error)
	CreateTask(ctx context.Context, t *domain.Task) error
	UpdateTask(ctx context.Context, id string, t *domain.Task) error
	LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error)
}
```

- [ ] **Step 2: Implement dbDataSource**

Create `internal/transfer/db_source.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"

	"github.com/Work-Fort/Hive/internal/domain"
)

// dbDataSource wraps a domain.Store for direct database access.
type dbDataSource struct {
	store domain.Store
}

// NewDBDataSource creates a DataSource backed by a domain.Store.
func NewDBDataSource(store domain.Store) DataSource {
	return &dbDataSource{store: store}
}

func (d *dbDataSource) ListTeams(ctx context.Context) ([]*domain.Team, error) {
	return d.store.ListTeams(ctx)
}

func (d *dbDataSource) CreateTeam(ctx context.Context, t *domain.Team) error {
	return d.store.CreateTeam(ctx, t)
}

func (d *dbDataSource) UpdateTeam(ctx context.Context, id, name string) error {
	return d.store.UpdateTeam(ctx, id, name)
}

func (d *dbDataSource) LookupTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	return d.store.LookupTeamByName(ctx, name)
}

func (d *dbDataSource) ListAllRoles(ctx context.Context) ([]*domain.Role, error) {
	return d.store.ListRoles(ctx, "")
}

func (d *dbDataSource) CreateRole(ctx context.Context, r *domain.Role) error {
	return d.store.CreateRole(ctx, r)
}

func (d *dbDataSource) UpdateRole(ctx context.Context, id, name, parentID string) error {
	return d.store.UpdateRole(ctx, id, name, parentID)
}

func (d *dbDataSource) LookupRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	return d.store.LookupRoleByName(ctx, name)
}

func (d *dbDataSource) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	return d.store.ListPermissions(ctx)
}

func (d *dbDataSource) EnsurePermission(ctx context.Context, name string) error {
	return d.store.SeedPermissions(ctx, []string{name})
}

func (d *dbDataSource) LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	return d.store.LookupPermissionByName(ctx, name)
}

func (d *dbDataSource) ListAllAgents(ctx context.Context) ([]*domain.Agent, error) {
	return d.store.ListAgents(ctx, "")
}

func (d *dbDataSource) CreateAgent(ctx context.Context, a *domain.Agent) error {
	return d.store.CreateAgent(ctx, a)
}

func (d *dbDataSource) UpdateAgent(ctx context.Context, id, name, teamID string) error {
	return d.store.UpdateAgent(ctx, id, name, teamID)
}

func (d *dbDataSource) LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error) {
	return d.store.LookupAgentByName(ctx, name)
}

func (d *dbDataSource) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	return d.store.GetAgentRoles(ctx, agentID)
}

func (d *dbDataSource) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	return d.store.SetAgentRoles(ctx, agentID, roles)
}

func (d *dbDataSource) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	return d.store.GetAgentPermissions(ctx, agentID)
}

func (d *dbDataSource) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	return d.store.SetAgentPermissions(ctx, agentID, perms)
}

func (d *dbDataSource) ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error) {
	return d.store.ListRoleDocuments(ctx, roleID)
}

func (d *dbDataSource) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	return d.store.ListAgentMemory(ctx, agentID)
}

func (d *dbDataSource) CreateDocument(ctx context.Context, doc *domain.Document) error {
	return d.store.CreateDocument(ctx, doc)
}

func (d *dbDataSource) UpdateDocument(ctx context.Context, id, title, content string) error {
	return d.store.UpdateDocument(ctx, id, title, content)
}

func (d *dbDataSource) LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error) {
	return d.store.LookupDocumentByOwnerAndTitle(ctx, ownerID, title)
}

func (d *dbDataSource) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	return d.store.ListTeamTasks(ctx, teamID)
}

func (d *dbDataSource) CreateTask(ctx context.Context, t *domain.Task) error {
	return d.store.CreateTask(ctx, t)
}

func (d *dbDataSource) UpdateTask(ctx context.Context, id string, t *domain.Task) error {
	return d.store.UpdateTask(ctx, id, t)
}

func (d *dbDataSource) LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error) {
	return d.store.LookupTaskByTeamAndTitle(ctx, teamID, title)
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/transfer/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transfer/datasource.go internal/transfer/db_source.go
git commit -m "feat: add DataSource interface and dbDataSource implementation"
```

### Task 7: restDataSource implementation

**Files:**
- Create: `internal/transfer/rest_source.go`

- [ ] **Step 1: Implement restDataSource**

Create `internal/transfer/rest_source.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/client"
	"github.com/Work-Fort/Hive/internal/domain"
)

// restDataSource wraps a client.Client for REST API access.
type restDataSource struct {
	c *client.Client
}

// NewRESTDataSource creates a DataSource backed by the Hive REST API.
func NewRESTDataSource(c *client.Client) DataSource {
	return &restDataSource{c: c}
}

func (r *restDataSource) ListTeams(ctx context.Context) ([]*domain.Team, error) {
	teams, err := r.c.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Team, len(teams))
	for i, t := range teams {
		out[i] = &domain.Team{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
	}
	return out, nil
}

func (r *restDataSource) CreateTeam(ctx context.Context, t *domain.Team) error {
	_, err := r.c.CreateTeam(ctx, t.Name)
	return err
}

func (r *restDataSource) UpdateTeam(ctx context.Context, id, name string) error {
	_, err := r.c.UpdateTeam(ctx, id, name)
	return err
}

func (r *restDataSource) LookupTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	teams, err := r.c.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range teams {
		if t.Name == name {
			return &domain.Team{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}, nil
		}
	}
	return nil, fmt.Errorf("%w: team named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) ListAllRoles(ctx context.Context) ([]*domain.Role, error) {
	roles, err := r.c.ListRoles(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Role, len(roles))
	for i, rl := range roles {
		out[i] = &domain.Role{ID: rl.ID, Name: rl.Name, ParentID: rl.ParentID, CreatedAt: rl.CreatedAt, UpdatedAt: rl.UpdatedAt}
	}
	return out, nil
}

func (r *restDataSource) CreateRole(ctx context.Context, rl *domain.Role) error {
	_, err := r.c.CreateRole(ctx, rl.Name, rl.ParentID)
	return err
}

func (r *restDataSource) UpdateRole(ctx context.Context, id, name, parentID string) error {
	_, err := r.c.UpdateRole(ctx, id, name, parentID)
	return err
}

func (r *restDataSource) LookupRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	roles, err := r.c.ListRoles(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, rl := range roles {
		if rl.Name == name {
			return &domain.Role{ID: rl.ID, Name: rl.Name, ParentID: rl.ParentID, CreatedAt: rl.CreatedAt, UpdatedAt: rl.UpdatedAt}, nil
		}
	}
	return nil, fmt.Errorf("%w: role named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	perms, err := r.c.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Permission, len(perms))
	for i, p := range perms {
		out[i] = &domain.Permission{ID: p.ID, Name: p.Name}
	}
	return out, nil
}

func (r *restDataSource) EnsurePermission(ctx context.Context, name string) error {
	_, err := r.c.CreatePermission(ctx, name)
	return err
}

func (r *restDataSource) LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	perms, err := r.c.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if p.Name == name {
			return &domain.Permission{ID: p.ID, Name: p.Name}, nil
		}
	}
	return nil, fmt.Errorf("%w: permission named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) ListAllAgents(ctx context.Context) ([]*domain.Agent, error) {
	agents, err := r.c.ListAgents(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Agent, len(agents))
	for i, a := range agents {
		out[i] = &domain.Agent{ID: a.ID, Name: a.Name, TeamID: a.TeamID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
	}
	return out, nil
}

func (r *restDataSource) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := r.c.CreateAgent(ctx, a.Name, a.TeamID)
	return err
}

func (r *restDataSource) UpdateAgent(ctx context.Context, id, name, teamID string) error {
	_, err := r.c.UpdateAgent(ctx, id, name, teamID)
	return err
}

func (r *restDataSource) LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error) {
	agents, err := r.c.ListAgents(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.Name == name {
			return &domain.Agent{ID: a.ID, Name: a.Name, TeamID: a.TeamID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}, nil
		}
	}
	return nil, fmt.Errorf("%w: agent named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	// GetAgent returns roles inline
	a, err := r.c.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	var roles []domain.AgentRole
	for _, ar := range a.Roles {
		roles = append(roles, domain.AgentRole{AgentID: agentID, RoleID: ar.RoleID, Priority: ar.Priority})
	}
	return roles, nil
}

func (r *restDataSource) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	assignments := make([]client.RoleAssignment, len(roles))
	for i, ar := range roles {
		assignments[i] = client.RoleAssignment{RoleID: ar.RoleID, Priority: ar.Priority}
	}
	_, err := r.c.SetAgentRoles(ctx, agentID, assignments)
	return err
}

func (r *restDataSource) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	perms, err := r.c.GetAgentPermissions(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentPermission, len(perms))
	for i, p := range perms {
		out[i] = domain.AgentPermission{AgentID: agentID, PermissionID: p.PermissionID, ScopeTeamID: p.ScopeTeamID}
	}
	return out, nil
}

func (r *restDataSource) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	grants := make([]client.PermissionGrant, len(perms))
	for i, p := range perms {
		grants[i] = client.PermissionGrant{Permission: p.PermissionID, ScopeTeamID: p.ScopeTeamID}
	}
	_, err := r.c.SetAgentPermissions(ctx, agentID, grants)
	return err
}

func (r *restDataSource) ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error) {
	docs, err := r.c.ListRoleDocuments(ctx, roleID)
	if err != nil {
		return nil, err
	}
	return clientDocsToDomain(docs), nil
}

func (r *restDataSource) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	docs, err := r.c.ListAgentMemory(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return clientDocsToDomain(docs), nil
}

func (r *restDataSource) CreateDocument(ctx context.Context, d *domain.Document) error {
	if d.RoleID != "" {
		_, err := r.c.CreateRoleDocument(ctx, d.RoleID, d.Title, d.Content)
		return err
	}
	_, err := r.c.CreateAgentMemory(ctx, d.AgentID, d.Title, d.Content)
	return err
}

func (r *restDataSource) UpdateDocument(ctx context.Context, id, title, content string) error {
	_, err := r.c.UpdateDocument(ctx, id, title, content)
	return err
}

func (r *restDataSource) LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error) {
	// Try role documents first, then agent memory
	roleDocs, err := r.c.ListRoleDocuments(ctx, ownerID)
	if err == nil {
		for _, d := range roleDocs {
			if d.Title == title {
				return clientDocToDomain(d), nil
			}
		}
	}
	agentDocs, err2 := r.c.ListAgentMemory(ctx, ownerID)
	if err2 == nil {
		for _, d := range agentDocs {
			if d.Title == title {
				return clientDocToDomain(d), nil
			}
		}
	}
	if err != nil && err2 != nil {
		return nil, fmt.Errorf("lookup document: role err: %w, agent err: %v", err, err2)
	}
	return nil, fmt.Errorf("%w: document %q for owner %q", domain.ErrNotFound, title, ownerID)
}

func (r *restDataSource) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	tasks, err := r.c.ListTeamTasks(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Task, len(tasks))
	for i, tk := range tasks {
		out[i] = &domain.Task{
			ID: tk.ID, TeamID: tk.TeamID, AgentID: tk.AgentID,
			Title: tk.Title, Description: tk.Description,
			Status: domain.TaskStatus(tk.Status),
			CreatedAt: tk.CreatedAt, UpdatedAt: tk.UpdatedAt,
		}
	}
	return out, nil
}

func (r *restDataSource) CreateTask(ctx context.Context, t *domain.Task) error {
	_, err := r.c.CreateTask(ctx, client.CreateTaskInput{
		TeamID:      t.TeamID,
		Title:       t.Title,
		Description: t.Description,
	})
	return err
}

func (r *restDataSource) UpdateTask(ctx context.Context, id string, t *domain.Task) error {
	_, err := r.c.UpdateTask(ctx, id, client.UpdateTaskInput{
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		AgentID:     t.AgentID,
	})
	return err
}

func (r *restDataSource) LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error) {
	tasks, err := r.c.ListTeamTasks(ctx, teamID)
	if err != nil {
		return nil, err
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return &domain.Task{
				ID: tk.ID, TeamID: tk.TeamID, AgentID: tk.AgentID,
				Title: tk.Title, Description: tk.Description,
				Status: domain.TaskStatus(tk.Status),
				CreatedAt: tk.CreatedAt, UpdatedAt: tk.UpdatedAt,
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: task %q in team %q", domain.ErrNotFound, title, teamID)
}

// helpers

func clientDocsToDomain(docs []client.Document) []*domain.Document {
	out := make([]*domain.Document, len(docs))
	for i, d := range docs {
		out[i] = clientDocToDomain(d)
	}
	return out
}

func clientDocToDomain(d client.Document) *domain.Document {
	return &domain.Document{
		ID: d.ID, Kind: domain.DocumentKind(d.Kind), Title: d.Title,
		Content: d.Content, RoleID: d.RoleID, AgentID: d.AgentID,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}
```


- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/transfer/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/transfer/rest_source.go
git commit -m "feat: add restDataSource wrapping client.Client"
```

---

## Chunk 3: Export and Import Logic

### Task 8: Exporter

**Files:**
- Create: `internal/transfer/exporter.go`
- Create: `internal/transfer/exporter_test.go`

- [ ] **Step 1: Write exporter test**

Create `internal/transfer/exporter_test.go`. Test uses `dbDataSource` with an in-memory SQLite store:

```go
package transfer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
	"github.com/Work-Fort/Hive/internal/transfer"
)

func seedTestData(t *testing.T, s *sqlite.Store) {
	t.Helper()
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(s.CreateTeam(ctx, &domain.Team{ID: "t1", Name: "engineering"}))
	must(s.CreateRole(ctx, &domain.Role{ID: "r1", Name: "developer"}))
	must(s.CreateRole(ctx, &domain.Role{ID: "r2", Name: "reviewer", ParentID: "r1"}))
	must(s.SeedPermissions(ctx, []string{"read-docs", "write-docs"}))
	must(s.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "claude", TeamID: "t1"}))
	must(s.SetAgentRoles(ctx, "a1", []domain.AgentRole{
		{AgentID: "a1", RoleID: "r1", Priority: 1},
	}))
	must(s.SetAgentPermissions(ctx, "a1", []domain.AgentPermission{
		{AgentID: "a1", PermissionID: "read-docs"},
	}))
	must(s.CreateDocument(ctx, &domain.Document{
		ID: "d1", Kind: domain.DocumentKindRole, Title: "Standards", RoleID: "r1",
	}))
	must(s.CreateDocument(ctx, &domain.Document{
		ID: "d2", Kind: domain.DocumentKindMemory, Title: "Notes", Content: "some notes", AgentID: "a1",
	}))
	must(s.CreateTask(ctx, &domain.Task{
		ID: "tk1", TeamID: "t1", Title: "Fix bug", Status: domain.TaskStatusPending,
	}))
}

func TestExport(t *testing.T) {
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	seedTestData(t, s)

	dir := t.TempDir()
	ds := transfer.NewDBDataSource(s)
	result, err := transfer.Export(context.Background(), ds, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Teams != 1 {
		t.Errorf("teams = %d, want 1", result.Teams)
	}
	if result.Roles != 2 {
		t.Errorf("roles = %d, want 2", result.Roles)
	}
	if result.Permissions != 2 {
		t.Errorf("permissions = %d, want 2", result.Permissions)
	}
	if result.Agents != 1 {
		t.Errorf("agents = %d, want 1", result.Agents)
	}
	if result.Documents != 2 {
		t.Errorf("documents = %d, want 2", result.Documents)
	}
	if result.Tasks != 1 {
		t.Errorf("tasks = %d, want 1", result.Tasks)
	}

	// Verify files exist
	assertFileExists(t, filepath.Join(dir, "teams", "engineering.yaml"))
	assertFileExists(t, filepath.Join(dir, "roles", "developer.yaml"))
	assertFileExists(t, filepath.Join(dir, "roles", "reviewer.yaml"))
	assertFileExists(t, filepath.Join(dir, "permissions", "read-docs.yaml"))
	assertFileExists(t, filepath.Join(dir, "agents", "claude.yaml"))
	assertFileExists(t, filepath.Join(dir, "documents", "role-developer--standards.md"))
	assertFileExists(t, filepath.Join(dir, "documents", "agent-claude--notes.md"))
	assertFileExists(t, filepath.Join(dir, "tasks", "engineering--fix-bug.yaml"))
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", path)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transfer/ -run TestExport -v`
Expected: FAIL — `Export` function not defined

- [ ] **Step 3: Implement exporter**

Create `internal/transfer/exporter.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ExportResult holds counts of exported entities.
type ExportResult struct {
	Teams       int
	Roles       int
	Permissions int
	Agents      int
	Documents   int
	Tasks       int
}

// Export fetches all entities from the DataSource and writes them
// to the target directory in dependency order.
func Export(ctx context.Context, ds DataSource, dir string) (*ExportResult, error) {
	subdirs := []string{"teams", "roles", "permissions", "agents", "documents", "tasks"}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", sub, err)
		}
	}

	var result ExportResult

	// Teams
	teams, err := ds.ListTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	for _, t := range teams {
		tf := TeamFile{Name: t.Name, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
		if err := writeYAML(filepath.Join(dir, "teams", SanitizeName(t.Name)+".yaml"), tf); err != nil {
			return nil, fmt.Errorf("write team %q: %w", t.Name, err)
		}
		result.Teams++
	}

	// Roles
	roles, err := ds.ListAllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	// Build ID→name map for parent references
	roleNames := make(map[string]string)
	for _, r := range roles {
		roleNames[r.ID] = r.Name
	}
	for _, r := range roles {
		parentName := ""
		if r.ParentID != "" {
			parentName = roleNames[r.ParentID]
		}
		rf := RoleFile{Name: r.Name, Parent: parentName, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
		if err := writeYAML(filepath.Join(dir, "roles", SanitizeName(r.Name)+".yaml"), rf); err != nil {
			return nil, fmt.Errorf("write role %q: %w", r.Name, err)
		}
		result.Roles++
	}

	// Permissions
	perms, err := ds.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	for _, p := range perms {
		pf := PermissionFile{Name: p.Name}
		if err := writeYAML(filepath.Join(dir, "permissions", SanitizeName(p.Name)+".yaml"), pf); err != nil {
			return nil, fmt.Errorf("write permission %q: %w", p.Name, err)
		}
		result.Permissions++
	}

	// Agents (with roles and permissions)
	agents, err := ds.ListAllAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	// Build team ID→name map
	teamNames := make(map[string]string)
	for _, t := range teams {
		teamNames[t.ID] = t.Name
	}
	// Build permission ID→name map (PermissionID in AgentPermission stores the name already via store query)
	for _, a := range agents {
		agentRoles, err := ds.GetAgentRoles(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("get roles for agent %q: %w", a.Name, err)
		}
		agentPerms, err := ds.GetAgentPermissions(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("get permissions for agent %q: %w", a.Name, err)
		}

		var roleEntries []AgentRoleEntry
		for _, ar := range agentRoles {
			roleEntries = append(roleEntries, AgentRoleEntry{
				Role:     roleNames[ar.RoleID],
				Priority: ar.Priority,
			})
		}

		var permEntries []AgentPermissionEntry
		for _, ap := range agentPerms {
			entry := AgentPermissionEntry{Permission: ap.PermissionID}
			if ap.ScopeTeamID != "" {
				entry.ScopeTeam = teamNames[ap.ScopeTeamID]
			}
			permEntries = append(permEntries, entry)
		}

		af := AgentFile{
			Name: a.Name, Team: teamNames[a.TeamID],
			CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
			Roles: roleEntries, Permissions: permEntries,
		}
		if err := writeYAML(filepath.Join(dir, "agents", SanitizeName(a.Name)+".yaml"), af); err != nil {
			return nil, fmt.Errorf("write agent %q: %w", a.Name, err)
		}
		result.Agents++
	}

	// Documents
	for _, r := range roles {
		docs, err := ds.ListRoleDocuments(ctx, r.ID)
		if err != nil {
			return nil, fmt.Errorf("list documents for role %q: %w", r.Name, err)
		}
		for _, d := range docs {
			if err := writeDocument(dir, "role", r.Name, d); err != nil {
				return nil, err
			}
			result.Documents++
		}
	}
	for _, a := range agents {
		docs, err := ds.ListAgentMemory(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("list memory for agent %q: %w", a.Name, err)
		}
		for _, d := range docs {
			if err := writeDocument(dir, "agent", a.Name, d); err != nil {
				return nil, err
			}
			result.Documents++
		}
	}

	// Tasks
	for _, t := range teams {
		tasks, err := ds.ListTeamTasks(ctx, t.ID)
		if err != nil {
			return nil, fmt.Errorf("list tasks for team %q: %w", t.Name, err)
		}
		agentNames := make(map[string]string)
		for _, a := range agents {
			agentNames[a.ID] = a.Name
		}
		for _, tk := range tasks {
			taskF := TaskFile{
				Title: tk.Title, Team: t.Name,
				Status: string(tk.Status),
				CreatedAt: tk.CreatedAt, UpdatedAt: tk.UpdatedAt,
				Description: tk.Description,
			}
			if tk.AgentID != "" {
				taskF.Agent = agentNames[tk.AgentID]
			}
			filename := SanitizeName(t.Name) + "--" + SanitizeName(tk.Title) + ".yaml"
			if err := writeYAML(filepath.Join(dir, "tasks", filename), taskF); err != nil {
				return nil, fmt.Errorf("write task %q: %w", tk.Title, err)
			}
			result.Tasks++
		}
	}

	return &result, nil
}

func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func writeDocument(dir, kind, ownerName string, d *domain.Document) error {
	fm := &DocumentFrontMatter{
		Title: d.Title, Kind: string(d.Kind),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if kind == "role" {
		fm.Role = ownerName
	} else {
		fm.Agent = ownerName
	}
	data, err := MarshalFrontMatter(fm, d.Content)
	if err != nil {
		return fmt.Errorf("marshal document %q: %w", d.Title, err)
	}
	filename := kind + "-" + SanitizeName(ownerName) + "--" + SanitizeName(d.Title) + ".md"
	return os.WriteFile(filepath.Join(dir, "documents", filename), data, 0644)
}
```

Note: The `writeDocument` function references `domain.Document` — the `domain` import (`github.com/Work-Fort/Hive/internal/domain`) must be added to the import block alongside `yaml.v3`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/transfer/ -run TestExport -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transfer/exporter.go internal/transfer/exporter_test.go
git commit -m "feat: add export logic with dependency-ordered entity serialization"
```

### Task 9: Importer

**Files:**
- Create: `internal/transfer/importer.go`
- Create: `internal/transfer/importer_test.go`

- [ ] **Step 1: Write importer test — round-trip**

Add to `internal/transfer/importer_test.go`:

```go
package transfer_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/infra/sqlite"
	"github.com/Work-Fort/Hive/internal/transfer"
)

func TestImportRoundTrip(t *testing.T) {
	// Export from a seeded store
	srcStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer srcStore.Close()
	seedTestData(t, srcStore)

	dir := t.TempDir()
	srcDS := transfer.NewDBDataSource(srcStore)
	if _, err := transfer.Export(context.Background(), srcDS, dir); err != nil {
		t.Fatal(err)
	}

	// Import into a fresh store
	dstStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dstStore.Close()

	dstDS := transfer.NewDBDataSource(dstStore)
	result, err := transfer.Import(context.Background(), dstDS, dir, transfer.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if result.Teams != 1 {
		t.Errorf("teams = %d, want 1", result.Teams)
	}
	if result.Roles != 2 {
		t.Errorf("roles = %d, want 2", result.Roles)
	}
	if result.Agents != 1 {
		t.Errorf("agents = %d, want 1", result.Agents)
	}
	if result.Documents != 2 {
		t.Errorf("documents = %d, want 2", result.Documents)
	}
	if result.Tasks != 1 {
		t.Errorf("tasks = %d, want 1", result.Tasks)
	}
}

func TestImportConflictFails(t *testing.T) {
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedTestData(t, s)

	dir := t.TempDir()
	ds := transfer.NewDBDataSource(s)
	if _, err := transfer.Export(context.Background(), ds, dir); err != nil {
		t.Fatal(err)
	}

	// Import into same store — should fail on conflicts
	_, err = transfer.Import(context.Background(), ds, dir, transfer.ImportOptions{})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

func TestImportUpsert(t *testing.T) {
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedTestData(t, s)

	dir := t.TempDir()
	ds := transfer.NewDBDataSource(s)
	if _, err := transfer.Export(context.Background(), ds, dir); err != nil {
		t.Fatal(err)
	}

	// Import with upsert — should succeed
	result, err := transfer.Import(context.Background(), ds, dir, transfer.ImportOptions{Upsert: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated == 0 {
		t.Error("expected some updates with upsert")
	}
}

func TestImportDryRun(t *testing.T) {
	srcStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer srcStore.Close()
	seedTestData(t, srcStore)

	dir := t.TempDir()
	srcDS := transfer.NewDBDataSource(srcStore)
	if _, err := transfer.Export(context.Background(), srcDS, dir); err != nil {
		t.Fatal(err)
	}

	dstStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dstStore.Close()

	dstDS := transfer.NewDBDataSource(dstStore)
	result, err := transfer.Import(context.Background(), dstDS, dir, transfer.ImportOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	// Dry run should report creates but not actually create
	if result.Teams != 1 {
		t.Errorf("teams = %d, want 1", result.Teams)
	}

	// Verify nothing was actually created
	teams, _ := dstDS.ListTeams(context.Background())
	if len(teams) != 0 {
		t.Errorf("expected 0 teams in store after dry run, got %d", len(teams))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transfer/ -run TestImport -v`
Expected: FAIL — `Import` function not defined

- [ ] **Step 3: Implement importer**

Create `internal/transfer/importer.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Work-Fort/Hive/internal/domain"
	"gopkg.in/yaml.v3"
)

// ImportOptions configures import behavior.
type ImportOptions struct {
	Upsert bool // Update existing entities instead of failing
	DryRun bool // Validate only, don't create
}

// ImportResult holds counts of import actions.
type ImportResult struct {
	Teams       int
	Roles       int
	Permissions int
	Agents      int
	Documents   int
	Tasks       int
	Updated     int
}

// Import reads entity files from the directory and creates them in the
// DataSource in dependency order.
func Import(ctx context.Context, ds DataSource, dir string, opts ImportOptions) (*ImportResult, error) {
	// Phase 1: Parse all files
	teams, err := parseDir[TeamFile](filepath.Join(dir, "teams"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse teams: %w", err)
	}
	roles, err := parseDir[RoleFile](filepath.Join(dir, "roles"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse roles: %w", err)
	}
	permissions, err := parseDir[PermissionFile](filepath.Join(dir, "permissions"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse permissions: %w", err)
	}
	agents, err := parseDir[AgentFile](filepath.Join(dir, "agents"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse agents: %w", err)
	}
	documents, err := parseDocuments(filepath.Join(dir, "documents"))
	if err != nil {
		return nil, fmt.Errorf("parse documents: %w", err)
	}
	tasks, err := parseDir[TaskFile](filepath.Join(dir, "tasks"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}

	var result ImportResult

	// Phase 2: Import teams
	teamIDs := make(map[string]string) // name → ID
	for _, tf := range teams {
		id, updated, err := importTeam(ctx, ds, tf, opts)
		if err != nil {
			return nil, fmt.Errorf("import team %q: %w", tf.Name, err)
		}
		teamIDs[tf.Name] = id
		if updated {
			result.Updated++
		} else {
			result.Teams++
		}
	}

	// Phase 3: Import roles (topological order)
	roleIDs := make(map[string]string) // name → ID
	sortedRoles := topoSortRoles(roles)
	for _, rf := range sortedRoles {
		parentID := ""
		if rf.Parent != "" {
			var ok bool
			parentID, ok = roleIDs[rf.Parent]
			if !ok {
				// Parent might already exist in the system
				existing, err := ds.LookupRoleByName(ctx, rf.Parent)
				if err != nil {
					return nil, fmt.Errorf("resolve parent role %q: %w", rf.Parent, err)
				}
				parentID = existing.ID
				roleIDs[rf.Parent] = parentID
			}
		}
		id, updated, err := importRole(ctx, ds, rf, parentID, opts)
		if err != nil {
			return nil, fmt.Errorf("import role %q: %w", rf.Name, err)
		}
		roleIDs[rf.Name] = id
		if updated {
			result.Updated++
		} else {
			result.Roles++
		}
	}

	// Phase 4: Import permissions
	for _, pf := range permissions {
		if !opts.DryRun {
			if err := ds.EnsurePermission(ctx, pf.Name); err != nil {
				return nil, fmt.Errorf("import permission %q: %w", pf.Name, err)
			}
		}
		result.Permissions++
	}

	// Phase 5: Import agents (with roles and permissions)
	agentIDs := make(map[string]string) // name → ID
	for _, af := range agents {
		tID, ok := teamIDs[af.Team]
		if !ok {
			existing, err := ds.LookupTeamByName(ctx, af.Team)
			if err != nil {
				return nil, fmt.Errorf("resolve team %q for agent %q: %w", af.Team, af.Name, err)
			}
			tID = existing.ID
			teamIDs[af.Team] = tID
		}

		id, updated, err := importAgent(ctx, ds, af, tID, roleIDs, teamIDs, opts)
		if err != nil {
			return nil, fmt.Errorf("import agent %q: %w", af.Name, err)
		}
		agentIDs[af.Name] = id
		if updated {
			result.Updated++
		} else {
			result.Agents++
		}
	}

	// Phase 6: Import documents
	for _, doc := range documents {
		ownerID := ""
		if doc.FM.Role != "" {
			id, ok := roleIDs[doc.FM.Role]
			if !ok {
				existing, err := ds.LookupRoleByName(ctx, doc.FM.Role)
				if err != nil {
					return nil, fmt.Errorf("resolve role %q for document %q: %w", doc.FM.Role, doc.FM.Title, err)
				}
				id = existing.ID
			}
			ownerID = id
		} else {
			id, ok := agentIDs[doc.FM.Agent]
			if !ok {
				existing, err := ds.LookupAgentByName(ctx, doc.FM.Agent)
				if err != nil {
					return nil, fmt.Errorf("resolve agent %q for document %q: %w", doc.FM.Agent, doc.FM.Title, err)
				}
				id = existing.ID
			}
			ownerID = id
		}

		updated, err := importDocument(ctx, ds, doc, ownerID, opts)
		if err != nil {
			return nil, fmt.Errorf("import document %q: %w", doc.FM.Title, err)
		}
		if updated {
			result.Updated++
		} else {
			result.Documents++
		}
	}

	// Phase 7: Import tasks
	for _, tf := range tasks {
		tID, ok := teamIDs[tf.Team]
		if !ok {
			existing, err := ds.LookupTeamByName(ctx, tf.Team)
			if err != nil {
				return nil, fmt.Errorf("resolve team %q for task %q: %w", tf.Team, tf.Title, err)
			}
			tID = existing.ID
		}

		agentID := ""
		if tf.Agent != "" {
			aID, ok := agentIDs[tf.Agent]
			if !ok {
				existing, err := ds.LookupAgentByName(ctx, tf.Agent)
				if err != nil {
					return nil, fmt.Errorf("resolve agent %q for task %q: %w", tf.Agent, tf.Title, err)
				}
				aID = existing.ID
			}
			agentID = aID
		}

		updated, err := importTask(ctx, ds, tf, tID, agentID, opts)
		if err != nil {
			return nil, fmt.Errorf("import task %q: %w", tf.Title, err)
		}
		if updated {
			result.Updated++
		} else {
			result.Tasks++
		}
	}

	return &result, nil
}

// --- helpers ---

type parsedDocument struct {
	FM   DocumentFrontMatter
	Body string
}

func parseDir[T any](dir, ext string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // empty directory is fine
	}
	if err != nil {
		return nil, err
	}
	var items []T
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var item T
		if err := yaml.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		items = append(items, item)
	}
	return items, nil
}

func parseDocuments(dir string) ([]parsedDocument, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var docs []parsedDocument
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		fm, body, err := UnmarshalFrontMatter(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		docs = append(docs, parsedDocument{FM: *fm, Body: body})
	}
	return docs, nil
}

func topoSortRoles(roles []RoleFile) []RoleFile {
	byName := make(map[string]RoleFile)
	for _, r := range roles {
		byName[r.Name] = r
	}
	var sorted []RoleFile
	visited := make(map[string]bool)
	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		r, ok := byName[name]
		if !ok {
			return // external parent, already in system
		}
		if r.Parent != "" {
			visit(r.Parent)
		}
		sorted = append(sorted, r)
	}
	for _, r := range roles {
		visit(r.Name)
	}
	return sorted
}

func newID() string {
	// Generate a random ID matching the existing pattern
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func importTeam(ctx context.Context, ds DataSource, tf TeamFile, opts ImportOptions) (string, bool, error) {
	existing, err := ds.LookupTeamByName(ctx, tf.Name)
	if err == nil {
		// Already exists
		if !opts.Upsert {
			return "", false, fmt.Errorf("team %q already exists", tf.Name)
		}
		if !opts.DryRun {
			if err := ds.UpdateTeam(ctx, existing.ID, tf.Name); err != nil {
				return "", false, err
			}
		}
		return existing.ID, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", false, err
	}
	// Create new
	id := newID()
	if !opts.DryRun {
		t := &domain.Team{ID: id, Name: tf.Name, CreatedAt: tf.CreatedAt, UpdatedAt: tf.UpdatedAt}
		if err := ds.CreateTeam(ctx, t); err != nil {
			return "", false, err
		}
	}
	return id, false, nil
}

func importRole(ctx context.Context, ds DataSource, rf RoleFile, parentID string, opts ImportOptions) (string, bool, error) {
	existing, err := ds.LookupRoleByName(ctx, rf.Name)
	if err == nil {
		if !opts.Upsert {
			return "", false, fmt.Errorf("role %q already exists", rf.Name)
		}
		if !opts.DryRun {
			if err := ds.UpdateRole(ctx, existing.ID, rf.Name, parentID); err != nil {
				return "", false, err
			}
		}
		return existing.ID, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", false, err
	}
	id := newID()
	if !opts.DryRun {
		r := &domain.Role{ID: id, Name: rf.Name, ParentID: parentID, CreatedAt: rf.CreatedAt, UpdatedAt: rf.UpdatedAt}
		if err := ds.CreateRole(ctx, r); err != nil {
			return "", false, err
		}
	}
	return id, false, nil
}

func importAgent(ctx context.Context, ds DataSource, af AgentFile, teamID string, roleIDs, teamIDs map[string]string, opts ImportOptions) (string, bool, error) {
	existing, err := ds.LookupAgentByName(ctx, af.Name)
	isNew := errors.Is(err, domain.ErrNotFound)
	if err != nil && !isNew {
		return "", false, err
	}
	if !isNew && !opts.Upsert {
		return "", false, fmt.Errorf("agent %q already exists", af.Name)
	}

	var id string
	updated := false
	if isNew {
		id = newID()
		if !opts.DryRun {
			a := &domain.Agent{ID: id, Name: af.Name, TeamID: teamID, CreatedAt: af.CreatedAt, UpdatedAt: af.UpdatedAt}
			if err := ds.CreateAgent(ctx, a); err != nil {
				return "", false, err
			}
		}
	} else {
		id = existing.ID
		updated = true
		if !opts.DryRun {
			if err := ds.UpdateAgent(ctx, id, af.Name, teamID); err != nil {
				return "", false, err
			}
		}
	}

	if opts.DryRun {
		return id, updated, nil
	}

	// Set roles
	if len(af.Roles) > 0 {
		var domainRoles []domain.AgentRole
		for _, ar := range af.Roles {
			rID, ok := roleIDs[ar.Role]
			if !ok {
				r, err := ds.LookupRoleByName(ctx, ar.Role)
				if err != nil {
					return "", false, fmt.Errorf("resolve role %q: %w", ar.Role, err)
				}
				rID = r.ID
			}
			domainRoles = append(domainRoles, domain.AgentRole{AgentID: id, RoleID: rID, Priority: ar.Priority})
		}
		if err := ds.SetAgentRoles(ctx, id, domainRoles); err != nil {
			return "", false, fmt.Errorf("set roles: %w", err)
		}
	}

	// Set permissions
	if len(af.Permissions) > 0 {
		var domainPerms []domain.AgentPermission
		for _, ap := range af.Permissions {
			scopeTeamID := ""
			if ap.ScopeTeam != "" {
				tID, ok := teamIDs[ap.ScopeTeam]
				if !ok {
					t, err := ds.LookupTeamByName(ctx, ap.ScopeTeam)
					if err != nil {
						return "", false, fmt.Errorf("resolve scope team %q: %w", ap.ScopeTeam, err)
					}
					tID = t.ID
				}
				scopeTeamID = tID
			}
			domainPerms = append(domainPerms, domain.AgentPermission{
				AgentID: id, PermissionID: ap.Permission, ScopeTeamID: scopeTeamID,
			})
		}
		if err := ds.SetAgentPermissions(ctx, id, domainPerms); err != nil {
			return "", false, fmt.Errorf("set permissions: %w", err)
		}
	}

	return id, updated, nil
}

func importDocument(ctx context.Context, ds DataSource, doc parsedDocument, ownerID string, opts ImportOptions) (bool, error) {
	existing, err := ds.LookupDocumentByOwnerAndTitle(ctx, ownerID, doc.FM.Title)
	if err == nil {
		if !opts.Upsert {
			return false, fmt.Errorf("document %q already exists", doc.FM.Title)
		}
		if !opts.DryRun {
			if err := ds.UpdateDocument(ctx, existing.ID, doc.FM.Title, doc.Body); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if !opts.DryRun {
		d := &domain.Document{
			ID: newID(), Kind: domain.DocumentKind(doc.FM.Kind),
			Title: doc.FM.Title, Content: doc.Body,
			CreatedAt: doc.FM.CreatedAt, UpdatedAt: doc.FM.UpdatedAt,
		}
		if doc.FM.Role != "" {
			d.RoleID = ownerID
		} else {
			d.AgentID = ownerID
		}
		if err := ds.CreateDocument(ctx, d); err != nil {
			return false, err
		}
	}
	return false, nil
}

func importTask(ctx context.Context, ds DataSource, tf TaskFile, teamID, agentID string, opts ImportOptions) (bool, error) {
	existing, err := ds.LookupTaskByTeamAndTitle(ctx, teamID, tf.Title)
	if err == nil {
		if !opts.Upsert {
			return false, fmt.Errorf("task %q already exists", tf.Title)
		}
		if !opts.DryRun {
			t := &domain.Task{
				Title: tf.Title, Description: tf.Description,
				Status: domain.TaskStatus(tf.Status), AgentID: agentID,
			}
			if err := ds.UpdateTask(ctx, existing.ID, t); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if !opts.DryRun {
		t := &domain.Task{
			ID: newID(), TeamID: teamID, AgentID: agentID,
			Title: tf.Title, Description: tf.Description,
			Status: domain.TaskStatus(tf.Status),
			CreatedAt: tf.CreatedAt, UpdatedAt: tf.UpdatedAt,
		}
		if err := ds.CreateTask(ctx, t); err != nil {
			return false, err
		}
	}
	return false, nil
}
```

Note: The import logic resolves references inline during each creation phase rather than doing an upfront validation pass. This means earlier entities (teams) may be created before a broken reference is discovered later (e.g., a task referencing a nonexistent agent). This is an intentional simplification — the fail-fast behavior combined with `--upsert` for recovery handles this cleanly.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/transfer/ -run TestImport -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transfer/importer.go internal/transfer/importer_test.go
git commit -m "feat: add import logic with reference resolution, upsert, and dry-run support"
```

---

## Chunk 4: CLI Commands and E2E Tests

### Task 10: CLI subcommands

**Files:**
- Create: `cmd/export/export.go`
- Create: `cmd/importcmd/importcmd.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Create export command**

Create `cmd/export/export.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package export

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/client"
	"github.com/Work-Fort/Hive/internal/infra"
	"github.com/Work-Fort/Hive/internal/transfer"
)

// NewCmd returns the export cobra command.
func NewCmd() *cobra.Command {
	var host string
	var port int
	var apiKey string
	var db string

	cmd := &cobra.Command{
		Use:   "export <dir>",
		Short: "Export all entities to a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("host") {
				host = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("api-key") {
				apiKey = viper.GetString("api-key")
			}
			return run(args[0], host, port, apiKey, db)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Daemon host")
	cmd.Flags().IntVar(&port, "port", 17000, "Daemon port")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN (bypass REST, direct DB access)")

	return cmd
}

func run(dir, host string, port int, apiKey, db string) error {
	ctx := context.Background()

	var ds transfer.DataSource
	if db != "" {
		store, err := infra.Open(db)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer store.Close()
		ds = transfer.NewDBDataSource(store)
	} else {
		baseURL := fmt.Sprintf("http://%s:%d", host, port)
		ds = transfer.NewRESTDataSource(client.New(baseURL, apiKey))
	}

	result, err := transfer.Export(ctx, ds, dir)
	if err != nil {
		return err
	}

	fmt.Printf("Exported %d teams, %d roles, %d permissions, %d agents, %d documents, %d tasks\n",
		result.Teams, result.Roles, result.Permissions, result.Agents, result.Documents, result.Tasks)
	return nil
}
```

Note: The `run` function needs `context.Background()` — the implementer should use `cmd.Context()` from the RunE closure or pass context through. Follow the daemon command pattern.

- [ ] **Step 2: Create import command**

Create `cmd/importcmd/importcmd.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package importcmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/client"
	"github.com/Work-Fort/Hive/internal/infra"
	"github.com/Work-Fort/Hive/internal/transfer"
)

// NewCmd returns the import cobra command.
func NewCmd() *cobra.Command {
	var host string
	var port int
	var apiKey string
	var db string
	var upsert bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "import <dir>",
		Short: "Import entities from a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("host") {
				host = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("api-key") {
				apiKey = viper.GetString("api-key")
			}
			return run(args[0], host, port, apiKey, db, upsert, dryRun)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Daemon host")
	cmd.Flags().IntVar(&port, "port", 17000, "Daemon port")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN (bypass REST, direct DB access)")
	cmd.Flags().BoolVar(&upsert, "upsert", false, "Update existing entities instead of failing")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate only, report what would happen")

	return cmd
}

func run(dir, host string, port int, apiKey, db string, upsert, dryRun bool) error {
	ctx := context.Background()

	var ds transfer.DataSource
	if db != "" {
		store, err := infra.Open(db)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer store.Close()
		ds = transfer.NewDBDataSource(store)
	} else {
		baseURL := fmt.Sprintf("http://%s:%d", host, port)
		ds = transfer.NewRESTDataSource(client.New(baseURL, apiKey))
	}

	opts := transfer.ImportOptions{Upsert: upsert, DryRun: dryRun}
	result, err := transfer.Import(ctx, ds, dir, opts)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Println("Dry run results:")
	} else {
		fmt.Println("Imported:")
	}
	fmt.Printf("  teams:       %d create, %d update\n", result.Teams, result.Updated)
	fmt.Printf("  roles:       %d create\n", result.Roles)
	fmt.Printf("  permissions: %d create\n", result.Permissions)
	fmt.Printf("  agents:      %d create\n", result.Agents)
	fmt.Printf("  documents:   %d create\n", result.Documents)
	fmt.Printf("  tasks:       %d create\n", result.Tasks)

	return nil
}
```

- [ ] **Step 3: Wire subcommands into root**

In `cmd/root.go`, add imports and registration:

```go
// Add to imports:
exportCmd "github.com/Work-Fort/Hive/cmd/export"
importCmd "github.com/Work-Fort/Hive/cmd/importcmd"

// Add in init() after existing AddCommand calls:
rootCmd.AddCommand(exportCmd.NewCmd())
rootCmd.AddCommand(importCmd.NewCmd())
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/export/ cmd/importcmd/ cmd/root.go
git commit -m "feat: add hive export and hive import CLI subcommands"
```

### Task 11: E2E tests

**Files:**
- Create: `tests/e2e/export_import_test.go`

- [ ] **Step 1: Write E2E round-trip test via REST**

Create `tests/e2e/export_import_test.go`:

```go
package e2e_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	h := newHarness(t)

	// Seed data via REST client
	ctx := context.Background()
	team, err := h.Client.CreateTeam(ctx, "engineering")
	if err != nil {
		t.Fatal(err)
	}
	role, err := h.Client.CreateRole(ctx, "developer", "")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := h.Client.CreateAgent(ctx, "claude", team.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Client.SetAgentRoles(ctx, agent.ID, []client.RoleAssignment{
		{RoleID: role.ID, Priority: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Client.CreateRoleDocument(ctx, role.ID, "Standards", "# Coding Standards")
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Client.CreateTask(ctx, client.CreateTaskInput{
		TeamID: team.ID, Title: "Fix bug", Description: "Fix the auth bug",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Export via CLI
	exportDir := filepath.Join(t.TempDir(), "export")
	cmd := exec.Command(hiveBin, "export", exportDir,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", h.port),
		"--api-key", testAPIKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export failed: %s\n%s", err, out)
	}
	t.Logf("export output: %s", out)

	// Verify export directory structure
	assertFileExists(t, filepath.Join(exportDir, "teams", "engineering.yaml"))
	assertFileExists(t, filepath.Join(exportDir, "roles", "developer.yaml"))
	assertFileExists(t, filepath.Join(exportDir, "agents", "claude.yaml"))
	assertFileExists(t, filepath.Join(exportDir, "documents", "role-developer--standards.md"))
	assertFileExists(t, filepath.Join(exportDir, "tasks", "engineering--fix-bug.yaml"))

	// Import into a fresh database via --db
	freshDB := filepath.Join(t.TempDir(), "fresh.db")
	cmd = exec.Command(hiveBin, "import", exportDir, "--db", freshDB)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("import failed: %s\n%s", err, out)
	}
	t.Logf("import output: %s", out)
}

func TestExportImportDryRun(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, err := h.Client.CreateTeam(ctx, "eng")
	if err != nil {
		t.Fatal(err)
	}

	exportDir := filepath.Join(t.TempDir(), "export")
	cmd := exec.Command(hiveBin, "export", exportDir,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", h.port),
		"--api-key", testAPIKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export: %s\n%s", err, out)
	}

	// Import with --dry-run should not fail
	freshDB := filepath.Join(t.TempDir(), "fresh.db")
	cmd = exec.Command(hiveBin, "import", exportDir, "--db", freshDB, "--dry-run")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run import: %s\n%s", err, out)
	}
	if !strings.Contains(string(out), "Dry run") {
		t.Errorf("expected dry run output, got: %s", out)
	}
}

func TestImportConflictAndUpsert(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, err := h.Client.CreateTeam(ctx, "eng")
	if err != nil {
		t.Fatal(err)
	}

	exportDir := filepath.Join(t.TempDir(), "export")
	cmd := exec.Command(hiveBin, "export", exportDir,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", h.port),
		"--api-key", testAPIKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export: %s\n%s", err, out)
	}

	// Import into same daemon — should fail on conflict
	cmd = exec.Command(hiveBin, "import", exportDir,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", h.port),
		"--api-key", testAPIKey)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected conflict error")
	}
	t.Logf("expected conflict output: %s", out)

	// Import with --upsert should succeed
	cmd = exec.Command(hiveBin, "import", exportDir,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", h.port),
		"--api-key", testAPIKey,
		"--upsert")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("upsert import: %s\n%s", err, out)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", path)
	}
}
```

Note: Add appropriate imports (`"context"`, `"fmt"`, `"os"`, `"strings"`, `"github.com/Work-Fort/Hive/client"`). The `testAPIKey` constant and `newHarness` function are defined in `tests/e2e/harness_test.go`. The `h.port` field is unexported but accessible from the same `e2e_test` package.

- [ ] **Step 2: Run E2E tests**

Run: `cd tests/e2e && go test -v -count=1 -run TestExportImport`
Expected: PASS

- [ ] **Step 3: Run full E2E suite to check regressions**

Run: `cd tests/e2e && go test -v -count=1`
Expected: PASS — all existing + new tests pass

- [ ] **Step 4: Commit**

```bash
git add tests/e2e/export_import_test.go
git commit -m "test: add E2E tests for export/import round-trip, dry-run, and upsert"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add name-lookup methods to domain ports | `internal/domain/ports.go` |
| 2 | Implement lookups in SQLite store | `internal/infra/sqlite/*.go` |
| 3 | Modify Create methods for timestamp preservation | `internal/infra/sqlite/*.go` |
| 4 | Permissions REST endpoints + client | `rest_huma.go`, `rest_types.go`, `client/` |
| 5 | YAML serialization + filename sanitization | `internal/transfer/yaml.go`, `sanitize.go` |
| 6 | DataSource interface + dbDataSource | `internal/transfer/datasource.go`, `db_source.go` |
| 7 | restDataSource implementation | `internal/transfer/rest_source.go` |
| 8 | Exporter | `internal/transfer/exporter.go` |
| 9 | Importer | `internal/transfer/importer.go` |
| 10 | CLI subcommands | `cmd/export/`, `cmd/importcmd/`, `cmd/root.go` |
| 11 | E2E tests | `tests/e2e/export_import_test.go` |

Total: 11 tasks across 4 chunks. Each task produces a working, testable commit.
