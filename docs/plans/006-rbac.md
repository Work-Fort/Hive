# RBAC Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement permission enforcement across MCP tool handlers, ensuring agents can only execute tools and access resources they are authorized for — defense in depth on top of Plan 005's tool list filtering.

**Architecture:** A thin `AuthzService` in `internal/daemon/authz.go` wraps the existing `PermissionStore.HasPermission` method and provides high-level `CheckPermission` / `RequireAny` helpers that return structured MCP-compatible errors. Each MCP tool handler calls the authz service before executing its logic. The REST API remains unprotected by per-agent RBAC (it is an admin surface gated by a shared API key). Scope enforcement ensures task tools respect team boundaries and memory tools enforce self-only access.

**Tech Stack:** Go, net/http

**Depends on:** Plan 003 (REST API), Plan 005 (MCP server) must be complete.

---

## Chunk 1: Authorization Service

### Task 1: Domain error for permission denied

**Files:**
- Modify: `internal/domain/errors.go`

- [ ] **Step 1: Add ErrPermissionDenied sentinel error**

Add to the existing error variables in `internal/domain/errors.go`:

```go
// ErrPermissionDenied is returned when an agent lacks a required permission.
ErrPermissionDenied = errors.New("permission denied")
```

The full file after modification:

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

	// ErrPermissionDenied is returned when an agent lacks a required permission.
	ErrPermissionDenied = errors.New("permission denied")
)
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/domain/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/errors.go
git commit -m "feat: add ErrPermissionDenied sentinel error"
```

### Task 2: AuthzService implementation

**Files:**
- Create: `internal/daemon/authz.go`

- [ ] **Step 1: Create the AuthzService with CheckPermission and RequireAny**

Create `internal/daemon/authz.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

// PermCheck describes a single permission requirement for authorization.
type PermCheck struct {
	// Name is the permission name (e.g., "task:read", "memory:write").
	Name string
	// ScopeTeamID limits the permission to a specific team.
	// Empty string means the check accepts either a global grant or a
	// grant scoped to the agent's own team.
	ScopeTeamID string
}

// AuthzService provides high-level authorization methods on top of the
// permission store. It is the single point of enforcement for MCP tool
// handlers — every tool calls into this service before executing.
type AuthzService struct {
	store domain.Store
}

// NewAuthzService creates an AuthzService backed by the given store.
func NewAuthzService(store domain.Store) *AuthzService {
	return &AuthzService{store: store}
}

// CheckPermission verifies that agentID holds permName, either globally
// or scoped to scopeTeamID. Returns nil on success, domain.ErrPermissionDenied
// on failure. Store errors are propagated unwrapped.
func (a *AuthzService) CheckPermission(ctx context.Context, agentID, permName, scopeTeamID string) error {
	ok, err := a.store.HasPermission(ctx, agentID, permName, scopeTeamID)
	if err != nil {
		return fmt.Errorf("check permission %s for agent %s: %w", permName, agentID, err)
	}
	if !ok {
		return fmt.Errorf("%w: agent %s lacks %s", domain.ErrPermissionDenied, agentID, permName)
	}
	return nil
}

// RequireAny checks that the agent holds ALL of the listed permissions.
// The name is "RequireAny" in the sense that each PermCheck is evaluated
// independently (a global grant OR a scoped grant satisfies it), but ALL
// checks in the slice must pass. This handles compound requirements like
// get_provisioning needing both role:read AND memory:read.
//
// Returns nil if all checks pass. Returns domain.ErrPermissionDenied
// wrapping the first failing check on denial.
func (a *AuthzService) RequireAny(ctx context.Context, agentID string, perms ...PermCheck) error {
	for _, p := range perms {
		if err := a.CheckPermission(ctx, agentID, p.Name, p.ScopeTeamID); err != nil {
			return err
		}
	}
	return nil
}

// ResolveAgentTeam looks up the agent and returns their team ID.
// This is used by tool handlers that need to scope permission checks
// to the agent's own team (e.g., task tools).
func (a *AuthzService) ResolveAgentTeam(ctx context.Context, agentID string) (string, error) {
	agent, err := a.store.GetAgent(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("resolve agent team: %w", err)
	}
	return agent.TeamID, nil
}

