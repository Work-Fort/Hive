// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestRoles(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Create root role
	root, err := c.CreateRole(ctx(), "developer", "")
	if err != nil {
		t.Fatalf("CreateRole root: %v", err)
	}
	if root.Name != "developer" {
		t.Errorf("Name: got %q, want %q", root.Name, "developer")
	}
	if root.ParentID != "" {
		t.Errorf("ParentID: expected empty, got %q", root.ParentID)
	}

	// Create child role
	child, err := c.CreateRole(ctx(), "frontend-developer", root.ID)
	if err != nil {
		t.Fatalf("CreateRole child: %v", err)
	}
	if child.ParentID != root.ID {
		t.Errorf("ParentID: got %q, want %q", child.ParentID, root.ID)
	}

	// Get
	got, err := c.GetRole(ctx(), root.ID)
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got.ID != root.ID {
		t.Errorf("GetRole ID: got %q, want %q", got.ID, root.ID)
	}

	// List (all)
	roles, err := c.ListRoles(ctx(), "")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if !containsRole(roles, root.ID) || !containsRole(roles, child.ID) {
		t.Errorf("ListRoles missing created roles")
	}

	// List filtered by parent
	children, err := c.ListRoles(ctx(), root.ID)
	if err != nil {
		t.Fatalf("ListRoles by parent: %v", err)
	}
	if len(children) != 1 || children[0].ID != child.ID {
		t.Errorf("ListRoles(parent=%s): expected [%s], got %v", root.ID, child.ID, children)
	}

	// Update
	updated, err := c.UpdateRole(ctx(), root.ID, "engineer", "")
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updated.Name != "engineer" {
		t.Errorf("UpdateRole Name: got %q, want %q", updated.Name, "engineer")
	}

	// Delete child first (parent still has a child, can't delete parent yet)
	if err := c.DeleteRole(ctx(), child.ID); err != nil {
		t.Fatalf("DeleteRole child: %v", err)
	}

	// Delete parent
	if err := c.DeleteRole(ctx(), root.ID); err != nil {
		t.Fatalf("DeleteRole root: %v", err)
	}

	// Confirm gone
	_, err = c.GetRole(ctx(), root.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetRole after delete: expected ErrNotFound, got %v", err)
	}
}

// TestRoleDeleteWithChildren verifies that a role with child roles cannot be
// deleted.
func TestRoleDeleteWithChildren(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	parent, err := c.CreateRole(ctx(), "base", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if _, err := c.CreateRole(ctx(), "derived", parent.ID); err != nil {
		t.Fatalf("CreateRole child: %v", err)
	}

	err = c.DeleteRole(ctx(), parent.ID)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("delete role with children: expected ErrConflict, got %v", err)
	}
}

func containsRole(roles []client.Role, id string) bool {
	for _, r := range roles {
		if r.ID == id {
			return true
		}
	}
	return false
}
