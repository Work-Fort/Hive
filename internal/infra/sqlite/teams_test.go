// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

func newTestStore(t *testing.T) domain.Store {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGetTeam(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t_001", Name: "alpha"}
	if err := store.CreateTeam(ctx, team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	got, err := store.GetTeam(ctx, "t_001")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("got name %q, want %q", got.Name, "alpha")
	}
}

func TestCreateTeamDuplicate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	team := &domain.Team{ID: "t_001", Name: "alpha"}
	store.CreateTeam(ctx, team)

	dup := &domain.Team{ID: "t_002", Name: "alpha"}
	err := store.CreateTeam(ctx, dup)
	if err == nil {
		t.Fatal("expected error for duplicate team name")
	}
}

func TestListTeams(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTeam(ctx, &domain.Team{ID: "t_002", Name: "beta"})

	teams, err := store.ListTeams(ctx)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 2 {
		t.Errorf("got %d teams, want 2", len(teams))
	}
}

func TestDeleteTeamWithAgents(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	err := store.DeleteTeam(ctx, "t_001")
	if err == nil {
		t.Fatal("expected error when deleting team with agents")
	}
}

func TestDeleteTeamEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	if err := store.DeleteTeam(ctx, "t_001"); err != nil {
		t.Fatalf("delete team: %v", err)
	}

	_, err := store.GetTeam(ctx, "t_001")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}
