# OpenAPI Spec Generation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenAPI 3.1 spec generation to the Hive REST API using Huma v2, following the Nexus pattern. The spec is served at `/openapi`, interactive docs at `/docs`. All 30 REST endpoints migrate from raw `http.HandlerFunc` to Huma's typed handler pattern with `doc:` tags for automatic schema documentation.

**Architecture:** Replace the manual JSON encode/decode REST layer with Huma v2. The `humago` adapter wraps Go's stdlib `http.ServeMux`, so the existing API key auth middleware and non-REST routes (MCP, health) remain unchanged. Input/Output types live alongside handlers. Error mapping switches from `writeError` to `huma.NewError()`.

**Tech Stack:** Huma v2 (`github.com/danielgtaylor/huma/v2`), `humago` adapter

**Depends on:** Plans 001–006 must be complete (REST API, RBAC).

**Reference:** Nexus handler at `nexus/lead/internal/infra/httpapi/handler.go`

---

## File Structure

Before defining tasks, here is the complete file map:

| File | Action | Responsibility |
|---|---|---|
| `go.mod`, `go.sum` | Modify | Add `github.com/danielgtaylor/huma/v2` |
| `internal/daemon/rest_types.go` | Create | Huma Input/Output structs + response body types with `json:` and `doc:` tags |
| `internal/daemon/rest_huma.go` | Create | `mapDomainErr()` + one `register*Routes(api, store)` per resource group (6 functions, 30 total `huma.Register` calls) |
| `internal/daemon/server.go` | Modify | Replace manual mux registration with `humago.New(mux, config)` + Huma route registration |
| `internal/daemon/rest.go` | Modify | Keep `writeJSON` and `writeError` (used by middleware + health); remove `REST` struct, `readJSON`, `mapDomainError` |
| `internal/daemon/rest_teams.go` | Delete | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_roles.go` | Delete | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_documents.go` | Delete | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_agents.go` | Delete | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_tasks.go` | Delete | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_permissions.go` | Delete | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_teams_test.go` | Delete | Tests wire old `REST` struct directly; E2E tests cover these paths |
| `internal/daemon/rest_roles_test.go` | Delete | Same reason |
| `internal/daemon/rest_documents_test.go` | Delete | Same reason |
| `internal/daemon/rest_agents_test.go` | Delete | Same reason |
| `internal/daemon/rest_tasks_test.go` | Delete | Same reason |
| `internal/daemon/rest_permissions_test.go` | Delete | Same reason |
| `client/errors.go` | May modify | Handle Huma's RFC 9457 error format (`{"status":N,"title":"...","detail":"..."}`) |
| `tests/e2e/*.go` | May modify | Fix any assertions on error response shape |

**Key constraint:** The current API returns PascalCase JSON keys (`"ID"`, `"Name"`, `"CreatedAt"`) because the domain types have no `json:` tags. The Huma response body types must match the existing API output to preserve backward compatibility with the `client/` package. This means `json:"ID"`, `json:"Name"` etc. for domain fields. Exception: the `roles` field in `GET /v1/agents/{id}` uses lowercase `json:"roles"` (it was added as a custom field, not from a domain type).

**Middleware note:** `internal/daemon/middleware.go` uses `writeError()` from `rest.go`. The auth middleware already skips paths that don't start with `/v1/`, so `/openapi` and `/docs` are already unauthenticated — no middleware changes needed.

---

## Chunk 1: Dependencies & Types

### Task 1: Add Huma v2 dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add Huma v2**

```bash
go get github.com/danielgtaylor/huma/v2
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add huma v2 for OpenAPI spec generation"
```

### Task 2: Create Huma input/output types

**Files:**
- Create: `internal/daemon/rest_types.go`

- [ ] **Step 1: Create `internal/daemon/rest_types.go` with all types**

