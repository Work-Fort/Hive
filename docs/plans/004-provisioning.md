# Provisioning Resolution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the provisioning resolution engine that composes role documents and agent memory into a hierarchical response, with write-time cycle/depth validation and boot-time depth auditing.

**Architecture:** A `ProvisioningService` in `internal/daemon/` that depends only on `domain.Store` and `HealthService`. The service's `Resolve` method orchestrates the four store queries (GetAgentRoles, GetRoleChain, ListRoleDocuments, ListAgentMemory) into a `ProvisioningResponse`. Write-time validation methods (`ValidateRoleParent`) provide cycle detection and depth checking before role mutations. A boot-time `AuditRoleDepths` method scans all chains and reports violations to the `HealthService`.

**Tech Stack:** Go, database/sql

**Depends on:** Plan 002 (domain model + SQLite store) must be complete.

---

## Chunk 1: ProvisioningService Core

### Task 1: Create ProvisioningService struct

**Files:**
- Create: `internal/daemon/provisioning.go`

- [ ] **Step 1: Define ProvisioningService and constructor**

Create `internal/daemon/provisioning.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ProvisioningService resolves agent provisioning data by composing
// role inheritance chains and agent memory into a hierarchical response.
type ProvisioningService struct {
	store        domain.Store
	health       *HealthService
	maxRoleDepth int
}

// NewProvisioningService creates a new ProvisioningService.
func NewProvisioningService(store domain.Store, health *HealthService, maxRoleDepth int) *ProvisioningService {
	return &ProvisioningService{
		store:        store,
		health:       health,
		maxRoleDepth: maxRoleDepth,
	}
}
```

### Task 2: Implement Resolve method

**Files:**
- Modify: `internal/daemon/provisioning.go`

- [ ] **Step 1: Add the Resolve method**

Append to `internal/daemon/provisioning.go`:

```go
// Resolve builds the complete provisioning response for an agent.
//
// Algorithm:
//  1. Fetch the agent's role assignments ordered by priority
//  2. For each role, walk the inheritance chain to root via GetRoleChain
//  3. For each role in the chain, collect its documents
//  4. Fetch the agent's own memory documents
//  5. Return the assembled ProvisioningResponse
func (ps *ProvisioningService) Resolve(ctx context.Context, agentID string) (*domain.ProvisioningResponse, error) {
	// 1. Get agent's role assignments (ordered by priority, ascending).
	agentRoles, err := ps.store.GetAgentRoles(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent roles: %w", err)
	}

	// 2-3. Build role groups with inheritance chains and documents.
	groups := make([]domain.ProvisioningRoleGroup, 0, len(agentRoles))
	for _, ar := range agentRoles {
		chain, err := ps.store.GetRoleChain(ctx, ar.RoleID, ps.maxRoleDepth)
		if err != nil {
			return nil, fmt.Errorf("get role chain for %s: %w", ar.RoleID, err)
		}

		entries := make([]domain.ProvisioningChainEntry, 0, len(chain))
		for _, role := range chain {
			docs, err := ps.store.ListRoleDocuments(ctx, role.ID)
			if err != nil {
				return nil, fmt.Errorf("list documents for role %s: %w", role.ID, err)
			}

			// Convert []*Document to []Document for the response.
			docSlice := make([]domain.Document, len(docs))
			for i, d := range docs {
				docSlice[i] = *d
			}

			entries = append(entries, domain.ProvisioningChainEntry{
				Role:      role.Name,
				Documents: docSlice,
			})
		}

		groups = append(groups, domain.ProvisioningRoleGroup{
			Priority: ar.Priority,
			Chain:    entries,
		})
	}

	// 4. Fetch agent's own memory documents.
	memDocs, err := ps.store.ListAgentMemory(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent memory: %w", err)
	}

	memory := make([]domain.Document, len(memDocs))
	for i, d := range memDocs {
		memory[i] = *d
	}

	// 5. Assemble and return.
	return &domain.ProvisioningResponse{
		Roles:  groups,
		Memory: memory,
	}, nil
}
```

---

## Chunk 2: Write-Time Validation

### Task 3: Cycle detection and depth validation

**Files:**
- Modify: `internal/daemon/provisioning.go`

- [ ] **Step 1: Add ValidateRoleParent method**

This method is called before creating or updating a role's `parent_id`. It walks the chain from `newParentID` upward to detect cycles and verify depth. Append to `internal/daemon/provisioning.go`:

