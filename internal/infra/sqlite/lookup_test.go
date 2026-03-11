// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestLookupTeamByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	got, err := store.LookupTeamByName(ctx, "alpha")
	if err != nil {
		t.Fatalf("lookup team: %v", err)
	}
	if got.ID != "t_001" || got.Name != "alpha" {
		t.Errorf("got %+v, want id=t_001 name=alpha", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("expected non-zero timestamps")
	}
}

func TestLookupTeamByNameNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.LookupTeamByName(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLookupRoleByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "reviewer", ParentID: "r_001"})

	got, err := store.LookupRoleByName(ctx, "reviewer")
	if err != nil {
		t.Fatalf("lookup role: %v", err)
	}
	if got.ID != "r_002" || got.Name != "reviewer" || got.ParentID != "r_001" {
		t.Errorf("got %+v, want id=r_002 name=reviewer parent=r_001", got)
	}
}

func TestLookupRoleByNameNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.LookupRoleByName(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLookupRoleByNameNoParent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "root"})

	got, err := store.LookupRoleByName(ctx, "root")
	if err != nil {
		t.Fatalf("lookup role: %v", err)
	}
	if got.ParentID != "" {
		t.Errorf("expected empty ParentID, got %q", got.ParentID)
	}
}

func TestLookupAgentByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	got, err := store.LookupAgentByName(ctx, "alice")
	if err != nil {
		t.Fatalf("lookup agent: %v", err)
	}
	if got.ID != "a_001" || got.Name != "alice" || got.TeamID != "t_001" {
		t.Errorf("got %+v, want id=a_001 name=alice team=t_001", got)
	}
}

func TestLookupAgentByNameNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.LookupAgentByName(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLookupDocumentByOwnerAndTitle_RoleDoc(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})
	store.CreateDocument(ctx, &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindRole,
		Title: "Dev Guide", Content: "# Guide",
		RoleID: "r_001",
	})

	got, err := store.LookupDocumentByOwnerAndTitle(ctx, "r_001", "Dev Guide")
	if err != nil {
		t.Fatalf("lookup document: %v", err)
	}
	if got.ID != "d_001" || got.Title != "Dev Guide" || got.RoleID != "r_001" {
		t.Errorf("got %+v", got)
	}
}

func TestLookupDocumentByOwnerAndTitle_AgentMemory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
	store.CreateDocument(ctx, &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindMemory,
		Title: "Patterns", Content: "# Patterns",
		AgentID: "a_001",
	})

	got, err := store.LookupDocumentByOwnerAndTitle(ctx, "a_001", "Patterns")
	if err != nil {
		t.Fatalf("lookup document: %v", err)
	}
	if got.ID != "d_001" || got.AgentID != "a_001" {
		t.Errorf("got %+v", got)
	}
}

func TestLookupDocumentByOwnerAndTitleNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.LookupDocumentByOwnerAndTitle(ctx, "r_bogus", "Nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLookupTaskByTeamAndTitle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTask(ctx, &domain.Task{
		ID: "tk_001", TeamID: "t_001",
		Title: "Fix bug", Description: "Fix login",
		Status: domain.TaskStatusPending,
	})

	got, err := store.LookupTaskByTeamAndTitle(ctx, "t_001", "Fix bug")
	if err != nil {
		t.Fatalf("lookup task: %v", err)
	}
	if got.ID != "tk_001" || got.Title != "Fix bug" || got.TeamID != "t_001" {
		t.Errorf("got %+v", got)
	}
}

func TestLookupTaskByTeamAndTitleNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.LookupTaskByTeamAndTitle(ctx, "t_bogus", "Nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestLookupTaskByTeamAndTitleWrongTeam(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTeam(ctx, &domain.Team{ID: "t_002", Name: "beta"})
	store.CreateTask(ctx, &domain.Task{
		ID: "tk_001", TeamID: "t_001",
		Title: "Fix bug", Status: domain.TaskStatusPending,
	})

	// Looking up in a different team should return not found.
	_, err := store.LookupTaskByTeamAndTitle(ctx, "t_002", "Fix bug")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong team, got: %v", err)
	}
}