The file contains three categories of types: input structs (Huma decodes request into these), output structs (Huma encodes response from these), and response body types (the JSON shapes). All response body types MUST use PascalCase `json:` tags to match the current API output (domain types have no json tags, so Go defaults to field names).

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import "time"

// --- shared input ---

type IDPathInput struct {
	ID string `path:"id" doc:"Resource ID"`
}

// --- teams ---

type CreateTeamInput struct {
	Body struct {
		Name string `json:"name" doc:"Team name" minLength:"1"`
	}
}

type UpdateTeamInput struct {
	ID   string `path:"id" doc:"Team ID"`
	Body struct {
		Name string `json:"name" doc:"Team name" minLength:"1"`
	}
}

type TeamOutput struct {
	Body teamResponse
}

type TeamListOutput struct {
	Body []teamResponse
}

type teamResponse struct {
	ID        string    `json:"ID" doc:"Team ID"`
	Name      string    `json:"Name" doc:"Team name"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- roles ---

type ListRolesInput struct {
	ParentID string `query:"parent_id" doc:"Filter by parent role ID"`
}

type CreateRoleInput struct {
	Body struct {
		Name     string `json:"name" doc:"Role name" minLength:"1"`
		ParentID string `json:"parent_id,omitempty" doc:"Parent role ID for inheritance"`
	}
}

type UpdateRoleInput struct {
	ID   string `path:"id" doc:"Role ID"`
	Body struct {
		Name     string `json:"name" doc:"Role name" minLength:"1"`
		ParentID string `json:"parent_id" doc:"Parent role ID (empty to clear)"`
	}
}

type RoleOutput struct {
	Body roleResponse
}

type RoleListOutput struct {
	Body []roleResponse
}

type roleResponse struct {
	ID        string    `json:"ID" doc:"Role ID"`
	Name      string    `json:"Name" doc:"Role name"`
	ParentID  string    `json:"ParentID" doc:"Parent role ID (empty if root)"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- documents ---

type RoleDocPathInput struct {
	ID string `path:"id" doc:"Role ID"`
}

type CreateRoleDocumentInput struct {
	ID   string `path:"id" doc:"Role ID"`
	Body struct {
		Title   string `json:"title" doc:"Document title" minLength:"1"`
		Content string `json:"content" doc:"Markdown content"`
	}
}

type CreateAgentMemoryInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Title   string `json:"title" doc:"Document title" minLength:"1"`
		Content string `json:"content" doc:"Markdown content"`
	}
}

type UpdateDocumentInput struct {
	ID   string `path:"id" doc:"Document ID"`
	Body struct {
		Title   string `json:"title" doc:"Document title" minLength:"1"`
		Content string `json:"content" doc:"Markdown content"`
	}
}

type AgentMemoryPathInput struct {
	ID string `path:"id" doc:"Agent ID"`
}

type DocumentOutput struct {
	Body documentResponse
}

type DocumentListOutput struct {
	Body []documentResponse
}

