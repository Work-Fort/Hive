# MCP Server Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the MCP server with 11 tools for agent self-service provisioning, memory management, and task management.

**Architecture:** The MCP server uses `mcp-go` (`github.com/mark3labs/mcp-go`) to handle JSON-RPC protocol on `POST /mcp`. Agent identity is extracted from the `X-Agent-Id` HTTP header via `WithHTTPContextFunc` and propagated through Go context to tool handlers. A `ToolFilterFunc` dynamically filters the tool list per request based on the agent's permissions from the store.

**Tech Stack:** Go, mcp-go (github.com/mark3labs/mcp-go), net/http

**Depends on:** Plan 002 (store), Plan 004 (provisioning resolution) must be complete.

---

## Chunk 1: Add mcp-go Dependency

### Task 1: Add mcp-go to go.mod

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the mcp-go dependency**

```bash
go get github.com/mark3labs/mcp-go@latest
go mod tidy
```

- [ ] **Step 2: Verify the dependency resolves**

```bash
go build ./...
```

Expected: clean build, no errors. `go.mod` now includes `github.com/mark3labs/mcp-go`.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add mcp-go dependency for MCP server"
```

---

## Chunk 2: Session Context

### Task 2: Create session context helpers

The session layer extracts the agent ID from HTTP context, looks up the agent from the store, and makes it available to tool handlers. This uses Go context values — no struct-level state.

**Files:**
- Create: `internal/daemon/session.go`

- [ ] **Step 1: Create session context helpers**

Create `internal/daemon/session.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// contextKey is an unexported type to prevent context key collisions.
type contextKey int

const (
	// agentIDKey stores the agent ID extracted from X-Agent-Id header.
	agentIDKey contextKey = iota

	// agentKey stores the resolved *domain.Agent.
	agentKey
)

// AgentIDFromContext returns the agent ID from the context, or empty string.
func AgentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(agentIDKey).(string)
	return v
}

// AgentFromContext returns the resolved agent from the context, or nil.
func AgentFromContext(ctx context.Context) *domain.Agent {
	v, _ := ctx.Value(agentKey).(*domain.Agent)
	return v
}

// contextWithAgentID returns a new context with the agent ID set.
func contextWithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentIDKey, agentID)
}

// contextWithAgent returns a new context with the resolved agent set.
func contextWithAgent(ctx context.Context, agent *domain.Agent) context.Context {
	return context.WithValue(ctx, agentKey, agent)
}

// httpContextFunc returns an HTTPContextFunc that extracts X-Agent-Id from
// the HTTP request and stores it in the context. The agent lookup is deferred
// to the tool filter/handler layer to avoid a DB hit on every request
// (including SSE pings).
func httpContextFunc() func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		agentID := r.Header.Get("X-Agent-Id")
		if agentID != "" {
			ctx = contextWithAgentID(ctx, agentID)
		}
		return ctx
	}
}

// resolveAgent looks up the agent by ID from the store and returns a context
// with the agent set. Returns an error if the agent ID is missing or the
// agent is not found.
func resolveAgent(ctx context.Context, store domain.Store) (context.Context, error) {
	agentID := AgentIDFromContext(ctx)
	if agentID == "" {
		return ctx, fmt.Errorf("missing X-Agent-Id header")
	}

	agent, err := store.GetAgent(ctx, agentID)
	if err != nil {
		return ctx, fmt.Errorf("resolve agent %q: %w", agentID, err)
	}

	return contextWithAgent(ctx, agent), nil
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/daemon/session.go
git commit -m "feat: add MCP session context helpers for agent identity"
```

---

## Chunk 3: MCP Server Setup

### Task 3: Create MCP server with tool registration and HTTP handler

**Files:**
- Create: `internal/daemon/mcp_server.go`

- [ ] **Step 1: Create the MCP server module**

Create `internal/daemon/mcp_server.go`:

```go
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

// permissionMap defines which permissions each tool requires.
// All listed permissions must be satisfied (AND logic).
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
// and filters tools based on the agent's permissions. If the agent cannot
// be resolved, all tools are hidden.
func toolFilter(store domain.Store) server.ToolFilterFunc {
	return func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
		// Resolve agent — if this fails, return no tools.
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
				// Tool not in permission map — skip (defensive).
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
// Memory and task permissions are checked with the agent's own team scope.
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
			mcp.WithDescription("Get your composed role documents and memory (hierarchical). Returns all role definitions with inherited documents plus your personal memory."),
		),
		makeGetProvisioning(deps),
	)

	// --- Memory tools ---
	s.AddTool(
		mcp.NewTool("list_memory",
			mcp.WithDescription("List your memory documents. Returns document IDs and titles."),
		),
		makeListMemory(deps),
	)

	s.AddTool(
		mcp.NewTool("get_memory",
			mcp.WithDescription("Get a specific memory document by ID."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Document ID"),
			),
		),
		makeGetMemory(deps),
	)

	s.AddTool(
		mcp.NewTool("create_memory",
			mcp.WithDescription("Create a new memory document."),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Document title"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Markdown content"),
			),
		),
		makeCreateMemory(deps),
	)

	s.AddTool(
		mcp.NewTool("update_memory",
			mcp.WithDescription("Update an existing memory document."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Document ID"),
			),
			mcp.WithString("title",
				mcp.Description("New title (omit to keep current)"),
			),
			mcp.WithString("content",
				mcp.Description("New markdown content (omit to keep current)"),
			),
		),
		makeUpdateMemory(deps),
	)

	s.AddTool(
		mcp.NewTool("delete_memory",
			mcp.WithDescription("Delete a memory document."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Document ID"),
			),
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
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Task ID"),
			),
		),
		makeGetTask(deps),
	)

	s.AddTool(
		mcp.NewTool("create_task",
			mcp.WithDescription("Create a task for your team."),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Task title"),
			),
			mcp.WithString("description",
				mcp.Description("Task description"),
			),
			mcp.WithString("agent_id",
				mcp.Description("Agent ID to assign (omit for unassigned)"),
			),
		),
		makeCreateTask(deps),
	)

	s.AddTool(
		mcp.NewTool("update_task",
			mcp.WithDescription("Update a task's status or details."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Task ID"),
			),
			mcp.WithString("title",
				mcp.Description("New title (omit to keep current)"),
			),
			mcp.WithString("description",
				mcp.Description("New description (omit to keep current)"),
			),
			mcp.WithString("status",
				mcp.Description("New status: pending, in_progress, or completed"),
				mcp.Enum("pending", "in_progress", "completed"),
			),
			mcp.WithString("agent_id",
				mcp.Description("Agent ID to assign (omit to keep current)"),
			),
		),
		makeUpdateTask(deps),
	)

	s.AddTool(
		mcp.NewTool("delete_task",
			mcp.WithDescription("Delete a task."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Task ID"),
			),
		),
		makeDeleteTask(deps),
	)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/daemon/mcp_server.go
