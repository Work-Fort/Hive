// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// ListRoleDocuments returns all documents attached to the given role.
func (c *Client) ListRoleDocuments(ctx context.Context, roleID string) ([]Document, error) {
	var out []Document
	return out, c.do(ctx, http.MethodGet, "/v1/roles/"+roleID+"/documents", nil, &out)
}

// CreateRoleDocument adds a new document to the given role.
func (c *Client) CreateRoleDocument(ctx context.Context, roleID, title, content string) (*Document, error) {
	body := map[string]string{"title": title, "content": content}
	var out Document
	return &out, c.do(ctx, http.MethodPost, "/v1/roles/"+roleID+"/documents", body, &out)
}

// GetDocument returns the document with the given ID.
func (c *Client) GetDocument(ctx context.Context, id string) (*Document, error) {
	var out Document
	return &out, c.do(ctx, http.MethodGet, "/v1/documents/"+id, nil, &out)
}

// UpdateDocument updates the title and content of the document with the given ID.
func (c *Client) UpdateDocument(ctx context.Context, id, title, content string) (*Document, error) {
	body := map[string]string{"title": title, "content": content}
	var out Document
	return &out, c.do(ctx, http.MethodPut, "/v1/documents/"+id, body, &out)
}

// DeleteDocument deletes the document with the given ID.
func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/documents/"+id, nil, nil)
}

// ListAgentMemory returns all memory documents attached to the given agent.
func (c *Client) ListAgentMemory(ctx context.Context, agentID string) ([]Document, error) {
	var out []Document
	return out, c.do(ctx, http.MethodGet, "/v1/agents/"+agentID+"/memory", nil, &out)
}

// CreateAgentMemory adds a new memory document to the given agent.
func (c *Client) CreateAgentMemory(ctx context.Context, agentID, title, content string) (*Document, error) {
	body := map[string]string{"title": title, "content": content}
	var out Document
	return &out, c.do(ctx, http.MethodPost, "/v1/agents/"+agentID+"/memory", body, &out)
}
