// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestSeedPermissions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	names := []string{"role:read", "role:write", "memory:read"}
	if err := store.SeedPermissions(ctx, names); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	// Seed again (idempotent)
	if err := store.SeedPermissions(ctx, names); err != nil {
		t.Fatalf("re-seed permissions: %v", err)
	}
}

func TestAgentPermissions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
	store.SeedPermissions(ctx, []string{"role:read", "memory:write", "task:read"})

	perms := []domain.AgentPermission{
		{AgentID: "a_001", PermissionID: "", ScopeTeamID: ""},
	}
	// We need permission IDs — get them from the store
	// For simplicity, test HasPermission directly
	_ = perms

	// Set permissions using the name-based approach
	store.SetAgentPermissions(ctx, "a_001", []domain.AgentPermission{
		{AgentID: "a_001", PermissionID: "role:read", ScopeTeamID: ""},
		{AgentID: "a_001", PermissionID: "task:read", ScopeTeamID: "t_001"},
	})

	// Global permission check
	has, err := store.HasPermission(ctx, "a_001", "role:read", "")
	if err != nil {
		t.Fatalf("has permission: %v", err)
	}
	if !has {
		t.Error("expected alice to have global role:read")
	}

	// Scoped permission check
	has, err = store.HasPermission(ctx, "a_001", "task:read", "t_001")
	if err != nil {
		t.Fatalf("has permission: %v", err)
	}
	if !has {
		t.Error("expected alice to have task:read scoped to t_001")
	}

	// Permission not granted
	has, err = store.HasPermission(ctx, "a_001", "memory:write", "")
	if err != nil {
		t.Fatalf("has permission: %v", err)
	}
	if has {
		t.Error("expected alice NOT to have memory:write")
	}
}
