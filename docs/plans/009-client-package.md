# Public Client Package Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a public Go HTTP client at `client/` with zero internal imports. The client defines its own response types mirroring the API JSON shapes, provides methods for every REST endpoint, and returns structured sentinel errors for common HTTP status codes.

**Architecture:** A thin HTTP wrapper. The `Client` struct holds an `http.Client`, `baseURL`, and `apiKey`. Each method builds a request, sets the `Authorization: Bearer <key>` header, executes it, checks the status code, and decodes the JSON response. Non-2xx responses are decoded into an `APIError` carrying the status code and server error message. Sentinel package-level variables allow callers to use `errors.Is`.

**Authentication:** The REST API uses `Authorization: Bearer <key>` (see `internal/daemon/middleware.go`). The `GET /v1/health` endpoint is unauthenticated.

**Tech Stack:** Go stdlib only (`net/http`, `encoding/json`). Zero external dependencies. Zero imports from `internal/`.

**Depends on:** Plan 003 (REST API) must be complete.

---

## Chunk 1: Core Client and Error Types

### Task 1: Scaffold client package with Client struct and constructor

**Files:**
- Create: `client/client.go`

- [ ] **Step 1: Create `client/client.go`**

```go
// SPDX-License-Identifier: GPL-3.0-or-later

// Package client provides a Go HTTP client for the Hive REST API.
// It has zero dependencies on internal Hive packages and can be imported
// freely by external consumers.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the Hive REST API.
type Client struct {
	http    http.Client
	baseURL string
	apiKey  string
}

// New creates a new Client. baseURL should be the scheme+host+port of the
// Hive daemon (e.g., "http://127.0.0.1:17000"). apiKey is sent as a Bearer
// token on every authenticated request.
func New(baseURL string, apiKey string) *Client {
	return &Client{
		http:    http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

// do executes an HTTP request, checks the status, and decodes the JSON
// response body into out (if non-nil). Returns an *APIError for non-2xx
// responses.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeAPIError(resp)
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// doWithQuery is like do but appends query parameters to the path.
func (c *Client) doWithQuery(ctx context.Context, method, path string, params url.Values, out any) error {
	p := path
	if len(params) > 0 {
		p = path + "?" + params.Encode()
	}
	return c.do(ctx, method, p, nil, out)
}
```

**Commit:** `feat(client): add Client struct, New constructor, and internal do helper`

---

### Task 2: Define error types and sentinel errors

**Files:**
- Create: `client/errors.go`

- [ ] **Step 1: Create `client/errors.go`**

The server always returns `{"error": "<message>"}` for non-2xx responses. Map the five
common status codes to sentinel errors; everything else wraps `APIError` directly.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common HTTP status codes. Use errors.Is to check.
var (
	// ErrBadRequest is returned when the server responds with 400.
	ErrBadRequest = errors.New("bad request")

	// ErrUnauthorized is returned when the server responds with 401.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrNotFound is returned when the server responds with 404.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when the server responds with 409.
	ErrConflict = errors.New("conflict")

	// ErrUnprocessable is returned when the server responds with 422.
	ErrUnprocessable = errors.New("unprocessable entity")
)

