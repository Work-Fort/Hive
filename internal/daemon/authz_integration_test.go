// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestMemoryToolEnforcement(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:memory:read:":  true,
			"agent-1:memory:write:": true,
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
			"agent-2": {ID: "agent-2", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)
	ctx := context.Background()

	if err := authz.CheckPermission(ctx, "agent-1", "memory:read", ""); err != nil {
		t.Errorf("agent-1 should have memory:read: %v", err)
	}
	if err := authz.CheckPermission(ctx, "agent-1", "memory:write", ""); err != nil {
		t.Errorf("agent-1 should have memory:write: %v", err)
	}
	err := authz.CheckPermission(ctx, "agent-2", "memory:read", "")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("agent-2 should be denied memory:read, got: %v", err)
	}
	err = authz.CheckPermission(ctx, "agent-2", "memory:write", "")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("agent-2 should be denied memory:write, got: %v", err)
	}
}

func TestTaskToolScopeEnforcement(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:team-a": true,
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)
	ctx := context.Background()

	teamID, err := authz.CheckTeamPermission(ctx, "agent-1", "task:read")
	if err != nil {
		t.Fatalf("agent-1 should have task:read for own team: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}

	err = authz.CheckPermission(ctx, "agent-1", "task:read", "team-b")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("agent-1 should be denied task:read for team-b, got: %v", err)
	}
}

func TestProvisioningCompoundPermission(t *testing.T) {
	tests := []struct {
		name    string
		perms   map[string]bool
		wantErr bool
	}{
		{
			name: "both permissions granted",
			perms: map[string]bool{
				"agent-1:role:read:":   true,
				"agent-1:memory:read:": true,
			},
			wantErr: false,
		},
		{
			name: "only role:read",
			perms: map[string]bool{
				"agent-1:role:read:": true,
			},
			wantErr: true,
		},
		{
			name: "only memory:read",
			perms: map[string]bool{
				"agent-1:memory:read:": true,
			},
			wantErr: true,
		},
		{
			name:    "neither permission",
			perms:   map[string]bool{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockPermStore{hasPermission: tt.perms}
			authz := NewAuthzService(store)

			err := authz.RequireAny(context.Background(), "agent-1",
				PermCheck{Name: "role:read"},
				PermCheck{Name: "memory:read"},
			)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected success, got: %v", err)
			}
			if tt.wantErr && err != nil && !errors.Is(err, domain.ErrPermissionDenied) {
				t.Errorf("expected ErrPermissionDenied, got: %v", err)
			}
		})
	}
}

func TestGlobalVsScopedPermission(t *testing.T) {
	store := &mockPermStore{
		hasPermission: map[string]bool{
			"agent-1:task:read:":       true,
			"agent-1:task:read:team-a": true,
			"agent-2:task:read:team-a": true,
		},
		agents: map[string]*domain.Agent{
			"agent-1": {ID: "agent-1", TeamID: "team-a"},
			"agent-2": {ID: "agent-2", TeamID: "team-a"},
		},
	}
	authz := NewAuthzService(store)
	ctx := context.Background()

	teamID, err := authz.CheckTeamPermission(ctx, "agent-1", "task:read")
	if err != nil {
		t.Fatalf("agent-1 global grant should work: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}

	teamID, err = authz.CheckTeamPermission(ctx, "agent-2", "task:read")
	if err != nil {
		t.Fatalf("agent-2 scoped grant should work for own team: %v", err)
	}
	if teamID != "team-a" {
		t.Fatalf("expected team-a, got: %s", teamID)
	}
}
