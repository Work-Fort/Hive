// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"

	"github.com/Work-Fort/Hive/internal/domain"
)

// dbDataSource wraps a domain.Store for direct database access.
type dbDataSource struct {
	store domain.Store
}

// NewDBDataSource creates a DataSource backed by a domain.Store.
func NewDBDataSource(store domain.Store) DataSource {
	return &dbDataSource{store: store}
}

func (d *dbDataSource) ListTeams(ctx context.Context) ([]*domain.Team, error) {
	return d.store.ListTeams(ctx)
}

func (d *dbDataSource) CreateTeam(ctx context.Context, t *domain.Team) error {
	return d.store.CreateTeam(ctx, t)
}

func (d *dbDataSource) UpdateTeam(ctx context.Context, id, name string) error {
	return d.store.UpdateTeam(ctx, id, name)
}

func (d *dbDataSource) LookupTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	return d.store.LookupTeamByName(ctx, name)
}

func (d *dbDataSource) ListAllRoles(ctx context.Context) ([]*domain.Role, error) {
	return d.store.ListRoles(ctx, "")
}

func (d *dbDataSource) CreateRole(ctx context.Context, r *domain.Role) error {
	return d.store.CreateRole(ctx, r)
}

func (d *dbDataSource) UpdateRole(ctx context.Context, id, name, parentID string) error {
	return d.store.UpdateRole(ctx, id, name, parentID)
}

func (d *dbDataSource) LookupRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	return d.store.LookupRoleByName(ctx, name)
}

func (d *dbDataSource) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	return d.store.ListPermissions(ctx)
}

func (d *dbDataSource) EnsurePermission(ctx context.Context, name string) error {
	return d.store.SeedPermissions(ctx, []string{name})
}

func (d *dbDataSource) LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	return d.store.LookupPermissionByName(ctx, name)
}

func (d *dbDataSource) ListAllAgents(ctx context.Context) ([]*domain.Agent, error) {
	return d.store.ListAgents(ctx, "")
}

func (d *dbDataSource) CreateAgent(ctx context.Context, a *domain.Agent) error {
	return d.store.CreateAgent(ctx, a)
}

func (d *dbDataSource) UpdateAgent(ctx context.Context, a *domain.Agent) error {
	return d.store.UpdateAgent(ctx, a)
}

func (d *dbDataSource) LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error) {
	return d.store.LookupAgentByName(ctx, name)
}

func (d *dbDataSource) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	return d.store.GetAgentRoles(ctx, agentID)
}

func (d *dbDataSource) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	return d.store.SetAgentRoles(ctx, agentID, roles)
}

func (d *dbDataSource) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	return d.store.GetAgentPermissions(ctx, agentID)
}

func (d *dbDataSource) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	return d.store.SetAgentPermissions(ctx, agentID, perms)
}

func (d *dbDataSource) ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error) {
	return d.store.ListRoleDocuments(ctx, roleID)
}

func (d *dbDataSource) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	return d.store.ListAgentMemory(ctx, agentID)
}

func (d *dbDataSource) CreateDocument(ctx context.Context, doc *domain.Document) error {
	return d.store.CreateDocument(ctx, doc)
}

func (d *dbDataSource) UpdateDocument(ctx context.Context, id, title, content string) error {
	return d.store.UpdateDocument(ctx, id, title, content)
}

func (d *dbDataSource) LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error) {
	return d.store.LookupDocumentByOwnerAndTitle(ctx, ownerID, title)
}

func (d *dbDataSource) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	return d.store.ListTeamTasks(ctx, teamID)
}

func (d *dbDataSource) CreateTask(ctx context.Context, t *domain.Task) error {
	return d.store.CreateTask(ctx, t)
}

func (d *dbDataSource) UpdateTask(ctx context.Context, id string, t *domain.Task) error {
	return d.store.UpdateTask(ctx, id, t)
}

func (d *dbDataSource) LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error) {
	return d.store.LookupTaskByTeamAndTitle(ctx, teamID, title)
}
