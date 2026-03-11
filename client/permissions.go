// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// PermissionGrant is the input type for a single permission entry in
// SetAgentPermissions.
type PermissionGrant struct {
	Permission  string `json:"permission"`
	ScopeTeamID string `json:"scope_team_id,omitempty"`
}

// GetAgentPermissions returns the permission grants for the given agent.
func (c *Client) GetAgentPermissions(ctx context.Context, agentID string) ([]AgentPermission, error) {
	var out []AgentPermission
	return out, c.do(ctx, http.MethodGet, "/v1/agents/"+agentID+"/permissions", nil, &out)
}

// SetAgentPermissions replaces the permission grants for the given agent.
// The full set is replaced atomically. Pass an empty slice to revoke all
// permissions. Returns the updated grant list.
func (c *Client) SetAgentPermissions(ctx context.Context, agentID string, grants []PermissionGrant) ([]AgentPermission, error) {
	body := map[string]any{"permissions": grants}
	var out []AgentPermission
	return out, c.do(ctx, http.MethodPut, "/v1/agents/"+agentID+"/permissions", body, &out)
}