git commit -m "feat: add MCP server setup with tool registration and permission filtering"
```

---

## Chunk 4: Tool Handler Implementations

### Task 4: Provisioning tool handler

**Files:**
- Create: `internal/daemon/mcp_tools.go`

- [ ] **Step 1: Create mcp_tools.go with the agent-resolution helper and get_provisioning handler**

Create `internal/daemon/mcp_tools.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/Work-Fort/Hive/internal/domain"
)

// requireAgent resolves the agent from context, returning an MCP error result
// if resolution fails. Tool handlers call this as their first step.
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

		resp, err := deps.Provisioning.Resolve(ctx, agent.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("provisioning resolve failed: %v", err)), nil
		}

		return jsonResult(resp)
	}
}
```

Note: The `server` import is `github.com/mark3labs/mcp-go/server` — it's needed for the `server.ToolHandlerFunc` type alias.

Add this import at the top of the file alongside the others:

```go
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
```

### Task 5: Memory tool handlers

**Files:**
- Modify: `internal/daemon/mcp_tools.go`

- [ ] **Step 1: Add memory tool handlers**

Append to `internal/daemon/mcp_tools.go`:

```go
// --- Memory tools ---

// memoryDocResponse is the JSON shape returned for memory documents.
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
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
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
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
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

		// Verify the document belongs to this agent and is a memory doc.
		if doc.Kind != domain.DocumentKindMemory || doc.AgentID != agent.ID {
			return mcp.NewToolResultError(fmt.Sprintf("memory document %q not found", id)), nil
		}

		return jsonResult(toMemoryDocResponse(doc))
	}
}

func makeCreateMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
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
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}

		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

		// Fetch existing document to verify ownership.
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

		// Apply partial updates — keep existing values for omitted fields.
		title := request.GetString("title", existing.Title)
		content := request.GetString("content", existing.Content)

		if err := deps.Store.UpdateDocument(ctx, id, title, content); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update memory failed: %v", err)), nil
		}

		// Re-fetch to get updated timestamps.
		updated, err := deps.Store.GetDocument(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("re-fetch memory failed: %v", err)), nil
		}

		return jsonResult(toMemoryDocResponse(updated))
	}
}

func makeDeleteMemory(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}

		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

		// Verify ownership before delete.
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
```

### Task 6: Task tool handlers

**Files:**
- Modify: `internal/daemon/mcp_tools.go`

- [ ] **Step 1: Add task tool handlers**

Append to `internal/daemon/mcp_tools.go`:

```go
// --- Task tools ---