// CheckTeamPermission is a convenience that resolves the agent's team and
// then checks if the agent holds the named permission scoped to that team
// (or globally). This is the common pattern for task tools.
func (a *AuthzService) CheckTeamPermission(ctx context.Context, agentID, permName string) (string, error) {
	teamID, err := a.ResolveAgentTeam(ctx, agentID)
	if err != nil {
		return "", err
	}
	if err := a.CheckPermission(ctx, agentID, permName, teamID); err != nil {
		return "", err
	}
	return teamID, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/authz.go
git commit -m "feat: add AuthzService for MCP permission enforcement"
```

### Task 3: AuthzService tests

**Files:**
- Create: `internal/daemon/authz_test.go`

- [ ] **Step 1: Create comprehensive tests for the authorization service**

Create `internal/daemon/authz_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

// mockPermStore implements the subset of domain.Store needed by AuthzService.
// It embeds a nil domain.Store to satisfy the interface; only the methods
// under test are overridden.
type mockPermStore struct {
	domain.Store

	// hasPermission maps "agentID:permName:scopeTeamID" → bool.
	hasPermission map[string]bool

	// agents maps agentID → *domain.Agent.
	agents map[string]*domain.Agent

	// err is returned by HasPermission when non-nil (simulates store failure).
	err error
}

func (m *mockPermStore) HasPermission(_ context.Context, agentID, permName, scopeTeamID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	key := agentID + ":" + permName + ":" + scopeTeamID
	return m.hasPermission[key], nil
}

func (m *mockPermStore) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	a, ok := m.agents[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return a, nil
}

func TestCheckPermission_GlobalGrant(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:": true, // global (empty scope)
		},
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:read", "")
	if err != nil {
		t.Fatalf("expected permission granted, got: %v", err)
	}
}

func TestCheckPermission_ScopedGrant(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:team-a": true,
		},
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:read", "team-a")
	if err != nil {
		t.Fatalf("expected scoped permission granted, got: %v", err)
	}
}

func TestCheckPermission_Denied(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{},
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:write", "")
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got: %v", err)
	}
}

func TestCheckPermission_StoreError(t *testing.T) {
	storeErr := errors.New("database is down")
	store := &mockPermStore{
		err: storeErr,
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:read", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatal("store error should not be ErrPermissionDenied")
	}
}

func TestRequireAny_AllPass(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:role:read:":   true,
			"agent-1:memory:read:": true,
		},
	}
	authz := NewAuthzService(store)

	err := authz.RequireAny(context.Background(), "agent-1",
		PermCheck{Name: "role:read"},
		PermCheck{Name: "memory:read"},
	)
	if err != nil {
		t.Fatalf("expected all permissions granted, got: %v", err)
	}
}

func TestRequireAny_OneFails(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:role:read:": true,
			// memory:read NOT granted
		},
	}
	authz := NewAuthzService(store)

	err := authz.RequireAny(context.Background(), "agent-1",
		PermCheck{Name: "role:read"},
		PermCheck{Name: "memory:read"},
	)
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got: %v", err)
	}
}

func TestCheckTeamPermission_Success(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:team-a": true,
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	teamID, err := authz.CheckTeamPermission(context.Background(), "agent-1", "task:read")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}
}

func TestCheckTeamPermission_Denied(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	_, err := authz.CheckTeamPermission(context.Background(), "agent-1", "task:write")
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got: %v", err)
	}
}