// APIError is returned for any non-2xx HTTP response. It carries the HTTP
// status code and the error message decoded from the server's JSON body.
type APIError struct {
	StatusCode int
	Message    string
	// sentinel wraps one of the package-level sentinel errors when the status
	// code matches a known value.
	sentinel error
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("hive api error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("hive api error %d", e.StatusCode)
}

// Unwrap allows errors.Is to match sentinel errors.
func (e *APIError) Unwrap() error { return e.sentinel }

// decodeAPIError reads the response body and returns an *APIError. It maps
// known status codes to sentinel errors via Unwrap.
func decodeAPIError(resp *http.Response) *APIError {
	var body struct {
		Error string `json:"error"`
	}
	// Best-effort decode; ignore errors (body may be empty or non-JSON).
	json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck

	ae := &APIError{StatusCode: resp.StatusCode, Message: body.Error}
	switch resp.StatusCode {
	case http.StatusBadRequest:
		ae.sentinel = ErrBadRequest
	case http.StatusUnauthorized:
		ae.sentinel = ErrUnauthorized
	case http.StatusNotFound:
		ae.sentinel = ErrNotFound
	case http.StatusConflict:
		ae.sentinel = ErrConflict
	case http.StatusUnprocessableEntity:
		ae.sentinel = ErrUnprocessable
	}
	return ae
}
```

**Commit:** `feat(client): add APIError type and sentinel errors for common HTTP status codes`

---

## Chunk 2: Response Types

### Task 3: Define all response types

**Files:**
- Create: `client/types.go`

- [ ] **Step 1: Create `client/types.go`**

Types mirror the API JSON shapes exactly. They are independent of any internal
domain types. JSON tags match what the server serialises (Go stdlib
`encoding/json` default: lower-camel or explicit tags from domain structs).

The domain types in `internal/domain/types.go` use plain struct fields without
explicit JSON tags — the server serialises them using Go's default lowercase
field name rule (e.g., `ID` → `"ID"`, `Name` → `"Name"`). However, the
handlers that return anonymous structs or slices with explicit tags follow those
tags. Read the exact JSON from the server to confirm:

- `domain.Team` serialises as `{"ID":"...","Name":"...","CreatedAt":"...","UpdatedAt":"..."}`
- `domain.Role` serialises with field `"ParentID"` (empty string when absent)
- `domain.Agent` serialises with `"TeamID"`
- `domain.AgentRole` serialises as `{"AgentID":"...","RoleID":"...","Priority":0}`
- `domain.Document` serialises with `"Kind"`, `"RoleID"`, `"AgentID"`
- `domain.Task` serialises with `"TeamID"`, `"AgentID"`, `"Status"`
- `domain.AgentPermission` serialises as `{"AgentID":"...","PermissionID":"...","ScopeTeamID":"..."}`
- `GetAgent` response embeds `Agent` plus `"roles": [...]`
- `SetAgentRoles` response is `[]AgentRole`
- Health report: `{"status":"...","checks":[{"name":"...","severity":"...","message":"..."}]}`

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import "time"

// Team is an organisational unit returned by the Teams endpoints.
type Team struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// Role is a capability definition with optional single-parent inheritance.
type Role struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	ParentID  string    `json:"ParentID"` // empty string when no parent
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// Agent is a provisioned identity belonging to one team.
type Agent struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	TeamID    string    `json:"TeamID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// AgentRole links an agent to a role with a priority ordering.
type AgentRole struct {
	AgentID  string `json:"AgentID"`
	RoleID   string `json:"RoleID"`
	Priority int    `json:"Priority"`
}

// AgentWithRoles is the response from GET /v1/agents/:id — the agent record
// plus its current role assignments.
type AgentWithRoles struct {
	Agent
	Roles []AgentRole `json:"roles"`
}

// Document holds markdown content attached to a role or an agent.
type Document struct {
	ID        string    `json:"ID"`
	Kind      string    `json:"Kind"`    // "role" or "memory"
	Title     string    `json:"Title"`
	Content   string    `json:"Content"`
	RoleID    string    `json:"RoleID"`  // set when Kind == "role"
	AgentID   string    `json:"AgentID"` // set when Kind == "memory"
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// Task is a work item belonging to a team, optionally assigned to an agent.
type Task struct {
	ID          string    `json:"ID"`
	TeamID      string    `json:"TeamID"`
	AgentID     string    `json:"AgentID"` // empty if unassigned
	Title       string    `json:"Title"`
	Description string    `json:"Description"`
	Status      string    `json:"Status"` // "pending", "in_progress", "completed"
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// AgentPermission grants a named permission to an agent, optionally scoped to
// a team. ScopeTeamID is empty for global grants.
type AgentPermission struct {
	AgentID      string `json:"AgentID"`
	PermissionID string `json:"PermissionID"`
	ScopeTeamID  string `json:"ScopeTeamID"`
}

// HealthCheckResult is a single named health check result.
type HealthCheckResult struct {
	Name     string `json:"name"`
	Severity string `json:"severity"` // "ok", "warning", "error"
	Message  string `json:"message,omitempty"`
}

// HealthReport is returned by GET /v1/health.
type HealthReport struct {
	Status string              `json:"status"` // "healthy", "degraded", "unhealthy"
	Checks []HealthCheckResult `json:"checks"`
}
```

**Commit:** `feat(client): add response types mirroring API JSON shapes`

---

## Chunk 3: Teams, Roles, and Documents

### Task 4: Implement Teams methods

**Files:**
- Create: `client/teams.go`

- [ ] **Step 1: Create `client/teams.go`**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// ListTeams returns all teams.
func (c *Client) ListTeams(ctx context.Context) ([]Team, error) {
	var out []Team
	return out, c.do(ctx, http.MethodGet, "/v1/teams", nil, &out)
}

// CreateTeam creates a new team with the given name.
func (c *Client) CreateTeam(ctx context.Context, name string) (*Team, error) {
	body := map[string]string{"name": name}
	var out Team
	return &out, c.do(ctx, http.MethodPost, "/v1/teams", body, &out)
}

// GetTeam returns the team with the given ID.
func (c *Client) GetTeam(ctx context.Context, id string) (*Team, error) {
	var out Team
	return &out, c.do(ctx, http.MethodGet, "/v1/teams/"+id, nil, &out)
}

// UpdateTeam renames the team with the given ID.
func (c *Client) UpdateTeam(ctx context.Context, id, name string) (*Team, error) {
	body := map[string]string{"name": name}
	var out Team
	return &out, c.do(ctx, http.MethodPut, "/v1/teams/"+id, body, &out)
}

// DeleteTeam deletes the team with the given ID. Returns ErrConflict if the
// team has dependent agents or tasks.
func (c *Client) DeleteTeam(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/teams/"+id, nil, nil)
}
```

**Commit:** `feat(client): add Teams CRUD methods`

---

### Task 5: Implement Roles methods

**Files:**
- Create: `client/roles.go`

- [ ] **Step 1: Create `client/roles.go`**

`ListRoles` accepts an optional `parentID` filter that maps to the `?parent_id=`
query parameter. Pass an empty string to list all roles.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
	"net/url"
)

// ListRoles returns all roles, optionally filtered by parentID. Pass an empty
// string to list all roles regardless of parent.
func (c *Client) ListRoles(ctx context.Context, parentID string) ([]Role, error) {
	params := url.Values{}
	if parentID != "" {
		params.Set("parent_id", parentID)
	}
	var out []Role
	return out, c.doWithQuery(ctx, http.MethodGet, "/v1/roles", params, &out)
}

// CreateRole creates a new role. parentID may be empty for a root role.
func (c *Client) CreateRole(ctx context.Context, name, parentID string) (*Role, error) {
	body := map[string]string{"name": name, "parent_id": parentID}
	var out Role
	return &out, c.do(ctx, http.MethodPost, "/v1/roles", body, &out)
}

// GetRole returns the role with the given ID.
func (c *Client) GetRole(ctx context.Context, id string) (*Role, error) {
	var out Role
	return &out, c.do(ctx, http.MethodGet, "/v1/roles/"+id, nil, &out)
}

// UpdateRole updates the name and parent of the role with the given ID.
// Pass an empty parentID to clear the parent (make it a root role).
func (c *Client) UpdateRole(ctx context.Context, id, name, parentID string) (*Role, error) {
	body := map[string]string{"name": name, "parent_id": parentID}
	var out Role
	return &out, c.do(ctx, http.MethodPut, "/v1/roles/"+id, body, &out)
}

// DeleteRole deletes the role with the given ID. Returns ErrConflict if the
// role has child roles or active agent assignments.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/roles/"+id, nil, nil)
}
```

**Commit:** `feat(client): add Roles CRUD methods`

---

### Task 6: Implement Documents methods

**Files:**
- Create: `client/documents.go`

- [ ] **Step 1: Create `client/documents.go`**

Covers role documents (`GET/POST /v1/roles/:id/documents`), standalone document
operations (`GET/PUT/DELETE /v1/documents/:id`), and agent memory
(`GET/POST /v1/agents/:id/memory`).

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// ListRoleDocuments returns all documents attached to the given role.
func (c *Client) ListRoleDocuments(ctx context.Context, roleID string) ([]Document, error) {
	var out []Document
	return out, c.do(ctx, http.MethodGet, "/v1/roles/"+roleID+"/documents", nil, &out)
}

// CreateRoleDocument adds a new document to the given role.
func (c *Client) CreateRoleDocument(ctx context.Context, roleID, title, content string) (*Document, error) {
	body := map[string]string{"title": title, "content": content}
	var out Document
	return &out, c.do(ctx, http.MethodPost, "/v1/roles/"+roleID+"/documents", body, &out)
}

// GetDocument returns the document with the given ID.
func (c *Client) GetDocument(ctx context.Context, id string) (*Document, error) {
	var out Document
	return &out, c.do(ctx, http.MethodGet, "/v1/documents/"+id, nil, &out)
}

// UpdateDocument updates the title and content of the document with the given ID.
func (c *Client) UpdateDocument(ctx context.Context, id, title, content string) (*Document, error) {
	body := map[string]string{"title": title, "content": content}
	var out Document
	return &out, c.do(ctx, http.MethodPut, "/v1/documents/"+id, body, &out)
}

// DeleteDocument deletes the document with the given ID.
func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/documents/"+id, nil, nil)
}

// ListAgentMemory returns all memory documents attached to the given agent.
func (c *Client) ListAgentMemory(ctx context.Context, agentID string) ([]Document, error) {
	var out []Document
	return out, c.do(ctx, http.MethodGet, "/v1/agents/"+agentID+"/memory", nil, &out)
}

// CreateAgentMemory adds a new memory document to the given agent.
func (c *Client) CreateAgentMemory(ctx context.Context, agentID, title, content string) (*Document, error) {
	body := map[string]string{"title": title, "content": content}
	var out Document
	return &out, c.do(ctx, http.MethodPost, "/v1/agents/"+agentID+"/memory", body, &out)
}
```

**Commit:** `feat(client): add Documents and AgentMemory methods`

---

## Chunk 4: Agents, Tasks, Permissions, and Health

### Task 7: Implement Agents methods

**Files:**
- Create: `client/agents.go`

- [ ] **Step 1: Create `client/agents.go`**

`GetAgent` returns `AgentWithRoles` because the server embeds the roles array
in the get-agent response. `SetAgentRoles` takes a slice of `AgentRole` and
returns the updated slice.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
	"net/url"
)

