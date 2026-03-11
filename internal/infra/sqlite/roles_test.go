// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/pressly/goose/v3"
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

// ---------------------------------------------------------------------------
// CTE query plan analysis
// ---------------------------------------------------------------------------

func TestRoleChainCTEQueryPlan(t *testing.T) {
	// Open a raw sql.DB so we can run EXPLAIN QUERY PLAN directly.
	db, err := sql.Open("sqlite", ":memory:?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	goose.SetBaseFS(nil) // reset any embedded FS from other tests
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}

	// Run the migration SQL inline so the test is self-contained.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS roles (
			id         TEXT PRIMARY KEY,
			name       TEXT UNIQUE NOT NULL,
			parent_id  TEXT REFERENCES roles(id),
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Seed a small hierarchy so the planner has rows to reason about.
	for i := 1; i <= 5; i++ {
		var parentID *string
		if i > 1 {
			pid := fmt.Sprintf("r_%03d", i-1)
			parentID = &pid
		}
		_, err := db.Exec(
			"INSERT INTO roles (id, name, parent_id) VALUES (?, ?, ?)",
			fmt.Sprintf("r_%03d", i), fmt.Sprintf("role_%d", i), parentID,
		)
		if err != nil {
			t.Fatalf("seed role %d: %v", i, err)
		}
	}

	// Run EXPLAIN QUERY PLAN on the recursive CTE.
	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		WITH RECURSIVE chain(id, name, parent_id, created_at, updated_at, depth) AS (
			SELECT id, name, parent_id, created_at, updated_at, 1
			FROM roles WHERE id = ?
			UNION ALL
			SELECT r.id, r.name, r.parent_id, r.created_at, r.updated_at, c.depth + 1
			FROM roles r
			JOIN chain c ON r.id = c.parent_id
			WHERE c.depth < ?
		)
		SELECT id, name, parent_id, created_at, updated_at FROM chain ORDER BY depth
	`, "r_005", 10)
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer rows.Close()

	var planLines []string
	usesPrimaryKey := false

	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan explain row: %v", err)
		}
		planLines = append(planLines, fmt.Sprintf("  id=%d parent=%d detail=%s", id, parent, detail))

		// Check for PRIMARY KEY usage on the recursive step's JOIN.
		// SQLite reports this as "SEARCH roles USING INDEX sqlite_autoindex_roles_1"
		// or "SEARCH TABLE roles USING PRIMARY KEY" depending on version.
		lower := strings.ToLower(detail)
		if strings.Contains(lower, "search") &&
			strings.Contains(lower, "roles") &&
			(strings.Contains(lower, "primary key") || strings.Contains(lower, "autoindex_roles_1")) {
			usesPrimaryKey = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	t.Logf("EXPLAIN QUERY PLAN output:\n%s", strings.Join(planLines, "\n"))

	if !usesPrimaryKey {
		t.Errorf("recursive CTE JOIN does not use PRIMARY KEY index on roles(id); "+
			"plan:\n%s", strings.Join(planLines, "\n"))
	}
}

// ---------------------------------------------------------------------------
// Edge case tests for GetRoleChain
// ---------------------------------------------------------------------------

func TestRoleChainSingleRole(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// A role with no parent.
	if err := store.CreateRole(ctx, &domain.Role{ID: "r_solo", Name: "solo"}); err != nil {
		t.Fatalf("create role: %v", err)
	}

	chain, err := store.GetRoleChain(ctx, "r_solo", 10)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("got %d roles, want exactly 1", len(chain))
	}
	if chain[0].ID != "r_solo" {
		t.Errorf("chain[0].ID = %q, want %q", chain[0].ID, "r_solo")
	}
	if chain[0].Name != "solo" {
		t.Errorf("chain[0].Name = %q, want %q", chain[0].Name, "solo")
	}
	if chain[0].ParentID != "" {
		t.Errorf("chain[0].ParentID = %q, want empty", chain[0].ParentID)
	}
}

func TestRoleChainNonExistentRole(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chain, err := store.GetRoleChain(ctx, "r_bogus_id", 10)
	if err != nil {
		t.Fatalf("expected no error for non-existent role, got: %v", err)
	}
	if len(chain) != 0 {
		t.Errorf("got %d roles, want empty slice for non-existent role", len(chain))
	}
}

func TestRoleChainDepthOne(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_root", Name: "root"})
	store.CreateRole(ctx, &domain.Role{ID: "r_mid", Name: "mid", ParentID: "r_root"})
	store.CreateRole(ctx, &domain.Role{ID: "r_leaf", Name: "leaf", ParentID: "r_mid"})

	// maxDepth=1 should return only the starting (leaf) role.
	chain, err := store.GetRoleChain(ctx, "r_leaf", 1)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("got %d roles with maxDepth=1, want exactly 1", len(chain))
	}
	if chain[0].ID != "r_leaf" {
		t.Errorf("chain[0].ID = %q, want %q", chain[0].ID, "r_leaf")
	}
}

func TestRoleChainDeepHierarchy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const depth = 10

	// Create a 10-level deep chain: role_01 (root) -> role_02 -> ... -> role_10 (leaf).
	for i := 1; i <= depth; i++ {
		r := &domain.Role{
			ID:   fmt.Sprintf("r_%02d", i),
			Name: fmt.Sprintf("level_%02d", i),
		}
		if i > 1 {
			r.ParentID = fmt.Sprintf("r_%02d", i-1)
		}
		if err := store.CreateRole(ctx, r); err != nil {
			t.Fatalf("create role level %d: %v", i, err)
		}
	}

	// Traverse from the deepest leaf upward.
	chain, err := store.GetRoleChain(ctx, fmt.Sprintf("r_%02d", depth), depth)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	if len(chain) != depth {
		t.Fatalf("got %d roles, want %d", len(chain), depth)
	}

	// Verify leaf-to-root ordering: chain[0] is the leaf, chain[9] is the root.
	for i, role := range chain {
		expectedLevel := depth - i
		expectedID := fmt.Sprintf("r_%02d", expectedLevel)
		expectedName := fmt.Sprintf("level_%02d", expectedLevel)
		if role.ID != expectedID {
			t.Errorf("chain[%d].ID = %q, want %q", i, role.ID, expectedID)
		}
		if role.Name != expectedName {
			t.Errorf("chain[%d].Name = %q, want %q", i, role.Name, expectedName)
		}
	}
}

func TestRoleChainParentIDIntegrity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Build a 4-level chain: root -> mid1 -> mid2 -> leaf
	store.CreateRole(ctx, &domain.Role{ID: "r_root", Name: "root"})
	store.CreateRole(ctx, &domain.Role{ID: "r_mid1", Name: "mid1", ParentID: "r_root"})
	store.CreateRole(ctx, &domain.Role{ID: "r_mid2", Name: "mid2", ParentID: "r_mid1"})
	store.CreateRole(ctx, &domain.Role{ID: "r_leaf", Name: "leaf", ParentID: "r_mid2"})

	chain, err := store.GetRoleChain(ctx, "r_leaf", 10)
	if err != nil {
		t.Fatalf("get role chain: %v", err)
	}
	if len(chain) != 4 {
		t.Fatalf("got %d roles, want 4", len(chain))
	}

	// Verify each role's ParentID points to the next role in the chain.
	// chain order is leaf-to-root, so chain[i].ParentID == chain[i+1].ID.
	for i := 0; i < len(chain)-1; i++ {
		if chain[i].ParentID != chain[i+1].ID {
			t.Errorf("chain[%d].ParentID = %q, want %q (chain[%d].ID)",
				i, chain[i].ParentID, chain[i+1].ID, i+1)
		}
	}

	// The root role must have an empty ParentID.
	last := chain[len(chain)-1]
	if last.ParentID != "" {
		t.Errorf("root role ParentID = %q, want empty", last.ParentID)
	}
}
