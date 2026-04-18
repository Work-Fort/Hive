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

// testSetupWithHealth creates an in-memory store, HealthService, and ProvisioningService.
func testSetupWithHealth(t *testing.T, maxDepth int) (domain.Store, *daemon.HealthService, *daemon.ProvisioningService) {
	t.Helper()
	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	health := daemon.NewHealthService()
	ps := daemon.NewProvisioningService(store, health, maxDepth)
	return store, health, ps
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

func TestResolve_IncludesAgentIdentity(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "alpha")
	agentID := seedAgent(t, ctx, store, "worker-01", teamID)
	roleID := seedRole(t, ctx, store, "worker", "")
	seedDoc(t, ctx, store, "doc-1", "Instructions", "do work", roleID)

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

	if resp.Agent.ID != agentID {
		t.Errorf("Agent.ID = %q, want %q", resp.Agent.ID, agentID)
	}
	if resp.Agent.Name != "worker-01" {
		t.Errorf("Agent.Name = %q, want %q", resp.Agent.Name, "worker-01")
	}
	if resp.Agent.TeamID != teamID {
		t.Errorf("Agent.TeamID = %q, want %q", resp.Agent.TeamID, teamID)
	}
}

func TestResolve_IncludesModelAndRuntime(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "alpha")
	// seedAgent creates with empty model/runtime; update directly.
	agentID := seedAgent(t, ctx, store, "worker-02", teamID)
	if err := store.UpdateAgent(ctx, &domain.Agent{
		ID: agentID, Name: "worker-02", TeamID: teamID,
		Model: "claude-sonnet-4-6", Runtime: "claude-cli",
	}); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resp.Agent.Model != "claude-sonnet-4-6" {
		t.Errorf("Agent.Model = %q, want %q", resp.Agent.Model, "claude-sonnet-4-6")
	}
	if resp.Agent.Runtime != "claude-cli" {
		t.Errorf("Agent.Runtime = %q, want %q", resp.Agent.Runtime, "claude-cli")
	}
}

func TestResolve_UnknownAgent_ReturnsError(t *testing.T) {
	ctx := context.Background()
	_, ps := testSetup(t, 10)

	_, err := ps.Resolve(ctx, "agent-does-not-exist")
	if err == nil {
		t.Fatalf("expected error for unknown agent, got nil")
	}
}

func TestResolve_SingleRole_NoInheritance(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)
	roleID := seedRole(t, ctx, store, "developer", "")
	seedDoc(t, ctx, store, "doc-1", "Setup", "setup content", roleID)

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
		t.Fatalf("got %d role groups, want 1", len(resp.Roles))
	}

	g := resp.Roles[0]
	if g.Priority != 1 {
		t.Errorf("got priority %d, want 1", g.Priority)
	}
	if len(g.Chain) != 1 {
		t.Fatalf("got chain length %d, want 1", len(g.Chain))
	}
	if g.Chain[0].Role != "developer" {
		t.Errorf("got role name %q, want %q", g.Chain[0].Role, "developer")
	}
	if len(g.Chain[0].Documents) != 1 {
		t.Errorf("got %d documents, want 1", len(g.Chain[0].Documents))
	}
	if len(resp.Memory) != 0 {
		t.Errorf("got %d memory docs, want 0", len(resp.Memory))
	}
}

func TestResolve_InheritanceChain_ThreeLevels(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)

	rootID := seedRole(t, ctx, store, "root", "")
	midID := seedRole(t, ctx, store, "mid", rootID)
	leafID := seedRole(t, ctx, store, "leaf", midID)

	seedDoc(t, ctx, store, "doc-root", "Root Doc", "root content", rootID)
	seedDoc(t, ctx, store, "doc-mid", "Mid Doc", "mid content", midID)
	seedDoc(t, ctx, store, "doc-leaf", "Leaf Doc", "leaf content", leafID)

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
		t.Fatalf("got %d role groups, want 1", len(resp.Roles))
	}

	chain := resp.Roles[0].Chain
	if len(chain) != 3 {
		t.Fatalf("got chain length %d, want 3", len(chain))
	}

	// Leaf-to-root order
	wantOrder := []string{"leaf", "mid", "root"}
	for i, want := range wantOrder {
		if chain[i].Role != want {
			t.Errorf("chain[%d].Role = %q, want %q", i, chain[i].Role, want)
		}
		if len(chain[i].Documents) != 1 {
			t.Errorf("chain[%d] got %d documents, want 1", i, len(chain[i].Documents))
		}
	}
}