type documentResponse struct {
	ID        string    `json:"ID" doc:"Document ID"`
	Kind      string    `json:"Kind" doc:"Document kind: role or memory"`
	Title     string    `json:"Title" doc:"Document title"`
	Content   string    `json:"Content" doc:"Markdown content"`
	RoleID    string    `json:"RoleID" doc:"Owning role ID (set when kind=role)"`
	AgentID   string    `json:"AgentID" doc:"Owning agent ID (set when kind=memory)"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- agents ---

type ListAgentsInput struct {
	TeamID string `query:"team_id" doc:"Filter by team ID"`
}

type CreateAgentInput struct {
	Body struct {
		Name   string `json:"name" doc:"Agent name" minLength:"1"`
		TeamID string `json:"team_id" doc:"Team ID" minLength:"1"`
	}
}

type UpdateAgentInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Name   string `json:"name" doc:"Agent name" minLength:"1"`
		TeamID string `json:"team_id" doc:"Team ID" minLength:"1"`
	}
}

type SetAgentRolesInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Roles []agentRoleEntry `json:"roles" doc:"Role assignments with priority"`
	}
}

type agentRoleEntry struct {
	RoleID   string `json:"role_id" doc:"Role ID" minLength:"1"`
	Priority int    `json:"priority" doc:"Priority (lower = higher precedence)"`
}

type AgentOutput struct {
	Body agentResponse
}

type AgentListOutput struct {
	Body []agentResponse
}

type AgentDetailOutput struct {
	Body agentDetailResponse
}

type agentResponse struct {
	ID        string    `json:"ID" doc:"Agent ID"`
	Name      string    `json:"Name" doc:"Agent name"`
	TeamID    string    `json:"TeamID" doc:"Team ID"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

type agentDetailResponse struct {
	ID        string           `json:"ID" doc:"Agent ID"`
	Name      string           `json:"Name" doc:"Agent name"`
	TeamID    string           `json:"TeamID" doc:"Team ID"`
	CreatedAt time.Time        `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time        `json:"UpdatedAt" doc:"Last update timestamp"`
	Roles     []agentRoleResponse `json:"roles" doc:"Assigned roles"`
}

type agentRoleResponse struct {
	AgentID  string `json:"AgentID" doc:"Agent ID"`
	RoleID   string `json:"RoleID" doc:"Role ID"`
	Priority int    `json:"Priority" doc:"Priority ordering"`
}

type AgentRoleListOutput struct {
	Body []agentRoleResponse
}

// --- tasks ---

type TeamTasksPathInput struct {
	ID string `path:"id" doc:"Team ID"`
}

type CreateTaskInput struct {
	Body struct {
		TeamID      string `json:"team_id" doc:"Team ID" minLength:"1"`
		AgentID     string `json:"agent_id,omitempty" doc:"Assigned agent ID"`
		Title       string `json:"title" doc:"Task title" minLength:"1"`
		Description string `json:"description,omitempty" doc:"Task description"`
		Status      string `json:"status,omitempty" doc:"Status: pending, in_progress, completed"`
	}
}

type UpdateTaskInput struct {
	ID   string `path:"id" doc:"Task ID"`
	Body struct {
		Title       string `json:"title,omitempty" doc:"Task title"`
		Description string `json:"description,omitempty" doc:"Task description"`
		Status      string `json:"status,omitempty" doc:"Status: pending, in_progress, completed"`
		AgentID     string `json:"agent_id" doc:"Assigned agent ID (empty to unassign)"`
	}
}

type TaskOutput struct {
	Body taskResponse
}

type TaskListOutput struct {
	Body []taskResponse
}

type taskResponse struct {
	ID          string    `json:"ID" doc:"Task ID"`
	TeamID      string    `json:"TeamID" doc:"Team ID"`
	AgentID     string    `json:"AgentID" doc:"Assigned agent ID"`
	Title       string    `json:"Title" doc:"Task title"`
	Description string    `json:"Description" doc:"Task description"`
	Status      string    `json:"Status" doc:"Task status"`
	CreatedAt   time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt   time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- permissions ---

type SetAgentPermissionsInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Permissions []permissionEntry `json:"permissions" doc:"Permission grants"`
	}
}

type permissionEntry struct {
	Permission  string `json:"permission" doc:"Permission name" minLength:"1"`
	ScopeTeamID string `json:"scope_team_id,omitempty" doc:"Team scope (empty = global)"`
}

type PermissionsOutput struct {
	Body []permissionResponse
}

type permissionResponse struct {
	AgentID      string `json:"AgentID" doc:"Agent ID"`
	PermissionID string `json:"PermissionID" doc:"Permission name"`
	ScopeTeamID  string `json:"ScopeTeamID" doc:"Team scope (empty = global)"`
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: clean build (types are defined but not yet used).

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/rest_types.go
git commit -m "feat: add Huma input/output types for OpenAPI"
```

---

## Chunk 2: Migrate Handlers

### Task 3: Create Huma error mapper and response helpers

**Files:**
- Create: `internal/daemon/rest_huma.go`

- [ ] **Step 1: Create `internal/daemon/rest_huma.go` with error mapper and response converters**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Hive/internal/domain"
)

// mapDomainErr converts a domain error to a Huma error with the appropriate
// HTTP status code. Returns nil if err is nil.
func mapDomainErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return huma.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrHasDependencies):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrDepthExceeded):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrCycleDetected):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	default:
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
}

