// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestAgent_ModelAndRuntime_Roundtrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	agent := &domain.Agent{
		ID:      "a_001",
		Name:    "worker-01",
		TeamID:  "t_001",
		Model:   "claude-sonnet-4-6",
		Runtime: "claude-cli",
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := store.GetAgent(ctx, "a_001")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-sonnet-4-6")
	}
	if got.Runtime != "claude-cli" {
		t.Errorf("Runtime = %q, want %q", got.Runtime, "claude-cli")
	}

	// Update through new signature taking *Agent.
	agent.Model = "claude-opus-4-7"
	agent.Runtime = "go-adk"
	if err := store.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("update agent: %v", err)
	}
	got2, _ := store.GetAgent(ctx, "a_001")
	if got2.Model != "claude-opus-4-7" || got2.Runtime != "go-adk" {
		t.Errorf("after update: got %+v", got2)
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

func TestAgent_CurrentAssignment_Roundtrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	agent := &domain.Agent{
		ID: "a_001", Name: "worker-01", TeamID: "t_001",
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := store.GetAgent(ctx, "a_001")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.CurrentRole != "" {
		t.Errorf("CurrentRole = %q, want empty", got.CurrentRole)
	}
	if got.CurrentProject != "" {
		t.Errorf("CurrentProject = %q, want empty", got.CurrentProject)
	}
	if got.CurrentWorkflowID != "" {
		t.Errorf("CurrentWorkflowID = %q, want empty", got.CurrentWorkflowID)
	}
	if !got.LeaseExpiresAt.IsZero() {
		t.Errorf("LeaseExpiresAt = %v, want zero", got.LeaseExpiresAt)
	}

	lease := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	agent.CurrentRole = "developer"
	agent.CurrentProject = "hive"
	agent.CurrentWorkflowID = "wf-abc"
	agent.LeaseExpiresAt = lease
	if err := store.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	got2, err := store.GetAgent(ctx, "a_001")
	if err != nil {
		t.Fatalf("get agent after update: %v", err)
	}
	if got2.CurrentRole != "developer" {
		t.Errorf("CurrentRole = %q, want %q", got2.CurrentRole, "developer")
	}
	if got2.CurrentProject != "hive" {
		t.Errorf("CurrentProject = %q, want %q", got2.CurrentProject, "hive")
	}
	if got2.CurrentWorkflowID != "wf-abc" {
		t.Errorf("CurrentWorkflowID = %q, want %q", got2.CurrentWorkflowID, "wf-abc")
	}
	if !got2.LeaseExpiresAt.Equal(lease) {
		t.Errorf("LeaseExpiresAt = %v, want %v", got2.LeaseExpiresAt, lease)
	}
}

func TestAgent_CurrentAssignment_AllOrNothingRejected(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	// Insert with partial assignment (role set, others empty) should fail.
	partial := &domain.Agent{
		ID: "a_001", Name: "alice", TeamID: "t_001",
		CurrentRole: "developer", // others deliberately left zero
	}
	if err := store.CreateAgent(ctx, partial); err == nil {
		t.Fatalf("CreateAgent with partial current_* unexpectedly succeeded")
	}

	// Fully free agent succeeds.
	free := &domain.Agent{ID: "a_002", Name: "bob", TeamID: "t_001"}
	if err := store.CreateAgent(ctx, free); err != nil {
		t.Fatalf("CreateAgent with free agent failed: %v", err)
	}

	// Now try to update it to a partial state — should fail.
	free.CurrentWorkflowID = "wf-xyz"
	if err := store.UpdateAgent(ctx, free); err == nil {
		t.Fatalf("UpdateAgent with partial current_* unexpectedly succeeded")
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

func TestClaimAgent_PicksFreeAgent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "agent-one", TeamID: "t_001"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_002", Name: "agent-two", TeamID: "t_001"})

	expires := time.Now().UTC().Add(time.Hour)
	claimed, err := store.ClaimAgent(ctx, "developer", "hive", "wf-abc", expires)
	if err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}
	if claimed.CurrentWorkflowID != "wf-abc" {
		t.Errorf("CurrentWorkflowID = %q, want %q", claimed.CurrentWorkflowID, "wf-abc")
	}
	if claimed.CurrentRole != "developer" {
		t.Errorf("CurrentRole = %q, want %q", claimed.CurrentRole, "developer")
	}
	if claimed.CurrentProject != "hive" {
		t.Errorf("CurrentProject = %q, want %q", claimed.CurrentProject, "hive")
	}

	// The other agent should remain free.
	otherID := "a_001"
	if claimed.ID == "a_001" {
		otherID = "a_002"
	}
	other, err := store.GetAgent(ctx, otherID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if other.CurrentWorkflowID != "" {
		t.Errorf("second agent should be free, got workflow %q", other.CurrentWorkflowID)
	}
}

func TestClaimAgent_PoolExhausted(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	expires := time.Now().UTC().Add(time.Hour)
	store.CreateAgent(ctx, &domain.Agent{
		ID: "a_001", Name: "agent-one", TeamID: "t_001",
		CurrentRole: "dev", CurrentProject: "p", CurrentWorkflowID: "wf-1", LeaseExpiresAt: expires,
	})
	store.CreateAgent(ctx, &domain.Agent{
		ID: "a_002", Name: "agent-two", TeamID: "t_001",
		CurrentRole: "dev", CurrentProject: "p", CurrentWorkflowID: "wf-2", LeaseExpiresAt: expires,
	})

	_, err := store.ClaimAgent(ctx, "developer", "hive", "wf-new", time.Now().UTC().Add(time.Hour))
	if !errors.Is(err, domain.ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
}

func TestReleaseAgent_WorkflowMismatchRejected(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	expires := time.Now().UTC().Add(time.Hour)
	if _, err := store.ClaimAgent(ctx, "developer", "hive", "wf-1", expires); err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}

	err := store.ReleaseAgent(ctx, "a_001", "wf-2")
	if !errors.Is(err, domain.ErrWorkflowMismatch) {
		t.Errorf("expected ErrWorkflowMismatch, got %v", err)
	}

	// Agent must still be claimed.
	got, _ := store.GetAgent(ctx, "a_001")
	if got.CurrentWorkflowID != "wf-1" {
		t.Errorf("agent should still be claimed by wf-1, got %q", got.CurrentWorkflowID)
	}
}

func TestRenewAgentLease_ExtendsExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	t0 := time.Now().UTC().Truncate(time.Second)
	if _, err := store.ClaimAgent(ctx, "developer", "hive", "wf-1", t0); err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}

	newExpiry := t0.Add(time.Hour)
	if err := store.RenewAgentLease(ctx, "a_001", "wf-1", newExpiry); err != nil {
		t.Fatalf("RenewAgentLease: %v", err)
	}

	got, err := store.GetAgent(ctx, "a_001")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !got.LeaseExpiresAt.Equal(newExpiry) {
		t.Errorf("LeaseExpiresAt = %v, want %v", got.LeaseExpiresAt, newExpiry)
	}
}

