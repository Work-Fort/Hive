---
type: plan
step: "4"
title: "Agent pool — get_provisioning honors current assignment"
status: approved
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "4"
dates:
  created: "2026-04-17"
  approved: "2026-04-18"
  completed: null
related_plans:
  - 2026-04-17-agent-pool-01-schema.md
  - 2026-04-17-agent-pool-02-endpoints.md
  - 2026-04-17-agent-pool-03-sweeper.md
---

# Agent Pool — Step 4: `get_provisioning` Honors Current Assignment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When an agent has a `current_assignment`, the MCP `get_provisioning` tool returns role documents for `current_role` and exposes `current_project` as a memory-style context document — instead of the persistent role-set.

**Architecture:** Step 4 (final) in the four-plan series for
`docs/2026-04-18-agent-assignment-schema.md`. Prerequisites: plans
`-01-schema.md` (columns exist) and `-02-endpoints.md` (claim can set
them). Plan `-03-sweeper.md` is not strictly required but recommended so
idle claims don't leak.

The existing `ProvisioningService.Resolve` in
`internal/daemon/provisioning.go` builds a `ProvisioningResponse` from
`GetAgentRoles` + `GetRoleChain` + `ListRoleDocuments` + agent memory.
We extend it: when the fetched agent has `CurrentWorkflowID != ""`,
replace the persistent role lookup with a lookup by name
(`current_role`) and synthesize a memory document describing the
current project. When the agent is free, existing behavior is preserved.

We keep `AgentIdentity` in the response shape stable and add an
optional `CurrentAssignment` block so callers can tell "you are
claimed vs. idle." Existing callers that ignore the new field see
no change.

**Tech Stack:** Go 1.26, Huma, mcp-go, the existing role-chain/document resolution logic.

---

## Conventions (apply to every task)

- Never run `go build`/`go test` directly — use `mise run <task>`.
- Focused unit runs go through `./internal/daemon/...`.
- Commit after each task with Conventional Commits prefixes.

---

### Task 1: Extend `ProvisioningResponse` with an optional `CurrentAssignment` block

**Files:**
- Modify: `internal/domain/types.go`

**Step 1: Add type**

```go
// CurrentAssignment is the runtime assignment context surfaced to an
// agent by get_provisioning. Zero value means the agent is free.
type CurrentAssignment struct {
    Role           string    `json:"role"`
    Project        string    `json:"project"`
    WorkflowID     string    `json:"workflow_id"`
    LeaseExpiresAt time.Time `json:"lease_expires_at"`
}
```

Extend `ProvisioningResponse`:

```go
type ProvisioningResponse struct {
    Agent             AgentIdentity           `json:"agent"`
    CurrentAssignment *CurrentAssignment      `json:"current_assignment,omitempty"`
    Roles             []ProvisioningRoleGroup `json:"roles"`
    Memory            []Document              `json:"memory"`
}
```

`*CurrentAssignment` is a pointer so the field is omitted entirely on
free agents (omitempty works on pointer nil).

**Step 2: Verify compile**

```
mise run lint
```

**Step 3: Commit**

```
git add internal/domain/types.go
git commit -m "feat(domain): add CurrentAssignment to ProvisioningResponse"
```

---

### Task 2: Write failing test — free agent keeps the old shape

**Files:**
- Modify: `internal/daemon/provisioning_test.go`

**Step 1: Add test**

```go
func TestResolve_FreeAgent_NoCurrentAssignment(t *testing.T) {
    ctx := context.Background()
    store, ps := testSetup(t, 10)

    teamID  := seedTeam(t, ctx, store, "alpha")
    agentID := seedAgent(t, ctx, store, "worker-01", teamID)
    roleID  := seedRole(t, ctx, store, "developer", "")
    seedDoc(t, ctx, store, "doc-1", "Dev", "dev content", roleID)

    store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
        {AgentID: agentID, RoleID: roleID, Priority: 1},
    })

    resp, err := ps.Resolve(ctx, agentID)
    if err != nil { t.Fatalf("resolve: %v", err) }

    if resp.CurrentAssignment != nil {
        t.Errorf("CurrentAssignment should be nil for free agent, got %+v", resp.CurrentAssignment)
    }
    if len(resp.Roles) != 1 || resp.Roles[0].Chain[0].Role != "developer" {
        t.Errorf("expected persistent role developer, got %+v", resp.Roles)
    }
}
```

**Step 2: Run**