func TestListPermissions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SeedPermissions(ctx, []string{"role:write", "role:read", "memory:read"})

	perms, err := store.ListPermissions(ctx)
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	if len(perms) != 3 {
		t.Fatalf("got %d permissions, want 3", len(perms))
	}
	// Should be sorted by name.
	if perms[0].Name != "memory:read" {
		t.Errorf("perms[0].Name = %q, want %q", perms[0].Name, "memory:read")
	}
	if perms[1].Name != "role:read" {
		t.Errorf("perms[1].Name = %q, want %q", perms[1].Name, "role:read")
	}
	if perms[2].Name != "role:write" {
		t.Errorf("perms[2].Name = %q, want %q", perms[2].Name, "role:write")
	}
}

func TestListPermissionsEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	perms, err := store.ListPermissions(ctx)
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	if len(perms) != 0 {
		t.Errorf("got %d permissions, want 0", len(perms))
	}
}

func TestLookupPermissionByName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.SeedPermissions(ctx, []string{"role:read", "role:write"})

	got, err := store.LookupPermissionByName(ctx, "role:read")
	if err != nil {
		t.Fatalf("lookup permission: %v", err)
	}
	if got.Name != "role:read" || got.ID == "" {
		t.Errorf("got %+v", got)
	}
}

func TestLookupPermissionByNameNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.LookupPermissionByName(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Timestamp preservation tests (Task 3)
// ---------------------------------------------------------------------------

func TestCreateTeamPreservesTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	team := &domain.Team{ID: "t_001", Name: "alpha", CreatedAt: ts, UpdatedAt: ts}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	got, err := store.GetTeam(ctx, "t_001")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
	if !got.UpdatedAt.Equal(ts) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, ts)
	}
}

func TestCreateTeamDefaultTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	team := &domain.Team{ID: "t_001", Name: "alpha"}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	got, err := store.GetTeam(ctx, "t_001")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, expected between %v and %v", got.CreatedAt, before, after)
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, expected between %v and %v", got.UpdatedAt, before, after)
	}
}

func TestCreateTeamSetsStructTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t_001", Name: "alpha"}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}
	// The struct itself should have timestamps set after Create.
	if team.CreatedAt.IsZero() {
		t.Error("expected struct CreatedAt to be set after Create")
	}
	if team.UpdatedAt.IsZero() {
		t.Error("expected struct UpdatedAt to be set after Create")
	}
}

func TestCreateRolePreservesTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	role := &domain.Role{ID: "r_001", Name: "dev", CreatedAt: ts, UpdatedAt: ts}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}

	got, err := store.GetRole(ctx, "r_001")
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestCreateAgentPreservesTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	ts := time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC)
	agent := &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001", CreatedAt: ts, UpdatedAt: ts}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := store.GetAgent(ctx, "a_001")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestCreateDocumentPreservesTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "dev"})

	ts := time.Date(2024, 2, 20, 15, 30, 0, 0, time.UTC)
	doc := &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindRole,
		Title: "Guide", Content: "# Guide",
		RoleID: "r_001", CreatedAt: ts, UpdatedAt: ts,
	}
	if err := store.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	got, err := store.GetDocument(ctx, "d_001")
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestCreateTaskPreservesTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	ts := time.Date(2024, 7, 4, 9, 0, 0, 0, time.UTC)
	task := &domain.Task{
		ID: "tk_001", TeamID: "t_001",
		Title: "Fix bug", Status: domain.TaskStatusPending,
		CreatedAt: ts, UpdatedAt: ts,
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ts)
	}
}

func TestCreateWithPartialTimestamps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Only set CreatedAt, leave UpdatedAt zero.
	ts := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	team := &domain.Team{ID: "t_001", Name: "alpha", CreatedAt: ts}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	got, err := store.GetTeam(ctx, "t_001")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want preserved %v", got.CreatedAt, ts)
	}
	// UpdatedAt should be auto-set to now, not zero.
	if got.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be auto-set when zero")
	}
}
