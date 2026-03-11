// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"

	"github.com/Work-Fort/Hive/internal/domain"
)

// DataSource abstracts read/write access to Hive entities, allowing
// export/import logic to work against either a REST API or a direct database.
type DataSource interface {
	// Teams
	ListTeams(ctx context.Context) ([]*domain.Team, error)
	CreateTeam(ctx context.Context, t *domain.Team) error
	UpdateTeam(ctx context.Context, id, name string) error
	LookupTeamByName(ctx context.Context, name string) (*domain.Team, error)

	// Roles
	ListAllRoles(ctx context.Context) ([]*domain.Role, error)
	CreateRole(ctx context.Context, r *domain.Role) error
	UpdateRole(ctx context.Context, id, name, parentID string) error
	LookupRoleByName(ctx context.Context, name string) (*domain.Role, error)

	// Permissions
	ListPermissions(ctx context.Context) ([]*domain.Permission, error)
	EnsurePermission(ctx context.Context, name string) error
	LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error)

	// Agents
	ListAllAgents(ctx context.Context) ([]*domain.Agent, error)
	CreateAgent(ctx context.Context, a *domain.Agent) error
	UpdateAgent(ctx context.Context, id, name, teamID string) error
	LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error)
	GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error)
	SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error
	GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error)
	SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error

	// Documents
	ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error)
	ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error)
	CreateDocument(ctx context.Context, d *domain.Document) error
	UpdateDocument(ctx context.Context, id, title, content string) error
	LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error)

	// Tasks
	ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error)
	CreateTask(ctx context.Context, t *domain.Task) error
	UpdateTask(ctx context.Context, id string, t *domain.Task) error
	LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error)
}