```
mise run test -- -run TestResolve_FreeAgent_NoCurrentAssignment ./internal/daemon/...
```
Expected: PASS (the existing Resolve leaves CurrentAssignment nil — it's just a new optional field). If it fails, the zero-value of the struct literal may be an empty (non-nil) pointer somewhere; fix.

**Step 3: Commit**

```
git add internal/daemon/provisioning_test.go
git commit -m "test(daemon): provisioning returns no CurrentAssignment when agent free"
```

---

### Task 3: Write failing test — claimed agent gets assignment-specific role

**Files:**
- Modify: `internal/daemon/provisioning_test.go`

**Step 1: Add test**

```go
func TestResolve_ClaimedAgent_OverridesPersistentRoles(t *testing.T) {
    ctx := context.Background()
    store, ps := testSetup(t, 10)

    teamID  := seedTeam(t, ctx, store, "alpha")
    agentID := seedAgent(t, ctx, store, "worker-01", teamID)

    // Persistent role-set: generic "developer".
    devRoleID := seedRole(t, ctx, store, "developer", "")
    seedDoc(t, ctx, store, "doc-dev", "Dev", "persistent dev content", devRoleID)
    store.SetAgentRoles(ctx, agentID, []domain.AgentRole{
        {AgentID: agentID, RoleID: devRoleID, Priority: 1},
    })

    // Persisted agent memory — must survive the role-set override.
    seedAgentMemory(t, ctx, store, "mem-1", "Persistent Note", "persistent memory body", agentID)

    // Also a "reviewer" role exists with its own document.
    revRoleID := seedRole(t, ctx, store, "reviewer", "")
    seedDoc(t, ctx, store, "doc-rev", "Rev", "reviewer content", revRoleID)

    // Claim the agent as reviewer for flow.
    expires := time.Now().UTC().Add(2 * time.Minute)
    _, err := store.ClaimAgent(ctx, "reviewer", "flow", "wf-117", expires)
    if err != nil { t.Fatalf("claim: %v", err) }

    resp, err := ps.Resolve(ctx, agentID)
    if err != nil { t.Fatalf("resolve: %v", err) }

    if resp.CurrentAssignment == nil {
        t.Fatalf("expected CurrentAssignment populated")
    }
    if resp.CurrentAssignment.Role != "reviewer" {
        t.Errorf("Role = %q, want reviewer", resp.CurrentAssignment.Role)
    }
    if resp.CurrentAssignment.Project != "flow" {
        t.Errorf("Project = %q, want flow", resp.CurrentAssignment.Project)
    }
    if resp.CurrentAssignment.WorkflowID != "wf-117" {
        t.Errorf("WorkflowID = %q, want wf-117", resp.CurrentAssignment.WorkflowID)
    }

    // Should NOT include the persistent developer role; should include
    // the reviewer chain instead.
    if len(resp.Roles) != 1 {
        t.Fatalf("expected exactly 1 role group, got %d", len(resp.Roles))
    }
    if resp.Roles[0].Chain[0].Role != "reviewer" {
        t.Errorf("expected reviewer chain, got %q", resp.Roles[0].Chain[0].Role)
    }

    // Should include a project context memory doc.
    var projectDoc *domain.Document
    var persistentDoc *domain.Document
    for i, d := range resp.Memory {
        if d.Kind == domain.DocumentKindMemory && d.Title == "Current Project" {
            projectDoc = &resp.Memory[i]
        }
        if d.ID == "mem-1" {
            persistentDoc = &resp.Memory[i]
        }
    }
    if projectDoc == nil {
        t.Errorf("expected synthesized 'Current Project' memory doc; memory = %+v", resp.Memory)
    } else if !strings.Contains(projectDoc.Content, "flow") {
        t.Errorf("project doc content does not mention flow: %q", projectDoc.Content)
    }
    // Memory additivity: persisted memory must NOT be replaced by the
    // synthetic project doc — both should be present.
    if persistentDoc == nil {
        t.Errorf("expected persisted memory doc 'mem-1' alongside the synthetic project doc; memory = %+v", resp.Memory)
    }
}
```

Add `"strings"` / `"time"` imports if missing.

**Step 2: Run — confirm fail**

```
mise run test -- -run TestResolve_ClaimedAgent_OverridesPersistentRoles ./internal/daemon/...
```
Expected: either CurrentAssignment is nil, or the persistent developer role is returned. Both mean we haven't implemented the branch yet.

**Step 3: Commit the failing test**

```
git add internal/daemon/provisioning_test.go
git commit -m "test(daemon): claimed agent provisioning overrides role-set"
```

---

### Task 4: Implement the override in `ProvisioningService.Resolve`

**Files:**
- Modify: `internal/daemon/provisioning.go`

**Important invariant: persisted memory is additive.** The override only
swaps the *role-set* — persisted agent memory documents (from
`ListAgentMemory`) are ALWAYS appended to the response, regardless of
whether the agent is free or claimed. The synthetic "Current Project"
document is added on top of (not in place of) the persisted memory. The
free-agent path and the claimed-agent path both call
`ListAgentMemory` after the role-set is built and concatenate the
results into the same `memory` slice — see the structure below.

**Step 1: Refactor**

Where Resolve currently fetches `agentRoles`, first check for a claim:

```go
var groups []domain.ProvisioningRoleGroup
var memory []domain.Document
var currentAssignment *domain.CurrentAssignment

if agent.CurrentWorkflowID != "" {
    currentAssignment = &domain.CurrentAssignment{
        Role:           agent.CurrentRole,
        Project:        agent.CurrentProject,
        WorkflowID:     agent.CurrentWorkflowID,
        LeaseExpiresAt: agent.LeaseExpiresAt,
    }

    // Resolve the role by name for this assignment.
    role, err := ps.store.LookupRoleByName(ctx, agent.CurrentRole)
    if err != nil {
        return nil, fmt.Errorf("lookup current role %q: %w", agent.CurrentRole, err)
    }
    chain, err := ps.store.GetRoleChain(ctx, role.ID, ps.maxRoleDepth)
    if err != nil {
        return nil, fmt.Errorf("get current role chain: %w", err)
    }
    entries := make([]domain.ProvisioningChainEntry, 0, len(chain))
    for _, r := range chain {
        docs, err := ps.store.ListRoleDocuments(ctx, r.ID)
        if err != nil {
            return nil, fmt.Errorf("list documents for role %s: %w", r.ID, err)
        }
        docSlice := make([]domain.Document, len(docs))
        for i, d := range docs { docSlice[i] = *d }
        entries = append(entries, domain.ProvisioningChainEntry{
            Role: r.Name, Documents: docSlice,
        })
    }
    groups = []domain.ProvisioningRoleGroup{{Priority: 0, Chain: entries}}

    // Synthesize a 'Current Project' memory document (not persisted).
    now := time.Now().UTC()
    memory = append(memory, domain.Document{
        ID:      "synthetic:current-project",
        Kind:    domain.DocumentKindMemory,
        Title:   "Current Project",
        Content: fmt.Sprintf("You are currently acting as **%s** for project **%s**.\n", agent.CurrentRole, agent.CurrentProject),
        AgentID: agent.ID,
        CreatedAt: now, UpdatedAt: now,
    })
} else {
    // Existing path: build groups from persistent agent_roles …
    // (leave the existing for-range intact here)
}

// Then append the agent's persisted memory documents.
memDocs, err := ps.store.ListAgentMemory(ctx, agentID)
if err != nil {
    return nil, fmt.Errorf("list agent memory: %w", err)
}
for _, d := range memDocs {
    memory = append(memory, *d)
}
```

Add `"time"` import if missing.

Add `CurrentAssignment: currentAssignment` to the returned
`ProvisioningResponse`.

**Step 2: Run tests**

```
mise run test -- ./internal/daemon/...
```
Expected: new tests pass, existing provisioning tests still pass (free-agent path unchanged).

**Step 3: Commit**

```
git add internal/daemon/provisioning.go
git commit -m "feat(provisioning): honor current assignment in Resolve"
```

---

### Task 5: Handle missing role-name lookup gracefully

**Files:**
- Modify: `internal/daemon/provisioning.go`
- Modify: `internal/daemon/provisioning_test.go`

The spec stores `current_role` as a free-form string. If an operator
writes a role name that doesn't exist in the `roles` table, Resolve
should not crash — it should return a response that still tells the
agent about its assignment, even if no role documents can be
resolved.

**Step 1: Write failing test**

```go
func TestResolve_ClaimedAgent_UnknownRoleName_StillReturnsAssignment(t *testing.T) {
    ctx := context.Background()
    store, ps := testSetup(t, 10)
    teamID  := seedTeam(t, ctx, store, "alpha")
    agentID := seedAgent(t, ctx, store, "worker-01", teamID)

    // Claim with a role name that does NOT exist in the roles table.
    expires := time.Now().UTC().Add(time.Minute)
    if _, err := store.ClaimAgent(ctx, "phantom-role", "flow", "wf-1", expires); err != nil {
        t.Fatalf("claim: %v", err)
    }

    resp, err := ps.Resolve(ctx, agentID)
    if err != nil {
        t.Fatalf("resolve should not error on unknown role: %v", err)
    }
    if resp.CurrentAssignment == nil {
        t.Fatalf("expected CurrentAssignment populated")
    }
    if len(resp.Roles) != 0 {
        t.Errorf("expected no role groups, got %d", len(resp.Roles))
    }
}
```

**Step 2: Run, confirm fail**

The current implementation from Task 4 returns an error from
`LookupRoleByName`.

**Step 3: Fix**

In the override branch, wrap the lookup:

```go
role, err := ps.store.LookupRoleByName(ctx, agent.CurrentRole)
if err != nil {
    if errors.Is(err, domain.ErrNotFound) {
        log.Warn("provisioning: claimed agent has unknown current_role",
            "agent_id", agent.ID, "current_role", agent.CurrentRole)
        // groups stays empty; fall through to memory synthesis.
    } else {
        return nil, fmt.Errorf("lookup current role %q: %w", agent.CurrentRole, err)
    }
} else {
    // … build entries / groups as before …
}
```

Add `"errors"` and `"github.com/charmbracelet/log"` imports if missing.

**Step 4: Run**

```
mise run test -- -run TestResolve_ClaimedAgent_UnknownRoleName ./internal/daemon/...
```
Expected: PASS.

**Step 5: Commit**

```
git add internal/daemon/provisioning.go internal/daemon/provisioning_test.go
git commit -m "fix(provisioning): tolerate unknown role name on claimed agent"
```

---

### Task 6: Verify MCP `get_provisioning` surfaces the new data

**Files:**
- No changes expected in `internal/daemon/mcp_tools.go` or `internal/daemon/mcp_server.go`. The `get_provisioning` tool calls `deps.Provisioning.Resolve` (line ~55 of `mcp_tools.go`) and marshals whatever it returns. Since we extended the domain type with `omitempty` JSON tags, the MCP output automatically gains the new field.
- Modify: `internal/daemon/mcp_tools_test.go` — add a test that exercises the full MCP path through `get_provisioning` to confirm JSON round-trip. If that file already tests `get_provisioning`, extend it; otherwise add a focused test that claims an agent and invokes the handler.

**Step 1: Inspect the existing test file**

```
mise run test -- -run TestGetProvisioning ./internal/daemon/... -v
```

(If no such test exists, skip to Step 2.)

**Step 2: Add an MCP-tool-level test**

Write a test that:
1. Seeds store with team, agent, role `reviewer`.
2. Claims the agent via `store.ClaimAgent`.
3. Calls `makeGetProvisioning(deps)` with an MCP request carrying the agent's ID in context.
4. Parses the JSON result.
5. Asserts the parsed JSON has `current_assignment.role == "reviewer"`.

Use whatever context-injection pattern `mcp_tools_test.go` already uses
for authentication. If no pattern exists, skip this task — the
`provisioning_test.go` tests already exercise the service.

**Step 3: Run**

```
mise run test -- ./internal/daemon/...
```

**Step 4: Commit if any changes**

```
git add internal/daemon/mcp_tools_test.go
git commit -m "test(mcp): get_provisioning surfaces current assignment"
```

---

### Task 7: E2E smoke — claim, fetch provisioning via REST, assert role override

**Files:**
- Extend: `tests/e2e/agents_pool_test.go` (from plan 02) — or create a new test file if that one doesn't exist yet.

Hive exposes `/v1/agents/{id}/provisioning` (check `rest_huma.go` — if
there is no REST provisioning endpoint, skip this task; Task 6 has the
MCP-level coverage). If there is no REST endpoint, the Passport-auth'd
MCP tool is the only surface and we rely on plan-04 Task 6 for
end-to-end confidence.

Inspect:

```
mise run test -- -run TestProvisioning -v ./tests/e2e/...
```

If there's an existing E2E provisioning test, add a sibling that:
1. Creates a team + agent + two roles (`developer`, `reviewer`).
2. Sets persistent role to `developer` via `SetAgentRoles`.
3. Claims the agent as `reviewer` for `flow`.
4. Fetches provisioning.
5. Asserts the role chain is `reviewer` (not `developer`) and
   `current_assignment.role == "reviewer"`.

**Step 1–3:** follow the conditional above; commit if code changed.

---

### Task 8: Full green run

**Step 1:**

```
mise run lint
mise run test
mise run e2e
```

All green. If e2e fails on Passport connectivity, surface and stop.

---

## Completion Criteria

- `ProvisioningResponse` has an optional `CurrentAssignment` field (nil for free agents, populated for claimed).
- When an agent is claimed, `Roles` contains the chain for `current_role` (by name) — not the persistent `agent_roles` table.
- A synthetic "Current Project" memory document appears in `Memory` for claimed agents.
- Unknown `current_role` does not cause Resolve to return an error; it returns a warning log plus an empty `Roles` slice.
- Existing free-agent behavior is byte-for-byte unchanged (tests in `provisioning_test.go` still pass).
- MCP `get_provisioning` surfaces the same JSON shape to agents automatically.

At the end of this plan, all four spec migration steps are complete: schema, endpoints, sweeper, and provisioning integration.
