// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/client"
)

func TestClaimReleaseRoundTrip(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "pool-team-1")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000010", "pool-agent-1", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent 1: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000011", "pool-agent-2", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent 2: %v", err)
	}

	before := time.Now()
	claimed, err := c.ClaimAgent(ctx(), "developer", "hive", "wf-rrt", 120)
	if err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}
	if claimed.CurrentWorkflowID != "wf-rrt" {
		t.Errorf("CurrentWorkflowID = %q, want %q", claimed.CurrentWorkflowID, "wf-rrt")
	}
	if claimed.LeaseExpiresAt.Before(before.Add(100 * time.Second)) {
		t.Errorf("LeaseExpiresAt %v looks too early", claimed.LeaseExpiresAt)
	}

	if err := c.ReleaseAgent(ctx(), claimed.ID, "wf-rrt"); err != nil {
		t.Fatalf("ReleaseAgent: %v", err)
	}

	free, err := c.ListAgentsByAssignment(ctx(), "", "false", "", "", "")
	if err != nil {
		t.Fatalf("ListAgentsByAssignment: %v", err)
	}
	if len(free) < 2 {
		t.Errorf("expected both agents free after release, got %d", len(free))
	}
}

func TestClaimPoolExhausted(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "pool-team-2")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000012", "pool-only-1", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if _, err := c.ClaimAgent(ctx(), "developer", "hive", "wf-first", 60); err != nil {
		t.Fatalf("first ClaimAgent: %v", err)
	}

	_, err = c.ClaimAgent(ctx(), "developer", "hive", "wf-second", 60)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("expected ErrConflict on exhausted pool, got %v", err)
	}
}

func TestReleaseWorkflowMismatch(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "pool-team-3")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000013", "pool-mismatch-1", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	claimed, err := c.ClaimAgent(ctx(), "developer", "hive", "wf-1", 60)
	if err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}

	err = c.ReleaseAgent(ctx(), claimed.ID, "wf-2")
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("expected ErrConflict on workflow mismatch, got %v", err)
	}
}

func TestRenewExtendsLease(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "pool-team-4")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000014", "pool-renew-1", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	claimed, err := c.ClaimAgent(ctx(), "developer", "hive", "wf-renew", 60)
	if err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}

	if err := c.RenewAgentLease(ctx(), claimed.ID, "wf-renew", 600); err != nil {
		t.Fatalf("RenewAgentLease: %v", err)
	}

	got, err := c.GetAgent(ctx(), claimed.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	minExpiry := time.Now().Add(500 * time.Second)
	if got.LeaseExpiresAt.Before(minExpiry) {
		t.Errorf("LeaseExpiresAt %v should be ~10 min out, got too early", got.LeaseExpiresAt)
	}
}

func TestListAssignedFilters(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "pool-team-5")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000015", "pool-filter-1", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent 1: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000016", "pool-filter-2", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent 2: %v", err)
	}

	claimed, err := c.ClaimAgent(ctx(), "developer", "proj", "wf-filter", 60)
	if err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}

	assigned, err := c.ListAgentsByAssignment(ctx(), "", "true", "", "", "")
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(assigned=true): %v", err)
	}
	if len(assigned) != 1 || assigned[0].ID != claimed.ID {
		t.Errorf("assigned=true: expected claimed agent, got %+v", assigned)
	}

	free, err := c.ListAgentsByAssignment(ctx(), "", "false", "", "", "")
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(assigned=false): %v", err)
	}
	if len(free) != 1 {
		t.Errorf("assigned=false: expected 1 free agent, got %d", len(free))
	}

	byRole, err := c.ListAgentsByAssignment(ctx(), "", "", "", "developer", "")
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(role=developer): %v", err)
	}
	if len(byRole) != 1 || byRole[0].AssignedRole != "developer" {
		t.Errorf("role=developer: expected 1 developer, got %+v", byRole)
	}
}