```go
// ValidateRoleParent checks that setting roleID's parent to newParentID
// would not create a cycle or exceed the max role depth.
//
// Pass roleID="" when creating a brand-new role (no cycle possible with
// itself, but depth must still be checked).
//
// Returns:
//   - domain.ErrCycleDetected if roleID appears in newParentID's ancestor chain
//   - domain.ErrDepthExceeded if the resulting chain would exceed maxRoleDepth
//   - nil if the assignment is safe
func (ps *ProvisioningService) ValidateRoleParent(ctx context.Context, roleID, newParentID string) error {
	if newParentID == "" {
		return nil // removing parent is always safe
	}

	// Self-assignment is always a cycle.
	if roleID != "" && roleID == newParentID {
		return fmt.Errorf("%w: role cannot be its own parent", domain.ErrCycleDetected)
	}

	// Walk the ancestor chain from newParentID to root.
	ancestorChain, err := ps.store.GetRoleChain(ctx, newParentID, ps.maxRoleDepth)
	if err != nil {
		return fmt.Errorf("get ancestor chain: %w", err)
	}

	// Cycle detection: if roleID appears anywhere in the ancestor chain,
	// setting newParentID as its parent would create a cycle.
	if roleID != "" {
		for _, ancestor := range ancestorChain {
			if ancestor.ID == roleID {
				return fmt.Errorf("%w: %s already appears in the ancestor chain of %s",
					domain.ErrCycleDetected, roleID, newParentID)
			}
		}
	}

	// Depth validation: the new chain for roleID would be
	// 1 (roleID itself) + len(ancestorChain from newParentID to root).
	// Additionally, we must account for roleID's own descendants.
	// However, we only need to check the upward depth here because
	// the deepest descendant depth was already validated when those
	// descendants were created. The total depth from the deepest
	// descendant through roleID to root is what matters.
	//
	// To find the depth below roleID, we check its longest descendant
	// chain. For a new role (roleID=""), descendant depth is 0.
	descendantDepth := 0
	if roleID != "" {
		descendantDepth, err = ps.maxDescendantDepth(ctx, roleID)
		if err != nil {
			return fmt.Errorf("get descendant depth: %w", err)
		}
	}

	totalDepth := descendantDepth + 1 + len(ancestorChain) // descendants + self + ancestors
	if totalDepth > ps.maxRoleDepth {
		return fmt.Errorf("%w: depth %d exceeds max %d",
			domain.ErrDepthExceeded, totalDepth, ps.maxRoleDepth)
	}

	return nil
}

// maxDescendantDepth returns the length of the longest downward chain
// from the given role (not counting the role itself). Returns 0 if the
// role has no children.
func (ps *ProvisioningService) maxDescendantDepth(ctx context.Context, roleID string) (int, error) {
	return ps.walkDescendants(ctx, roleID, 0)
}

func (ps *ProvisioningService) walkDescendants(ctx context.Context, roleID string, currentDepth int) (int, error) {
	if currentDepth >= ps.maxRoleDepth {
		return currentDepth, nil // safety cap
	}

	children, err := ps.store.ListRoles(ctx, roleID)
	if err != nil {
		return 0, fmt.Errorf("list children of %s: %w", roleID, err)
	}

	if len(children) == 0 {
		return currentDepth, nil
	}

	maxDepth := currentDepth
	for _, child := range children {
		d, err := ps.walkDescendants(ctx, child.ID, currentDepth+1)
		if err != nil {
			return 0, err
		}
		if d > maxDepth {
			maxDepth = d
		}
	}
	return maxDepth, nil
}
```

---

## Chunk 3: Boot-Time Depth Audit

### Task 4: Implement AuditRoleDepths

**Files:**
- Modify: `internal/daemon/provisioning.go`

- [ ] **Step 1: Add AuditRoleDepths method**

Append to `internal/daemon/provisioning.go`:

