// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
	"net/url"
)

// RoleAssignment is the input type for SetAgentRoles.
type RoleAssignment struct {
	RoleID   string `json:"role_id"`
	Priority int    `json:"priority"`
}

// ListAgents returns all agents, optionally filtered by teamID. Pass an empty
// string to list all agents.
func (c *Client) ListAgents(ctx context.Context, teamID string) ([]Agent, error) {
	params := url.Values{}
	if teamID != "" {
		params.Set("team_id", teamID)
	}
	var out []Agent
	return out, c.doWithQuery(ctx, http.MethodGet, "/v1/agents", params, &out)
}

// CreateAgent creates a new agent with the given Passport UUID, name, and team.
func (c *Client) CreateAgent(ctx context.Context, id, name, teamID string) (*Agent, error) {
	body := map[string]string{"id": id, "name": name, "team_id": teamID}
	var out Agent
	return &out, c.do(ctx, http.MethodPost, "/v1/agents", body, &out)
}

// GetAgent returns the agent with the given ID, including its role assignments.
func (c *Client) GetAgent(ctx context.Context, id string) (*AgentWithRoles, error) {
	var out AgentWithRoles
	return &out, c.do(ctx, http.MethodGet, "/v1/agents/"+id, nil, &out)
}

// UpdateAgent updates the name and team of the agent with the given ID.
func (c *Client) UpdateAgent(ctx context.Context, id, name, teamID string) (*Agent, error) {
	body := map[string]string{"name": name, "team_id": teamID}
	var out Agent
	return &out, c.do(ctx, http.MethodPut, "/v1/agents/"+id, body, &out)
}

// DeleteAgent deletes the agent with the given ID. Returns ErrConflict if the
// agent has tasks assigned.
func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/agents/"+id, nil, nil)
}

// SetAgentRoles replaces the role assignments for the given agent. The full
// set of assignments is replaced atomically. Pass an empty slice to clear all
// roles. Returns the updated list of role assignments.
func (c *Client) SetAgentRoles(ctx context.Context, agentID string, roles []RoleAssignment) ([]AgentRole, error) {
	body := map[string]any{"roles": roles}
	var out []AgentRole
	return out, c.do(ctx, http.MethodPut, "/v1/agents/"+agentID+"/roles", body, &out)
}
