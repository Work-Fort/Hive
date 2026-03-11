// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

// PermCheck describes a single permission requirement for authorization.
type PermCheck struct {
	// Name is the permission name (e.g., "task:read", "memory:write").
	Name string
	// ScopeTeamID limits the permission to a specific team.
	// Empty string means the check accepts either a global grant or a
	// grant scoped to the agent's own team.
	ScopeTeamID string
}

// AuthzService provides high-level authorization methods on top of the
// permission store. It is the single point of enforcement for MCP tool
// handlers — every tool calls into this service before executing.
type AuthzService struct {
	store domain.Store
}

// NewAuthzService creates an AuthzService backed by the given store.
func NewAuthzService(store domain.Store) *AuthzService {
	return &AuthzService{store: store}
}

// CheckPermission verifies that agentID holds permName, either globally
// or scoped to scopeTeamID. Returns nil on success, domain.ErrPermissionDenied
// on failure. Store errors are propagated unwrapped.
func (a *AuthzService) CheckPermission(ctx context.Context, agentID, permName, scopeTeamID string) error {
	ok, err := a.store.HasPermission(ctx, agentID, permName, scopeTeamID)
	if err != nil {
		return fmt.Errorf("check permission %s for agent %s: %w", permName, agentID, err)
	}
	if !ok {
		return fmt.Errorf("%w: agent %s lacks %s", domain.ErrPermissionDenied, agentID, permName)
	}
	return nil
}

// RequireAny checks that the agent holds ALL of the listed permissions.
// The name is "RequireAny" in the sense that each PermCheck is evaluated
// independently (a global grant OR a scoped grant satisfies it), but ALL
// checks in the slice must pass. This handles compound requirements like
// get_provisioning needing both role:read AND memory:read.
//
// Returns nil if all checks pass. Returns domain.ErrPermissionDenied
// wrapping the first failing check on denial.
func (a *AuthzService) RequireAny(ctx context.Context, agentID string, perms ...PermCheck) error {
	for _, p := range perms {
		if err := a.CheckPermission(ctx, agentID, p.Name, p.ScopeTeamID); err != nil {
			return err
		}
	}
	return nil
}

// ResolveAgentTeam looks up the agent and returns their team ID.
// This is used by tool handlers that need to scope permission checks
// to the agent's own team (e.g., task tools).
func (a *AuthzService) ResolveAgentTeam(ctx context.Context, agentID string) (string, error) {
	agent, err := a.store.GetAgent(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("resolve agent team: %w", err)
	}
	return agent.TeamID, nil
}

// CheckTeamPermission is a convenience that resolves the agent's team and
// then checks if the agent holds the named permission scoped to that team
// (or globally). This is the common pattern for task tools.
func (a *AuthzService) CheckTeamPermission(ctx context.Context, agentID, permName string) (string, error) {
	teamID, err := a.ResolveAgentTeam(ctx, agentID)
	if err != nil {
		return "", err
	}
	if err := a.CheckPermission(ctx, agentID, permName, teamID); err != nil {
		return "", err
	}
	return teamID, nil
}
