---
type: plan
step: "2"
title: "Agent pool — claim/release/renew/pool-list endpoints"
status: approved
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "2"
dates:
  created: "2026-04-17"
  approved: "2026-04-17"
  completed: null
related_plans:
  - 2026-04-17-agent-pool-01-schema.md
  - 2026-04-17-agent-pool-03-sweeper.md
  - 2026-04-17-agent-pool-04-get-provisioning.md
---

# Agent Pool — Step 2: Claim / Release / Renew / Pool-List Endpoints Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add four REST endpoints (`POST /v1/agents/claim`, `POST /v1/agents/{id}/release`, `POST /v1/agents/{id}/renew`, and `GET /v1/agents?assigned=…`) that atomically manipulate the current-assignment columns introduced in plan `-01-schema.md`.

**Architecture:** This is step 2 of the four-plan series for
`docs/2026-04-18-agent-assignment-schema.md`. Prerequisite: plan
`2026-04-17-agent-pool-01-schema.md` must already be merged — the
columns and domain fields must exist.

Atomicity comes from single UPDATE statements with WHERE-clause compare-
and-swap. `claim` picks one free agent via `SELECT … LIMIT 1` + `UPDATE
agents SET … WHERE id = ? AND current_workflow_id IS NULL` in a
transaction; if `RowsAffected() == 0` we retry up to a small bound and
otherwise return 409. `release` / `renew` CAS on `current_workflow_id
= ?` to prevent cross-workflow tampering.

We extend the existing Huma route registration in
`internal/daemon/rest_huma.go` and teach `domain.AgentStore` three new
methods. All endpoints fall behind the existing Passport auth middleware
(wired in `internal/daemon/server.go` — no change needed, the middleware
blanket-protects all non-public paths).

**Tech Stack:** Go 1.26, Huma v2 for HTTP, `database/sql` transactions, Passport auth middleware, the project's REST-shape conventions.

---

## Conventions (apply to every task)

- Never run `go build`/`go test` directly — use `mise run <task>` from repo root.
- Focused test: `mise run test -- -run TestName ./internal/...`.
- Full flow: `mise run test` then `mise run e2e`.
- Use lowercase `snake_case` JSON keys on request bodies (matches existing
  `team_id`, `agent_id`, etc.). Responses use `PascalCase` keys to match
  the rest of Huma output in `rest_types.go` (e.g. `agentResponse` uses
  `"ID"`, `"Name"`). Follow that split — don't invent a new convention.
- Reuse `mapDomainErr` in `rest_huma.go` for error → HTTP mapping. Add any
  new sentinel errors to `internal/domain/errors.go` and register them in
  `mapDomainErr`.
- Commit after each task with Conventional Commits prefixes.

---

### Task 1: New domain errors for pool operations

**Files:**
- Modify: `internal/domain/errors.go`
- Modify: `internal/daemon/rest_huma.go` (mapDomainErr)

**Step 1: Add sentinels**

Append to `errors.go`:

```go
// ErrPoolExhausted is returned when no free agent matches a claim.
ErrPoolExhausted = errors.New("no free agents available")

// ErrWorkflowMismatch is returned when a release or renew is issued by a
// different workflow than the one currently holding the claim.
ErrWorkflowMismatch = errors.New("workflow id does not match current claim")
```

**Step 2: Map to HTTP statuses in `mapDomainErr`**

Add cases before the `default`:

```go
case errors.Is(err, domain.ErrPoolExhausted):
    return huma.NewError(http.StatusConflict, err.Error())
case errors.Is(err, domain.ErrWorkflowMismatch):
    return huma.NewError(http.StatusConflict, err.Error())
```

**Step 3: Verify**

```
mise run lint
```

**Step 4: Commit**

```
git add internal/domain/errors.go internal/daemon/rest_huma.go
git commit -m "feat(domain): add ErrPoolExhausted and ErrWorkflowMismatch"
```

---

### Task 2: Add store interface methods (unimplemented) + failing tests

**Files:**
- Modify: `internal/domain/ports.go` (AgentStore interface)
- Test: `internal/infra/sqlite/agents_test.go`

