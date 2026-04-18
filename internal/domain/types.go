// SPDX-License-Identifier: GPL-3.0-or-later

// Package domain defines the core types and port interfaces for Hive.
// This package has zero dependencies on infrastructure — it defines
// what the system does, not how.
package domain

import "time"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
)

// DocumentKind identifies whether a document belongs to a role or an agent.
type DocumentKind string

const (
	DocumentKindRole   DocumentKind = "role"
	DocumentKindMemory DocumentKind = "memory"
)

// Team is an organizational unit. Agents belong to exactly one team.
type Team struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Role is a reusable capability definition with optional single-parent
// inheritance. Roles are composable — an agent can have multiple roles.
type Role struct {
	ID        string
	Name      string
	ParentID  string // empty if no parent
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Agent is a provisioned identity that belongs to one team and can be
// assigned multiple roles with priority ordering.
//
// Model and Runtime describe how the agent is executed. Both are free-form
// strings — Hive does not validate them against a catalog, since LLM model
// IDs and adjutant runtime variants change independently. Runtime is
// descriptive (tells an operator which adjutant image is deployed) rather
// than prescriptive (Hive does not spawn VMs).
type Agent struct {
	ID      string
	Name    string
	TeamID  string
	Model   string // e.g. "claude-sonnet-4-6", "claude-opus-4-7"
	Runtime string // e.g. "claude-cli", "go-adk"

	// Current assignment — all-or-nothing. When the agent is free, all four
	// are zero values. When claimed by a workflow, all four are set.
	// See docs/2026-04-18-agent-assignment-schema.md for the invariant.
	CurrentRole       string
	CurrentProject    string
	CurrentWorkflowID string
	LeaseExpiresAt    time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentRole links an agent to a role with a priority. Lower priority
// number = higher precedence in document resolution.
type AgentRole struct {
	AgentID  string
	RoleID   string
	Priority int
}

// Document holds markdown content attached to either a role or an agent.
// Exactly one of RoleID or AgentID must be set.
type Document struct {
	ID        string
	Kind      DocumentKind
	Title     string
	Content   string
	RoleID    string // set when Kind == DocumentKindRole
	AgentID   string // set when Kind == DocumentKindMemory
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Task is a work item belonging to a team, optionally assigned to an agent.
type Task struct {
	ID          string
	TeamID      string
	AgentID     string // empty if unassigned
	Title       string
	Description string
	Status      TaskStatus
	// FlowTaskRef is a free-form string referencing the originating Flow
	// workflow task. Opaque to Hive — Hive does not parse or validate it.
	FlowTaskRef string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Permission is a named capability that can be granted to agents.
type Permission struct {
	ID   string
	Name string
}

// AgentPermission grants a permission to an agent, optionally scoped to a
// specific team. If ScopeTeamID is empty, the permission is global.
type AgentPermission struct {
	AgentID      string
	PermissionID string
	ScopeTeamID  string // empty = global scope
}

// CurrentAssignment is the runtime assignment context surfaced to an
// agent by get_provisioning. Zero value means the agent is free.
type CurrentAssignment struct {
	Role           string    `json:"role"`
	Project        string    `json:"project"`
	WorkflowID     string    `json:"workflow_id"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

// ProvisioningResponse is the hierarchical response returned when an agent
// requests its provisioning data.
type ProvisioningResponse struct {
	Agent             AgentIdentity           `json:"agent"`
	CurrentAssignment *CurrentAssignment      `json:"current_assignment,omitempty"`
	Roles             []ProvisioningRoleGroup `json:"roles"`
	Memory            []Document              `json:"memory"`
}

// AgentIdentity is the subset of agent fields returned in a provisioning
// response so an agent can learn its own identity from a single call.
type AgentIdentity struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	TeamID  string `json:"team_id"`
	Model   string `json:"model,omitempty"`
	Runtime string `json:"runtime,omitempty"`
}

// ProvisioningRoleGroup is a single role assignment with its full
// inheritance chain of documents.
type ProvisioningRoleGroup struct {
	Priority int                      `json:"priority"`
	Chain    []ProvisioningChainEntry `json:"chain"`
}

// ProvisioningChainEntry is one link in the role inheritance chain,
// ordered from leaf (index 0) to root (last element).
type ProvisioningChainEntry struct {
	Role      string     `json:"role"`
	Documents []Document `json:"documents"`
}

// AgentAssignmentFilter narrows ListAgentsByAssignment. Zero values mean
// "no filter" except Assigned which uses *bool to distinguish
// "unspecified" from "false".
type AgentAssignmentFilter struct {
	Assigned   *bool  // nil = all; true = claimed only; false = free only
	WorkflowID string // empty = no filter
	Role       string // empty = no filter
	Project    string // empty = no filter
	TeamID     string // empty = no filter (matches existing ListAgents)
}
