// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/client"
	"github.com/Work-Fort/Hive/internal/domain"
)

// restDataSource wraps a client.Client for REST API access.
type restDataSource struct {
	c *client.Client
}

// NewRESTDataSource creates a DataSource backed by the Hive REST API.
func NewRESTDataSource(c *client.Client) DataSource {
	return &restDataSource{c: c}
}

func (r *restDataSource) ListTeams(ctx context.Context) ([]*domain.Team, error) {
	teams, err := r.c.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Team, len(teams))
	for i, t := range teams {
		out[i] = &domain.Team{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
	}
	return out, nil
}

func (r *restDataSource) CreateTeam(ctx context.Context, t *domain.Team) error {
	_, err := r.c.CreateTeam(ctx, t.Name)
	return err
}

func (r *restDataSource) UpdateTeam(ctx context.Context, id, name string) error {
	_, err := r.c.UpdateTeam(ctx, id, name)
	return err
}

func (r *restDataSource) LookupTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	teams, err := r.c.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range teams {
		if t.Name == name {
			return &domain.Team{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}, nil
		}
	}
	return nil, fmt.Errorf("%w: team named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) ListAllRoles(ctx context.Context) ([]*domain.Role, error) {
	roles, err := r.c.ListRoles(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Role, len(roles))
	for i, rl := range roles {
		out[i] = &domain.Role{ID: rl.ID, Name: rl.Name, ParentID: rl.ParentID, CreatedAt: rl.CreatedAt, UpdatedAt: rl.UpdatedAt}
	}
	return out, nil
}

func (r *restDataSource) CreateRole(ctx context.Context, rl *domain.Role) error {
	_, err := r.c.CreateRole(ctx, rl.Name, rl.ParentID)
	return err
}

func (r *restDataSource) UpdateRole(ctx context.Context, id, name, parentID string) error {
	_, err := r.c.UpdateRole(ctx, id, name, parentID)
	return err
}

func (r *restDataSource) LookupRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	roles, err := r.c.ListRoles(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, rl := range roles {
		if rl.Name == name {
			return &domain.Role{ID: rl.ID, Name: rl.Name, ParentID: rl.ParentID, CreatedAt: rl.CreatedAt, UpdatedAt: rl.UpdatedAt}, nil
		}
	}
	return nil, fmt.Errorf("%w: role named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	perms, err := r.c.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Permission, len(perms))
	for i, p := range perms {
		out[i] = &domain.Permission{ID: p.ID, Name: p.Name}
	}
	return out, nil
}

func (r *restDataSource) EnsurePermission(ctx context.Context, name string) error {
	_, err := r.c.CreatePermission(ctx, name)
	return err
}

func (r *restDataSource) LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	perms, err := r.c.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if p.Name == name {
			return &domain.Permission{ID: p.ID, Name: p.Name}, nil
		}
	}
	return nil, fmt.Errorf("%w: permission named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) ListAllAgents(ctx context.Context) ([]*domain.Agent, error) {
	agents, err := r.c.ListAgents(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Agent, len(agents))
	for i, a := range agents {
		out[i] = &domain.Agent{ID: a.ID, Name: a.Name, TeamID: a.TeamID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
	}
	return out, nil
}

func (r *restDataSource) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := r.c.CreateAgent(ctx, a.ID, a.Name, a.TeamID)
	return err
}

func (r *restDataSource) UpdateAgent(ctx context.Context, id, name, teamID string) error {
	_, err := r.c.UpdateAgent(ctx, id, name, teamID)
	return err
}

func (r *restDataSource) LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error) {
	agents, err := r.c.ListAgents(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.Name == name {
			return &domain.Agent{ID: a.ID, Name: a.Name, TeamID: a.TeamID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}, nil
		}
	}
	return nil, fmt.Errorf("%w: agent named %q", domain.ErrNotFound, name)
}

func (r *restDataSource) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	// GetAgent returns roles inline
	a, err := r.c.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	var roles []domain.AgentRole
	for _, ar := range a.Roles {
		roles = append(roles, domain.AgentRole{AgentID: agentID, RoleID: ar.RoleID, Priority: ar.Priority})
	}
	return roles, nil
}

func (r *restDataSource) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	assignments := make([]client.RoleAssignment, len(roles))
	for i, ar := range roles {
		assignments[i] = client.RoleAssignment{RoleID: ar.RoleID, Priority: ar.Priority}
	}
	_, err := r.c.SetAgentRoles(ctx, agentID, assignments)
	return err
}