**Step 1: Extend `AgentStore`**

```go
// ClaimAgent atomically picks one free agent and sets its current
// assignment. Returns the chosen agent, or ErrPoolExhausted when no
// free agent is available.
ClaimAgent(ctx context.Context, role, project, workflowID string, leaseExpiresAt time.Time) (*Agent, error)

// ReleaseAgent clears the current assignment if current_workflow_id
// matches workflowID. Returns ErrWorkflowMismatch on mismatch,
// ErrNotFound if the agent doesn't exist.
ReleaseAgent(ctx context.Context, agentID, workflowID string) error

// RenewAgentLease extends lease_expires_at if current_workflow_id
// matches workflowID. Returns ErrWorkflowMismatch on mismatch.
RenewAgentLease(ctx context.Context, agentID, workflowID string, leaseExpiresAt time.Time) error

// ListAgents + assigned filter: instead of overloading ListAgents,
// add a dedicated query method so the existing signature stays stable.
ListAgentsByAssignment(ctx context.Context, filter AgentAssignmentFilter) ([]*Agent, error)
```

Define `AgentAssignmentFilter` in `types.go`:

```go
// AgentAssignmentFilter narrows ListAgentsByAssignment. Zero values mean
// "no filter" except Assigned which uses *bool to distinguish
// "unspecified" from "false".
type AgentAssignmentFilter struct {
    Assigned   *bool  // nil = all; true = claimed only; false = free only
    WorkflowID string // empty = no filter
    Role       string // empty = no filter
    Project    string // empty = no filter
    TeamID     string // empty = no filter (matches existing ListAgents)
}
```

Import `time` in `ports.go` if not already present.

**Step 2: Write failing store tests** (append to `agents_test.go`)

Four tests:
- `TestClaimAgent_PicksFreeAgent` — seed two free agents; call ClaimAgent; assert exactly one is now claimed with the given fields; second remains free.
- `TestClaimAgent_PoolExhausted` — seed two agents, both already claimed (via UpdateAgent with all four fields set); call ClaimAgent; assert `errors.Is(err, domain.ErrPoolExhausted)`.
- `TestReleaseAgent_WorkflowMismatchRejected` — claim with `wf-1`; call ReleaseAgent with `wf-2`; assert `ErrWorkflowMismatch`; assert agent still claimed.
- `TestRenewAgentLease_ExtendsExpiry` — claim with lease `t0`; call RenewAgentLease with lease `t0 + 1h`; re-fetch; assert `LeaseExpiresAt.Equal(t0.Add(time.Hour))` via `.Equal()`.

One more:
- `TestListAgentsByAssignment_FreeAndClaimed` — seed three agents (two free, one claimed); call with `Assigned: &trueVal`; expect one. With `&falseVal`; expect two. With `Role: "developer"`; expect only the claimed developer.

**Step 3: Run**

