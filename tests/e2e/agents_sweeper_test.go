// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"testing"
	"time"
)

func TestSweeperClearsExpiredLease(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "sweeper-team-1")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000020", "sweeper-agent-1", team.ID, "", ""); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Claim with a TTL of 1 second so the lease expires quickly.
	claimed, err := c.ClaimAgent(ctx(), "developer", "hive", "wf-sweep-e2e", 1)
	if err != nil {
		t.Fatalf("ClaimAgent: %v", err)
	}
	if claimed.CurrentWorkflowID != "wf-sweep-e2e" {
		t.Errorf("CurrentWorkflowID = %q, want %q", claimed.CurrentWorkflowID, "wf-sweep-e2e")
	}

	// Wait long enough for the lease to expire and the sweeper (200ms interval) to fire.
	time.Sleep(2 * time.Second)

	assigned, err := c.ListAgentsByAssignment(ctx(), "", "true", "", "", "")
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(assigned=true): %v", err)
	}
	if len(assigned) != 0 {
		t.Errorf("expected 0 assigned agents after sweep, got %d: %+v", len(assigned), assigned)
	}

	free, err := c.ListAgentsByAssignment(ctx(), "", "false", "", "", "")
	if err != nil {
		t.Fatalf("ListAgentsByAssignment(assigned=false): %v", err)
	}
	if len(free) != 1 {
		t.Errorf("expected 1 free agent after sweep, got %d", len(free))
	}
}
