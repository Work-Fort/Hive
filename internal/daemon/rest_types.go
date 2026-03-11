// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import "time"

// --- shared input ---

type IDPathInput struct {
	ID string `path:"id" doc:"Resource ID"`
}

// --- teams ---

type CreateTeamInput struct {
	Body struct {
		Name string `json:"name" doc:"Team name" minLength:"1"`
	}
}

type UpdateTeamInput struct {
	ID   string `path:"id" doc:"Team ID"`
	Body struct {
		Name string `json:"name" doc:"Team name" minLength:"1"`
	}
}

type TeamOutput struct {
	Body teamResponse
}

type TeamListOutput struct {
	Body []teamResponse
}

type teamResponse struct {
	ID        string    `json:"ID" doc:"Team ID"`
	Name      string    `json:"Name" doc:"Team name"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- roles ---

type ListRolesInput struct {
	ParentID string `query:"parent_id" doc:"Filter by parent role ID"`
}

type CreateRoleInput struct {
	Body struct {
		Name     string `json:"name" doc:"Role name" minLength:"1"`
		ParentID string `json:"parent_id,omitempty" doc:"Parent role ID for inheritance"`
	}
}

type UpdateRoleInput struct {
	ID   string `path:"id" doc:"Role ID"`
	Body struct {
		Name     string `json:"name" doc:"Role name" minLength:"1"`
		ParentID string `json:"parent_id" doc:"Parent role ID (empty to clear)"`
	}
}

type RoleOutput struct {
	Body roleResponse
}

type RoleListOutput struct {
	Body []roleResponse
}

type roleResponse struct {
	ID        string    `json:"ID" doc:"Role ID"`
	Name      string    `json:"Name" doc:"Role name"`
	ParentID  string    `json:"ParentID" doc:"Parent role ID (empty if root)"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- documents ---

type RoleDocPathInput struct {
	ID string `path:"id" doc:"Role ID"`
}

type CreateRoleDocumentInput struct {
	ID   string `path:"id" doc:"Role ID"`
	Body struct {
		Title   string `json:"title" doc:"Document title" minLength:"1"`
		Content string `json:"content" doc:"Markdown content"`
	}
}

type CreateAgentMemoryInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Title   string `json:"title" doc:"Document title" minLength:"1"`
		Content string `json:"content" doc:"Markdown content"`
	}
}

type UpdateDocumentInput struct {
	ID   string `path:"id" doc:"Document ID"`
	Body struct {
		Title   string `json:"title" doc:"Document title" minLength:"1"`
		Content string `json:"content" doc:"Markdown content"`
	}
}

type AgentMemoryPathInput struct {
	ID string `path:"id" doc:"Agent ID"`
}

type DocumentOutput struct {
	Body documentResponse
}

type DocumentListOutput struct {
	Body []documentResponse
}

type documentResponse struct {
	ID        string    `json:"ID" doc:"Document ID"`
	Kind      string    `json:"Kind" doc:"Document kind: role or memory"`
	Title     string    `json:"Title" doc:"Document title"`
	Content   string    `json:"Content" doc:"Markdown content"`
	RoleID    string    `json:"RoleID" doc:"Owning role ID (set when kind=role)"`
	AgentID   string    `json:"AgentID" doc:"Owning agent ID (set when kind=memory)"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- agents ---

type ListAgentsInput struct {
	TeamID string `query:"team_id" doc:"Filter by team ID"`
}

type CreateAgentInput struct {
	Body struct {
		Name   string `json:"name" doc:"Agent name" minLength:"1"`
		TeamID string `json:"team_id" doc:"Team ID" minLength:"1"`
	}
}

type UpdateAgentInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Name   string `json:"name" doc:"Agent name" minLength:"1"`
		TeamID string `json:"team_id" doc:"Team ID" minLength:"1"`
	}
}

type SetAgentRolesInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Roles []agentRoleEntry `json:"roles" doc:"Role assignments with priority"`
	}
}

type agentRoleEntry struct {
	RoleID   string `json:"role_id" doc:"Role ID" minLength:"1"`
	Priority int    `json:"priority" doc:"Priority (lower = higher precedence)"`
}

type AgentOutput struct {
	Body agentResponse
}

type AgentListOutput struct {
	Body []agentResponse
}

type AgentDetailOutput struct {
	Body agentDetailResponse
}

type agentResponse struct {
	ID        string    `json:"ID" doc:"Agent ID"`
	Name      string    `json:"Name" doc:"Agent name"`
	TeamID    string    `json:"TeamID" doc:"Team ID"`
	CreatedAt time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

type agentDetailResponse struct {
	ID        string              `json:"ID" doc:"Agent ID"`
	Name      string              `json:"Name" doc:"Agent name"`
	TeamID    string              `json:"TeamID" doc:"Team ID"`
	CreatedAt time.Time           `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt time.Time           `json:"UpdatedAt" doc:"Last update timestamp"`
	Roles     []agentRoleResponse `json:"roles" doc:"Assigned roles"`
}