// --- response converters ---

func teamToResponse(t *domain.Team) teamResponse {
	return teamResponse{
		ID: t.ID, Name: t.Name,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func roleToResponse(r *domain.Role) roleResponse {
	return roleResponse{
		ID: r.ID, Name: r.Name, ParentID: r.ParentID,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func docToResponse(d *domain.Document) documentResponse {
	return documentResponse{
		ID: d.ID, Kind: string(d.Kind), Title: d.Title, Content: d.Content,
		RoleID: d.RoleID, AgentID: d.AgentID,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func agentToResponse(a *domain.Agent) agentResponse {
	return agentResponse{
		ID: a.ID, Name: a.Name, TeamID: a.TeamID,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func agentRolesToResponse(roles []domain.AgentRole) []agentRoleResponse {
	out := make([]agentRoleResponse, len(roles))
	for i, r := range roles {
		out[i] = agentRoleResponse{AgentID: r.AgentID, RoleID: r.RoleID, Priority: r.Priority}
	}
	return out
}

func taskToResponse(t *domain.Task) taskResponse {
	return taskResponse{
		ID: t.ID, TeamID: t.TeamID, AgentID: t.AgentID,
		Title: t.Title, Description: t.Description, Status: string(t.Status),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func permToResponse(p domain.AgentPermission) permissionResponse {
	return permissionResponse{
		AgentID: p.AgentID, PermissionID: p.PermissionID, ScopeTeamID: p.ScopeTeamID,
	}
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/rest_huma.go
git commit -m "feat: add Huma error mapper and response converters"
```

### Task 4: Register team and role routes with Huma

**Files:**
- Modify: `internal/daemon/rest_huma.go` (append)

- [ ] **Step 1: Add `registerTeamRoutes` to `rest_huma.go`**

Port the 5 team handlers from `rest_teams.go`. Each `huma.Register` call replaces one `REST` method. Complete code for teams to establish the pattern:

```go
func registerTeamRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-teams",
		Method:      http.MethodGet,
		Path:        "/v1/teams",
		Summary:     "List teams",
		Tags:        []string{"Teams"},
	}, func(ctx context.Context, input *struct{}) (*TeamListOutput, error) {
		teams, err := store.ListTeams(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]teamResponse, len(teams))
		for i, t := range teams {
			resp[i] = teamToResponse(t)
		}
		return &TeamListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-team",
		Method:        http.MethodPost,
		Path:          "/v1/teams",
		Summary:       "Create a team",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Teams"},
	}, func(ctx context.Context, input *CreateTeamInput) (*TeamOutput, error) {
		team := &domain.Team{ID: NewID("tm"), Name: input.Body.Name}
		if err := store.CreateTeam(ctx, team); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetTeam(ctx, team.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TeamOutput{Body: teamToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-team",
		Method:      http.MethodGet,
		Path:        "/v1/teams/{id}",
		Summary:     "Get a team",
		Tags:        []string{"Teams"},
	}, func(ctx context.Context, input *IDPathInput) (*TeamOutput, error) {
		team, err := store.GetTeam(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TeamOutput{Body: teamToResponse(team)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-team",
		Method:      http.MethodPut,
		Path:        "/v1/teams/{id}",
		Summary:     "Update a team",
		Tags:        []string{"Teams"},
	}, func(ctx context.Context, input *UpdateTeamInput) (*TeamOutput, error) {
		if err := store.UpdateTeam(ctx, input.ID, input.Body.Name); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetTeam(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TeamOutput{Body: teamToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-team",
		Method:        http.MethodDelete,
		Path:          "/v1/teams/{id}",
		Summary:       "Delete a team",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Teams"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteTeam(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})
}
```

- [ ] **Step 2: Add `registerRoleRoutes` to `rest_huma.go`**

Port the 5 role handlers from `rest_roles.go`, following the same pattern. Key differences from teams:
- `ListRoles` takes `input *ListRolesInput` with `input.ParentID` query param
- `CreateRole` sets `ParentID: input.Body.ParentID`
- `UpdateRole` calls `store.UpdateRole(ctx, input.ID, input.Body.Name, input.Body.ParentID)`

Operation details:

| OperationID | Method | Path | Input | Output |
|---|---|---|---|---|
| `list-roles` | GET | `/v1/roles` | `*ListRolesInput` | `*RoleListOutput` |
| `create-role` | POST | `/v1/roles` | `*CreateRoleInput` | `*RoleOutput` (201) |
| `get-role` | GET | `/v1/roles/{id}` | `*IDPathInput` | `*RoleOutput` |
| `update-role` | PUT | `/v1/roles/{id}` | `*UpdateRoleInput` | `*RoleOutput` |
| `delete-role` | DELETE | `/v1/roles/{id}` | `*IDPathInput` | `*struct{}` (204) |

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/rest_huma.go
git commit -m "feat: add Huma team and role route registration"
```

### Task 5: Register document, agent, task, and permission routes

**Files:**
- Modify: `internal/daemon/rest_huma.go` (append)

- [ ] **Step 1: Add `registerDocumentRoutes` — 7 endpoints**

Port from `rest_documents.go`. Key patterns:
- `ListRoleDocuments` and `CreateRoleDocument` verify the role exists first (`store.GetRole`)
- `ListAgentMemory` and `CreateAgentMemory` verify the agent exists first (`store.GetAgent`)
- `CreateRoleDocument` sets `Kind: domain.DocumentKindRole`, `RoleID: input.ID`
- `CreateAgentMemory` sets `Kind: domain.DocumentKindMemory`, `AgentID: input.ID`

Operation details:

| OperationID | Method | Path | Input | Output |
|---|---|---|---|---|
| `list-role-documents` | GET | `/v1/roles/{id}/documents` | `*RoleDocPathInput` | `*DocumentListOutput` |
| `create-role-document` | POST | `/v1/roles/{id}/documents` | `*CreateRoleDocumentInput` | `*DocumentOutput` (201) |
| `get-document` | GET | `/v1/documents/{id}` | `*IDPathInput` | `*DocumentOutput` |
| `update-document` | PUT | `/v1/documents/{id}` | `*UpdateDocumentInput` | `*DocumentOutput` |
| `delete-document` | DELETE | `/v1/documents/{id}` | `*IDPathInput` | `*struct{}` (204) |
| `list-agent-memory` | GET | `/v1/agents/{id}/memory` | `*AgentMemoryPathInput` | `*DocumentListOutput` |
| `create-agent-memory` | POST | `/v1/agents/{id}/memory` | `*CreateAgentMemoryInput` | `*DocumentOutput` (201) |

- [ ] **Step 2: Add `registerAgentRoutes` — 6 endpoints**

Port from `rest_agents.go`. Key patterns:
- `GetAgent` returns `AgentDetailOutput` (agent + roles), NOT `AgentOutput`
- `CreateAgent` verifies team exists via `store.GetTeam`
- `SetAgentRoles` validates agent exists, builds `[]domain.AgentRole`, calls `store.SetAgentRoles`

Operation details:

| OperationID | Method | Path | Input | Output |
|---|---|---|---|---|
| `list-agents` | GET | `/v1/agents` | `*ListAgentsInput` | `*AgentListOutput` |
| `create-agent` | POST | `/v1/agents` | `*CreateAgentInput` | `*AgentOutput` (201) |
| `get-agent` | GET | `/v1/agents/{id}` | `*IDPathInput` | `*AgentDetailOutput` |
| `update-agent` | PUT | `/v1/agents/{id}` | `*UpdateAgentInput` | `*AgentOutput` |
| `delete-agent` | DELETE | `/v1/agents/{id}` | `*IDPathInput` | `*struct{}` (204) |
| `set-agent-roles` | PUT | `/v1/agents/{id}/roles` | `*SetAgentRolesInput` | `*AgentRoleListOutput` |

- [ ] **Step 3: Add `registerTaskRoutes` — 5 endpoints**

Port from `rest_tasks.go`. Key patterns:
- `ListTeamTasks` verifies team exists first
- `CreateTask` verifies team exists, defaults status to `pending`, validates status if provided
- `UpdateTask` fetches existing task first for partial update (same merge logic as current code)
- Move the `validTaskStatus` helper function from `rest_tasks.go` into `rest_huma.go` (the old file is deleted in Task 7)

Operation details:

| OperationID | Method | Path | Input | Output |
|---|---|---|---|---|
| `list-team-tasks` | GET | `/v1/teams/{id}/tasks` | `*TeamTasksPathInput` | `*TaskListOutput` |
| `create-task` | POST | `/v1/tasks` | `*CreateTaskInput` | `*TaskOutput` (201) |
| `get-task` | GET | `/v1/tasks/{id}` | `*IDPathInput` | `*TaskOutput` |
| `update-task` | PUT | `/v1/tasks/{id}` | `*UpdateTaskInput` | `*TaskOutput` |
| `delete-task` | DELETE | `/v1/tasks/{id}` | `*IDPathInput` | `*struct{}` (204) |

- [ ] **Step 4: Add `registerPermissionRoutes` — 2 endpoints**

Port from `rest_permissions.go`. Key patterns:
- Both verify agent exists first
- `SetAgentPermissions` validates each permission has a name, builds `[]domain.AgentPermission`

Operation details:

| OperationID | Method | Path | Input | Output |
|---|---|---|---|---|
| `get-agent-permissions` | GET | `/v1/agents/{id}/permissions` | `*IDPathInput` | `*PermissionsOutput` |
| `set-agent-permissions` | PUT | `/v1/agents/{id}/permissions` | `*SetAgentPermissionsInput` | `*PermissionsOutput` |

- [ ] **Step 5: Verify build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/rest_huma.go
git commit -m "feat: add Huma registration for all REST endpoints"
```

### Task 6: Wire Huma into server.go and update server setup

**Files:**
- Modify: `internal/daemon/server.go`

- [ ] **Step 1: Update `NewServer` to use Huma**

Replace all manual `mux.HandleFunc` route registrations with Huma API setup. The health endpoint and MCP handler stay as raw mux registrations.

```go
import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Huma API — registers /openapi and /docs automatically on the mux.
	config := huma.DefaultConfig("Hive API", "1.0.0")
	api := humago.New(mux, config)

	// REST API routes via Huma
	registerTeamRoutes(api, cfg.Store)
	registerRoleRoutes(api, cfg.Store)
	registerDocumentRoutes(api, cfg.Store)
	registerAgentRoutes(api, cfg.Store)
	registerTaskRoutes(api, cfg.Store)
	registerPermissionRoutes(api, cfg.Store)

	// Health — raw handler (conditional status codes 200/218/503)
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// MCP server — raw handler (JSON-RPC 2.0, not REST)
	authz := NewAuthzService(cfg.Store)
	mcpHandler := NewMCPHandler(MCPDeps{
		Store:        cfg.Store,
		Provisioning: cfg.Provisioning,
		Authz:        authz,
	})
	mux.Handle("/mcp", mcpHandler)

	// Wrap with API key auth middleware.
	// Middleware already skips non-/v1/ paths, so /openapi and /docs are public.
	handler := APIKeyAuth(cfg.APIKey, mux)

	// ... rest unchanged
}
```

Remove the old `rest := NewREST(cfg.Store)` and all `mux.HandleFunc("...", rest.*)` lines.

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: build should succeed. There will be unused imports/symbols in the old rest_*.go files but since they're still present, the `REST` struct methods still compile.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/server.go
git commit -m "feat: wire Huma API into server, serve OpenAPI at /openapi"
```

---

## Chunk 3: Cleanup & Verification

### Task 7: Remove old REST handler files and their tests

**Files:**
- Modify: `internal/daemon/rest.go` (trim to keep `writeJSON` + `writeError` used by middleware and health)
- Delete: `internal/daemon/rest_teams.go`
- Delete: `internal/daemon/rest_roles.go`
- Delete: `internal/daemon/rest_documents.go`
- Delete: `internal/daemon/rest_agents.go`
- Delete: `internal/daemon/rest_tasks.go`
- Delete: `internal/daemon/rest_permissions.go`
- Delete: `internal/daemon/rest_teams_test.go`
- Delete: `internal/daemon/rest_roles_test.go`
- Delete: `internal/daemon/rest_documents_test.go`
- Delete: `internal/daemon/rest_agents_test.go`
- Delete: `internal/daemon/rest_tasks_test.go`
- Delete: `internal/daemon/rest_permissions_test.go`

The 6 test files directly reference `daemon.REST` and `daemon.NewREST` which no longer exist. E2E tests in `tests/e2e/` provide equivalent coverage through the HTTP API.

- [ ] **Step 1: Delete the 6 handler files and 6 test files**

```bash
rm internal/daemon/rest_teams.go internal/daemon/rest_roles.go \
   internal/daemon/rest_documents.go internal/daemon/rest_agents.go \
   internal/daemon/rest_tasks.go internal/daemon/rest_permissions.go \
   internal/daemon/rest_teams_test.go internal/daemon/rest_roles_test.go \
   internal/daemon/rest_documents_test.go internal/daemon/rest_agents_test.go \
   internal/daemon/rest_tasks_test.go internal/daemon/rest_permissions_test.go
```

- [ ] **Step 2: Trim `rest.go`**

Remove the `REST` struct, `NewREST`, `readJSON`, and `mapDomainError` function. Keep `writeJSON` and `writeError` — they are used by `middleware.go` (`APIKeyAuth` calls `writeError`) and `health.go` (`HandleHealth` calls `writeJSON`).

After trimming, `rest.go` should contain only:

```go
package daemon

import (
	"encoding/json"
	"net/http"

	"github.com/charmbracelet/log"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

- [ ] **Step 3: Verify build and tests**

```bash
go build ./...
go test ./internal/daemon/...
```

Expected: clean build. No references to `REST` struct, `readJSON`, or old `mapDomainError` should remain.

- [ ] **Step 4: Commit**

```bash
git add -u internal/daemon/
git commit -m "refactor: remove old REST handler files replaced by Huma"
```

### Task 8: Verify OpenAPI spec and fix E2E/client compatibility

**Files:**
- May modify: `client/errors.go` (Huma error format)
- May modify: `tests/e2e/*.go` (error assertions)

- [ ] **Step 1: Run unit tests**

```bash
go test ./internal/daemon/...
```

Expected: all existing daemon tests pass. If any fail, fix before proceeding.

- [ ] **Step 2: Run E2E tests**

```bash
cd tests/e2e && go test -v -count=1 ./...
```

The success response shapes should be identical (our response body types match the old domain type JSON). However, **error responses will differ**:

- Old format: `{"error": "not found: team hv_xxx"}`
- Huma format: `{"status": 404, "title": "Not Found", "detail": "not found: team hv_xxx"}`

If E2E tests fail on error parsing, check `client/errors.go`. The client currently parses `{"error": "..."}`. It needs to also handle Huma's `{"detail": "..."}` field for the error message.

Fix in `client/errors.go`: update the error response struct to check both `Error` and `Detail` fields:

```go
type errorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail"`
}
// Use whichever field is non-empty for the message.
```

- [ ] **Step 3: Verify OpenAPI spec is served**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go build -o /tmp/hive-test .
HIVE_API_KEY=test-key /tmp/hive-test daemon &
sleep 2
curl -sf http://127.0.0.1:17000/openapi | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Title: {d[\"info\"][\"title\"]}'); print(f'Paths: {len(d[\"paths\"])}'); print('Tags:', [t[\"name\"] for t in d.get(\"tags\",[])])"
kill %1
```

Expected: Title is "Hive API", paths count matches endpoint count, tags include Teams, Roles, Documents, Agents, Tasks, Permissions.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: update client error parsing for Huma response format"
```

---

## Summary

### Task Overview

| Task | Description | Files |
|---|---|---|
| 1 | Add Huma v2 dependency | `go.mod`, `go.sum` |
| 2 | Create Huma input/output types | Create `rest_types.go` |
| 3 | Error mapper + response converters | Create `rest_huma.go` |
| 4 | Register team + role routes | Append to `rest_huma.go` |
| 5 | Register document, agent, task, permission routes | Append to `rest_huma.go` |
| 6 | Wire Huma into server.go | Modify `server.go` |
| 7 | Remove old REST handler files | Delete 6 files, trim `rest.go` |
| 8 | Verify OpenAPI spec + fix E2E/client compat | May modify `client/errors.go`, `tests/e2e/` |

### Files Created
| File | Purpose |
|---|---|
| `internal/daemon/rest_types.go` | Huma input/output types with `json:` + `doc:` tags for all 30 endpoints |
| `internal/daemon/rest_huma.go` | `mapDomainErr()`, response converters, 6 `register*Routes()` functions |

### Files Modified
| File | Change |
|---|---|
| `go.mod` | Add `github.com/danielgtaylor/huma/v2` |
| `internal/daemon/server.go` | Replace manual mux registration with Huma API |
| `internal/daemon/rest.go` | Trim to keep only `writeJSON` + `writeError` (used by middleware + health) |
| `client/errors.go` | Handle Huma's RFC 9457 error format (if needed) |

### Files Deleted
| File | Reason |
|---|---|
| `internal/daemon/rest_teams.go` | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_roles.go` | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_documents.go` | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_agents.go` | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_tasks.go` | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_permissions.go` | Handlers moved to `rest_huma.go` |
| `internal/daemon/rest_*_test.go` (6 files) | Tests wire old `REST` struct; E2E tests provide coverage |

### Key Design Decisions

1. **Huma v2 with `humago` adapter.** Matches Nexus. Runtime spec generation from Go types — no YAML maintenance. The `humago` adapter wraps stdlib `http.ServeMux`, so existing middleware and MCP handler stay unchanged.

2. **PascalCase JSON keys preserved.** Domain types have no `json:` tags, so the current API returns PascalCase keys (`"ID"`, `"Name"`, `"CreatedAt"`). The Huma response body types use matching `json:"ID"` etc. to maintain backward compatibility with the `client/` package.

3. **Health and MCP stay as raw handlers.** Health uses conditional status codes (200/218/503) from a single endpoint — Huma operations require a single default status. MCP is JSON-RPC, not REST.

4. **`/openapi` and `/docs` are unauthenticated.** The existing `APIKeyAuth` middleware already skips paths not starting with `/v1/`. No middleware changes needed.

5. **`rest.go` trimmed, not deleted.** The `writeJSON` and `writeError` helpers are still used by `middleware.go` and `health.go`. Only the `REST` struct, `readJSON`, `NewREST`, and `mapDomainError` are removed.

6. **Error format changes.** Huma returns `{"status": N, "title": "...", "detail": "..."}` (RFC 9457 Problem Details) instead of `{"error": "..."}`. The client package error parsing needs to handle the new format.