```go
// AuditRoleDepths scans all role chains at boot time and reports any that
// exceed maxRoleDepth to the HealthService as warnings.
//
// This is a safety net: write-time validation should prevent deep chains,
// but data could be imported or the limit could be lowered after roles
// were already created.
func (ps *ProvisioningService) AuditRoleDepths(ctx context.Context) {
	// List all roles (parentID="" means no filter, returns all).
	roles, err := ps.store.ListRoles(ctx, "")
	if err != nil {
		ps.health.AddError(fmt.Sprintf("role depth audit failed: %v", err))
		return
	}

	// Find root roles (no parent) and walk their chains.
	// We use GetRoleChain on every leaf instead, since the CTE walks
	// upward. Instead, iterate all roles and check each chain length.
	checked := 0
	violations := 0
	for _, role := range roles {
		chain, err := ps.store.GetRoleChain(ctx, role.ID, ps.maxRoleDepth+1)
		if err != nil {
			ps.health.AddWarning(fmt.Sprintf("role depth audit: failed to get chain for role %q: %v", role.Name, err))
			continue
		}
		checked++
		if len(chain) > ps.maxRoleDepth {
			violations++
			ps.health.AddWarning(fmt.Sprintf(
				"role %q has chain depth %d (max %d)",
				role.Name, len(chain), ps.maxRoleDepth,
			))
		}
	}

	log.Debug("role depth audit complete", "roles_checked", checked, "violations", violations)
}
```

---

## Chunk 4: Wire Into Daemon

### Task 5: Add ProvisioningService to ServerConfig

**Files:**
- Modify: `internal/daemon/server.go`

- [ ] **Step 1: Add Provisioning field to ServerConfig**

In `internal/daemon/server.go`, add a field to `ServerConfig`:

```go
// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind         string
	Port         int
	Health       *HealthService
	Store        domain.Store
	Provisioning *ProvisioningService
}
```

No route changes needed in this plan. The MCP tools in Plan 005 will use `cfg.Provisioning.Resolve(...)` to serve `get_provisioning` requests.

### Task 6: Create ProvisioningService in daemon run() and call boot audit

**Files:**
- Modify: `cmd/daemon/daemon.go`

- [ ] **Step 1: Import config package (already imported) and create ProvisioningService**

In `cmd/daemon/daemon.go`, update the `run` function. After the store is opened and permissions are seeded, add:

```go
func run(bind string, port int, db, apiKey string) error {
	health := hiveDaemon.NewHealthService()

	// Database
	dsn := db
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

	// Provisioning
	maxRoleDepth := viper.GetInt("max-role-depth")
	provisioning := hiveDaemon.NewProvisioningService(store, health, maxRoleDepth)

	// Boot-time checks
	provisioning.AuditRoleDepths(context.Background())

	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:         bind,
		Port:         port,
		Health:       health,
		Store:        store,
		Provisioning: provisioning,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := hiveDaemon.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http shutdown", "err", err)
	}

	return nil
}
```

---

## Chunk 5: Tests

### Task 7: Comprehensive tests for ProvisioningService

**Files:**
- Create: `internal/daemon/provisioning_test.go`

- [ ] **Step 1: Create test file with helpers and all test cases**