type agentRoleResponse struct {
	AgentID  string `json:"AgentID" doc:"Agent ID"`
	RoleID   string `json:"RoleID" doc:"Role ID"`
	Priority int    `json:"Priority" doc:"Priority ordering"`
}

type AgentRoleListOutput struct {
	Body []agentRoleResponse
}

// --- tasks ---

type TeamTasksPathInput struct {
	ID string `path:"id" doc:"Team ID"`
}

type CreateTaskInput struct {
	Body struct {
		TeamID      string `json:"team_id" doc:"Team ID" minLength:"1"`
		AgentID     string `json:"agent_id,omitempty" doc:"Assigned agent ID"`
		Title       string `json:"title" doc:"Task title" minLength:"1"`
		Description string `json:"description,omitempty" doc:"Task description"`
		Status      string `json:"status,omitempty" doc:"Status: pending, in_progress, completed"`
	}
}

type UpdateTaskInput struct {
	ID   string `path:"id" doc:"Task ID"`
	Body struct {
		Title       string `json:"title,omitempty" doc:"Task title"`
		Description string `json:"description,omitempty" doc:"Task description"`
		Status      string `json:"status,omitempty" doc:"Status: pending, in_progress, completed"`
		AgentID     string `json:"agent_id" doc:"Assigned agent ID (empty to unassign)"`
	}
}

type TaskOutput struct {
	Body taskResponse
}

type TaskListOutput struct {
	Body []taskResponse
}

type taskResponse struct {
	ID          string    `json:"ID" doc:"Task ID"`
	TeamID      string    `json:"TeamID" doc:"Team ID"`
	AgentID     string    `json:"AgentID" doc:"Assigned agent ID"`
	Title       string    `json:"Title" doc:"Task title"`
	Description string    `json:"Description" doc:"Task description"`
	Status      string    `json:"Status" doc:"Task status"`
	CreatedAt   time.Time `json:"CreatedAt" doc:"Creation timestamp"`
	UpdatedAt   time.Time `json:"UpdatedAt" doc:"Last update timestamp"`
}

// --- permissions ---

type SetAgentPermissionsInput struct {
	ID   string `path:"id" doc:"Agent ID"`
	Body struct {
		Permissions []permissionEntry `json:"permissions" doc:"Permission grants"`
	}
}

type permissionEntry struct {
	Permission  string `json:"permission" doc:"Permission name" minLength:"1"`
	ScopeTeamID string `json:"scope_team_id,omitempty" doc:"Team scope (empty = global)"`
}

type PermissionsOutput struct {
	Body []permissionResponse
}

type permissionResponse struct {
	AgentID      string `json:"AgentID" doc:"Agent ID"`
	PermissionID string `json:"PermissionID" doc:"Permission name"`
	ScopeTeamID  string `json:"ScopeTeamID" doc:"Team scope (empty = global)"`
}

// -- Permission Entities --

type listPermissionsOutput struct {
	Body []permissionEntityResponse `json:"body"`
}

type permissionEntityResponse struct {
	ID   string `json:"ID" doc:"Permission ID"`
	Name string `json:"Name" doc:"Permission name"`
}

type createPermissionInput struct {
	Body struct {
		Name string `json:"name" doc:"Permission name" minLength:"1"`
	}
}

type createPermissionOutput struct {
	Body permissionEntityResponse `json:"body"`
}