```
mise run test -- -run TestClaimAgent ./internal/infra/sqlite/...
```
Expected: compile-fails (store doesn't implement new methods) or link-fails. This is intentional — covered by next task.

**Step 4: Commit failing tests & interface**

```
git add internal/domain/ports.go internal/domain/types.go internal/infra/sqlite/agents_test.go
git commit -m "test(sqlite): add failing pool operation tests and interface"
```

---

### Task 3: Implement pool operations in SQLite store

**Files:**
- Modify: `internal/infra/sqlite/agents.go`

**Step 1: Implement `ClaimAgent`**

Transaction-wrapped. Pick a candidate with `SELECT id FROM agents WHERE current_workflow_id IS NULL ORDER BY id LIMIT 1`; then `UPDATE … WHERE id = ? AND current_workflow_id IS NULL`; if rows_affected == 0, loop once more (handles race with concurrent claim). If after 3 retries still 0 rows, return `ErrPoolExhausted`. Finally `GetAgent(ctx, picked)` inside the same tx to return the populated record.

Important: pass `leaseExpiresAt.UTC()` to the UPDATE. Use `datetime('now')` for `updated_at`. Write the UPDATE against the connection from the tx (`tx.ExecContext`), not `s.db`.

Return the freshly-read agent.

**Step 2: Implement `ReleaseAgent`**

Single UPDATE:

```
UPDATE agents
SET current_role = NULL,
    current_project = NULL,
    current_workflow_id = NULL,
    lease_expires_at = NULL,
    updated_at = datetime('now')
WHERE id = ? AND current_workflow_id = ?
```

If `rows_affected == 0`, check existence. If `GetAgent` returns
`ErrNotFound`, return `ErrNotFound`. Otherwise return
`ErrWorkflowMismatch` (the agent exists but workflow_id didn't match).

**Step 3: Implement `RenewAgentLease`**

```
UPDATE agents
SET lease_expires_at = ?, updated_at = datetime('now')
WHERE id = ? AND current_workflow_id = ?
```

If 0 rows, check existence, return `ErrNotFound` or `ErrWorkflowMismatch`.

**Step 4: Implement `ListAgentsByAssignment`**

Build the query dynamically (similar to how `ListAgents` switches on
teamID). Compose WHERE fragments and args:

```go
var where []string
var args []any
if f.TeamID != "" { where = append(where, "team_id = ?"); args = append(args, f.TeamID) }
if f.Assigned != nil {
    if *f.Assigned {
        where = append(where, "current_workflow_id IS NOT NULL")
    } else {
        where = append(where, "current_workflow_id IS NULL")
    }
}
if f.WorkflowID != "" { where = append(where, "current_workflow_id = ?"); args = append(args, f.WorkflowID) }
if f.Role != "" { where = append(where, "current_role = ?"); args = append(args, f.Role) }
if f.Project != "" { where = append(where, "current_project = ?"); args = append(args, f.Project) }

q := "SELECT " + agentCols + " FROM agents"
if len(where) > 0 { q += " WHERE " + strings.Join(where, " AND ") }
q += " ORDER BY name"
```

**Step 5: Run tests**

```
mise run test -- ./internal/infra/sqlite/...
```
Expected: all five new tests pass, no regressions.

**Step 6: Commit**

```
git add internal/infra/sqlite/agents.go
git commit -m "feat(sqlite): implement claim/release/renew/list-by-assignment"
```

---

### Task 4: Mirror in Postgres store

**Files:**
- Modify: `internal/infra/postgres/agents.go`

**Step 1: Implement the same four methods**

For `ReleaseAgent`, `RenewAgentLease`, and `ListAgentsByAssignment`:
same logic as the SQLite implementation, with `$N` placeholders and
`NOW()` instead of `datetime('now')`.

For `ClaimAgent`, the canonical postgres implementation is a single
`UPDATE ... WHERE id = (SELECT id ... WHERE current_workflow_id IS
NULL LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING *` query — NOT the
SQLite-style select-then-update retry loop. The combined statement is
atomic, requires no application-level retries, and degrades cleanly
under concurrency: each contending claimer's inner SELECT skips rows
already locked by a peer, so two claims racing against the same free
agent can never both succeed and never need to back off. Do not port
the SQLite retry loop here — use the locking variant directly:

```sql
UPDATE agents
SET current_role = $1,
    current_project = $2,
    current_workflow_id = $3,
    lease_expires_at = $4,
    updated_at = NOW()
WHERE id = (
    SELECT id FROM agents
    WHERE current_workflow_id IS NULL
    ORDER BY id
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING <agentCols>
```

Use `pgx`-compatible `QueryRowContext` with the RETURNING clause to
read the agent row back via the existing `scanAgent` helper.

If the RETURNING query returns `sql.ErrNoRows`, return `ErrPoolExhausted`.

**Step 2: Verify**

```
mise run lint
mise run build:dev
```

Postgres store tests typically skip when no database is available; rely
on CI / local PG to exercise this path.

**Step 3: Commit**

```
git add internal/infra/postgres/agents.go
git commit -m "feat(postgres): implement claim/release/renew/list-by-assignment"
```

---

### Task 5: REST input/output types for the four endpoints

**Files:**
- Modify: `internal/daemon/rest_types.go`

**Step 1: Add input & output types**

```go
// --- agent pool ---

type ClaimAgentInput struct {
    Body struct {
        Role            string `json:"role" doc:"Role the agent will fill" minLength:"1"`
        Project         string `json:"project" doc:"Project the agent will work on" minLength:"1"`
        WorkflowID      string `json:"workflow_id" doc:"Flow workflow holding the lease" minLength:"1"`
        LeaseTTLSeconds int    `json:"lease_ttl_seconds" doc:"Lease duration in seconds" minimum:"1"`
    }
}

type ReleaseAgentInput struct {
    ID   string `path:"id" doc:"Agent ID"`
    Body struct {
        WorkflowID string `json:"workflow_id" doc:"Workflow holding the lease" minLength:"1"`
    }
}

type RenewAgentInput struct {
    ID   string `path:"id" doc:"Agent ID"`
    Body struct {
        WorkflowID      string `json:"workflow_id" doc:"Workflow holding the lease" minLength:"1"`
        LeaseTTLSeconds int    `json:"lease_ttl_seconds" doc:"New lease duration in seconds from now" minimum:"1"`
    }
}

// Existing ListAgentsInput extended with new filters.
// IMPORTANT: add fields to the existing struct, don't rename.
// type ListAgentsInput struct {
//     TeamID     string `query:"team_id" doc:"Filter by team ID"`
//     Assigned   *bool  `query:"assigned" doc:"Filter by assignment state (true/false)"`
//     WorkflowID string `query:"workflow_id" doc:"Filter by claiming workflow"`
//     Role       string `query:"role" doc:"Filter by current role"`
//     Project    string `query:"project" doc:"Filter by current project"`
// }
```

Update `ListAgentsInput` in-place: add the four new query fields. The
existing callers that only populate `TeamID` continue to work because
unspecified `Assigned *bool` is `nil`.

Extend `agentResponse` to include the current-assignment fields (omit
when empty):

```go
CurrentRole       string    `json:"CurrentRole,omitempty" doc:"Role the agent is filling"`
CurrentProject    string    `json:"CurrentProject,omitempty" doc:"Project the agent is working on"`
CurrentWorkflowID string    `json:"CurrentWorkflowID,omitempty" doc:"Flow workflow holding the lease"`
LeaseExpiresAt    time.Time `json:"LeaseExpiresAt,omitempty" doc:"Lease expiry timestamp"`
```

Also update `agentDetailResponse` for parity. Update
`agentToResponse` in `rest_huma.go` to copy the new fields.

**Step 2: Verify compile**

```
mise run lint
```

**Step 3: Commit**

```
git add internal/daemon/rest_types.go internal/daemon/rest_huma.go
git commit -m "feat(rest): input/output types for agent pool endpoints"
```

---

### Task 6: Write failing E2E tests for claim/release/renew/list

**Files:**
- Create: `tests/e2e/agents_pool_test.go`
- Modify: `client/agents.go` (add thin client methods — see Task 8)

The test file will fail to compile until the client methods exist; we
accept that and add the client in Task 8 before running the test.

**Step 1: Write the test file**

Test cases:
1. `TestClaimReleaseRoundTrip` — create 2 agents; call `Claim(role, project, workflow_id, ttl)`; assert one agent returned with `CurrentWorkflowID` set and `LeaseExpiresAt` roughly `now + ttl`. Call `Release(id, workflow_id)`; list with `assigned=false` and expect both agents in the response.
2. `TestClaimPoolExhausted` — create 1 agent, claim it, claim again; expect 409 (client `ErrConflict`).
3. `TestReleaseWorkflowMismatch` — claim with `wf-1`; call `Release(id, "wf-2")`; expect 409.
4. `TestRenewExtendsLease` — claim with TTL 60s; call `Renew(id, wf, 600)`; fetch the agent; assert `LeaseExpiresAt` is now ~10 min out.
5. `TestListAssignedFilters` — two agents, one claimed; `ListAgents(assigned=true)` returns the claimed one; `ListAgents(assigned=false)` returns the free one; `ListAgents(role=...)` returns only the matching-role claim.

Use the existing `newHarness(t)` + `h.Client` pattern from
`agents_test.go`. Use Passport UUIDs for agent IDs (distinct from the
ones in other tests).

**Step 2: Don't run yet** — client methods needed first.

**Step 3: Commit**

```
git add tests/e2e/agents_pool_test.go
git commit -m "test(e2e): failing tests for agent pool endpoints"
```

---

### Task 7: Register the four endpoints in `rest_huma.go`

**Files:**
- Modify: `internal/daemon/rest_huma.go` (inside `registerAgentRoutes`)

**Step 1: Register `POST /v1/agents/claim`**

```go
huma.Register(api, huma.Operation{
    OperationID:   "claim-agent",
    Method:        http.MethodPost,
    Path:          "/v1/agents/claim",
    Summary:       "Atomically claim a free agent",
    DefaultStatus: http.StatusOK,
    Tags:          []string{"Agents"},
}, func(ctx context.Context, input *ClaimAgentInput) (*AgentOutput, error) {
    expires := time.Now().UTC().Add(time.Duration(input.Body.LeaseTTLSeconds) * time.Second)
    agent, err := store.ClaimAgent(ctx, input.Body.Role, input.Body.Project, input.Body.WorkflowID, expires)
    if err != nil {
        return nil, mapDomainErr(err)
    }
    return &AgentOutput{Body: agentToResponse(agent)}, nil
})
```

**Step 2: Register `POST /v1/agents/{id}/release`**

```go
huma.Register(api, huma.Operation{
    OperationID:   "release-agent",
    Method:        http.MethodPost,
    Path:          "/v1/agents/{id}/release",
    Summary:       "Release an agent's current assignment",
    DefaultStatus: http.StatusNoContent,
    Tags:          []string{"Agents"},
}, func(ctx context.Context, input *ReleaseAgentInput) (*struct{}, error) {
    if err := store.ReleaseAgent(ctx, input.ID, input.Body.WorkflowID); err != nil {
        return nil, mapDomainErr(err)
    }
    return nil, nil
})
```

**Step 3: Register `POST /v1/agents/{id}/renew`**

```go
huma.Register(api, huma.Operation{
    OperationID:   "renew-agent",
    Method:        http.MethodPost,
    Path:          "/v1/agents/{id}/renew",
    Summary:       "Renew an agent's lease",
    DefaultStatus: http.StatusNoContent,
    Tags:          []string{"Agents"},
}, func(ctx context.Context, input *RenewAgentInput) (*struct{}, error) {
    expires := time.Now().UTC().Add(time.Duration(input.Body.LeaseTTLSeconds) * time.Second)
    if err := store.RenewAgentLease(ctx, input.ID, input.Body.WorkflowID, expires); err != nil {
        return nil, mapDomainErr(err)
    }
    return nil, nil
})
```

**Step 4: Update the existing `list-agents` handler to use the extended filter**

Replace the `store.ListAgents(ctx, input.TeamID)` call with
`store.ListAgentsByAssignment` when any of the new filter fields are
set, otherwise keep the existing call:

```go
func(ctx context.Context, input *ListAgentsInput) (*AgentListOutput, error) {
    // If any of the pool filters are specified, route through the
    // richer query; otherwise preserve the existing ListAgents call
    // (keeps backwards-compatible ordering semantics).
    if input.Assigned != nil || input.WorkflowID != "" || input.Role != "" || input.Project != "" {
        filter := domain.AgentAssignmentFilter{
            TeamID: input.TeamID, Assigned: input.Assigned,
            WorkflowID: input.WorkflowID, Role: input.Role, Project: input.Project,
        }
        agents, err := store.ListAgentsByAssignment(ctx, filter)
        if err != nil { return nil, mapDomainErr(err) }
        resp := make([]agentResponse, len(agents))
        for i, a := range agents { resp[i] = agentToResponse(a) }
        return &AgentListOutput{Body: resp}, nil
    }
    // existing path …
}
```

**Step 5: Import `time` if not already imported**

**Step 6: Verify**

```
mise run lint
```

**Step 7: Commit**

```
git add internal/daemon/rest_huma.go
git commit -m "feat(rest): register claim/release/renew/pool-list endpoints"
```

---

### Task 8: Add client-side methods & types for the new endpoints

**Files:**
- Modify: `client/agents.go`
- Modify: `client/types.go` (add fields to Agent / AgentWithRoles response types)

**Step 1: Extend response types**

In `client/types.go`, add the four current-assignment fields to `Agent`
(and `AgentWithRoles`) — mirror the server response JSON names
(`CurrentRole`, `CurrentProject`, `CurrentWorkflowID`, `LeaseExpiresAt`).

**Step 2: Add client methods**

Append to `client/agents.go`:

```go
// ClaimAgent calls POST /v1/agents/claim.
func (c *Client) ClaimAgent(ctx context.Context, role, project, workflowID string, ttlSeconds int) (*Agent, error) {
    body := map[string]any{
        "role": role, "project": project,
        "workflow_id": workflowID, "lease_ttl_seconds": ttlSeconds,
    }
    var out Agent
    return &out, c.do(ctx, http.MethodPost, "/v1/agents/claim", body, &out)
}

// ReleaseAgent calls POST /v1/agents/{id}/release.
func (c *Client) ReleaseAgent(ctx context.Context, id, workflowID string) error {
    body := map[string]string{"workflow_id": workflowID}
    return c.do(ctx, http.MethodPost, "/v1/agents/"+id+"/release", body, nil)
}

// RenewAgentLease calls POST /v1/agents/{id}/renew.
func (c *Client) RenewAgentLease(ctx context.Context, id, workflowID string, ttlSeconds int) error {
    body := map[string]any{"workflow_id": workflowID, "lease_ttl_seconds": ttlSeconds}
    return c.do(ctx, http.MethodPost, "/v1/agents/"+id+"/renew", body, nil)
}
```

Update `ListAgents` to take an options struct or add sibling
`ListAgentsByAssignment(ctx, teamID string, assigned *bool, workflowID, role, project string)`. Prefer a sibling method to avoid breaking existing callers.

**Step 3: Verify**

```
mise run lint
mise run test
```

**Step 4: Commit**

```
git add client/agents.go client/types.go
git commit -m "feat(client): methods for claim/release/renew/pool-list"
```

---

### Task 9: Run the E2E tests written in Task 6

**Step 1: Build + run e2e**

```
mise run e2e
```
Expected: the five new tests in `agents_pool_test.go` pass, plus the
pre-existing e2e suite.

**Step 2: Iterate on any failures**

If failure is "cannot reach Passport at passport.nexus" — environmental;
surface it in the completion report.
If failure is real (HTTP status mismatch, missing field) — fix source
and re-run.

**Step 3: Commit any follow-up fixes**

---

### Task 10: Document the endpoints in the hive-design or README if there's a section for endpoints

**Files:** check whether `docs/hive-design.md` enumerates endpoints. If it does, append the four new ones with one-line descriptions. If it doesn't (or the section is clearly "design" not "inventory"), skip this task.

Do NOT create new documentation files.

**Step 1 (conditional): append to `hive-design.md`**

**Step 2 (conditional): commit**

```
git commit -m "docs(hive): list claim/release/renew/pool-list endpoints"
```

---

## Completion Criteria

- `mise run lint` passes.
- `mise run test` passes.
- `mise run e2e` passes (or only fails due to Passport unavailability).
- The four endpoints are reachable via `curl` from a local daemon and behave as specified in `docs/2026-04-18-agent-assignment-schema.md` (claim returns agent or 409, release/renew CAS on workflow_id).
- Client has `ClaimAgent`, `ReleaseAgent`, `RenewAgentLease`, and assignment-filtered list methods.
- Passport middleware covers all four routes (verified by trying them without a token → 401).

The sweeper and get_provisioning changes remain for plans `-03-sweeper.md` and `-04-get-provisioning.md`.