// taskResponse is the JSON shape returned for tasks.
type taskResponse struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	AgentID     string    `json:"agent_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toTaskResponse(t *domain.Task) taskResponse {
	return taskResponse{
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
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}

		tasks, err := deps.Store.ListTeamTasks(ctx, agent.TeamID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list tasks failed: %v", err)), nil
		}

		items := make([]taskResponse, len(tasks))
		for i, t := range tasks {
			items[i] = toTaskResponse(t)
		}

		return jsonResult(items)
	}
}

func makeGetTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, agent, errResult := requireAgent(ctx, deps.Store)
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

		// Verify the task belongs to the agent's team.
		if task.TeamID != agent.TeamID {
			return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
		}

		return jsonResult(toTaskResponse(task))
	}
}

func makeCreateTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}

		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}

		description := request.GetString("description", "")
		assignee := request.GetString("agent_id", "")

		task := &domain.Task{
			TeamID:      agent.TeamID,
			AgentID:     assignee,
			Title:       title,
			Description: description,
			Status:      domain.TaskStatusPending,
		}

		if err := deps.Store.CreateTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create task failed: %v", err)), nil
		}

		return jsonResult(toTaskResponse(task))
	}
}

func makeUpdateTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}

		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

		// Fetch existing task to verify team ownership.
		existing, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get task failed: %v", err)), nil
		}

		if existing.TeamID != agent.TeamID {
			return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
		}

		// Apply partial updates — keep existing values for omitted fields.
		updated := &domain.Task{
			ID:          existing.ID,
			TeamID:      existing.TeamID,
			AgentID:     request.GetString("agent_id", existing.AgentID),
			Title:       request.GetString("title", existing.Title),
			Description: request.GetString("description", existing.Description),
			Status:      existing.Status,
		}

		// Handle status separately for validation.
		if statusStr := request.GetString("status", ""); statusStr != "" {
			switch domain.TaskStatus(statusStr) {
			case domain.TaskStatusPending, domain.TaskStatusInProgress, domain.TaskStatusCompleted:
				updated.Status = domain.TaskStatus(statusStr)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("invalid status: %q (must be pending, in_progress, or completed)", statusStr)), nil
			}
		}

		if err := deps.Store.UpdateTask(ctx, id, updated); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update task failed: %v", err)), nil
		}

		// Re-fetch to get updated timestamps.
		result, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("re-fetch task failed: %v", err)), nil
		}

		return jsonResult(toTaskResponse(result))
	}
}

func makeDeleteTask(deps MCPDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, agent, errResult := requireAgent(ctx, deps.Store)
		if errResult != nil {
			return errResult, nil
		}

		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

		// Verify team ownership before delete.
		existing, err := deps.Store.GetTask(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return mcp.NewToolResultError(fmt.Sprintf("task %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get task failed: %v", err)), nil
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
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

Expected: clean build with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/mcp_tools.go
git commit -m "feat: add MCP tool handler implementations (provisioning, memory, tasks)"
```

---

## Chunk 5: Wire MCP Server into HTTP Server

### Task 7: Update ServerConfig and server.go

**Files:**
- Modify: `internal/daemon/server.go`

- [ ] **Step 1: Replace the MCP placeholder with the real handler**

Replace the entire contents of `internal/daemon/server.go` with:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind         string
	Port         int
	Health       *HealthService
	Store        domain.Store
	Provisioning *ProvisioningService
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// MCP server — handles POST /mcp, GET /mcp (SSE), DELETE /mcp (session end).
	mcpHandler := NewMCPHandler(MCPDeps{
		Store:        cfg.Store,
		Provisioning: cfg.Provisioning,
	})
	mux.Handle("/mcp", mcpHandler)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// ListenAndServe starts the server on the configured address.
func ListenAndServe(srv *http.Server) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	fmt.Printf("Hive daemon listening on %s\n", ln.Addr())
	return srv.Serve(ln)
}
```

Note the key changes from the placeholder:

1. `ServerConfig` now includes `Provisioning *ProvisioningService` (added field).
2. The `POST /mcp` placeholder is replaced with `mux.Handle("/mcp", mcpHandler)` — using `Handle` (not `HandleFunc`) because `mcpHandler` is an `http.Handler`. The pattern `/mcp` without a method prefix allows the mcp-go StreamableHTTPServer to handle POST, GET, and DELETE internally.

### Task 8: Update daemon.go to create ProvisioningService and pass it to ServerConfig

**Files:**
- Modify: `cmd/daemon/daemon.go`

- [ ] **Step 1: Update daemon run function**

Replace the entire contents of `cmd/daemon/daemon.go` with:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/internal/config"
	hiveDaemon "github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/infra"
)

// NewCmd returns the daemon cobra command.
func NewCmd() *cobra.Command {
	var bind string
	var port int
	var db string
	var apiKey string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Hive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("bind") {
				bind = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("db") {
				db = viper.GetString("db")
			}
			if !cmd.Flags().Changed("api-key") {
				apiKey = viper.GetString("api-key")
			}
			return run(bind, port, db, apiKey)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", 17000, "Listen port")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN (postgres://... or SQLite file path)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for REST authentication")

	return cmd
}

func run(bind string, port int, db, apiKey string) error {
	health := hiveDaemon.NewHealthService()

	// Database
	dsn := db
	if dsn == "" {
		dsn = filepath.Join(config.GlobalPaths.StateDir, "hive.db")
	}

	store, err := infra.Open(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	// Seed permissions
	permNames := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), permNames); err != nil {
		return fmt.Errorf("seed permissions: %w", err)
	}

	// Provisioning service
	maxRoleDepth := viper.GetInt("max-role-depth")
	if maxRoleDepth <= 0 {
		maxRoleDepth = config.DefaultMaxRoleDepth
	}
	provisioning := hiveDaemon.NewProvisioningService(store, health, maxRoleDepth)

	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:         bind,
		Port:         port,
		Health:       health,
		Store:        store,
		Provisioning: provisioning,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := hiveDaemon.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http shutdown", "err", err)
	}

	return nil
}
```

- [ ] **Step 2: Verify full build**

```bash
go build ./...
```

Expected: clean build. The daemon now creates a `ProvisioningService` and passes it to `ServerConfig`, which wires it into the MCP handler.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/server.go cmd/daemon/daemon.go
git commit -m "feat: wire MCP server into HTTP server, replacing 501 placeholder"
```

---

## Chunk 6: Tests

### Task 9: Unit tests for tool handlers

**Files:**
- Create: `internal/daemon/mcp_tools_test.go`

- [ ] **Step 1: Create test file with helpers and provisioning test**

Create `internal/daemon/mcp_tools_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/Work-Fort/Hive/internal/domain"
)

