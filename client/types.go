// SPDX-License-Identifier: GPL-3.0-or-later
package client

import "time"

// Team is an organisational unit returned by the Teams endpoints.
type Team struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// Role is a capability definition with optional single-parent inheritance.
type Role struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	ParentID  string    `json:"ParentID"` // empty string when no parent
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// Agent is a provisioned identity belonging to one team.
type Agent struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	TeamID    string    `json:"TeamID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// AgentRole links an agent to a role with a priority ordering.
type AgentRole struct {
	AgentID  string `json:"AgentID"`
	RoleID   string `json:"RoleID"`
	Priority int    `json:"Priority"`
}

// AgentWithRoles is the response from GET /v1/agents/:id — the agent record
// plus its current role assignments.
type AgentWithRoles struct {
	Agent
	Roles []AgentRole `json:"roles"`
}

// Document holds markdown content attached to a role or an agent.
type Document struct {
	ID        string    `json:"ID"`
	Kind      string    `json:"Kind"`    // "role" or "memory"
	Title     string    `json:"Title"`
	Content   string    `json:"Content"`
	RoleID    string    `json:"RoleID"`  // set when Kind == "role"
	AgentID   string    `json:"AgentID"` // set when Kind == "memory"
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// Task is a work item belonging to a team, optionally assigned to an agent.
type Task struct {
	ID          string    `json:"ID"`
	TeamID      string    `json:"TeamID"`
	AgentID     string    `json:"AgentID"` // empty if unassigned
	Title       string    `json:"Title"`
	Description string    `json:"Description"`
	Status      string    `json:"Status"` // "pending", "in_progress", "completed"
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// AgentPermission grants a named permission to an agent, optionally scoped to
// a team. ScopeTeamID is empty for global grants.
type AgentPermission struct {
	AgentID      string `json:"AgentID"`
	PermissionID string `json:"PermissionID"`
	ScopeTeamID  string `json:"ScopeTeamID"`
}

// HealthCheckResult is a single named health check result.
type HealthCheckResult struct {
	Name     string `json:"name"`
	Severity string `json:"severity"` // "ok", "warning", "error"
	Message  string `json:"message,omitempty"`
}

// HealthReport is returned by GET /v1/health.
type HealthReport struct {
	Status string              `json:"status"` // "healthy", "degraded", "unhealthy"
	Checks []HealthCheckResult `json:"checks"`
}