func TestCheckTeamPermission_AgentNotFound(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{},
		agents:        map[string]*domain.Agent{},
	}
	authz := NewAuthzService(store)

	_, err := authz.CheckTeamPermission(context.Background(), "no-such-agent", "task:read")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveAgentTeam(t *testing.T) {
	store := &mockPermStore{
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-b"},
		},
	}
	authz := NewAuthzService(store)

	teamID, err := authz.ResolveAgentTeam(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if teamID != "team-b" {
		t.Fatalf("expected team-b, got: %s", teamID)
	}
}
```

- [ ] **Step 2: Verify tests compile and pass**

```bash
go test ./internal/daemon/ -run TestCheck -v
go test ./internal/daemon/ -run TestRequire -v
go test ./internal/daemon/ -run TestResolve -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/authz_test.go
git commit -m "test: add AuthzService unit tests"
```

---

## Chunk 2: MCP Tool Permission Enforcement

Plan 005 filters the tool list so agents only *see* tools they have permission for. This chunk adds defense-in-depth: even if a tool is somehow called, the handler verifies permissions before executing. This protects against session reuse, race conditions, or bugs in the filtering layer.

### Task 4: Add permission enforcement to MCP memory tools

**Files:**
- Modify: `internal/daemon/mcp_tools.go`

Memory tools enforce two things:
1. The agent holds the required permission (`memory:read` or `memory:write`)
2. The agent can only access their own memory (agent ID from session = owner of documents)

- [ ] **Step 1: Add authorization checks to memory tool handlers**

In `internal/daemon/mcp_tools.go`, each memory tool handler receives the agent ID from the MCP session context (established in Plan 005). Add permission checks at the top of each handler, before any store calls.

The `agentID` is extracted from the request context by Plan 005's session middleware. Each handler already has access to it. Add the `AuthzService` as a field on whatever struct holds the tool handlers (established in Plan 005).

Add to the **list_memory** handler:

```go
// At the top of the list_memory handler, before any store calls:
if err := s.authz.CheckPermission(ctx, agentID, "memory:read", ""); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
```

Add to the **get_memory** handler:

```go
// Permission check
if err := s.authz.CheckPermission(ctx, agentID, "memory:read", ""); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Ownership check — verify the document belongs to this agent
doc, err := s.store.GetDocument(ctx, docID)
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
if doc.AgentID != agentID {
	return mcp.NewToolResultError("permission denied: document belongs to another agent"), nil
}
```

Add to the **create_memory** handler:

```go
if err := s.authz.CheckPermission(ctx, agentID, "memory:write", ""); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
// Ensure the document's AgentID is set to the session agent (no impersonation)
doc.AgentID = agentID
```

Add to the **update_memory** handler:

```go
if err := s.authz.CheckPermission(ctx, agentID, "memory:write", ""); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Ownership check
existing, err := s.store.GetDocument(ctx, docID)
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
if existing.AgentID != agentID {
	return mcp.NewToolResultError("permission denied: document belongs to another agent"), nil
}
```

Add to the **delete_memory** handler:

```go
if err := s.authz.CheckPermission(ctx, agentID, "memory:write", ""); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Ownership check
existing, err := s.store.GetDocument(ctx, docID)
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
if existing.AgentID != agentID {
	return mcp.NewToolResultError("permission denied: document belongs to another agent"), nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/mcp_tools.go
git commit -m "feat: add permission enforcement to MCP memory tools"
```

### Task 5: Add permission enforcement to MCP task tools

**Files:**
- Modify: `internal/daemon/mcp_tools.go`

Task tools enforce:
1. The agent holds the required permission (`task:read` or `task:write`)
2. The permission scope covers the task's team (agent's own team)
3. For `get_task` / `update_task` / `delete_task`, the target task belongs to a team the agent has access to

- [ ] **Step 1: Add authorization checks to task tool handlers**

Add to the **list_tasks** handler:

```go
// Resolve agent's team and verify task:read permission scoped to that team.
// CheckTeamPermission handles both global and team-scoped grants.
teamID, err := s.authz.CheckTeamPermission(ctx, agentID, "task:read")
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Only list tasks for the agent's own team
tasks, err := s.store.ListTeamTasks(ctx, teamID)
```

Add to the **get_task** handler:

```go
// First, fetch the task to learn which team it belongs to
task, err := s.store.GetTask(ctx, taskID)
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Verify the agent has task:read scoped to the task's team
if err := s.authz.CheckPermission(ctx, agentID, "task:read", task.TeamID); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
```

Add to the **create_task** handler:

```go
// Resolve agent's team and verify task:write permission
teamID, err := s.authz.CheckTeamPermission(ctx, agentID, "task:write")
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Force the task to belong to the agent's team (no cross-team creation via MCP)
task.TeamID = teamID
```

Add to the **update_task** handler:

```go
// Fetch existing task to verify team scope
existing, err := s.store.GetTask(ctx, taskID)
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Verify agent has task:write scoped to the task's team
if err := s.authz.CheckPermission(ctx, agentID, "task:write", existing.TeamID); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
```

Add to the **delete_task** handler:

```go
// Fetch existing task to verify team scope
existing, err := s.store.GetTask(ctx, taskID)
if err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}