// testSetup creates a store, seeds it with a team/agent/permissions, and
// returns the deps and agent for use in tool handler tests.
func testSetup(t *testing.T) (MCPDeps, *domain.Agent, context.Context) {
	t.Helper()

	store := openTestStore(t)

	// Create team.
	team := &domain.Team{Name: "test-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	// Create agent.
	agent := &domain.Agent{Name: "test-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Seed permissions.
	permNames := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), permNames); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	// Grant all permissions to the test agent (global scope).
	perms := make([]domain.AgentPermission, len(permNames))
	for i, name := range permNames {
		// We need the permission ID, but SetAgentPermissions takes
		// AgentPermission with PermissionID. The SeedPermissions
		// method creates them with generated IDs. We need to use
		// HasPermission which checks by name, so we can grant by
		// using a well-known structure.
		//
		// Actually, looking at the store interface, SetAgentPermissions
		// takes []AgentPermission which has PermissionID. We need to
		// look up the permission IDs. But the store doesn't expose
		// ListPermissions. Instead, we'll use the permission names
		// directly since the SQLite implementation resolves names.
		//
		// For testing, we grant all permissions. The simplest approach
		// is to use GetAgentPermissions after setting them, but we
		// need IDs first. Let's use a different approach: seed all
		// perms and then use the SQL store's internal behavior where
		// SetAgentPermissions uses permission names in the ID field.
		//
		// Looking at the actual store implementation more carefully,
		// SetAgentPermissions takes PermissionID which is the actual
		// UUID. We need a helper to look up IDs by name.
		//
		// The cleanest approach for tests: query permission IDs
		// from the store. Since PermissionStore doesn't expose a
		// lookup-by-name method, we'll use HasPermission to verify
		// grants work, and grant via direct SQL in tests.
		_ = i
		_ = name
	}

	// Grant permissions via the store. Since the PermissionStore interface
	// uses SetAgentPermissions with PermissionID (the UUID), and we need
	// the IDs from SeedPermissions, the test helper queries them directly.
	grantAllPermissions(t, store, agent.ID)

	health := NewHealthService()
	provisioning := NewProvisioningService(store, health, 10)

	deps := MCPDeps{
		Store:        store,
		Provisioning: provisioning,
	}

	ctx := contextWithAgentID(context.Background(), agent.ID)

	return deps, agent, ctx
}

// grantAllPermissions grants all seeded permissions to an agent using
// the store's SetAgentPermissions. This requires looking up permission
// IDs from the database.
func grantAllPermissions(t *testing.T, store domain.Store, agentID string) {
	t.Helper()

	// Use GetAgentPermissions to verify after setting.
	// The permission IDs need to come from the database. Since the
	// PermissionStore doesn't expose a ListPermissions method, we'll
	// use a type assertion to access the underlying SQLite store for
	// test setup. In a real test, we'd add a test helper method.
	//
	// Alternative: use the HasPermission check which looks up by name.
	// For granting, we need the permission table IDs.
	//
	// Simplest test approach: execute raw SQL via the store.
	// But we're writing package-level tests in daemon, not infra.
	//
	// Best approach: Add a test-only helper that uses the store.
	// Since HasPermission checks by name, we need SetAgentPermissions
	// to accept name-based grants.
	//
	// Looking at the PermissionStore interface:
	//   SetAgentPermissions(ctx, agentID string, perms []AgentPermission) error
	//   HasPermission(ctx, agentID, permName, scopeTeamID string) (bool, error)
	//
	// The AgentPermission struct has PermissionID (the ID from permissions table).
	// We need those IDs.
	//
	// For this test, we'll use a different strategy: create a test
	// helper that opens the SQLite database directly and runs the
	// grant SQL.
	ctx := context.Background()

	// We need to query permission IDs. The cleanest way is to use
	// the database directly. Since we're in the daemon package (not
	// infra), we rely on the Store interface.
	//
	// Approach: GetAgentPermissions returns what's granted. We need
	// to set them. SetAgentPermissions needs PermissionID values.
	//
	// Since the test store is SQLite, we can use a type assertion
	// or a test-only interface. For pragmatism, we'll define a
	// PermissionSeeder interface that the SQLite store satisfies.

	type permissionQuerier interface {
		QueryPermissionIDs(ctx context.Context, names []string) (map[string]string, error)
	}

	// If the store supports direct permission ID queries, use that.
	// Otherwise, fall back to granting each permission individually
	// using the store methods.
	//
	// For now, we use a simpler approach: call SeedPermissions
	// (which is idempotent) and then construct AgentPermission entries
	// assuming the store implementation generates deterministic IDs.
	//
	// Actually, the simplest real approach: add a GrantAllForTest helper
	// to the infra test package. But since we're in daemon_test...
	//
	// Let's just use the store directly by querying the DB.
	// The sqlite store embeds *sql.DB. We can use a test helper
	// in the infra package that exposes this.
	//
	// PRAGMATIC SOLUTION: Add a test-only method to open the store
	// and grant permissions in one shot.

	// For now, use a compile-time interface check.
	if gs, ok := store.(interface {
		GrantPermissionByName(ctx context.Context, agentID, permName, scopeTeamID string) error
	}); ok {
		for _, name := range []string{
			"role:read", "role:write",
			"memory:read", "memory:write",
			"task:read", "task:write",
		} {
			if err := gs.GrantPermissionByName(ctx, agentID, name, ""); err != nil {
				t.Fatalf("grant permission %q: %v", name, err)
			}
		}
	} else {
		t.Fatal("store does not support GrantPermissionByName; add test helper to infra package")
	}
}

