// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	auth "github.com/Work-Fort/Passport/go/service-auth"

	"github.com/Work-Fort/Hive/internal/domain"
)

// mapDomainErr converts a domain error to a Huma error with the appropriate
// HTTP status code. Returns nil if err is nil.
func mapDomainErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return huma.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrHasDependencies):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrDepthExceeded):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrCycleDetected):
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrPoolExhausted):
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrWorkflowMismatch):
		return huma.NewError(http.StatusConflict, err.Error())
	default:
		return huma.NewError(http.StatusInternalServerError, "internal error")
	}
}

// --- response converters ---

func teamToResponse(t *domain.Team) teamResponse {
	return teamResponse{
		ID: t.ID, Name: t.Name,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func roleToResponse(r *domain.Role) roleResponse {
	return roleResponse{
		ID: r.ID, Name: r.Name, ParentID: r.ParentID,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func docToResponse(d *domain.Document) documentResponse {
	return documentResponse{
		ID: d.ID, Kind: string(d.Kind), Title: d.Title, Content: d.Content,
		RoleID: d.RoleID, AgentID: d.AgentID,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func agentToResponse(a *domain.Agent) agentResponse {
	return agentResponse{
		ID: a.ID, Name: a.Name, TeamID: a.TeamID,
		Model: a.Model, Runtime: a.Runtime,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func agentRolesToResponse(roles []domain.AgentRole) []agentRoleResponse {
	out := make([]agentRoleResponse, len(roles))
	for i, r := range roles {
		out[i] = agentRoleResponse{AgentID: r.AgentID, RoleID: r.RoleID, Priority: r.Priority}
	}
	return out
}

func taskToResponse(t *domain.Task) taskResponse {
	return taskResponse{
		ID: t.ID, TeamID: t.TeamID, AgentID: t.AgentID,
		Title: t.Title, Description: t.Description, Status: string(t.Status),
		FlowTaskRef: t.FlowTaskRef,
		CreatedAt:   t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func permToResponse(p domain.AgentPermission) permissionResponse {
	return permissionResponse{
		AgentID: p.AgentID, PermissionID: p.PermissionID, ScopeTeamID: p.ScopeTeamID,
	}
}

func permEntityToResponse(p *domain.Permission) permissionEntityResponse {
	return permissionEntityResponse{ID: p.ID, Name: p.Name}
}

// --- team routes ---

func registerTeamRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-teams",
		Method:      http.MethodGet,
		Path:        "/v1/teams",
		Summary:     "List teams",
		Tags:        []string{"Teams"},
	}, func(ctx context.Context, input *struct{}) (*TeamListOutput, error) {
		teams, err := store.ListTeams(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]teamResponse, len(teams))
		for i, t := range teams {
			resp[i] = teamToResponse(t)
		}
		return &TeamListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-team",
		Method:        http.MethodPost,
		Path:          "/v1/teams",
		Summary:       "Create a team",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Teams"},
	}, func(ctx context.Context, input *CreateTeamInput) (*TeamOutput, error) {
		team := &domain.Team{ID: NewID("tm"), Name: input.Body.Name}
		if err := store.CreateTeam(ctx, team); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetTeam(ctx, team.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TeamOutput{Body: teamToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-team",
		Method:      http.MethodGet,
		Path:        "/v1/teams/{id}",
		Summary:     "Get a team",
		Tags:        []string{"Teams"},
	}, func(ctx context.Context, input *IDPathInput) (*TeamOutput, error) {
		team, err := store.GetTeam(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TeamOutput{Body: teamToResponse(team)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-team",
		Method:      http.MethodPut,
		Path:        "/v1/teams/{id}",
		Summary:     "Update a team",
		Tags:        []string{"Teams"},
	}, func(ctx context.Context, input *UpdateTeamInput) (*TeamOutput, error) {
		if err := store.UpdateTeam(ctx, input.ID, input.Body.Name); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetTeam(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TeamOutput{Body: teamToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-team",
		Method:        http.MethodDelete,
		Path:          "/v1/teams/{id}",
		Summary:       "Delete a team",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Teams"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteTeam(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})
}

// --- role routes ---

func registerRoleRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-roles",
		Method:      http.MethodGet,
		Path:        "/v1/roles",
		Summary:     "List roles",
		Tags:        []string{"Roles"},
	}, func(ctx context.Context, input *ListRolesInput) (*RoleListOutput, error) {
		roles, err := store.ListRoles(ctx, input.ParentID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]roleResponse, len(roles))
		for i, r := range roles {
			resp[i] = roleToResponse(r)
		}
		return &RoleListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-role",
		Method:        http.MethodPost,
		Path:          "/v1/roles",
		Summary:       "Create a role",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Roles"},
	}, func(ctx context.Context, input *CreateRoleInput) (*RoleOutput, error) {
		role := &domain.Role{ID: NewID("rl"), Name: input.Body.Name, ParentID: input.Body.ParentID}
		if err := store.CreateRole(ctx, role); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetRole(ctx, role.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &RoleOutput{Body: roleToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-role",
		Method:      http.MethodGet,
		Path:        "/v1/roles/{id}",
		Summary:     "Get a role",
		Tags:        []string{"Roles"},
	}, func(ctx context.Context, input *IDPathInput) (*RoleOutput, error) {
		role, err := store.GetRole(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &RoleOutput{Body: roleToResponse(role)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-role",
		Method:      http.MethodPut,
		Path:        "/v1/roles/{id}",
		Summary:     "Update a role",
		Tags:        []string{"Roles"},
	}, func(ctx context.Context, input *UpdateRoleInput) (*RoleOutput, error) {
		if err := store.UpdateRole(ctx, input.ID, input.Body.Name, input.Body.ParentID); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetRole(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &RoleOutput{Body: roleToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-role",
		Method:        http.MethodDelete,
		Path:          "/v1/roles/{id}",
		Summary:       "Delete a role",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Roles"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteRole(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})
}

// --- document routes ---

func registerDocumentRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-role-documents",
		Method:      http.MethodGet,
		Path:        "/v1/roles/{id}/documents",
		Summary:     "List documents for a role",
		Tags:        []string{"Documents"},
	}, func(ctx context.Context, input *RoleDocPathInput) (*DocumentListOutput, error) {
		if _, err := store.GetRole(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		docs, err := store.ListRoleDocuments(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]documentResponse, len(docs))
		for i, d := range docs {
			resp[i] = docToResponse(d)
		}
		return &DocumentListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-role-document",
		Method:        http.MethodPost,
		Path:          "/v1/roles/{id}/documents",
		Summary:       "Create a document for a role",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Documents"},
	}, func(ctx context.Context, input *CreateRoleDocumentInput) (*DocumentOutput, error) {
		if _, err := store.GetRole(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		doc := &domain.Document{
			ID: NewID("doc"), Kind: domain.DocumentKindRole,
			Title: input.Body.Title, Content: input.Body.Content, RoleID: input.ID,
		}
		if err := store.CreateDocument(ctx, doc); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetDocument(ctx, doc.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &DocumentOutput{Body: docToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-document",
		Method:      http.MethodGet,
		Path:        "/v1/documents/{id}",
		Summary:     "Get a document",
		Tags:        []string{"Documents"},
	}, func(ctx context.Context, input *IDPathInput) (*DocumentOutput, error) {
		doc, err := store.GetDocument(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &DocumentOutput{Body: docToResponse(doc)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-document",
		Method:      http.MethodPut,
		Path:        "/v1/documents/{id}",
		Summary:     "Update a document",
		Tags:        []string{"Documents"},
	}, func(ctx context.Context, input *UpdateDocumentInput) (*DocumentOutput, error) {
		if err := store.UpdateDocument(ctx, input.ID, input.Body.Title, input.Body.Content); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetDocument(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &DocumentOutput{Body: docToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-document",
		Method:        http.MethodDelete,
		Path:          "/v1/documents/{id}",
		Summary:       "Delete a document",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Documents"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteDocument(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-agent-memory",
		Method:      http.MethodGet,
		Path:        "/v1/agents/{id}/memory",
		Summary:     "List memory documents for an agent",
		Tags:        []string{"Documents"},
	}, func(ctx context.Context, input *AgentMemoryPathInput) (*DocumentListOutput, error) {
		if _, err := store.GetAgent(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		docs, err := store.ListAgentMemory(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]documentResponse, len(docs))
		for i, d := range docs {
			resp[i] = docToResponse(d)
		}
		return &DocumentListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-agent-memory",
		Method:        http.MethodPost,
		Path:          "/v1/agents/{id}/memory",
		Summary:       "Create a memory document for an agent",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Documents"},
	}, func(ctx context.Context, input *CreateAgentMemoryInput) (*DocumentOutput, error) {
		if _, err := store.GetAgent(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		doc := &domain.Document{
			ID: NewID("doc"), Kind: domain.DocumentKindMemory,
			Title: input.Body.Title, Content: input.Body.Content, AgentID: input.ID,
		}
		if err := store.CreateDocument(ctx, doc); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetDocument(ctx, doc.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &DocumentOutput{Body: docToResponse(created)}, nil
	})
}

// --- agent routes ---

func registerAgentRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-agents",
		Method:      http.MethodGet,
		Path:        "/v1/agents",
		Summary:     "List agents",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, input *ListAgentsInput) (*AgentListOutput, error) {
		agents, err := store.ListAgents(ctx, input.TeamID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]agentResponse, len(agents))
		for i, a := range agents {
			resp[i] = agentToResponse(a)
		}
		return &AgentListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-me",
		Method:      http.MethodGet,
		Path:        "/v1/agents/me",
		Summary:     "Get the agent record for the caller",
		Description: "Resolves the caller's Passport identity from the Authorization header and returns the matching agent record (including Model and Runtime). Used by adjutant runtimes to fetch their own configuration at startup without knowing their agent ID.",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, _ *struct{}) (*AgentOutput, error) {
		id, ok := auth.IdentityFromContext(ctx)
		if !ok || id.ID == "" {
			return nil, huma.Error401Unauthorized("no authenticated identity")
		}
		agent, err := store.GetAgent(ctx, id.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &AgentOutput{Body: agentToResponse(agent)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-agent",
		Method:        http.MethodPost,
		Path:          "/v1/agents",
		Summary:       "Create an agent",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Agents"},
	}, func(ctx context.Context, input *CreateAgentInput) (*AgentOutput, error) {
		if _, err := store.GetTeam(ctx, input.Body.TeamID); err != nil {
			return nil, mapDomainErr(err)
		}
		agent := &domain.Agent{
			ID: input.Body.ID, Name: input.Body.Name, TeamID: input.Body.TeamID,
			Model: input.Body.Model, Runtime: input.Body.Runtime,
		}
		if err := store.CreateAgent(ctx, agent); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetAgent(ctx, agent.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &AgentOutput{Body: agentToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-agent",
		Method:      http.MethodGet,
		Path:        "/v1/agents/{id}",
		Summary:     "Get an agent with role assignments",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, input *IDPathInput) (*AgentDetailOutput, error) {
		agent, err := store.GetAgent(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		roles, err := store.GetAgentRoles(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if roles == nil {
			roles = []domain.AgentRole{}
		}
		return &AgentDetailOutput{Body: agentDetailResponse{
			ID: agent.ID, Name: agent.Name, TeamID: agent.TeamID,
			Model: agent.Model, Runtime: agent.Runtime,
			CreatedAt: agent.CreatedAt, UpdatedAt: agent.UpdatedAt,
			Roles: agentRolesToResponse(roles),
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-agent",
		Method:      http.MethodPut,
		Path:        "/v1/agents/{id}",
		Summary:     "Update an agent",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, input *UpdateAgentInput) (*AgentOutput, error) {
		agent := &domain.Agent{
			ID: input.ID, Name: input.Body.Name, TeamID: input.Body.TeamID,
			Model: input.Body.Model, Runtime: input.Body.Runtime,
		}
		if err := store.UpdateAgent(ctx, agent); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetAgent(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &AgentOutput{Body: agentToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-agent",
		Method:        http.MethodDelete,
		Path:          "/v1/agents/{id}",
		Summary:       "Delete an agent",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Agents"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteAgent(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-agent-roles",
		Method:      http.MethodPut,
		Path:        "/v1/agents/{id}/roles",
		Summary:     "Set role assignments for an agent",
		Tags:        []string{"Agents"},
	}, func(ctx context.Context, input *SetAgentRolesInput) (*AgentRoleListOutput, error) {
		if _, err := store.GetAgent(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		roles := make([]domain.AgentRole, len(input.Body.Roles))
		for i, ar := range input.Body.Roles {
			roles[i] = domain.AgentRole{AgentID: input.ID, RoleID: ar.RoleID, Priority: ar.Priority}
		}
		if err := store.SetAgentRoles(ctx, input.ID, roles); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetAgentRoles(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if updated == nil {
			updated = []domain.AgentRole{}
		}
		return &AgentRoleListOutput{Body: agentRolesToResponse(updated)}, nil
	})
}

// --- task routes ---

func registerTaskRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-team-tasks",
		Method:      http.MethodGet,
		Path:        "/v1/teams/{id}/tasks",
		Summary:     "List tasks for a team",
		Tags:        []string{"Tasks"},
	}, func(ctx context.Context, input *TeamTasksPathInput) (*TaskListOutput, error) {
		if _, err := store.GetTeam(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		tasks, err := store.ListTeamTasks(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]taskResponse, len(tasks))
		for i, t := range tasks {
			resp[i] = taskToResponse(t)
		}
		return &TaskListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-task",
		Method:        http.MethodPost,
		Path:          "/v1/tasks",
		Summary:       "Create a task",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Tasks"},
	}, func(ctx context.Context, input *CreateTaskInput) (*TaskOutput, error) {
		if _, err := store.GetTeam(ctx, input.Body.TeamID); err != nil {
			return nil, mapDomainErr(err)
		}

		status := domain.TaskStatus(input.Body.Status)
		if status == "" {
			status = domain.TaskStatusPending
		}

		task := &domain.Task{
			ID: NewID("tk"), TeamID: input.Body.TeamID, AgentID: input.Body.AgentID,
			Title: input.Body.Title, Description: input.Body.Description, Status: status,
			FlowTaskRef: input.Body.FlowTaskRef,
		}
		if err := store.CreateTask(ctx, task); err != nil {
			return nil, mapDomainErr(err)
		}
		created, err := store.GetTask(ctx, task.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TaskOutput{Body: taskToResponse(created)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-task",
		Method:      http.MethodGet,
		Path:        "/v1/tasks/{id}",
		Summary:     "Get a task",
		Tags:        []string{"Tasks"},
	}, func(ctx context.Context, input *IDPathInput) (*TaskOutput, error) {
		task, err := store.GetTask(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TaskOutput{Body: taskToResponse(task)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-task",
		Method:      http.MethodPut,
		Path:        "/v1/tasks/{id}",
		Summary:     "Update a task",
		Tags:        []string{"Tasks"},
	}, func(ctx context.Context, input *UpdateTaskInput) (*TaskOutput, error) {
		existing, err := store.GetTask(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		if input.Body.Title != "" {
			existing.Title = input.Body.Title
		}
		if input.Body.Description != "" {
			existing.Description = input.Body.Description
		}
		if input.Body.Status != "" {
			existing.Status = domain.TaskStatus(input.Body.Status)
		}
		existing.AgentID = input.Body.AgentID
		existing.FlowTaskRef = input.Body.FlowTaskRef

		if err := store.UpdateTask(ctx, input.ID, existing); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetTask(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &TaskOutput{Body: taskToResponse(updated)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-task",
		Method:        http.MethodDelete,
		Path:          "/v1/tasks/{id}",
		Summary:       "Delete a task",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Tasks"},
	}, func(ctx context.Context, input *IDPathInput) (*struct{}, error) {
		if err := store.DeleteTask(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		return nil, nil
	})
}

// --- permission entity routes ---

func registerPermissionEntityRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "list-permissions",
		Method:      http.MethodGet,
		Path:        "/v1/permissions",
		Summary:     "List all permissions",
		Tags:        []string{"Permissions"},
	}, func(ctx context.Context, input *struct{}) (*ListPermissionsOutput, error) {
		perms, err := store.ListPermissions(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]permissionEntityResponse, len(perms))
		for i, p := range perms {
			resp[i] = permEntityToResponse(p)
		}
		return &ListPermissionsOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-permission",
		Method:        http.MethodPost,
		Path:          "/v1/permissions",
		Summary:       "Create a permission",
		DefaultStatus: 201,
		Tags:          []string{"Permissions"},
	}, func(ctx context.Context, input *CreatePermissionInput) (*CreatePermissionOutput, error) {
		if err := store.SeedPermissions(ctx, []string{input.Body.Name}); err != nil {
			return nil, mapDomainErr(err)
		}
		p, err := store.LookupPermissionByName(ctx, input.Body.Name)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &CreatePermissionOutput{Body: permEntityToResponse(p)}, nil
	})
}

// --- permission routes ---

func registerPermissionRoutes(api huma.API, store domain.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "get-agent-permissions",
		Method:      http.MethodGet,
		Path:        "/v1/agents/{id}/permissions",
		Summary:     "Get permissions for an agent",
		Tags:        []string{"Permissions"},
	}, func(ctx context.Context, input *IDPathInput) (*PermissionsOutput, error) {
		if _, err := store.GetAgent(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		perms, err := store.GetAgentPermissions(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]permissionResponse, len(perms))
		for i, p := range perms {
			resp[i] = permToResponse(p)
		}
		return &PermissionsOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-agent-permissions",
		Method:      http.MethodPut,
		Path:        "/v1/agents/{id}/permissions",
		Summary:     "Set permissions for an agent",
		Tags:        []string{"Permissions"},
	}, func(ctx context.Context, input *SetAgentPermissionsInput) (*PermissionsOutput, error) {
		if _, err := store.GetAgent(ctx, input.ID); err != nil {
			return nil, mapDomainErr(err)
		}
		perms := make([]domain.AgentPermission, len(input.Body.Permissions))
		for i, p := range input.Body.Permissions {
			perms[i] = domain.AgentPermission{AgentID: input.ID, PermissionID: p.Permission, ScopeTeamID: p.ScopeTeamID}
		}
		if err := store.SetAgentPermissions(ctx, input.ID, perms); err != nil {
			return nil, mapDomainErr(err)
		}
		updated, err := store.GetAgentPermissions(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]permissionResponse, len(updated))
		for i, p := range updated {
			resp[i] = permToResponse(p)
		}
		return &PermissionsOutput{Body: resp}, nil
	})
}
