// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

// mockPermStore implements the subset of domain.Store needed by AuthzService.
type mockPermStore struct {
	domain.Store

	hasPermission map[string]bool
	agents        map[string]*domain.Agent
	err           error
}

func (m *mockPermStore) HasPermission(_ context.Context, agentID, permName, scopeTeamID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	key := agentID + ":" + permName + ":" + scopeTeamID
	return m.hasPermission[key], nil
}

func (m *mockPermStore) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	a, ok := m.agents[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return a, nil
}

func TestCheckPermission_GlobalGrant(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:": true,
		},
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:read", "")
	if err != nil {
		t.Fatalf("expected permission granted, got: %v", err)
	}
}

func TestCheckPermission_ScopedGrant(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:team-a": true,
		},
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:read", "team-a")
	if err != nil {
		t.Fatalf("expected scoped permission granted, got: %v", err)
	}
}

func TestCheckPermission_Denied(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{},
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:write", "")
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got: %v", err)
	}
}

func TestCheckPermission_StoreError(t *testing.T) {
	storeErr := errors.New("database is down")
	store := &mockPermStore{
		err: storeErr,
	}
	authz := NewAuthzService(store)

	err := authz.CheckPermission(context.Background(), "agent-1", "task:read", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatal("store error should not be ErrPermissionDenied")
	}
}

func TestRequireAny_AllPass(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:role:read:":   true,
			"agent-1:memory:read:": true,
		},
	}
	authz := NewAuthzService(store)

	err := authz.RequireAny(context.Background(), "agent-1",
		PermCheck{Name: "role:read"},
		PermCheck{Name: "memory:read"},
	)
	if err != nil {
		t.Fatalf("expected all permissions granted, got: %v", err)
	}
}

func TestRequireAny_OneFails(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:role:read:": true,
		},
	}
	authz := NewAuthzService(store)

	err := authz.RequireAny(context.Background(), "agent-1",
		PermCheck{Name: "role:read"},
		PermCheck{Name: "memory:read"},
	)
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got: %v", err)
	}
}

func TestCheckTeamPermission_Success(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:team-a": true,
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	teamID, err := authz.CheckTeamPermission(context.Background(), "agent-1", "task:read")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}
}

func TestCheckTeamPermission_Denied(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)

	_, err := authz.CheckTeamPermission(context.Background(), "agent-1", "task:write")
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got: %v", err)
	}
}

func TestCheckTeamPermission_AgentNotFound(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{},
		agents:        map[string]*domain.Agent{},
	}
	authz := NewAuthzService(store)

	_, err := authz.CheckTeamPermission(context.Background(), "no-such-agent", "task:read")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveAgentTeam(t *testing.T) {
	store := &mockPermStore{
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-b"},
		},
	}
	authz := NewAuthzService(store)

	teamID, err := authz.ResolveAgentTeam(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if teamID != "team-b" {
		t.Fatalf("expected team-b, got: %s", teamID)
	}
}