// openTestStore opens an in-memory SQLite store for testing.
// This relies on infra.Open(":memory:") or a temp file.
func openTestStore(t *testing.T) domain.Store {
	t.Helper()

	// Use a temp file for SQLite since :memory: databases don't
	// share across connections.
	tmpDir := t.TempDir()
	dsn := tmpDir + "/test.db"

	// Import infra package to open the store.
	// Since we're in the daemon package, we import infra.
	store, err := openStore(dsn)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// openStore is a test seam — it opens a store via infra.Open.
// Defined here so the test file can import infra without polluting
// the main daemon package.
//
// NOTE: This function is defined in a _test.go file, so it only
// exists during testing. We import infra here, which is fine for
// tests but would be a layering violation in production code.
var openStore = func(dsn string) (domain.Store, error) {
	// This will be replaced in the actual test file with:
	//   infra.Open(dsn)
	// For now, return an error to indicate it needs wiring.
	return nil, fmt.Errorf("openStore not wired — import infra in test file")
}
```

The above test file has a bootstrapping problem: the `daemon` package cannot import `infra` in production code (layering violation), but tests need a real store. The standard pattern is a `_test.go` file that bridges the gap.

- [ ] **Step 2: Create test bridge file**

Create `internal/daemon/test_helpers_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra"
)

