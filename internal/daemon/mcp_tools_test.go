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
	store, err := openTestStore("")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	team := &domain.Team{ID: NewID("tm"), Name: "test-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := &domain.Agent{ID: NewID("ag"), Name: "test-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Seed and grant all permissions.
	allPerms := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), allPerms); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	perms := make([]domain.AgentPermission, len(allPerms))
	for i, name := range allPerms {
		perms[i] = domain.AgentPermission{
			AgentID:      agent.ID,
			PermissionID: name, // SetAgentPermissions resolves name -> UUID
			ScopeTeamID:  "",   // global scope
		}
	}
	if err := store.SetAgentPermissions(context.Background(), agent.ID, perms); err != nil {
		t.Fatalf("grant permissions: %v", err)
	}

	health := NewHealthService()
	provisioning := NewProvisioningService(store, health, 10)

	deps := MCPDeps{
		Store:        store,
		Provisioning: provisioning,
	}

	ctx := contextWithAgentID(context.Background(), agent.ID)
	return &testEnv{deps: deps, agent: agent, team: team, ctx: ctx}
}

// callTool invokes a tool handler function.
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
	tc, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatal("first content block is not TextContent")
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

// --- Test functions ---

func TestGetProvisioning(t *testing.T) {
	env := setupTestEnv(t)
	handler := makeGetProvisioning(env.deps)

	result := callTool(t, handler, env.ctx, nil)
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}

	var resp domain.ProvisioningResponse
	unmarshalResult(t, result, &resp)

	if len(resp.Roles) != 0 {
		t.Errorf("got %d role groups, want 0 (agent has no role assignments)", len(resp.Roles))
	}
	if len(resp.Memory) != 0 {
		t.Errorf("got %d memory docs, want 0", len(resp.Memory))
	}
}

func TestMemoryCRUD(t *testing.T) {
	env := setupTestEnv(t)

	// Create
	createHandler := makeCreateMemory(env.deps)
	result := callTool(t, createHandler, env.ctx, map[string]any{
		"title":   "Test Note",
		"content": "Hello world",
	})
	if result.IsError {
		t.Fatalf("create: expected success, got error: %s", resultText(t, result))
	}

	var created memoryDocResponse
	unmarshalResult(t, result, &created)

	if created.ID == "" {
		t.Fatal("create: got empty ID")
	}
	if created.Title != "Test Note" {
		t.Errorf("create: got title %q, want %q", created.Title, "Test Note")
	}
	if created.Content != "Hello world" {
		t.Errorf("create: got content %q, want %q", created.Content, "Hello world")
	}

	// List
	listHandler := makeListMemory(env.deps)
	result = callTool(t, listHandler, env.ctx, nil)
	if result.IsError {
		t.Fatalf("list: expected success, got error: %s", resultText(t, result))
	}

	var items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	unmarshalResult(t, result, &items)

	if len(items) != 1 {
		t.Fatalf("list: got %d items, want 1", len(items))
	}
	if items[0].ID != created.ID {
		t.Errorf("list: got ID %q, want %q", items[0].ID, created.ID)
	}

	// Get
	getHandler := makeGetMemory(env.deps)
	result = callTool(t, getHandler, env.ctx, map[string]any{"id": created.ID})
	if result.IsError {
		t.Fatalf("get: expected success, got error: %s", resultText(t, result))
	}

	var fetched memoryDocResponse
	unmarshalResult(t, result, &fetched)

	if fetched.Content != "Hello world" {
		t.Errorf("get: got content %q, want %q", fetched.Content, "Hello world")
	}

	// Update
	updateHandler := makeUpdateMemory(env.deps)
	result = callTool(t, updateHandler, env.ctx, map[string]any{
		"id":    created.ID,
		"title": "Updated Note",
	})
	if result.IsError {
		t.Fatalf("update: expected success, got error: %s", resultText(t, result))
	}

	var updated memoryDocResponse
	unmarshalResult(t, result, &updated)

	if updated.Title != "Updated Note" {
		t.Errorf("update: got title %q, want %q", updated.Title, "Updated Note")
	}

	// Delete
	deleteHandler := makeDeleteMemory(env.deps)
	result = callTool(t, deleteHandler, env.ctx, map[string]any{"id": created.ID})
	if result.IsError {
		t.Fatalf("delete: expected success, got error: %s", resultText(t, result))
	}

	// Verify get returns error after delete
	result = callTool(t, getHandler, env.ctx, map[string]any{"id": created.ID})
	if !result.IsError {
		t.Error("get after delete: expected isError, got success")
	}
}