// Verify agent has task:write scoped to the task's team
if err := s.authz.CheckPermission(ctx, agentID, "task:write", existing.TeamID); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/mcp_tools.go
git commit -m "feat: add permission enforcement to MCP task tools with scope checking"
```

### Task 6: Add permission enforcement to get_provisioning

**Files:**
- Modify: `internal/daemon/mcp_tools.go`

`get_provisioning` is the only tool with compound permission requirements. It needs BOTH `role:read` AND `memory:read`.

- [ ] **Step 1: Add compound authorization check to get_provisioning handler**

Add to the **get_provisioning** handler:

```go
// get_provisioning requires BOTH role:read AND memory:read
if err := s.authz.RequireAny(ctx, agentID,
	PermCheck{Name: "role:read"},
	PermCheck{Name: "memory:read"},
); err != nil {
	return mcp.NewToolResultError(err.Error()), nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/mcp_tools.go
git commit -m "feat: add compound permission check to get_provisioning tool"
```

---

## Chunk 3: Wiring and Integration

### Task 7: Wire AuthzService into the daemon

**Files:**
- Modify: `internal/daemon/server.go`
- Modify: `internal/daemon/mcp_server.go` (created by Plan 005)

- [ ] **Step 1: Create AuthzService in server setup and inject into MCP server**

In `internal/daemon/server.go`, add the `AuthzService` to the `ServerConfig` or create it during `NewServer`:

```go
// In NewServer, after the store is available:
authz := NewAuthzService(cfg.Store)
```

The MCP server struct (created by Plan 005 in `internal/daemon/mcp_server.go`) needs access to the authz service. Add it as a field:

```go
// In the MCP server struct (mcp_server.go, created by Plan 005):
type MCPServer struct {
	store domain.Store
	authz *AuthzService  // Add this field
	// ... other fields from Plan 005
}
```

Update the MCP server constructor to accept the authz service:

```go
func NewMCPServer(store domain.Store, authz *AuthzService) *MCPServer {
	return &MCPServer{
		store: store,
		authz: authz,
	}
}
```

Wire it up in `NewServer`:

```go
authz := NewAuthzService(cfg.Store)
mcpServer := NewMCPServer(cfg.Store, authz)
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/server.go internal/daemon/mcp_server.go
git commit -m "feat: wire AuthzService into daemon and MCP server"
```

### Task 8: Verify tool list filtering uses HasPermission correctly

**Files:**
- Modify: `internal/daemon/mcp_server.go` (if needed)

Plan 005 implements tool list filtering: when an MCP session starts, the server queries the agent's permissions and only exposes tools the agent has access to. This task verifies that filtering is correct and adds any missing mappings.

- [ ] **Step 1: Verify the tool-to-permission mapping is complete**

The MCP server (Plan 005) should have a mapping from tool names to required permissions. Verify it matches this table:

```go
// toolPermissions maps MCP tool names to their required permissions.
// Tools with multiple required permissions use a slice.
var toolPermissions = map[string][]string{
	"get_provisioning": {"role:read", "memory:read"},
	"list_memory":      {"memory:read"},
	"get_memory":       {"memory:read"},
	"create_memory":    {"memory:write"},
	"update_memory":    {"memory:write"},
	"delete_memory":    {"memory:write"},
	"list_tasks":       {"task:read"},
	"get_task":         {"task:read"},
	"create_task":      {"task:write"},
	"update_task":      {"task:write"},
	"delete_task":      {"task:write"},
}
```

The filtering logic (Plan 005) iterates this map and calls `HasPermission` for each required permission. If ANY permission in the slice is denied, the tool is excluded. For `get_provisioning`, the agent needs both `role:read` AND `memory:read` to see it.

If Plan 005 uses a simpler single-permission mapping, update it to use the slice-based approach above to handle `get_provisioning`'s compound requirement.

- [ ] **Step 2: Commit (if changes were needed)**

```bash
git add internal/daemon/mcp_server.go
git commit -m "fix: update tool-permission mapping for compound requirements"
```

---

## Chunk 4: Integration Tests

### Task 9: Integration test for MCP permission enforcement

**Files:**
- Create: `internal/daemon/authz_integration_test.go`

- [ ] **Step 1: Create integration tests that verify end-to-end permission enforcement**

Create `internal/daemon/authz_integration_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

// TestMemoryToolEnforcement verifies that memory tools check permissions
// and enforce self-only access.
func TestMemoryToolEnforcement(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			// agent-1 has memory:read and memory:write globally
			"agent-1:memory:read:":  true,
			"agent-1:memory:write:": true,
			// agent-2 has NO memory permissions
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
			"agent-2": {ID: "agent-2", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	ctx := context.Background()

	// agent-1 can read memory
	if err := authz.CheckPermission(ctx, "agent-1", "memory:read", ""); err != nil {
		t.Errorf("agent-1 should have memory:read: %v", err)
	}

	// agent-1 can write memory
	if err := authz.CheckPermission(ctx, "agent-1", "memory:write", ""); err != nil {
		t.Errorf("agent-1 should have memory:write: %v", err)
	}

	// agent-2 cannot read memory
	err := authz.CheckPermission(ctx, "agent-2", "memory:read", "")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("agent-2 should be denied memory:read, got: %v", err)
	}

	// agent-2 cannot write memory
	err = authz.CheckPermission(ctx, "agent-2", "memory:write", "")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("agent-2 should be denied memory:write, got: %v", err)
	}
}

