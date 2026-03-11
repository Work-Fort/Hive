// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
	"net/url"
)

// ListRoles returns all roles, optionally filtered by parentID. Pass an empty
// string to list all roles regardless of parent.
func (c *Client) ListRoles(ctx context.Context, parentID string) ([]Role, error) {
	params := url.Values{}
	if parentID != "" {
		params.Set("parent_id", parentID)
	}
	var out []Role
	return out, c.doWithQuery(ctx, http.MethodGet, "/v1/roles", params, &out)
}

// CreateRole creates a new role. parentID may be empty for a root role.
func (c *Client) CreateRole(ctx context.Context, name, parentID string) (*Role, error) {
	body := map[string]string{"name": name, "parent_id": parentID}
	var out Role
	return &out, c.do(ctx, http.MethodPost, "/v1/roles", body, &out)
}

// GetRole returns the role with the given ID.
func (c *Client) GetRole(ctx context.Context, id string) (*Role, error) {
	var out Role
	return &out, c.do(ctx, http.MethodGet, "/v1/roles/"+id, nil, &out)
}

// UpdateRole updates the name and parent of the role with the given ID.
// Pass an empty parentID to clear the parent (make it a root role).
func (c *Client) UpdateRole(ctx context.Context, id, name, parentID string) (*Role, error) {
	body := map[string]string{"name": name, "parent_id": parentID}
	var out Role
	return &out, c.do(ctx, http.MethodPut, "/v1/roles/"+id, body, &out)
}

// DeleteRole deletes the role with the given ID. Returns ErrConflict if the
// role has child roles or active agent assignments.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/roles/"+id, nil, nil)
}
