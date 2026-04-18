// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// CreateTaskInput holds the fields for creating a task.
type CreateTaskInput struct {
	TeamID      string `json:"team_id"`
	AgentID     string `json:"agent_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"` // defaults to "pending" if empty
	FlowTaskRef string `json:"flow_task_ref,omitempty"`
}

// UpdateTaskInput holds the fields for updating a task. Zero-value string
// fields are ignored by the server (partial update semantics).
type UpdateTaskInput struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	AgentID     string `json:"agent_id"` // always sent; empty string clears assignment
	FlowTaskRef string `json:"flow_task_ref,omitempty"`
}

// ListTeamTasks returns all tasks belonging to the given team.
func (c *Client) ListTeamTasks(ctx context.Context, teamID string) ([]Task, error) {
	var out []Task
	return out, c.do(ctx, http.MethodGet, "/v1/teams/"+teamID+"/tasks", nil, &out)
}

// CreateTask creates a new task.
func (c *Client) CreateTask(ctx context.Context, input CreateTaskInput) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPost, "/v1/tasks", input, &out)
}

// GetTask returns the task with the given ID.
func (c *Client) GetTask(ctx context.Context, id string) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodGet, "/v1/tasks/"+id, nil, &out)
}

// UpdateTask updates the task with the given ID.
func (c *Client) UpdateTask(ctx context.Context, id string, input UpdateTaskInput) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPut, "/v1/tasks/"+id, input, &out)
}

// DeleteTask deletes the task with the given ID.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tasks/"+id, nil, nil)
}