// RoleAssignment is the input type for SetAgentRoles.
type RoleAssignment struct {
	RoleID   string `json:"role_id"`
	Priority int    `json:"priority"`
}

// ListAgents returns all agents, optionally filtered by teamID. Pass an empty
// string to list all agents.
func (c *Client) ListAgents(ctx context.Context, teamID string) ([]Agent, error) {
	params := url.Values{}
	if teamID != "" {
		params.Set("team_id", teamID)
	}
	var out []Agent
	return out, c.doWithQuery(ctx, http.MethodGet, "/v1/agents", params, &out)
}

// CreateAgent creates a new agent with the given name in the given team.
func (c *Client) CreateAgent(ctx context.Context, name, teamID string) (*Agent, error) {
	body := map[string]string{"name": name, "team_id": teamID}
	var out Agent
	return &out, c.do(ctx, http.MethodPost, "/v1/agents", body, &out)
}

// GetAgent returns the agent with the given ID, including its role assignments.
func (c *Client) GetAgent(ctx context.Context, id string) (*AgentWithRoles, error) {
	var out AgentWithRoles
	return &out, c.do(ctx, http.MethodGet, "/v1/agents/"+id, nil, &out)
}

// UpdateAgent updates the name and team of the agent with the given ID.
func (c *Client) UpdateAgent(ctx context.Context, id, name, teamID string) (*Agent, error) {
	body := map[string]string{"name": name, "team_id": teamID}
	var out Agent
	return &out, c.do(ctx, http.MethodPut, "/v1/agents/"+id, body, &out)
}

