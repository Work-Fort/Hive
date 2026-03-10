// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateAndGetRole(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	role := &domain.Role{ID: "r_001", Name: "developer"}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}

	got, err := store.GetRole(ctx, "r_001")
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if got.Name != "developer" {
		t.Errorf("got name %q, want %q", got.Name, "developer")
	}
}

func TestRoleInheritanceChain(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "base"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "developer", ParentID: "r_001"})
	store.CreateRole(ctx, &domain.Role{ID: "r_003", Name: "frontend-dev", ParentID: "r_002"})

	chain, err := store.GetRoleChain(ctx, "r_003", 10)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	if len(chain) != 3 {
		t.Fatalf("got %d roles in chain, want 3", len(chain))
	}
	if chain[0].Name != "frontend-dev" {
		t.Errorf("chain[0] = %q, want %q", chain[0].Name, "frontend-dev")
	}
	if chain[1].Name != "developer" {
		t.Errorf("chain[1] = %q, want %q", chain[1].Name, "developer")
	}
	if chain[2].Name != "base" {
		t.Errorf("chain[2] = %q, want %q", chain[2].Name, "base")
	}
}

func TestRoleChainDepthLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "base"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "mid", ParentID: "r_001"})
	store.CreateRole(ctx, &domain.Role{ID: "r_003", Name: "leaf", ParentID: "r_002"})

	chain, err := store.GetRoleChain(ctx, "r_003", 2)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	// Should return only 2 levels (leaf + mid), truncated at depth limit
	if len(chain) != 2 {
		t.Errorf("got %d roles, want 2 (depth limited)", len(chain))
	}
}

func TestDeleteRoleWithChildren(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "base"})
	store.CreateRole(ctx, &domain.Role{ID: "r_002", Name: "child", ParentID: "r_001"})

	err := store.DeleteRole(ctx, "r_001")
	if !errors.Is(err, domain.ErrHasDependencies) {
		t.Fatalf("expected ErrHasDependencies, got: %v", err)
	}
}