func TestMemoryIsolation(t *testing.T) {
	env := setupTestEnv(t)

	// Create memory as agent1
	createHandler := makeCreateMemory(env.deps)
	result := callTool(t, createHandler, env.ctx, map[string]any{
		"title":   "Secret Note",
		"content": "agent1 only",
	})
	if result.IsError {
		t.Fatalf("create: expected success, got error: %s", resultText(t, result))
	}

	var created memoryDocResponse
	unmarshalResult(t, result, &created)

	// Create agent2 on a different team
	team2 := &domain.Team{ID: NewID("tm"), Name: "other-team"}
	if err := env.deps.Store.CreateTeam(context.Background(), team2); err != nil {
		t.Fatalf("create team2: %v", err)
	}

	agent2 := &domain.Agent{ID: NewID("ag"), Name: "agent2", TeamID: team2.ID}
	if err := env.deps.Store.CreateAgent(context.Background(), agent2); err != nil {
		t.Fatalf("create agent2: %v", err)
	}

	ctx2 := contextWithAgentID(context.Background(), agent2.ID)

	// Verify agent2 cannot get agent1's memory
	getHandler := makeGetMemory(env.deps)
	result = callTool(t, getHandler, ctx2, map[string]any{"id": created.ID})
	if !result.IsError {
		t.Error("get as agent2: expected isError (isolation violation), got success")
	}
}

func TestTaskCRUD(t *testing.T) {
	env := setupTestEnv(t)

	// Create
	createHandler := makeCreateTask(env.deps)
	result := callTool(t, createHandler, env.ctx, map[string]any{
		"title": "Test Task",
	})
	if result.IsError {
		t.Fatalf("create: expected success, got error: %s", resultText(t, result))
	}

	var created taskResponse
	unmarshalResult(t, result, &created)

	if created.ID == "" {
		t.Fatal("create: got empty ID")
	}
	if created.Title != "Test Task" {
		t.Errorf("create: got title %q, want %q", created.Title, "Test Task")
	}
	if created.Status != "pending" {
		t.Errorf("create: got status %q, want %q", created.Status, "pending")
	}
	if created.TeamID != env.team.ID {
		t.Errorf("create: got teamID %q, want %q", created.TeamID, env.team.ID)
	}

	// List
	listHandler := makeListTasks(env.deps)
	result = callTool(t, listHandler, env.ctx, nil)
	if result.IsError {
		t.Fatalf("list: expected success, got error: %s", resultText(t, result))
	}

	var items []taskResponse
	unmarshalResult(t, result, &items)

	if len(items) != 1 {
		t.Fatalf("list: got %d items, want 1", len(items))
	}

	// Get
	getHandler := makeGetTask(env.deps)
	result = callTool(t, getHandler, env.ctx, map[string]any{"id": created.ID})
	if result.IsError {
		t.Fatalf("get: expected success, got error: %s", resultText(t, result))
	}

	// Update status
	updateHandler := makeUpdateTask(env.deps)
	result = callTool(t, updateHandler, env.ctx, map[string]any{
		"id":     created.ID,
		"status": "in_progress",
	})
	if result.IsError {
		t.Fatalf("update: expected success, got error: %s", resultText(t, result))
	}

	var updated taskResponse
	unmarshalResult(t, result, &updated)

	if updated.Status != "in_progress" {
		t.Errorf("update: got status %q, want %q", updated.Status, "in_progress")
	}

	// Delete
	deleteHandler := makeDeleteTask(env.deps)
	result = callTool(t, deleteHandler, env.ctx, map[string]any{"id": created.ID})
	if result.IsError {
		t.Fatalf("delete: expected success, got error: %s", resultText(t, result))
	}

	// Verify get returns error after delete
	result = callTool(t, getHandler, env.ctx, map[string]any{"id": created.ID})
	if !result.IsError {
		t.Error("get after delete: expected isError, got success")
	}
}

func TestTaskTeamIsolation(t *testing.T) {
	env := setupTestEnv(t)

	// Create task on agent1's team
	createHandler := makeCreateTask(env.deps)
	result := callTool(t, createHandler, env.ctx, map[string]any{
		"title": "Team1 Task",
	})
	if result.IsError {
		t.Fatalf("create: expected success, got error: %s", resultText(t, result))
	}

	var created taskResponse
	unmarshalResult(t, result, &created)

	// Create agent2 on a different team
	team2 := &domain.Team{ID: NewID("tm"), Name: "other-team"}
	if err := env.deps.Store.CreateTeam(context.Background(), team2); err != nil {
		t.Fatalf("create team2: %v", err)
	}

	agent2 := &domain.Agent{ID: NewID("ag"), Name: "agent2", TeamID: team2.ID}
	if err := env.deps.Store.CreateAgent(context.Background(), agent2); err != nil {
		t.Fatalf("create agent2: %v", err)
	}

	ctx2 := contextWithAgentID(context.Background(), agent2.ID)

	// Verify agent2 cannot get team1's task
	getHandler := makeGetTask(env.deps)
	result = callTool(t, getHandler, ctx2, map[string]any{"id": created.ID})
	if !result.IsError {
		t.Error("get as agent2: expected isError (team isolation violation), got success")
	}
}