func (r *restDataSource) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	perms, err := r.c.GetAgentPermissions(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentPermission, len(perms))
	for i, p := range perms {
		out[i] = domain.AgentPermission{AgentID: agentID, PermissionID: p.PermissionID, ScopeTeamID: p.ScopeTeamID}
	}
	return out, nil
}

func (r *restDataSource) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	grants := make([]client.PermissionGrant, len(perms))
	for i, p := range perms {
		grants[i] = client.PermissionGrant{Permission: p.PermissionID, ScopeTeamID: p.ScopeTeamID}
	}
	_, err := r.c.SetAgentPermissions(ctx, agentID, grants)
	return err
}

func (r *restDataSource) ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error) {
	docs, err := r.c.ListRoleDocuments(ctx, roleID)
	if err != nil {
		return nil, err
	}
	return clientDocsToDomain(docs), nil
}

func (r *restDataSource) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	docs, err := r.c.ListAgentMemory(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return clientDocsToDomain(docs), nil
}

func (r *restDataSource) CreateDocument(ctx context.Context, d *domain.Document) error {
	if d.RoleID != "" {
		_, err := r.c.CreateRoleDocument(ctx, d.RoleID, d.Title, d.Content)
		return err
	}
	_, err := r.c.CreateAgentMemory(ctx, d.AgentID, d.Title, d.Content)
	return err
}

func (r *restDataSource) UpdateDocument(ctx context.Context, id, title, content string) error {
	_, err := r.c.UpdateDocument(ctx, id, title, content)
	return err
}

func (r *restDataSource) LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error) {
	// Try role documents first, then agent memory
	roleDocs, err := r.c.ListRoleDocuments(ctx, ownerID)
	if err == nil {
		for _, d := range roleDocs {
			if d.Title == title {
				return clientDocToDomain(d), nil
			}
		}
	}
	agentDocs, err2 := r.c.ListAgentMemory(ctx, ownerID)
	if err2 == nil {
		for _, d := range agentDocs {
			if d.Title == title {
				return clientDocToDomain(d), nil
			}
		}
	}
	if err != nil && err2 != nil {
		return nil, fmt.Errorf("lookup document: role err: %w, agent err: %v", err, err2)
	}
	return nil, fmt.Errorf("%w: document %q for owner %q", domain.ErrNotFound, title, ownerID)
}

func (r *restDataSource) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	tasks, err := r.c.ListTeamTasks(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Task, len(tasks))
	for i, tk := range tasks {
		out[i] = &domain.Task{
			ID: tk.ID, TeamID: tk.TeamID, AgentID: tk.AgentID,
			Title: tk.Title, Description: tk.Description,
			Status: domain.TaskStatus(tk.Status),
			CreatedAt: tk.CreatedAt, UpdatedAt: tk.UpdatedAt,
		}
	}
	return out, nil
}

func (r *restDataSource) CreateTask(ctx context.Context, t *domain.Task) error {
	_, err := r.c.CreateTask(ctx, client.CreateTaskInput{
		TeamID:      t.TeamID,
		Title:       t.Title,
		Description: t.Description,
	})
	return err
}

func (r *restDataSource) UpdateTask(ctx context.Context, id string, t *domain.Task) error {
	_, err := r.c.UpdateTask(ctx, id, client.UpdateTaskInput{
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		AgentID:     t.AgentID,
	})
	return err
}

func (r *restDataSource) LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error) {
	tasks, err := r.c.ListTeamTasks(ctx, teamID)
	if err != nil {
		return nil, err
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return &domain.Task{
				ID: tk.ID, TeamID: tk.TeamID, AgentID: tk.AgentID,
				Title: tk.Title, Description: tk.Description,
				Status: domain.TaskStatus(tk.Status),
				CreatedAt: tk.CreatedAt, UpdatedAt: tk.UpdatedAt,
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: task %q in team %q", domain.ErrNotFound, title, teamID)
}

// helpers

func clientDocsToDomain(docs []client.Document) []*domain.Document {
	out := make([]*domain.Document, len(docs))
	for i, d := range docs {
		out[i] = clientDocToDomain(d)
	}
	return out
}

func clientDocToDomain(d client.Document) *domain.Document {
	return &domain.Document{
		ID: d.ID, Kind: domain.DocumentKind(d.Kind), Title: d.Title,
		Content: d.Content, RoleID: d.RoleID, AgentID: d.AgentID,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}
