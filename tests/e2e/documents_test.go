// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestRoleDocuments(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	role, err := c.CreateRole(ctx(), "docs-role", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Create
	doc, err := c.CreateRoleDocument(ctx(), role.ID, "Dev Guide", "# Dev Guide\nContent here.")
	if err != nil {
		t.Fatalf("CreateRoleDocument: %v", err)
	}
	if doc.Title != "Dev Guide" {
		t.Errorf("Title: got %q, want %q", doc.Title, "Dev Guide")
	}
	if doc.Kind != "role" {
		t.Errorf("Kind: got %q, want %q", doc.Kind, "role")
	}
	if doc.RoleID != role.ID {
		t.Errorf("RoleID: got %q, want %q", doc.RoleID, role.ID)
	}

	// List
	docs, err := c.ListRoleDocuments(ctx(), role.ID)
	if err != nil {
		t.Fatalf("ListRoleDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != doc.ID {
		t.Errorf("ListRoleDocuments: expected [%s], got %v", doc.ID, docs)
	}

	// Get
	got, err := c.GetDocument(ctx(), doc.ID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.Content != "# Dev Guide\nContent here." {
		t.Errorf("Content mismatch")
	}

	// Update
	updated, err := c.UpdateDocument(ctx(), doc.ID, "Dev Guide v2", "# Updated")
	if err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}
	if updated.Title != "Dev Guide v2" {
		t.Errorf("UpdateDocument Title: got %q, want %q", updated.Title, "Dev Guide v2")
	}

	// Delete
	if err := c.DeleteDocument(ctx(), doc.ID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	// Confirm gone
	_, err = c.GetDocument(ctx(), doc.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetDocument after delete: expected ErrNotFound, got %v", err)
	}
}

func TestAgentMemoryDocuments(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "mem-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000005", "mem-agent", team.ID, "", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Create memory document
	doc, err := c.CreateAgentMemory(ctx(), agent.ID, "Notes", "Some notes.")
	if err != nil {
		t.Fatalf("CreateAgentMemory: %v", err)
	}
	if doc.Kind != "memory" {
		t.Errorf("Kind: got %q, want %q", doc.Kind, "memory")
	}
	if doc.AgentID != agent.ID {
		t.Errorf("AgentID: got %q, want %q", doc.AgentID, agent.ID)
	}

	// List
	docs, err := c.ListAgentMemory(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("ListAgentMemory: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != doc.ID {
		t.Errorf("ListAgentMemory: expected [%s], got %v", doc.ID, docs)
	}

	// Delete
	if err := c.DeleteDocument(ctx(), doc.ID); err != nil {
		t.Fatalf("DeleteDocument (memory): %v", err)
	}
}
