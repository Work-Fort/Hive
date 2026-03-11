// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Work-Fort/Hive/internal/domain"
)

// MCPDeps holds the dependencies injected into MCP tool handlers.
type MCPDeps struct {
	Store        domain.Store
	Provisioning *ProvisioningService
}

// permissionMap defines which permissions each tool requires (AND logic).
var permissionMap = map[string][]string{
	"get_provisioning": {"role:read", "memory:read"},
	"list_memory":      {"memory:read"},
	"get_memory":       {"memory:read"},
	"create_memory":    {"memory:write"},
	"update_memory":    {"memory:write"},
	"delete_memory":    {"memory:write"},
	"list_tasks":       {"task:read"},
	"get_task":         {"task:read"},
	"create_task":      {"task:write"},
	"update_task":      {"task:write"},
	"delete_task":      {"task:write"},
}

// NewMCPHandler creates the mcp-go StreamableHTTPServer and returns it as
// an http.Handler ready to be mounted on /mcp.
func NewMCPHandler(deps MCPDeps) http.Handler {
	mcpServer := server.NewMCPServer(
		"hive",
		"1.0",
		server.WithToolCapabilities(false),
		server.WithToolFilter(toolFilter(deps.Store)),
	)

	registerTools(mcpServer, deps)

	httpServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithHTTPContextFunc(httpContextFunc()),
	)

	return httpServer
}

// toolFilter returns a ToolFilterFunc that resolves the agent from context
// and filters tools based on the agent's permissions.
func toolFilter(store domain.Store) server.ToolFilterFunc {
	return func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
		ctx, err := resolveAgent(ctx, store)
		if err != nil {
			log.Warn("mcp tool filter: agent resolution failed", "err", err)
			return nil
		}

		agent := AgentFromContext(ctx)
		if agent == nil {
			return nil
		}

		var filtered []mcp.Tool
		for _, tool := range tools {
			perms, ok := permissionMap[tool.Name]
			if !ok {
				continue
			}
			if hasAllPermissions(ctx, store, agent.ID, agent.TeamID, perms) {
				filtered = append(filtered, tool)
			}
		}
		return filtered
	}
}

// hasAllPermissions checks that the agent has ALL listed permissions.
func hasAllPermissions(ctx context.Context, store domain.Store, agentID, teamID string, perms []string) bool {
	for _, perm := range perms {
		has, err := store.HasPermission(ctx, agentID, perm, teamID)
		if err != nil {
			log.Warn("permission check failed", "agent", agentID, "perm", perm, "err", err)
			return false
		}
		if !has {
			return false
		}
	}
	return true
}

// registerTools registers all 11 MCP tools on the server.
func registerTools(s *server.MCPServer, deps MCPDeps) {
	// --- Provisioning ---
	s.AddTool(
		mcp.NewTool("get_provisioning",
			mcp.WithDescription("Get your composed role documents and memory (hierarchical)."),
		),
		makeGetProvisioning(deps),
	)

	// --- Memory tools ---
	s.AddTool(
		mcp.NewTool("list_memory",
			mcp.WithDescription("List your memory documents."),
		),
		makeListMemory(deps),
	)
	s.AddTool(
		mcp.NewTool("get_memory",
			mcp.WithDescription("Get a specific memory document by ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Document ID")),
		),
		makeGetMemory(deps),
	)
	s.AddTool(
		mcp.NewTool("create_memory",
			mcp.WithDescription("Create a new memory document."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Document title")),
			mcp.WithString("content", mcp.Required(), mcp.Description("Markdown content")),
		),
		makeCreateMemory(deps),
	)
	s.AddTool(
		mcp.NewTool("update_memory",
			mcp.WithDescription("Update an existing memory document."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Document ID")),
			mcp.WithString("title", mcp.Description("New title (omit to keep current)")),
			mcp.WithString("content", mcp.Description("New content (omit to keep current)")),
		),
		makeUpdateMemory(deps),
	)
	s.AddTool(
		mcp.NewTool("delete_memory",
			mcp.WithDescription("Delete a memory document."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Document ID")),
		),
		makeDeleteMemory(deps),
	)

	// --- Task tools ---
	s.AddTool(
		mcp.NewTool("list_tasks",
			mcp.WithDescription("List your team's tasks."),
		),
		makeListTasks(deps),
	)
	s.AddTool(
		mcp.NewTool("get_task",
			mcp.WithDescription("Get a specific task by ID."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID")),
		),
		makeGetTask(deps),
	)
	s.AddTool(
		mcp.NewTool("create_task",
			mcp.WithDescription("Create a task for your team."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Task title")),
			mcp.WithString("description", mcp.Description("Task description")),
			mcp.WithString("agent_id", mcp.Description("Agent ID to assign")),
		),
		makeCreateTask(deps),
	)
	s.AddTool(
		mcp.NewTool("update_task",
			mcp.WithDescription("Update a task's status or details."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID")),
			mcp.WithString("title", mcp.Description("New title")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithString("status", mcp.Description("New status: pending, in_progress, or completed"), mcp.Enum("pending", "in_progress", "completed")),
			mcp.WithString("agent_id", mcp.Description("Agent ID to assign")),
		),
		makeUpdateTask(deps),
	)
	s.AddTool(
		mcp.NewTool("delete_task",
			mcp.WithDescription("Delete a task."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Task ID")),
		),
		makeDeleteTask(deps),
	)
}
