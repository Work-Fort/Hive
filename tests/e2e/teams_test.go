// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestTeams(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Create
	team, err := c.CreateTeam(ctx(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.Name != "alpha" {
		t.Errorf("Name: got %q, want %q", team.Name, "alpha")
	}
	if team.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Get
	got, err := c.GetTeam(ctx(), team.ID)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.ID != team.ID {
		t.Errorf("GetTeam ID mismatch: %q vs %q", got.ID, team.ID)
	}

	// List
	teams, err := c.ListTeams(ctx())
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if !containsTeam(teams, team.ID) {
		t.Errorf("ListTeams: created team %q not found in list", team.ID)
	}

	// Update
	updated, err := c.UpdateTeam(ctx(), team.ID, "alpha-renamed")
	if err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	if updated.Name != "alpha-renamed" {
		t.Errorf("UpdateTeam Name: got %q, want %q", updated.Name, "alpha-renamed")
	}

	// Delete
	if err := c.DeleteTeam(ctx(), team.ID); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}

	// Confirm gone
	_, err = c.GetTeam(ctx(), team.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetTeam after delete: expected ErrNotFound, got %v", err)
	}
}

// TestTeamDuplicateName verifies that creating two teams with the same name
// returns a conflict error.
func TestTeamDuplicateName(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	if _, err := c.CreateTeam(ctx(), "beta"); err != nil {
		t.Fatalf("first CreateTeam: %v", err)
	}

	_, err := c.CreateTeam(ctx(), "beta")
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("duplicate team name: expected ErrConflict, got %v", err)
	}
}

// TestTeamDeleteWithDependencies verifies that a team with agents cannot be
// deleted.
func TestTeamDeleteWithDependencies(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "gamma")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000007", "dep-agent", team.ID); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	err = c.DeleteTeam(ctx(), team.ID)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("delete team with agent: expected ErrConflict, got %v", err)
	}
}

func containsTeam(teams []client.Team, id string) bool {
	for _, t := range teams {
		if t.ID == id {
			return true
		}
	}
	return false
}