// DeleteAgent deletes the agent with the given ID. Returns ErrConflict if the
// agent has tasks assigned.
func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/agents/"+id, nil, nil)
}

// SetAgentRoles replaces the role assignments for the given agent. The full
// set of assignments is replaced atomically. Pass an empty slice to clear all
// roles. Returns the updated list of role assignments.
func (c *Client) SetAgentRoles(ctx context.Context, agentID string, roles []RoleAssignment) ([]AgentRole, error) {
	body := map[string]any{"roles": roles}
	var out []AgentRole
	return out, c.do(ctx, http.MethodPut, "/v1/agents/"+agentID+"/roles", body, &out)
}
```

**Commit:** `feat(client): add Agents CRUD and SetAgentRoles methods`

---

### Task 8: Implement Tasks methods

**Files:**
- Create: `client/tasks.go`

- [ ] **Step 1: Create `client/tasks.go`**

`CreateTaskInput` and `UpdateTaskInput` are dedicated input types so callers
can omit optional fields cleanly. `AgentID` is optional on both; `Status`
defaults to `"pending"` server-side if omitted on create.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// CreateTaskInput holds the fields for creating a task.
type CreateTaskInput struct {
	TeamID      string `json:"team_id"`
	AgentID     string `json:"agent_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"` // defaults to "pending" if empty
}

// UpdateTaskInput holds the fields for updating a task. Zero-value string
// fields are ignored by the server (partial update semantics).
type UpdateTaskInput struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	AgentID     string `json:"agent_id"` // always sent; empty string clears assignment
}

// ListTeamTasks returns all tasks belonging to the given team.
func (c *Client) ListTeamTasks(ctx context.Context, teamID string) ([]Task, error) {
	var out []Task
	return out, c.do(ctx, http.MethodGet, "/v1/teams/"+teamID+"/tasks", nil, &out)
}

// CreateTask creates a new task.
func (c *Client) CreateTask(ctx context.Context, input CreateTaskInput) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPost, "/v1/tasks", input, &out)
}

// GetTask returns the task with the given ID.
func (c *Client) GetTask(ctx context.Context, id string) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodGet, "/v1/tasks/"+id, nil, &out)
}

// UpdateTask updates the task with the given ID.
func (c *Client) UpdateTask(ctx context.Context, id string, input UpdateTaskInput) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPut, "/v1/tasks/"+id, input, &out)
}

// DeleteTask deletes the task with the given ID.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tasks/"+id, nil, nil)
}
```

