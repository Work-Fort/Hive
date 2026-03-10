// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateRoleDocument(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})

	doc := &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindRole,
		Title: "Dev Guide", Content: "# Developer Guide\n...",
		RoleID: "r_001",
	}
	if err := store.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	got, err := store.GetDocument(ctx, "d_001")
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if got.Title != "Dev Guide" || got.RoleID != "r_001" {
		t.Errorf("got %+v", got)
	}
}

func TestCreateMemoryDocument(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	doc := &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindMemory,
		Title: "Patterns", Content: "# Learned Patterns\n...",
		AgentID: "a_001",
	}
	if err := store.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("create document: %v", err)
	}

	docs, err := store.ListAgentMemory(ctx, "a_001")
	if err != nil {
		t.Fatalf("list agent memory: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "Patterns" {
		t.Errorf("got %+v", docs)
	}
}

func TestListRoleDocuments(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateRole(ctx, &domain.Role{ID: "r_001", Name: "developer"})
	store.CreateDocument(ctx, &domain.Document{
		ID: "d_001", Kind: domain.DocumentKindRole,
		Title: "Guide A", RoleID: "r_001",
	})
	store.CreateDocument(ctx, &domain.Document{
		ID: "d_002", Kind: domain.DocumentKindRole,
		Title: "Guide B", RoleID: "r_001",
	})

	docs, err := store.ListRoleDocuments(ctx, "r_001")
	if err != nil {
		t.Fatalf("list role documents: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("got %d documents, want 2", len(docs))
	}
}