func TestResolve_MultipleRoles_PriorityOrdering(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)

	role1ID := seedRole(t, ctx, store, "primary", "")
	role2ID := seedRole(t, ctx, store, "secondary", "")

	seedDoc(t, ctx, store, "doc-p", "Primary Doc", "primary content", role1ID)
	seedDoc(t, ctx, store, "doc-s", "Secondary Doc", "secondary content", role2ID)

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: role1ID, Priority: 1},
		{AgentID: agentID, RoleID: role2ID, Priority: 2},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 2 {
		t.Fatalf("got %d role groups, want 2", len(resp.Roles))
	}

	if resp.Roles[0].Priority != 1 {
		t.Errorf("first group priority = %d, want 1", resp.Roles[0].Priority)
	}
	if resp.Roles[1].Priority != 2 {
		t.Errorf("second group priority = %d, want 2", resp.Roles[1].Priority)
	}
	if resp.Roles[0].Chain[0].Role != "primary" {
		t.Errorf("first group role = %q, want %q", resp.Roles[0].Chain[0].Role, "primary")
	}
	if resp.Roles[1].Chain[0].Role != "secondary" {
		t.Errorf("second group role = %q, want %q", resp.Roles[1].Chain[0].Role, "secondary")
	}
}

func TestResolve_WithMemory(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)
	roleID := seedRole(t, ctx, store, "dev", "")

	seedDoc(t, ctx, store, "doc-1", "Dev Doc", "dev content", roleID)
	seedMemory(t, ctx, store, "mem-1", "Memory A", "mem a content", agentID)
	seedMemory(t, ctx, store, "mem-2", "Memory B", "mem b content", agentID)

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
		t.Fatalf("got %d role groups, want 1", len(resp.Roles))
	}
	if len(resp.Memory) != 2 {
		t.Fatalf("got %d memory docs, want 2", len(resp.Memory))
	}

	// ListAgentMemory orders by title alphabetically
	if resp.Memory[0].Title != "Memory A" {
		t.Errorf("memory[0].Title = %q, want %q", resp.Memory[0].Title, "Memory A")
	}
	if resp.Memory[1].Title != "Memory B" {
		t.Errorf("memory[1].Title = %q, want %q", resp.Memory[1].Title, "Memory B")
	}
}

func TestResolve_NoRoles_NoMemory(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 0 {
		t.Errorf("got %d role groups, want 0", len(resp.Roles))
	}
	if len(resp.Memory) != 0 {
		t.Errorf("got %d memory docs, want 0", len(resp.Memory))
	}
}

func TestResolve_RoleWithNoDocuments(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)
	roleID := seedRole(t, ctx, store, "empty", "")

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
		t.Fatalf("got %d role groups, want 1", len(resp.Roles))
	}
	if len(resp.Roles[0].Chain) != 1 {
		t.Fatalf("got chain length %d, want 1", len(resp.Roles[0].Chain))
	}
	if len(resp.Roles[0].Chain[0].Documents) != 0 {
		t.Errorf("got %d documents, want 0", len(resp.Roles[0].Chain[0].Documents))
	}
}

func TestResolve_SharedAncestor_NoDedupe(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "t1")
	agentID := seedAgent(t, ctx, store, "a1", teamID)

	rootID := seedRole(t, ctx, store, "root", "")
	leaf1ID := seedRole(t, ctx, store, "leaf1", rootID)
	leaf2ID := seedRole(t, ctx, store, "leaf2", rootID)

	seedDoc(t, ctx, store, "doc-root", "Root Doc", "root content", rootID)

	err := store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: leaf1ID, Priority: 1},
		{AgentID: agentID, RoleID: leaf2ID, Priority: 2},
	})
	if err != nil {
		t.Fatalf("set agent roles: %v", err)
	}

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(resp.Roles) != 2 {
		t.Fatalf("got %d role groups, want 2", len(resp.Roles))
	}

	// Both chains should include root independently (no dedup)
	for i, g := range resp.Roles {
		if len(g.Chain) != 2 {
			t.Errorf("group[%d] chain length = %d, want 2", i, len(g.Chain))
			continue
		}
		if g.Chain[1].Role != "root" {
			t.Errorf("group[%d] chain[1].Role = %q, want %q", i, g.Chain[1].Role, "root")
		}
	}
}

func TestResolve_FreeAgent_NoCurrentAssignment(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	teamID := seedTeam(t, ctx, store, "alpha")
	agentID := seedAgent(t, ctx, store, "worker-01", teamID)
	roleID := seedRole(t, ctx, store, "developer", "")
	seedDoc(t, ctx, store, "doc-1", "Dev", "dev content", roleID)

	store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
		{AgentID: agentID, RoleID: roleID, Priority: 1},
	})

	resp, err := ps.Resolve(ctx, agentID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if resp.CurrentAssignment != nil {
		t.Errorf("CurrentAssignment should be nil for free agent, got %+v", resp.CurrentAssignment)
	}
	if len(resp.Roles) != 1 || resp.Roles[0].Chain[0].Role != "developer" {
		t.Errorf("expected persistent role developer, got %+v", resp.Roles)
	}
}

