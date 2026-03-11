// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Work-Fort/Hive/internal/domain"
)

// requireAgent resolves the agent from context, returning an MCP error result
// if resolution fails.
func requireAgent(ctx context.Context, store domain.Store) (context.Context, *domain.Agent, *mcp.CallToolResult) {
	ctx, err := resolveAgent(ctx, store)
	if err != nil {
		return ctx, nil, mcp.NewToolResultError(fmt.Sprintf("authentication failed: %v", err))
	}
	agent := AgentFromContext(ctx)
	if agent == nil {
		return ctx, nil, mcp.NewToolResultError("authentication failed: agent not found in context")
	}
	return ctx, agent, nil
}

// jsonResult serializes v to indented JSON and returns it as an MCP text result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// --- get_provisioning ---

func makeGetProvisioning(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.RequireAny(ctx, agent.ID,
			PermCheck{Name: "role:read"},
			PermCheck{Name: "memory:read"},
		); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resp, err := deps.Provisioning.Resolve(ctx, agent.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("provisioning resolve failed: %v", err)), nil
		}
		return jsonResult(resp)
	}
}

// --- Memory tools ---

type memoryDocResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toMemoryDocResponse(d *domain.Document) memoryDocResponse {
	return memoryDocResponse{
		ID:        d.ID,
		Title:     d.Title,
		Content:   d.Content,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}

func makeListMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "memory:read", ""); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		docs, err := deps.Store.ListAgentMemory(ctx, agent.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list memory failed: %v", err)), nil
		}
		type listItem struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		items := make([]listItem, len(docs))
		for i, d := range docs {
			items[i] = listItem{ID: d.ID, Title: d.Title}
		}
		return jsonResult(items)
	}
}

func makeGetMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "memory:read", ""); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		doc, err := deps.Store.GetDocument(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get memory failed: %v", err)), nil
		}
		if doc.Kind != domain.DocumentKindMemory || doc.AgentID != agent.ID {
			return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
		}
		return jsonResult(toMemoryDocResponse(doc))
	}
}

func makeCreateMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "memory:write", ""); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: content"), nil
		}
		doc := &domain.Document{
			ID:      NewID("doc"),
			Kind:    domain.DocumentKindMemory,
			Title:   title,
			Content: content,
			AgentID: agent.ID,
		}
		if err := deps.Store.CreateDocument(ctx, doc); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create memory failed: %v", err)), nil
		}
		return jsonResult(toMemoryDocResponse(doc))
	}
}

func makeUpdateMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "memory:write", ""); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		existing, err := deps.Store.GetDocument(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get memory failed: %v", err)), nil
		}
		if existing.Kind != domain.DocumentKindMemory || existing.AgentID != agent.ID {
			return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
		}
		title := request.GetString("title", existing.Title)
		content := request.GetString("content", existing.Content)
		if err := deps.Store.UpdateDocument(ctx, id, title, content); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update memory failed: %v", err)), nil
		}
		updated, err := deps.Store.GetDocument(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("re-fetch memory failed: %v", err)), nil
		}
		return jsonResult(toMemoryDocResponse(updated))
	}
}

func makeDeleteMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "memory:write", ""); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		existing, err := deps.Store.GetDocument(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get memory failed: %v", err)), nil
		}
		if existing.Kind != domain.DocumentKindMemory || existing.AgentID != agent.ID {
			return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
		}
		if err := deps.Store.DeleteDocument(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete memory failed: %v", err)), nil
		}
		return mcp.NewToolResultText("deleted"), nil
	}
}

// --- Task tools ---

type mcpTaskResponse struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	AgentID     string    `json:"agent_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toMCPTaskResponse(t *domain.Task) mcpTaskResponse {
	return mcpTaskResponse{
		ID:          t.ID,
		TeamID:      t.TeamID,
		AgentID:     t.AgentID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func makeListTasks(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "task:read", agent.TeamID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		tasks, err := deps.Store.ListTeamTasks(ctx, agent.TeamID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list tasks failed: %v", err)), nil
		}
		items := make([]mcpTaskResponse, len(tasks))
		for i, t := range tasks {
			items[i] = toMCPTaskResponse(t)
		}
		return jsonResult(items)
	}
}

func makeGetTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		task, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get task failed: %v", err)), nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "task:read", task.TeamID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if task.TeamID != agent.TeamID {
			return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
		}
		return jsonResult(toMCPTaskResponse(task))
	}
}

func makeCreateTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "task:write", agent.TeamID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}
		description := request.GetString("description", "")
		assignee := request.GetString("agent_id", "")
		task := &domain.Task{
			ID:          NewID("tk"),
			TeamID:      agent.TeamID,
			AgentID:     assignee,
			Title:       title,
			Description: description,
			Status:      domain.TaskStatusPending,
		}
		if err := deps.Store.CreateTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create task failed: %v", err)), nil
		}
		return jsonResult(toMCPTaskResponse(task))
	}
}

func makeUpdateTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		existing, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get task failed: %v", err)), nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "task:write", existing.TeamID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if existing.TeamID != agent.TeamID {
			return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
		}
		updated := &domain.Task{
			ID:          existing.ID,
			TeamID:      existing.TeamID,
			AgentID:     request.GetString("agent_id", existing.AgentID),
			Title:       request.GetString("title", existing.Title),
			Description: request.GetString("description", existing.Description),
			Status:      existing.Status,
		}
		if statusStr := request.GetString("status", ""); statusStr != "" {
			switch domain.TaskStatus(statusStr) {
			case domain.TaskStatusPending, domain.TaskStatusInProgress, domain.TaskStatusCompleted:
				updated.Status = domain.TaskStatus(statusStr)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("invalid status: %q", statusStr)), nil
			}
		}
		if err := deps.Store.UpdateTask(ctx, id, updated); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update task failed: %v", err)), nil
		}
		result, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("re-fetch task failed: %v", err)), nil
		}
		return jsonResult(toMCPTaskResponse(result))
	}
}

func makeDeleteTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		existing, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get task failed: %v", err)), nil
		}
		if err := deps.Authz.CheckPermission(ctx, agent.ID, "task:write", existing.TeamID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if existing.TeamID != agent.TeamID {
			return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
		}
		if err := deps.Store.DeleteTask(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete task failed: %v", err)), nil
		}
		return mcp.NewToolResultText("deleted"), nil
	}
}
