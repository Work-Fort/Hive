// SPDX-License-Identifier: GPL-3.0-or-later
package domain

import (
	"context"
	"io"
	"time"
)

// TeamStore persists team metadata.
type TeamStore interface {
	CreateTeam(ctx context.Context, t *Team) error
	GetTeam(ctx context.Context, id string) (*Team, error)
	ListTeams(ctx context.Context) ([]*Team, error)
	UpdateTeam(ctx context.Context, id, name string) error
	DeleteTeam(ctx context.Context, id string) error
	LookupTeamByName(ctx context.Context, name string) (*Team, error)
}

// RoleStore persists role metadata and handles inheritance queries.
type RoleStore interface {
	CreateRole(ctx context.Context, r *Role) error
	GetRole(ctx context.Context, id string) (*Role, error)
	ListRoles(ctx context.Context, parentID string) ([]*Role, error)
	UpdateRole(ctx context.Context, id, name, parentID string) error
	DeleteRole(ctx context.Context, id string) error

	// GetRoleChain returns the inheritance chain from the given role
	// to the root, up to maxDepth levels. The result is ordered
	// leaf-to-root (index 0 = the given role).
	GetRoleChain(ctx context.Context, roleID string, maxDepth int) ([]*Role, error)
	LookupRoleByName(ctx context.Context, name string) (*Role, error)
}

// AgentStore persists agent metadata and role assignments.
type AgentStore interface {
	CreateAgent(ctx context.Context, a *Agent) error
	GetAgent(ctx context.Context, id string) (*Agent, error)
	ListAgents(ctx context.Context, teamID string) ([]*Agent, error)
	UpdateAgent(ctx context.Context, a *Agent) error
	DeleteAgent(ctx context.Context, id string) error

	SetAgentRoles(ctx context.Context, agentID string, roles []AgentRole) error
	GetAgentRoles(ctx context.Context, agentID string) ([]AgentRole, error)
	LookupAgentByName(ctx context.Context, name string) (*Agent, error)

	// ClaimAgent atomically picks one free agent and sets its current
	// assignment. Returns the chosen agent, or ErrPoolExhausted when no
	// free agent is available.
	ClaimAgent(ctx context.Context, role, project, workflowID string, leaseExpiresAt time.Time) (*Agent, error)

	// ReleaseAgent clears the current assignment if current_workflow_id
	// matches workflowID. Returns ErrWorkflowMismatch on mismatch,
	// ErrNotFound if the agent doesn't exist.
	ReleaseAgent(ctx context.Context, agentID, workflowID string) error

	// RenewAgentLease extends lease_expires_at if current_workflow_id
	// matches workflowID. Returns ErrWorkflowMismatch on mismatch.
	RenewAgentLease(ctx context.Context, agentID, workflowID string, leaseExpiresAt time.Time) error

	// ListAgentsByAssignment lists agents with optional pool filters.
	ListAgentsByAssignment(ctx context.Context, filter AgentAssignmentFilter) ([]*Agent, error)
}

// DocumentStore persists markdown documents for roles and agents.
type DocumentStore interface {
	CreateDocument(ctx context.Context, d *Document) error
	GetDocument(ctx context.Context, id string) (*Document, error)
	UpdateDocument(ctx context.Context, id, title, content string) error
	DeleteDocument(ctx context.Context, id string) error

	ListRoleDocuments(ctx context.Context, roleID string) ([]*Document, error)
	ListAgentMemory(ctx context.Context, agentID string) ([]*Document, error)
	LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*Document, error)
}

// TaskStore persists tasks scoped to teams.
type TaskStore interface {
	CreateTask(ctx context.Context, t *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	ListTeamTasks(ctx context.Context, teamID string) ([]*Task, error)
	UpdateTask(ctx context.Context, id string, t *Task) error
	DeleteTask(ctx context.Context, id string) error
	LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*Task, error)
}

// PermissionStore manages RBAC permissions.
type PermissionStore interface {
	// SeedPermissions ensures all named permissions exist in the database.
	SeedPermissions(ctx context.Context, names []string) error

	GetAgentPermissions(ctx context.Context, agentID string) ([]AgentPermission, error)
	SetAgentPermissions(ctx context.Context, agentID string, perms []AgentPermission) error

	// HasPermission checks if an agent has a specific permission,
	// either globally or scoped to the given team.
	HasPermission(ctx context.Context, agentID, permName, scopeTeamID string) (bool, error)

	ListPermissions(ctx context.Context) ([]*Permission, error)
	LookupPermissionByName(ctx context.Context, name string) (*Permission, error)
}

// Store combines all storage interfaces.
type Store interface {
	TeamStore
	RoleStore
	AgentStore
	DocumentStore
	TaskStore
	PermissionStore
	// Ping verifies that the underlying storage is reachable.
	Ping(ctx context.Context) error
	io.Closer
}