func TestValidateRoleParent_CycleDetection_SelfReference(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	roleID := seedRole(t, ctx, store, "self", "")

	err := ps.ValidateRoleParent(ctx, roleID, roleID)
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Errorf("got error %v, want %v", err, domain.ErrCycleDetected)
	}
}

func TestValidateRoleParent_CycleDetection_Indirect(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	// Chain: A -> B -> C (A is root, C is leaf)
	aID := seedRole(t, ctx, store, "a", "")
	bID := seedRole(t, ctx, store, "b", aID)
	cID := seedRole(t, ctx, store, "c", bID)

	// Try to set A's parent to C — would create cycle C -> B -> A -> C
	err := ps.ValidateRoleParent(ctx, aID, cID)
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Errorf("got error %v, want %v", err, domain.ErrCycleDetected)
	}
}

func TestValidateRoleParent_CycleDetection_NoCycle(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 10)

	aID := seedRole(t, ctx, store, "a", "")
	seedRole(t, ctx, store, "b", aID) // B's parent is A
	cID := seedRole(t, ctx, store, "c", "")

	// Set C's parent to A — no cycle: C -> A (B is sibling)
	err := ps.ValidateRoleParent(ctx, cID, aID)
	if err != nil {
		t.Errorf("got error %v, want nil", err)
	}
}

func TestValidateRoleParent_DepthExceeded(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 3)

	// Chain of 3: root -> child -> grandchild (already at maxDepth=3)
	rootID := seedRole(t, ctx, store, "root", "")
	childID := seedRole(t, ctx, store, "child", rootID)
	grandchildID := seedRole(t, ctx, store, "grandchild", childID)

	// Adding a new role under grandchild would make depth 4 > maxDepth 3
	err := ps.ValidateRoleParent(ctx, "", grandchildID)
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Errorf("got error %v, want %v", err, domain.ErrDepthExceeded)
	}
}

func TestValidateRoleParent_DepthExceeded_WithDescendants(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 3)

	// Chain 1: A -> B (depth 2)
	aID := seedRole(t, ctx, store, "a", "")
	bID := seedRole(t, ctx, store, "b", aID)

	// Chain 2: C -> D (depth 2)
	cID := seedRole(t, ctx, store, "c", "")
	seedRole(t, ctx, store, "d", cID)

	// Moving C under B: B -> C -> D would make total A -> B -> C -> D = depth 4 > 3
	err := ps.ValidateRoleParent(ctx, cID, bID)
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Errorf("got error %v, want %v", err, domain.ErrDepthExceeded)
	}
}

func TestValidateRoleParent_EmptyParent(t *testing.T) {
	ctx := context.Background()
	_, ps := testSetup(t, 3)

	// Setting parent to "" (removing parent) should always succeed.
	err := ps.ValidateRoleParent(ctx, "role-whatever", "")
	if err != nil {
		t.Errorf("got error %v, want nil", err)
	}
}

func TestValidateRoleParent_NewRole(t *testing.T) {
	ctx := context.Background()
	store, ps := testSetup(t, 3)

	// Chain: root -> child (depth 2), maxDepth=3
	rootID := seedRole(t, ctx, store, "root", "")
	childID := seedRole(t, ctx, store, "child", rootID)

	// New role (roleID="") under child: root -> child -> new = depth 3, should be OK
	err := ps.ValidateRoleParent(ctx, "", childID)
	if err != nil {
		t.Errorf("got error %v, want nil", err)
	}
}

func TestAuditRoleDepths_WarnsOnDeepChain(t *testing.T) {
	ctx := context.Background()
	store, health, ps := testSetupWithHealth(t, 2)

	// Chain of 3: root -> mid -> leaf (depth 3, exceeds maxDepth=2)
	rootID := seedRole(t, ctx, store, "root", "")
	midID := seedRole(t, ctx, store, "mid", rootID)
	seedRole(t, ctx, store, "leaf", midID)

	ps.AuditRoleDepths(ctx)

	report := health.Report()
	if report.Status != string(daemon.StatusDegraded) {
		t.Errorf("got status %q, want %q", report.Status, daemon.StatusDegraded)
	}
	hasWarning := false
	for _, c := range report.Checks {
		if c.Severity == daemon.SeverityWarning {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Error("expected warning checks, got none")
	}
}

func TestAuditRoleDepths_NoWarningsWhenAllValid(t *testing.T) {
	ctx := context.Background()
	store, health, ps := testSetupWithHealth(t, 10)

	// Chain of 2: root -> child (well within maxDepth=10)
	rootID := seedRole(t, ctx, store, "root", "")
	seedRole(t, ctx, store, "child", rootID)

	ps.AuditRoleDepths(ctx)

	report := health.Report()
	if report.Status != string(daemon.StatusHealthy) {
		t.Errorf("got status %q, want %q", report.Status, daemon.StatusHealthy)
	}
	for _, c := range report.Checks {
		if c.Severity == daemon.SeverityWarning {
			t.Errorf("unexpected warning check: %s", c.Message)
		}
	}
}
