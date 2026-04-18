// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestPermissions(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "perm-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000001", "perm-agent", team.ID, "", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Initially no permissions.
	perms, err := c.GetAgentPermissions(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgentPermissions (empty): %v", err)
	}
	if len(perms) != 0 {
		t.Errorf("expected 0 permissions initially, got %d", len(perms))
	}

	// Set global permissions.
	grants := []client.PermissionGrant{
		{Permission: "role:read"},
		{Permission: "memory:read"},
		{Permission: "memory:write"},
		{Permission: "task:read"},
	}
	set, err := c.SetAgentPermissions(ctx(), agent.ID, grants)
	if err != nil {
		t.Fatalf("SetAgentPermissions: %v", err)
	}
	if len(set) != len(grants) {
		t.Errorf("SetAgentPermissions: got %d, want %d", len(set), len(grants))
	}

	// Read back.
	perms, err = c.GetAgentPermissions(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}
	if len(perms) != len(grants) {
		t.Errorf("GetAgentPermissions after set: got %d, want %d", len(perms), len(grants))
	}

	// Overwrite with a scoped grant.
	scopedGrants := []client.PermissionGrant{
		{Permission: "task:write", ScopeTeamID: team.ID},
	}
	scoped, err := c.SetAgentPermissions(ctx(), agent.ID, scopedGrants)
	if err != nil {
		t.Fatalf("SetAgentPermissions (scoped): %v", err)
	}
	if len(scoped) != 1 {
		t.Errorf("scoped SetAgentPermissions: got %d, want 1", len(scoped))
	}
	if scoped[0].ScopeTeamID != team.ID {
		t.Errorf("ScopeTeamID: got %q, want %q", scoped[0].ScopeTeamID, team.ID)
	}

	// Revoke all.
	revoked, err := c.SetAgentPermissions(ctx(), agent.ID, []client.PermissionGrant{})
	if err != nil {
		t.Fatalf("SetAgentPermissions (revoke all): %v", err)
	}
	if len(revoked) != 0 {
		t.Errorf("after revoke: expected 0, got %d", len(revoked))
	}
}

// TestUnauthorizedRequest verifies that requests with an invalid token are
// rejected with ErrUnauthorized (HTTP 401), and that requests with no
// Authorization header are also rejected with ErrUnauthorized.
func TestUnauthorizedRequest(t *testing.T) {
	h := newHarness(t)

	// Invalid token: Passport returns 401 Unauthorized.
	badClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"invalid-token",
	)
	_, err := badClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("invalid token: expected ErrUnauthorized, got %v", err)
	}

	// No token: Passport returns 401 Unauthorized.
	noTokenClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"",
	)
	_, err = noTokenClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("no token: expected ErrUnauthorized, got %v", err)
	}
}