// TestTaskToolScopeEnforcement verifies that task tools respect team scoping.
func TestTaskToolScopeEnforcement(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			// agent-1 has task:read scoped to team-a only
			"agent-1:task:read:team-a": true,
			// agent-1 does NOT have task:read for team-b
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	ctx := context.Background()

	// agent-1 can read tasks in team-a (their own team)
	teamID, err := authz.CheckTeamPermission(ctx, "agent-1", "task:read")
	if err != nil {
		t.Fatalf("agent-1 should have task:read for own team: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}

	// agent-1 cannot read tasks in team-b (cross-team)
	err = authz.CheckPermission(ctx, "agent-1", "task:read", "team-b")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("agent-1 should be denied task:read for team-b, got: %v", err)
	}
}

// TestProvisioningCompoundPermission verifies that get_provisioning requires
// both role:read AND memory:read.
func TestProvisioningCompoundPermission(t *testing.T) {
	tests := []struct {
		name    string
		perms   map[string]bool
		wantErr bool
	}{
		{
			name: "both permissions granted",
			perms: map[string]bool{
				"agent-1:role:read:":   true,
				"agent-1:memory:read:": true,
			},
			wantErr: false,
		},
		{
			name: "only role:read",
			perms: map[string]bool{
				"agent-1:role:read:": true,
			},
			wantErr: true,
		},
		{
			name: "only memory:read",
			perms: map[string]bool{
				"agent-1:memory:read:": true,
			},
			wantErr: true,
		},
		{
			name:    "neither permission",
			perms:   map[string]bool{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockPermStore{hasPermission: tt.perms}
			authz := NewAuthzService(store)

			err := authz.RequireAny(context.Background(), "agent-1",
				PermCheck{Name: "role:read"},
				PermCheck{Name: "memory:read"},
			)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected success, got: %v", err)
			}
			if tt.wantErr && err != nil && !errors.Is(err, domain.ErrPermissionDenied) {
				t.Errorf("expected ErrPermissionDenied, got: %v", err)
			}
		})
	}
}