func init() {
	// Wire the test store opener to use the real infra package.
	openStore = func(dsn string) (domain.Store, error) {
		return infra.Open(dsn)
	}
}
```

- [ ] **Step 3: No extra infra code needed for test permission grants**

The existing `SetAgentPermissions` method already accepts permission **names** in the `PermissionID` field and resolves them to UUIDs internally. The test helper can use it directly by constructing `[]domain.AgentPermission` with permission names. No `GrantPermissionByName` method is needed.

For reference, the `agent_permissions` table schema (from `001_init.sql`) has no separate `id` column:

```sql
CREATE TABLE agent_permissions (
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id),
    scope_team_id TEXT REFERENCES teams(id),
    PRIMARY KEY (agent_id, permission_id, scope_team_id)
);
```

And `SetAgentPermissions` resolves names to IDs internally, uses `*string` nil for global scope.

- [ ] **Step 4: Rewrite the test file cleanly**

Replace `internal/daemon/mcp_tools_test.go` with this cleaner version:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Work-Fort/Hive/internal/domain"
)

// testEnv holds the test environment for MCP tool handler tests.
type testEnv struct {
	deps  MCPDeps
	agent *domain.Agent
	team  *domain.Team
	ctx   context.Context
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	store := openTestStore(t)

	team := &domain.Team{Name: "test-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := &domain.Agent{Name: "test-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Seed and grant permissions.
	allPerms := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), allPerms); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	// Grant all permissions to the test agent using SetAgentPermissions.
	// The PermissionID field accepts permission names — the store resolves
	// them to UUIDs internally. ScopeTeamID="" means global scope.
	grantPermissions(t, store, agent.ID, allPerms)

	health := NewHealthService()
	provisioning := NewProvisioningService(store, health, 10)

	deps := MCPDeps{
		Store:        store,
		Provisioning: provisioning,
	}

	ctx := contextWithAgentID(context.Background(), agent.ID)

	return &testEnv{deps: deps, agent: agent, team: team, ctx: ctx}
}

func openTestStore(t *testing.T) domain.Store {
	t.Helper()
	dsn := t.TempDir() + "/test.db"
	store, err := openStore(dsn)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// grantPermissions grants the named permissions to the agent.
// Uses SetAgentPermissions which resolves permission names to IDs internally.
func grantPermissions(t *testing.T, store domain.Store, agentID string, permNames []string) {
	t.Helper()
	perms := make([]domain.AgentPermission, len(permNames))
	for i, name := range permNames {
		perms[i] = domain.AgentPermission{
			AgentID:      agentID,
			PermissionID: name, // SetAgentPermissions resolves name -> UUID
			ScopeTeamID:  "",   // global scope
		}
	}
	if err := store.SetAgentPermissions(context.Background(), agentID, perms); err != nil {
		t.Fatalf("grant permissions: %v", err)
	}
}

// callTool is a test helper that invokes a tool handler function.
func callTool(t *testing.T, handler server.ToolHandlerFunc, ctx context.Context, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("tool call returned error: %v", err)
	}
	return result
}

// resultText extracts the text content from an MCP tool result.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	// Content is an interface — extract the text.
	data, err := json.Marshal(result.Content[0])
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	var tc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &tc); err != nil {
		t.Fatalf("unmarshal text content: %v", err)
	}
	return tc.Text
}

// unmarshalResult unmarshals the JSON text content of a tool result.
func unmarshalResult(t *testing.T, result *mcp.CallToolResult, v any) {
	t.Helper()
	text := resultText(t, result)
	if err := json.Unmarshal([]byte(text), v); err != nil {
		t.Fatalf("unmarshal result JSON: %v\nraw: %s", err, text)
	}
}

// --- Tests ---

func TestGetProvisioning(t *testing.T) {
	env := setupTestEnv(t)

	handler := makeGetProvisioning(env.deps)
	result := callTool(t, handler, env.ctx, nil)

	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, result))
	}

	var resp domain.ProvisioningResponse
	unmarshalResult(t, result, &resp)

	// Agent has no roles assigned, so roles should be empty.
	if len(resp.Roles) != 0 {
		t.Errorf("expected 0 role groups, got %d", len(resp.Roles))
	}
	// Agent has no memory yet.
	if len(resp.Memory) != 0 {
		t.Errorf("expected 0 memory docs, got %d", len(resp.Memory))
	}
}

func TestMemoryCRUD(t *testing.T) {
	env := setupTestEnv(t)

	// Create
	createHandler := makeCreateMemory(env.deps)
	createResult := callTool(t, createHandler, env.ctx, map[string]any{
		"title":   "Test Memory",
		"content": "Some content here",
	})
	if createResult.IsError {
		t.Fatalf("create failed: %s", resultText(t, createResult))
	}

	var created memoryDocResponse
	unmarshalResult(t, createResult, &created)

	if created.Title != "Test Memory" {
		t.Errorf("title = %q, want %q", created.Title, "Test Memory")
	}
	if created.Content != "Some content here" {
		t.Errorf("content = %q, want %q", created.Content, "Some content here")
	}
	if created.ID == "" {
		t.Error("created document has empty ID")
	}

	// List
	listHandler := makeListMemory(env.deps)
	listResult := callTool(t, listHandler, env.ctx, nil)
	if listResult.IsError {
		t.Fatalf("list failed: %s", resultText(t, listResult))
	}

	var listed []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	unmarshalResult(t, listResult, &listed)
	if len(listed) != 1 {
		t.Fatalf("expected 1 document, got %d", len(listed))
	}
	if listed[0].ID != created.ID {
		t.Errorf("listed ID = %q, want %q", listed[0].ID, created.ID)
	}

	// Get
	getHandler := makeGetMemory(env.deps)
	getResult := callTool(t, getHandler, env.ctx, map[string]any{
		"id": created.ID,
	})
	if getResult.IsError {
		t.Fatalf("get failed: %s", resultText(t, getResult))
	}

	var got memoryDocResponse
	unmarshalResult(t, getResult, &got)
	if got.Content != "Some content here" {
		t.Errorf("content = %q, want %q", got.Content, "Some content here")
	}

	// Update
	updateHandler := makeUpdateMemory(env.deps)
	updateResult := callTool(t, updateHandler, env.ctx, map[string]any{
		"id":      created.ID,
		"title":   "Updated Title",
		"content": "Updated content",
	})
	if updateResult.IsError {
		t.Fatalf("update failed: %s", resultText(t, updateResult))
	}

	var updated memoryDocResponse
	unmarshalResult(t, updateResult, &updated)
	if updated.Title != "Updated Title" {
		t.Errorf("title = %q, want %q", updated.Title, "Updated Title")
	}

	// Delete
	deleteHandler := makeDeleteMemory(env.deps)
	deleteResult := callTool(t, deleteHandler, env.ctx, map[string]any{
		"id": created.ID,
	})
	if deleteResult.IsError {
		t.Fatalf("delete failed: %s", resultText(t, deleteResult))
	}

	// Verify deleted
	getResult2 := callTool(t, getHandler, env.ctx, map[string]any{
		"id": created.ID,
	})
	if !getResult2.IsError {
		t.Error("expected error getting deleted document")
	}
}

func TestMemoryIsolation(t *testing.T) {
	env := setupTestEnv(t)

	// Create a second agent on a different team.
	team2 := &domain.Team{Name: "other-team"}
	if err := env.deps.Store.CreateTeam(context.Background(), team2); err != nil {
		t.Fatalf("create team2: %v", err)
	}
	agent2 := &domain.Agent{Name: "other-agent", TeamID: team2.ID}
	if err := env.deps.Store.CreateAgent(context.Background(), agent2); err != nil {
		t.Fatalf("create agent2: %v", err)
	}

	// Create memory as agent 1.
	createHandler := makeCreateMemory(env.deps)
	createResult := callTool(t, createHandler, env.ctx, map[string]any{
		"title":   "Agent1 Memory",
		"content": "Secret stuff",
	})
	var created memoryDocResponse
	unmarshalResult(t, createResult, &created)

	// Try to read agent1's memory as agent2.
	ctx2 := contextWithAgentID(context.Background(), agent2.ID)
	getHandler := makeGetMemory(env.deps)
	getResult := callTool(t, getHandler, ctx2, map[string]any{
		"id": created.ID,
	})
	if !getResult.IsError {
		t.Error("agent2 should not be able to read agent1's memory")
	}
}

func TestTaskCRUD(t *testing.T) {
	env := setupTestEnv(t)

	// Create
	createHandler := makeCreateTask(env.deps)
	createResult := callTool(t, createHandler, env.ctx, map[string]any{
		"title":       "Fix bug",
		"description": "There's a bug in the login flow",
	})
	if createResult.IsError {
		t.Fatalf("create failed: %s", resultText(t, createResult))
	}

	var created taskResponse
	unmarshalResult(t, createResult, &created)

	if created.Title != "Fix bug" {
		t.Errorf("title = %q, want %q", created.Title, "Fix bug")
	}
	if created.Status != "pending" {
		t.Errorf("status = %q, want %q", created.Status, "pending")
	}
	if created.TeamID != env.team.ID {
		t.Errorf("team_id = %q, want %q", created.TeamID, env.team.ID)
	}

	// List
	listHandler := makeListTasks(env.deps)
	listResult := callTool(t, listHandler, env.ctx, nil)
	if listResult.IsError {
		t.Fatalf("list failed: %s", resultText(t, listResult))
	}

	var tasks []taskResponse
	unmarshalResult(t, listResult, &tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	// Get
	getHandler := makeGetTask(env.deps)
	getResult := callTool(t, getHandler, env.ctx, map[string]any{
		"id": created.ID,
	})
	if getResult.IsError {
		t.Fatalf("get failed: %s", resultText(t, getResult))
	}

	// Update status
	updateHandler := makeUpdateTask(env.deps)
	updateResult := callTool(t, updateHandler, env.ctx, map[string]any{
		"id":     created.ID,
		"status": "in_progress",
	})
	if updateResult.IsError {
		t.Fatalf("update failed: %s", resultText(t, updateResult))
	}

	var updated taskResponse
	unmarshalResult(t, updateResult, &updated)
	if updated.Status != "in_progress" {
		t.Errorf("status = %q, want %q", updated.Status, "in_progress")
	}

	// Delete
	deleteHandler := makeDeleteTask(env.deps)
	deleteResult := callTool(t, deleteHandler, env.ctx, map[string]any{
		"id": created.ID,
	})
	if deleteResult.IsError {
		t.Fatalf("delete failed: %s", resultText(t, deleteResult))
	}

	// Verify deleted
	getResult2 := callTool(t, getHandler, env.ctx, map[string]any{
		"id": created.ID,
	})
	if !getResult2.IsError {
		t.Error("expected error getting deleted task")
	}
}

func TestTaskTeamIsolation(t *testing.T) {
	env := setupTestEnv(t)

	// Create task on agent's team.
	createHandler := makeCreateTask(env.deps)
	createResult := callTool(t, createHandler, env.ctx, map[string]any{
		"title": "Team1 Task",
	})
	var created taskResponse
	unmarshalResult(t, createResult, &created)

	// Create a second agent on a different team.
	team2 := &domain.Team{Name: "other-team"}
	if err := env.deps.Store.CreateTeam(context.Background(), team2); err != nil {
		t.Fatalf("create team2: %v", err)
	}
	agent2 := &domain.Agent{Name: "other-agent", TeamID: team2.ID}
	if err := env.deps.Store.CreateAgent(context.Background(), agent2); err != nil {
		t.Fatalf("create agent2: %v", err)
	}

	// Try to read team1's task as agent2.
	ctx2 := contextWithAgentID(context.Background(), agent2.ID)
	getHandler := makeGetTask(env.deps)
	getResult := callTool(t, getHandler, ctx2, map[string]any{
		"id": created.ID,
	})
	if !getResult.IsError {
		t.Error("agent2 should not be able to read team1's task")
	}
}

func TestToolHandlerMissingAgentID(t *testing.T) {
	env := setupTestEnv(t)

	// Call with empty context (no agent ID).
	ctx := context.Background()
	handler := makeListMemory(env.deps)
	result := callTool(t, handler, ctx, nil)

	if !result.IsError {
		t.Error("expected error when agent ID is missing")
	}
}

func TestUpdateTaskInvalidStatus(t *testing.T) {
	env := setupTestEnv(t)

	// Create a task first.
	createHandler := makeCreateTask(env.deps)
	createResult := callTool(t, createHandler, env.ctx, map[string]any{
		"title": "Status Test",
	})
	var created taskResponse
	unmarshalResult(t, createResult, &created)

	// Try to update with invalid status.
	updateHandler := makeUpdateTask(env.deps)
	updateResult := callTool(t, updateHandler, env.ctx, map[string]any{
		"id":     created.ID,
		"status": "invalid_status",
	})
	if !updateResult.IsError {
		t.Error("expected error for invalid status")
	}
}
```