func TestSweepExpiredLeases(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_002", Name: "bob", TeamID: "t_001"})

	past := time.Now().UTC().Add(-time.Minute)
	future := time.Now().UTC().Add(time.Hour)

	// a_001 has an expired lease; a_002 has a fresh one.
	a1 := &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001",
		CurrentRole: "developer", CurrentProject: "flow",
		CurrentWorkflowID: "wf-1", LeaseExpiresAt: past}
	if err := store.UpdateAgent(ctx, a1); err != nil {
		t.Fatalf("update a_001: %v", err)
	}
	a2 := &domain.Agent{ID: "a_002", Name: "bob", TeamID: "t_001",
		CurrentRole: "reviewer", CurrentProject: "flow",
		CurrentWorkflowID: "wf-2", LeaseExpiresAt: future}
	if err := store.UpdateAgent(ctx, a2); err != nil {
		t.Fatalf("update a_002: %v", err)
	}

	released, err := store.SweepExpiredLeases(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(released) != 1 || released[0].ID != "a_001" {
		t.Fatalf("expected a_001 released, got %+v", released)
	}

	got1, _ := store.GetAgent(ctx, "a_001")
	if got1.CurrentWorkflowID != "" {
		t.Errorf("a_001 still claimed: %+v", got1)
	}
	got2, _ := store.GetAgent(ctx, "a_002")
	if got2.CurrentWorkflowID != "wf-2" {
		t.Errorf("a_002 unexpectedly released: %+v", got2)
	}
}

func TestListAgentsByAssignment_FreeAndClaimed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "free-one", TeamID: "t_001"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_002", Name: "free-two", TeamID: "t_001"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_003", Name: "claimed-dev", TeamID: "t_001"})

	expires := time.Now().UTC().Add(time.Hour)
	if _, err := store.ClaimAgent(ctx, "developer", "proj", "wf-1", expires); err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}

	trueVal := true
	falseVal := false

	claimed, err := store.ListAgentsByAssignment(ctx, domain.AgentAssignmentFilter{Assigned: &trueVal})
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(assigned=true): %v", err)
	}
	if len(claimed) != 1 {
		t.Errorf("assigned=true: expected 1, got %d", len(claimed))
	}

	free, err := store.ListAgentsByAssignment(ctx, domain.AgentAssignmentFilter{Assigned: &falseVal})
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(assigned=false): %v", err)
	}
	if len(free) != 2 {
		t.Errorf("assigned=false: expected 2, got %d", len(free))
	}

	byRole, err := store.ListAgentsByAssignment(ctx, domain.AgentAssignmentFilter{Role: "developer"})
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(role=developer): %v", err)
	}
	if len(byRole) != 1 || byRole[0].CurrentRole != "developer" {
		t.Errorf("role=developer: expected 1 developer, got %+v", byRole)
	}
}