// TestGlobalVsScopedPermission verifies that global permissions are checked
// with an empty scope, while scoped permissions use the team ID.
func TestGlobalVsScopedPermission(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			// agent-1 has task:read globally
			"agent-1:task:read:": true,
			// agent-2 has task:read scoped to team-a only
			"agent-2:task:read:team-a": true,
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
			"agent-2": {ID: "agent-2", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	ctx := context.Background()

	// agent-1 global permission: CheckTeamPermission passes because
	// HasPermission(agent-1, task:read, team-a) is called, but the store
	// only has the global entry. This depends on HasPermission in the store
	// checking BOTH global and scoped grants (as specified in the design).
	//
	// NOTE: The SQLite HasPermission implementation (Plan 002) checks:
	//   WHERE agent_id = ? AND perm_name = ?
	//     AND (scope_team_id IS NULL OR scope_team_id = ?)
	// This means a global grant (scope_team_id IS NULL) satisfies any scope check.

	// For this unit test, we simulate the store behavior:
	// Global grant means key "agent-1:task:read:" matches empty scope.
	// The store also needs to match when a specific scope is passed.
	// We add the scoped key too to simulate the store's OR behavior.
	store.hasPermission["agent-1:task:read:team-a"] = true

	teamID, err := authz.CheckTeamPermission(ctx, "agent-1", "task:read")
	if err != nil {
		t.Fatalf("agent-1 global grant should work: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}

	// agent-2 scoped permission: works for team-a
	teamID, err = authz.CheckTeamPermission(ctx, "agent-2", "task:read")
	if err != nil {
		t.Fatalf("agent-2 scoped grant should work for own team: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/daemon/ -run TestMemory -v
go test ./internal/daemon/ -run TestTaskTool -v
go test ./internal/daemon/ -run TestProvisioning -v
go test ./internal/daemon/ -run TestGlobalVsScoped -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/authz_integration_test.go
git commit -m "test: add integration tests for MCP permission enforcement"
```

---

## Summary

### Files Created
| File | Purpose |
|---|---|
| `internal/daemon/authz.go` | AuthzService with CheckPermission, RequireAny, CheckTeamPermission |
| `internal/daemon/authz_test.go` | Unit tests for AuthzService |
| `internal/daemon/authz_integration_test.go` | Integration tests for permission enforcement scenarios |

### Files Modified
| File | Change |
|---|---|
| `internal/domain/errors.go` | Add `ErrPermissionDenied` sentinel |
| `internal/daemon/mcp_tools.go` | Add permission checks to all 11 tool handlers |
| `internal/daemon/mcp_server.go` | Wire `AuthzService`, verify tool-permission mapping |
| `internal/daemon/server.go` | Create and inject `AuthzService` |

### Permission Enforcement Matrix

| Tool | Permission(s) | Scope Logic | Ownership Check |
|---|---|---|---|
| `get_provisioning` | `role:read` + `memory:read` | Global | Self (agentID from session) |
| `list_memory` | `memory:read` | Global | Self (agentID from session) |
| `get_memory` | `memory:read` | Global | Document.AgentID == agentID |
| `create_memory` | `memory:write` | Global | Forced to agentID |
| `update_memory` | `memory:write` | Global | Document.AgentID == agentID |
| `delete_memory` | `memory:write` | Global | Document.AgentID == agentID |
| `list_tasks` | `task:read` | Agent's team | N/A (filtered by team) |
| `get_task` | `task:read` | Task's team | N/A (checked via team scope) |
| `create_task` | `task:write` | Agent's team | Forced to agent's team |
| `update_task` | `task:write` | Task's team | N/A (checked via team scope) |
| `delete_task` | `task:write` | Task's team | N/A (checked via team scope) |

### Key Design Decisions

1. **Defense in depth, not primary enforcement.** Plan 005's tool list filtering is the first gate — agents never see tools they lack permissions for. Plan 006 adds a second gate inside each handler. This protects against bugs in the filtering layer.

2. **Memory is self-only.** Agents cannot access other agents' memory documents. The session's agent ID is the ownership key. No separate scope_team_id needed for memory tools — having `memory:read` or `memory:write` only grants access to your own documents.

3. **Task scope uses agent's team.** Task tools resolve the agent's team and check permissions scoped to that team. The store's `HasPermission` query uses `(scope_team_id IS NULL OR scope_team_id = ?)`, so a global grant satisfies any team-scoped check.

4. **MCP errors, not HTTP errors.** Permission failures return `mcp.NewToolResultError(err.Error())` — a JSON-RPC error the MCP client can display. They do not return HTTP 403 because the MCP transport layer manages its own HTTP session.

5. **REST API has no per-agent RBAC.** The REST API is an admin surface gated by a shared API key. If you have the key, you can do anything. This is by design — the REST API is consumed by the WorkFort frontend, not by agents.
