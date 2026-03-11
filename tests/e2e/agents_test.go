// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestAgents(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Setup: team and role required for agents and role assignment.
	team, err := c.CreateTeam(ctx(), "agents-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	role, err := c.CreateRole(ctx(), "agents-role", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Create
	agent, err := c.CreateAgent(ctx(), "alice", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if agent.Name != "alice" {
		t.Errorf("Name: got %q, want %q", agent.Name, "alice")
	}
	if agent.TeamID != team.ID {
		t.Errorf("TeamID: got %q, want %q", agent.TeamID, team.ID)
	}

	// Get (includes roles slice, initially empty)
	got, err := c.GetAgent(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.ID != agent.ID {
		t.Errorf("GetAgent ID: got %q, want %q", got.ID, agent.ID)
	}
	if len(got.Roles) != 0 {
		t.Errorf("GetAgent Roles: expected empty, got %v", got.Roles)
	}

	// List
	agents, err := c.ListAgents(ctx(), "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if !containsAgent(agents, agent.ID) {
		t.Errorf("ListAgents: agent %q not found", agent.ID)
	}

	// List filtered by team
	teamAgents, err := c.ListAgents(ctx(), team.ID)
	if err != nil {
		t.Fatalf("ListAgents by team: %v", err)
	}
	if !containsAgent(teamAgents, agent.ID) {
		t.Errorf("ListAgents(team=%s): agent not found", team.ID)
	}

	// Update
	updated, err := c.UpdateAgent(ctx(), agent.ID, "alice-updated", team.ID)
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if updated.Name != "alice-updated" {
		t.Errorf("UpdateAgent Name: got %q, want %q", updated.Name, "alice-updated")
	}

	// Set role assignments
	assignments := []client.RoleAssignment{
		{RoleID: role.ID, Priority: 1},
	}
	agentRoles, err := c.SetAgentRoles(ctx(), agent.ID, assignments)
	if err != nil {
		t.Fatalf("SetAgentRoles: %v", err)
	}
	if len(agentRoles) != 1 || agentRoles[0].RoleID != role.ID {
		t.Errorf("SetAgentRoles: expected 1 assignment for role %s, got %v", role.ID, agentRoles)
	}

	// Get after role assignment
	withRoles, err := c.GetAgent(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after SetAgentRoles: %v", err)
	}
	if len(withRoles.Roles) != 1 {
		t.Errorf("GetAgent Roles: expected 1, got %d", len(withRoles.Roles))
	}

	// Clear role assignments
	if _, err := c.SetAgentRoles(ctx(), agent.ID, []client.RoleAssignment{}); err != nil {
		t.Fatalf("SetAgentRoles (clear): %v", err)
	}

	// Delete
	if err := c.DeleteAgent(ctx(), agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// Confirm gone
	_, err = c.GetAgent(ctx(), agent.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetAgent after delete: expected ErrNotFound, got %v", err)
	}
}

func containsAgent(agents []client.Agent, id string) bool {
	for _, a := range agents {
		if a.ID == id {
			return true
		}
	}
	return false
}
