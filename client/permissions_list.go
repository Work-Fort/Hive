// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// Permission represents a named capability.
type Permission struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
}

// ListPermissions returns all registered permissions.
func (c *Client) ListPermissions(ctx context.Context) ([]Permission, error) {
	var out []Permission
	return out, c.do(ctx, http.MethodGet, "/v1/permissions", nil, &out)
}

// CreatePermission creates a permission by name (idempotent).
func (c *Client) CreatePermission(ctx context.Context, name string) (*Permission, error) {
	body := map[string]string{"name": name}
	var out Permission
	return &out, c.do(ctx, http.MethodPost, "/v1/permissions", body, &out)
}
