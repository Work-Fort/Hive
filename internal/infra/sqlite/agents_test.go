// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
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