func TestToolHandlerMissingAgentID(t *testing.T) {
	env := setupTestEnv(t)

	// Call with empty context (no agent ID)
	handler := makeListMemory(env.deps)
	result := callTool(t, handler, context.Background(), nil)
	if !result.IsError {
		t.Error("expected isError for missing agent ID, got success")
	}
}

func TestUpdateTaskInvalidStatus(t *testing.T) {
	env := setupTestEnv(t)

	// Create a task first
	createHandler := makeCreateTask(env.deps)
	result := callTool(t, createHandler, env.ctx, map[string]any{
		"title": "Status Test Task",
	})
	if result.IsError {
		t.Fatalf("create: expected success, got error: %s", resultText(t, result))
	}

	var created taskResponse
	unmarshalResult(t, result, &created)

	// Try to update with invalid status
	updateHandler := makeUpdateTask(env.deps)
	result = callTool(t, updateHandler, env.ctx, map[string]any{
		"id":     created.ID,
		"status": "invalid_status",
	})
	if !result.IsError {
		t.Error("expected isError for invalid status, got success")
	}
}

func TestToolFilterHidesUnpermittedTools(t *testing.T) {
	store, err := openTestStore("")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	team := &domain.Team{ID: NewID("tm"), Name: "filter-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := &domain.Agent{ID: NewID("ag"), Name: "limited-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Seed all permissions but only grant memory:read
	allPerms := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), allPerms); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}
	if err := store.SetAgentPermissions(context.Background(), agent.ID, []domain.AgentPermission{
		{AgentID: agent.ID, PermissionID: "memory:read", ScopeTeamID: ""},
	}); err != nil {
		t.Fatalf("grant permissions: %v", err)
	}

	// Build the full tool list (all 11 tools)
	allTools := make([]mcp.Tool, 0, len(permissionMap))
	for name := range permissionMap {
		allTools = append(allTools, mcp.NewTool(name))
	}

	ctx := contextWithAgentID(context.Background(), agent.ID)
	filterFn := toolFilter(store)
	filtered := filterFn(ctx, allTools)

	// With only memory:read, the agent should see list_memory and get_memory.
	// get_provisioning requires role:read AND memory:read, so it should be hidden.
	wantTools := map[string]bool{
		"list_memory": true,
		"get_memory":  true,
	}
	gotTools := make(map[string]bool)
	for _, tool := range filtered {
		gotTools[tool.Name] = true
	}

	if len(filtered) != len(wantTools) {
		t.Errorf("got %d filtered tools, want %d; got: %v", len(filtered), len(wantTools), gotTools)
	}
	for name := range wantTools {
		if !gotTools[name] {
			t.Errorf("expected tool %q in filtered list, but not found", name)
		}
	}
}

func TestToolFilterNoPermissions(t *testing.T) {
	store, err := openTestStore("")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	team := &domain.Team{ID: NewID("tm"), Name: "noperm-team"}
	if err := store.CreateTeam(context.Background(), team); err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := &domain.Agent{ID: NewID("ag"), Name: "noperm-agent", TeamID: team.ID}
	if err := store.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Seed permissions but grant none to this agent
	allPerms := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), allPerms); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	allTools := make([]mcp.Tool, 0, len(permissionMap))
	for name := range permissionMap {
		allTools = append(allTools, mcp.NewTool(name))
	}

	ctx := contextWithAgentID(context.Background(), agent.ID)
	filterFn := toolFilter(store)
	filtered := filterFn(ctx, allTools)

	if len(filtered) != 0 {
		names := make([]string, len(filtered))
		for i, tool := range filtered {
			names[i] = tool.Name
		}
		t.Errorf("got %d filtered tools, want 0; got: %v", len(filtered), names)
	}
}

func TestToolFilterMissingAgentID(t *testing.T) {
	store, err := openTestStore("")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	allTools := make([]mcp.Tool, 0, len(permissionMap))
	for name := range permissionMap {
		allTools = append(allTools, mcp.NewTool(name))
	}

	// Empty context — no agent ID
	filterFn := toolFilter(store)
	filtered := filterFn(context.Background(), allTools)

	if filtered != nil {
		t.Errorf("got %d filtered tools, want nil; got: %v", len(filtered), filtered)
	}
}