Create `internal/daemon/provisioning_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// testSetup creates an in-memory store and ProvisioningService for testing.
func testSetup(t *testing.T, maxDepth int) (domain.Store, *daemon.ProvisioningService) {
	t.Helper()

	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	health := daemon.NewHealthService()
	ps := daemon.NewProvisioningService(store, health, maxDepth)
	return store, ps
}

// seedTeam creates a team and returns its ID.
func seedTeam(t *testing.T, ctx context.Context, store domain.Store, name string) string {
	t.Helper()
	id := "team-" + name
	err := store.CreateTeam(ctx, &domain.Team{ID: id, Name: name})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	return id
}

// seedAgent creates an agent in the given team and returns its ID.
func seedAgent(t *testing.T, ctx context.Context, store domain.Store, name, teamID string) string {
	t.Helper()
	id := "agent-" + name
	err := store.CreateAgent(ctx, &domain.Agent{ID: id, Name: name, TeamID: teamID})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return id
}

// seedRole creates a role and returns its ID.
func seedRole(t *testing.T, ctx context.Context, store domain.Store, name, parentID string) string {
	t.Helper()
	id := "role-" + name
	err := store.CreateRole(ctx, &domain.Role{ID: id, Name: name, ParentID: parentID})
	if err != nil {
		t.Fatalf("create role %q: %v", name, err)
	}
	return id
}

// seedDoc creates a role document.
func seedDoc(t *testing.T, ctx context.Context, store domain.Store, id, title, content, roleID string) {
	t.Helper()
	err := store.CreateDocument(ctx, &domain.Document{
		ID:      id,
		Kind:    domain.DocumentKindRole,
		Title:   title,
		Content: content,
		RoleID:  roleID,
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
}

// seedMemory creates an agent memory document.
func seedMemory(t *testing.T, ctx context.Context, store domain.Store, id, title, content, agentID string) {
	t.Helper()
	err := store.CreateDocument(ctx, &domain.Document{
		ID:      id,
		Kind:    domain.DocumentKindMemory,
		Title:   title,
		Content: content,
		AgentID: agentID,
	})
	if err != nil {
		t.Fatalf("create memory document: %v", err)
	}
}

func TestResolve_SingleRole_NoInheritance(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "alice", teamID)
	roleID := seedRole(t, ctx, store, "developer", "")
	seedDoc(t, ctx, store, "doc-1", "Dev Guide", "Write good code.", roleID)

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: roleID, Priority: 1},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 1 {
		t.Fatalf("expected 1 role group, got %d", len(resp.Roles))
	}
	if resp.Roles[0].Priority != 1 {
		t.Errorf("expected priority 1, got %d", resp.Roles[0].Priority)
	}
	if len(resp.Roles[0].Chain) != 1 {
		t.Fatalf("expected chain length 1, got %d", len(resp.Roles[0].Chain))
	}
	if resp.Roles[0].Chain[0].Role != "developer" {
		t.Errorf("expected role name 'developer', got %q", resp.Roles[0].Chain[0].Role)
	}
	if len(resp.Roles[0].Chain[0].Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(resp.Roles[0].Chain[0].Documents))
	}
	if resp.Roles[0].Chain[0].Documents[0].Title != "Dev Guide" {
		t.Errorf("expected doc title 'Dev Guide', got %q", resp.Roles[0].Chain[0].Documents[0].Title)
	}
	if len(resp.Memory) != 0 {
		t.Errorf("expected 0 memory docs, got %d", len(resp.Memory))
	}
}

func TestResolve_InheritanceChain_ThreeLevels(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "bob", teamID)

	// root -> mid -> leaf
	rootID := seedRole(t, ctx, store, "root", "")
	midID := seedRole(t, ctx, store, "mid", rootID)
	leafID := seedRole(t, ctx, store, "leaf", midID)

	seedDoc(t, ctx, store, "doc-root", "Root Doc", "Root content.", rootID)
	seedDoc(t, ctx, store, "doc-mid", "Mid Doc", "Mid content.", midID)
	seedDoc(t, ctx, store, "doc-leaf", "Leaf Doc", "Leaf content.", leafID)

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: leafID, Priority: 1},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 1 {
		t.Fatalf("expected 1 role group, got %d", len(resp.Roles))
	}

	chain := resp.Roles[0].Chain
	if len(chain) != 3 {
		t.Fatalf("expected chain length 3, got %d", len(chain))
	}

	// Chain order: leaf-to-root.
	expectedOrder := []string{"leaf", "mid", "root"}
	for i, name := range expectedOrder {
		if chain[i].Role != name {
			t.Errorf("chain[%d]: expected %q, got %q", i, name, chain[i].Role)
		}
		if len(chain[i].Documents) != 1 {
			t.Errorf("chain[%d]: expected 1 doc, got %d", i, len(chain[i].Documents))
		}
	}
}

func TestResolve_MultipleRoles_PriorityOrdering(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "carol", teamID)

	role1 := seedRole(t, ctx, store, "frontend", "")
	role2 := seedRole(t, ctx, store, "security", "")

	seedDoc(t, ctx, store, "doc-fe", "FE Guide", "Frontend.", role1)
	seedDoc(t, ctx, store, "doc-sec", "Sec Guide", "Security.", role2)

	// Priority 2 first in slice, but Resolve should return ordered by priority.
	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: role1, Priority: 1},
		{AgentID: agentID, RoleID: role2, Priority: 2},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 2 {
		t.Fatalf("expected 2 role groups, got %d", len(resp.Roles))
	}
	if resp.Roles[0].Priority != 1 {
		t.Errorf("first group priority: expected 1, got %d", resp.Roles[0].Priority)
	}
	if resp.Roles[0].Chain[0].Role != "frontend" {
		t.Errorf("first group role: expected 'frontend', got %q", resp.Roles[0].Chain[0].Role)
	}
	if resp.Roles[1].Priority != 2 {
		t.Errorf("second group priority: expected 2, got %d", resp.Roles[1].Priority)
	}
	if resp.Roles[1].Chain[0].Role != "security" {
		t.Errorf("second group role: expected 'security', got %q", resp.Roles[1].Chain[0].Role)
	}
}

func TestResolve_WithMemory(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "dave", teamID)
	roleID := seedRole(t, ctx, store, "dev", "")

	seedDoc(t, ctx, store, "doc-1", "Dev Guide", "Coding.", roleID)
	seedMemory(t, ctx, store, "mem-1", "Learned Patterns", "Pattern A.", agentID)
	seedMemory(t, ctx, store, "mem-2", "Project Notes", "Note B.", agentID)

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: roleID, Priority: 1},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 1 {
		t.Fatalf("expected 1 role group, got %d", len(resp.Roles))
	}
	if len(resp.Memory) != 2 {
		t.Fatalf("expected 2 memory docs, got %d", len(resp.Memory))
	}
	// ListAgentMemory orders by title, so "Learned Patterns" < "Project Notes".
	if resp.Memory[0].Title != "Learned Patterns" {
		t.Errorf("memory[0] title: expected 'Learned Patterns', got %q", resp.Memory[0].Title)
	}
	if resp.Memory[1].Title != "Project Notes" {
		t.Errorf("memory[1] title: expected 'Project Notes', got %q", resp.Memory[1].Title)
	}
}

func TestResolve_NoRoles_NoMemory(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "empty", teamID)

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 0 {
		t.Errorf("expected 0 role groups, got %d", len(resp.Roles))
	}
	if len(resp.Memory) != 0 {
		t.Errorf("expected 0 memory docs, got %d", len(resp.Memory))
	}
}

func TestValidateRoleParent_CycleDetection_SelfReference(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	roleID := seedRole(t, ctx, store, "solo", "")

	err := ps.ValidateRoleParent(ctx, roleID, roleID)
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateRoleParent_CycleDetection_Indirect(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	// A -> B -> C; now try to set A as parent of C (C -> A would create cycle)
	aID := seedRole(t, ctx, store, "a", "")
	bID := seedRole(t, ctx, store, "b", aID)
	cID := seedRole(t, ctx, store, "c", bID)

	// Setting C's parent to... wait, C already has parent B.
	// The cycle test: try to set A's parent to C (A -> C, but C -> B -> A).
	err := ps.ValidateRoleParent(ctx, aID, cID)
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateRoleParent_CycleDetection_NoCycle(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	aID := seedRole(t, ctx, store, "a", "")
	_ = seedRole(t, ctx, store, "b", aID)
	cID := seedRole(t, ctx, store, "c", "")

	// Setting C's parent to A is fine (no cycle).
	err := ps.ValidateRoleParent(ctx, cID, aID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateRoleParent_DepthExceeded(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 3) // max depth = 3

	// Build chain: root -> child -> grandchild (depth 3)
	rootID := seedRole(t, ctx, store, "root", "")
	childID := seedRole(t, ctx, store, "child", rootID)
	grandchildID := seedRole(t, ctx, store, "grandchild", childID)

	// Adding a new role under grandchild would make depth 4, exceeding max 3.
	newRoleID := seedRole(t, ctx, store, "too-deep", "")
	err := ps.ValidateRoleParent(ctx, newRoleID, grandchildID)
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Fatalf("expected ErrDepthExceeded, got %v", err)
	}
}

func TestValidateRoleParent_DepthExceeded_WithDescendants(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 4) // max depth = 4

	// Chain 1: a -> b (depth 2)
	aID := seedRole(t, ctx, store, "a", "")
	_ = seedRole(t, ctx, store, "b", aID)

	// Chain 2: c -> d -> e (depth 3)
	cID := seedRole(t, ctx, store, "c", "")
	dID := seedRole(t, ctx, store, "d", cID)
	_ = seedRole(t, ctx, store, "e", dID)

	// Setting A's parent to C would make: e -> d -> c -> a -> b = depth 5 > 4.
	err := ps.ValidateRoleParent(ctx, aID, cID)
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Fatalf("expected ErrDepthExceeded, got %v", err)
	}
}

func TestValidateRoleParent_EmptyParent(t *testing.T) {
	ctx := context.Background()
	_, ps := testSetup(t, 10)

	// Removing parent (setting to "") is always valid.
	err := ps.ValidateRoleParent(ctx, "any-role", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateRoleParent_NewRole(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 3)

	rootID := seedRole(t, ctx, store, "root", "")
	childID := seedRole(t, ctx, store, "child", rootID)

	// New role (roleID="") under child makes depth 3 = max, should pass.
	err := ps.ValidateRoleParent(ctx, "", childID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAuditRoleDepths_WarnsOnDeepChain(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	health := daemon.NewHealthService()
	ps := daemon.NewProvisioningService(store, health, 2) // max depth = 2

	// Build chain of depth 3 (exceeds max 2).
	// We bypass validation by creating directly in the store.
	rootID := seedRole(t, ctx, store, "root", "")
	midID := seedRole(t, ctx, store, "mid", rootID)
	_ = seedRole(t, ctx, store, "leaf", midID)

	ps.AuditRoleDepths(ctx)

	report := health.Status()
	if report.Status != daemon.StatusDegraded {
		t.Fatalf("expected status degraded, got %s", report.Status)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected warnings, got none")
	}

	// At least one warning should mention a depth violation.
	found := false
	for _, w := range report.Warnings {
		if len(w) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one non-empty warning")
	}
}

func TestAuditRoleDepths_NoWarningsWhenAllValid(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	health := daemon.NewHealthService()
	ps := daemon.NewProvisioningService(store, health, 10)

	rootID := seedRole(t, ctx, store, "root", "")
	_ = seedRole(t, ctx, store, "child", rootID)

	ps.AuditRoleDepths(ctx)

	report := health.Status()
	if report.Status != daemon.StatusHealthy {
		t.Fatalf("expected status healthy, got %s", report.Status)
	}
	if len(report.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(report.Warnings))
	}
}

func TestResolve_RoleWithNoDocuments(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "eve", teamID)
	roleID := seedRole(t, ctx, store, "empty-role", "")

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: roleID, Priority: 1},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 1 {
		t.Fatalf("expected 1 role group, got %d", len(resp.Roles))
	}
	if len(resp.Roles[0].Chain) != 1 {
		t.Fatalf("expected 1 chain entry, got %d", len(resp.Roles[0].Chain))
	}
	if len(resp.Roles[0].Chain[0].Documents) != 0 {
		t.Errorf("expected 0 docs, got %d", len(resp.Roles[0].Chain[0].Documents))
	}
}

func TestResolve_SharedAncestor_NoDedupe(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "eng")
	agentID := seedAgent(t, ctx, store, "frank", teamID)

	// Shared root with two leaf branches.
	rootID := seedRole(t, ctx, store, "base", "")
	leaf1 := seedRole(t, ctx, store, "leaf1", rootID)
	leaf2 := seedRole(t, ctx, store, "leaf2", rootID)

	seedDoc(t, ctx, store, "doc-base", "Base Doc", "Base.", rootID)
	seedDoc(t, ctx, store, "doc-l1", "L1 Doc", "L1.", leaf1)
	seedDoc(t, ctx, store, "doc-l2", "L2 Doc", "L2.", leaf2)

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: leaf1, Priority: 1},
		{AgentID: agentID, RoleID: leaf2, Priority: 2},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 2 {
		t.Fatalf("expected 2 role groups, got %d", len(resp.Roles))
	}

	// Both chains should include the "base" role with its document (no dedup).
	for i, group := range resp.Roles {
		lastEntry := group.Chain[len(group.Chain)-1]
		if lastEntry.Role != "base" {
			t.Errorf("group[%d] last chain entry: expected 'base', got %q", i, lastEntry.Role)
		}
		if len(lastEntry.Documents) != 1 {
			t.Errorf("group[%d] base docs: expected 1, got %d", i, len(lastEntry.Documents))
		}
	}
}
```

---

## Summary

| File | Action | Purpose |
|------|--------|---------|
| `internal/daemon/provisioning.go` | Create | ProvisioningService: Resolve, ValidateRoleParent, AuditRoleDepths |
| `internal/daemon/provisioning_test.go` | Create | Comprehensive tests (12 test functions) |
| `internal/daemon/server.go` | Modify | Add `Provisioning` field to ServerConfig |
| `cmd/daemon/daemon.go` | Modify | Create ProvisioningService, call AuditRoleDepths on boot |

### Verification

After implementation, run:

```bash
go test ./internal/daemon/ -v -run TestResolve
go test ./internal/daemon/ -v -run TestValidateRoleParent
go test ./internal/daemon/ -v -run TestAuditRoleDepths
go vet ./...
```

All 12 tests must pass. `go vet` must report no issues.
