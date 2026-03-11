// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// ListTeams returns all teams.
func (c *Client) ListTeams(ctx context.Context) ([]Team, error) {
	var out []Team
	return out, c.do(ctx, http.MethodGet, "/v1/teams", nil, &out)
}

// CreateTeam creates a new team with the given name.
func (c *Client) CreateTeam(ctx context.Context, name string) (*Team, error) {
	body := map[string]string{"name": name}
	var out Team
	return &out, c.do(ctx, http.MethodPost, "/v1/teams", body, &out)
}

// GetTeam returns the team with the given ID.
func (c *Client) GetTeam(ctx context.Context, id string) (*Team, error) {
	var out Team
	return &out, c.do(ctx, http.MethodGet, "/v1/teams/"+id, nil, &out)
}

// UpdateTeam renames the team with the given ID.
func (c *Client) UpdateTeam(ctx context.Context, id, name string) (*Team, error) {
	body := map[string]string{"name": name}
	var out Team
	return &out, c.do(ctx, http.MethodPut, "/v1/teams/"+id, body, &out)
}

// DeleteTeam deletes the team with the given ID. Returns ErrConflict if the
// team has dependent agents or tasks.
func (c *Client) DeleteTeam(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/teams/"+id, nil, nil)
}