Note: The `callTool` helper references `server.ToolHandlerFunc`. Add this import:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Work-Fort/Hive/internal/domain"
)
```

Remove the unused `fmt` import if not needed. The `callTool` function uses `server.ToolHandlerFunc` as the parameter type — this is the type alias `func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)`.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/daemon/ -v -run TestMemory
go test ./internal/daemon/ -v -run TestTask
go test ./internal/daemon/ -v -run TestGetProvisioning
go test ./internal/daemon/ -v -run TestToolHandler
```

Expected: all tests pass. If `GrantPermissionByName` doesn't exist on the store yet, the test will fail with a clear message — implement the method first (see Task 3, Step 3 in this chunk).

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/mcp_tools_test.go internal/daemon/test_helpers_test.go internal/infra/sqlite/permissions_helper.go
git commit -m "test: add MCP tool handler tests with memory/task CRUD and isolation"
```

---

## Chunk 7: Permission-Based Tool Filtering Test

### Task 10: Test that tool filtering works based on permissions

**Files:**
- Modify: `internal/daemon/mcp_tools_test.go`

- [ ] **Step 1: Add permission filtering tests**

Append to `internal/daemon/mcp_tools_test.go`:

```go
func TestToolFilterHidesUnpermittedTools(t *testing.T) {
	store := openTestStore(t)

	team := &domain.Team{Name: "filter-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := &domain.Agent{Name: "limited-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	permNames := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), permNames); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	// Grant ONLY memory:read — no other permissions.
	grantPermissions(t, store, agent.ID, []string{"memory:read"})

	ctx := contextWithAgentID(context.Background(), agent.ID)

	// Build the full tool list.
	allTools := []mcp.Tool{
		mcp.NewTool("get_provisioning"),
		mcp.NewTool("list_memory"),
		mcp.NewTool("get_memory"),
		mcp.NewTool("create_memory"),
		mcp.NewTool("update_memory"),
		mcp.NewTool("delete_memory"),
		mcp.NewTool("list_tasks"),
		mcp.NewTool("get_task"),
		mcp.NewTool("create_task"),
		mcp.NewTool("update_task"),
		mcp.NewTool("delete_task"),
	}

	filter := toolFilter(store)
	filtered := filter(ctx, allTools)

	// With only memory:read, should see list_memory and get_memory.
	// get_provisioning requires role:read + memory:read, so it should be hidden.
	expectedNames := map[string]bool{
		"list_memory": true,
		"get_memory":  true,
	}

	if len(filtered) != len(expectedNames) {
		names := make([]string, len(filtered))
		for i, tool := range filtered {
			names[i] = tool.Name
		}
		t.Fatalf("expected %d tools, got %d: %v", len(expectedNames), len(filtered), names)
	}

	for _, tool := range filtered {
		if !expectedNames[tool.Name] {
			t.Errorf("unexpected tool in filtered list: %s", tool.Name)
		}
	}
}