**Commit:** `feat(client): add Tasks CRUD methods`

---

### Task 9: Implement Permissions and Health methods

**Files:**
- Create: `client/permissions.go`

- [ ] **Step 1: Create `client/permissions.go`**

Permissions has only two endpoints: get and set. `PermissionGrant` is the
input type for `SetAgentPermissions`. `Health` is placed here too since it is
a single method with no natural home file — alternatively it can live in its
own `client/health.go`. Use a separate file for clarity.

Create `client/permissions.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// PermissionGrant is the input type for a single permission entry in
// SetAgentPermissions.
type PermissionGrant struct {
	Permission  string `json:"permission"`
	ScopeTeamID string `json:"scope_team_id,omitempty"`
}

// GetAgentPermissions returns the permission grants for the given agent.
func (c *Client) GetAgentPermissions(ctx context.Context, agentID string) ([]AgentPermission, error) {
	var out []AgentPermission
	return out, c.do(ctx, http.MethodGet, "/v1/agents/"+agentID+"/permissions", nil, &out)
}

// SetAgentPermissions replaces the permission grants for the given agent.
// The full set is replaced atomically. Pass an empty slice to revoke all
// permissions. Returns the updated grant list.
func (c *Client) SetAgentPermissions(ctx context.Context, agentID string, grants []PermissionGrant) ([]AgentPermission, error) {
	body := map[string]any{"permissions": grants}
	var out []AgentPermission
	return out, c.do(ctx, http.MethodPut, "/v1/agents/"+agentID+"/permissions", body, &out)
}
```

Create `client/health.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// Health returns the current health report. This endpoint is unauthenticated.
// The returned HealthReport.Status will be "healthy", "degraded", or
// "unhealthy". Note that the server returns HTTP 218 for degraded — this is a
// non-standard code and is treated as a success (2xx) by the client.
func (c *Client) Health(ctx context.Context) (*HealthReport, error) {
	var out HealthReport
	return &out, c.do(ctx, http.MethodGet, "/v1/health", nil, &out)
}
```

**Commit:** `feat(client): add Permissions and Health methods`

---

## Chunk 5: Verification

### Task 10: Verify the package compiles and has no internal imports

**Files:**
- Read: `client/` (all files)

- [ ] **Step 1: Run `go build ./client/...` from the repo root**

```
go build ./client/...
```

Expected: no errors, no output.

- [ ] **Step 2: Confirm zero internal imports**

```
grep -r 'Work-Fort/Hive/internal' client/
```

Expected: no matches.

- [ ] **Step 3: Run `go vet ./client/...`**

```
go vet ./client/...
```

Expected: no issues.

**Commit:** none — verification only.

---

## Summary

| Chunk | Task | File | Description |
|---|---|---|---|
| 1 | 1 | `client/client.go` | Client struct, New, do helper |
| 1 | 2 | `client/errors.go` | APIError, sentinel errors |
| 2 | 3 | `client/types.go` | All response types |
| 3 | 4 | `client/teams.go` | Teams CRUD |
| 3 | 5 | `client/roles.go` | Roles CRUD |
| 3 | 6 | `client/documents.go` | Documents + AgentMemory |
| 4 | 7 | `client/agents.go` | Agents CRUD + SetAgentRoles |
| 4 | 8 | `client/tasks.go` | Tasks CRUD |
| 4 | 9 | `client/permissions.go`, `client/health.go` | Permissions + Health |
| 5 | 10 | — | Build + import verification |