func TestToolFilterNoPermissions(t *testing.T) {
	store := openTestStore(t)

	team := &domain.Team{Name: "noperm-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := &domain.Agent{Name: "noperm-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	permNames := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), permNames); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	// No permissions granted.
	ctx := contextWithAgentID(context.Background(), agent.ID)

	allTools := []mcp.Tool{
		mcp.NewTool("list_memory"),
		mcp.NewTool("list_tasks"),
	}

	filter := toolFilter(store)
	filtered := filter(ctx, allTools)

	if len(filtered) != 0 {
		t.Errorf("expected 0 tools, got %d", len(filtered))
	}
}

func TestToolFilterMissingAgentID(t *testing.T) {
	store := openTestStore(t)

	// Empty context — no agent ID.
	ctx := context.Background()

	allTools := []mcp.Tool{
		mcp.NewTool("list_memory"),
	}

	filter := toolFilter(store)
	filtered := filter(ctx, allTools)

	if len(filtered) != 0 {
		t.Errorf("expected 0 tools for missing agent, got %d", len(filtered))
	}
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./internal/daemon/ -v
```

Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/mcp_tools_test.go
git commit -m "test: add permission-based tool filtering tests"
```

---

## Chunk 8: Smoke Test

### Task 11: End-to-end smoke test

Manual verification that the MCP server is operational.

- [ ] **Step 1: Build and start daemon**

```bash
go build -o build/hive .
./build/hive daemon --port 17099 &
sleep 1
```

- [ ] **Step 2: Verify health endpoint still works**

```bash
curl -s http://127.0.0.1:17099/v1/health | jq .
```

Expected: `{"status":"healthy"}`

- [ ] **Step 3: Verify MCP endpoint responds to POST**

Send a JSON-RPC initialize request:

```bash
curl -s -X POST http://127.0.0.1:17099/mcp \
  -H "Content-Type: application/json" \
  -H "X-Agent-Id: nonexistent" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | jq .
```

Expected: a JSON-RPC response with server capabilities (the initialize handshake does not require a valid agent — that check happens at tool list/call time).

- [ ] **Step 4: Stop daemon**

```bash
kill %1
```

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 6: Final commit if any loose changes**

```bash
git status
# If any files need cleanup:
go mod tidy
git add go.mod go.sum
git commit -m "chore: tidy go modules after MCP server integration"
```

---

## Summary

| File | Action | Description |
|---|---|---|
| `go.mod` | Modify | Add `github.com/mark3labs/mcp-go` dependency |
| `internal/daemon/session.go` | Create | Agent ID extraction from HTTP context, agent resolution |
| `internal/daemon/mcp_server.go` | Create | MCP server setup, tool registration, permission-based tool filter |
| `internal/daemon/mcp_tools.go` | Create | 11 tool handler implementations (provisioning, memory CRUD, task CRUD) |
| `internal/daemon/mcp_tools_test.go` | Create | Unit tests for all tool handlers, isolation tests, filter tests |
| `internal/daemon/test_helpers_test.go` | Create | Test bridge to wire infra.Open for daemon package tests |
| `internal/infra/sqlite/permissions_helper.go` | Create | `GrantPermissionByName` convenience method for tests |
| `internal/daemon/server.go` | Modify | Replace MCP 501 placeholder with real mcp-go handler |
| `cmd/daemon/daemon.go` | Modify | Create ProvisioningService, pass to ServerConfig |
