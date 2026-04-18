---
type: plan
step: "1"
title: "Agent pool — schema additions"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "1"
dates:
  created: "2026-04-17"
  approved: null
  completed: null
related_plans:
  - 2026-04-17-agent-pool-02-endpoints.md
  - 2026-04-17-agent-pool-03-sweeper.md
  - 2026-04-17-agent-pool-04-get-provisioning.md
---

# Agent Pool — Step 1: Schema Additions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the four nullable `current_*` assignment columns to `agents` (with an all-or-nothing CHECK constraint) and a `flow_task_ref` column to `tasks`, exposed through `domain.Agent` / `domain.Task` and the sqlite/postgres stores.

**Architecture:** This is the foundation for the agent-pool design spec at
`docs/2026-04-18-agent-assignment-schema.md`. The full feature is split
into four plans matching the spec's migration order, each independently
shippable:
1. **This plan (`-01-schema`)** — DB columns, domain types, store CRUD.
2. `2026-04-17-agent-pool-02-endpoints.md` — REST endpoints (claim / release / renew / assigned list) + client.
3. `2026-04-17-agent-pool-03-sweeper.md` — background goroutine that clears expired leases.
4. `2026-04-17-agent-pool-04-get-provisioning.md` — MCP tool honors `current_role` / `current_project`.

The umbrella cross-repo design is `flow/lead/docs/2026-04-18-agent-pool.md`.

Step 1 is the narrowest possible foundation: migration + type fields + raw
read/write plumbing. No business logic (no atomic claim CAS, no sweeper) —
those belong to later plans. We mirror the established pattern: goose
migration + sibling sqlite/postgres packages + scan helper + table-driven
tests. When shipped alone, the extra columns are simply always NULL; no
behavior changes.

**Tech Stack:** Go 1.26, goose migrations, SQLite (`modernc.org/sqlite`) and PostgreSQL, `mise run <task>`.

---

## Conventions (apply to every task)

- Never run `go build`/`go test` directly. Use `mise run <task>` from the repo root.
- Relevant tasks:
  - `mise run test` — unit tests with race detector for the whole module.
  - `mise run lint` — `gofmt -l .` + `go vet ./...`.
  - `mise run build:dev` — builds `build/hive`.
  - `mise run e2e` — builds `build/hive` then runs `tests/e2e/...`.
  - Focused unit tests: `mise run test -- -run TestName ./internal/infra/sqlite/...`
    (mise passes `--` args through to the underlying `go test`).
- Follow existing file patterns. Agent code lives in `internal/infra/sqlite/agents.go` and `internal/infra/postgres/agents.go`; tests live in sibling `*_test.go`. Match them.
- All SQL identifiers are lowercase snake_case. SQLite DATETIME uses `datetime('now')`; postgres uses `TIMESTAMPTZ` with `NOW()`.
- Postgres migrations live in `internal/infra/postgres/migrations/`, SQLite in `internal/infra/sqlite/migrations/`. Both must move in lockstep (parallel migration numbers, parallel test coverage).
- **Commit after each task** with a Conventional Commits prefix (`feat`, `test`, `refactor`). Keep commits small. Use `git commit -m`.

---

### Task 1: Write failing store test for `current_*` round-trip (SQLite)

**Files:**
- Test: `internal/infra/sqlite/agents_test.go` (add new test)

**Step 1: Add the test**

Append a new test `TestAgent_CurrentAssignment_Roundtrip` that:
1. Creates a team and an agent with all four `Current*` fields zero-valued.
2. Fetches the agent; asserts `CurrentRole == ""`, `CurrentProject == ""`, `CurrentWorkflowID == ""`, `LeaseExpiresAt.IsZero()`.
3. Populates all four via `UpdateAgent`; re-fetches; asserts round-trip values match (compare `LeaseExpiresAt` using `.Equal()` on `time.Time`, not `==`).

Use the same style as `TestAgent_ModelAndRuntime_Roundtrip` already in the file.

**Step 2: Run and confirm failure**

```
mise run test -- -run TestAgent_CurrentAssignment_Roundtrip ./internal/infra/sqlite/...
```
Expected: compile error (fields `CurrentRole` etc. do not exist on `domain.Agent`).

**Step 3: Commit the failing test**

```
git add internal/infra/sqlite/agents_test.go
git commit -m "test(sqlite): add failing round-trip test for agent current assignment"
```

---

### Task 2: Add `Current*` fields to `domain.Agent`

**Files:**
- Modify: `internal/domain/types.go` (Agent struct, ~line 53)

**Step 1: Add fields**

Add four fields to `domain.Agent` (after `Runtime`):

```go
// Current assignment — all-or-nothing. When the agent is free, all four
// are zero values. When claimed by a workflow, all four are set.
CurrentRole       string
CurrentProject    string
CurrentWorkflowID string
LeaseExpiresAt    time.Time
```

Add a doc-comment above them explaining the all-or-nothing invariant and referencing `docs/2026-04-18-agent-assignment-schema.md`.

**Step 2: Verify compile-only**

```
mise run lint
```
Expected: passes (fields are unused by production code yet, but declared).

**Step 3: Commit**

```
git add internal/domain/types.go
git commit -m "feat(domain): add current-assignment fields to Agent"
```

---

### Task 3: Write the goose migration for SQLite

**Files:**
- Create: `internal/infra/sqlite/migrations/003_agent_current_assignment.sql`

**Step 1: Write the migration**

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN current_role TEXT;
ALTER TABLE agents ADD COLUMN current_project TEXT;
ALTER TABLE agents ADD COLUMN current_workflow_id TEXT;
ALTER TABLE agents ADD COLUMN lease_expires_at DATETIME;
-- +goose StatementEnd

-- +goose StatementBegin
-- All-or-nothing invariant: either the agent is free (all NULL) or fully
-- claimed (all four NOT NULL). SQLite cannot add a table-level CHECK after
-- the fact, so we enforce it with a trigger pair on INSERT and UPDATE.
CREATE TRIGGER agents_current_assignment_ck_insert
BEFORE INSERT ON agents
FOR EACH ROW
WHEN NOT (
    (NEW.current_role IS NULL
        AND NEW.current_project IS NULL
        AND NEW.current_workflow_id IS NULL
        AND NEW.lease_expires_at IS NULL)
    OR
    (NEW.current_role IS NOT NULL
        AND NEW.current_project IS NOT NULL
        AND NEW.current_workflow_id IS NOT NULL
        AND NEW.lease_expires_at IS NOT NULL)
)
BEGIN
    SELECT RAISE(ABORT, 'agents.current_* must be all NULL or all set');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER agents_current_assignment_ck_update
BEFORE UPDATE ON agents
FOR EACH ROW
WHEN NOT (
    (NEW.current_role IS NULL
        AND NEW.current_project IS NULL
        AND NEW.current_workflow_id IS NULL
        AND NEW.lease_expires_at IS NULL)
    OR
    (NEW.current_role IS NOT NULL
        AND NEW.current_project IS NOT NULL
        AND NEW.current_workflow_id IS NOT NULL
        AND NEW.lease_expires_at IS NOT NULL)
)
BEGIN
    SELECT RAISE(ABORT, 'agents.current_* must be all NULL or all set');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_agents_free
    ON agents(id)
    WHERE current_workflow_id IS NULL;
CREATE INDEX idx_agents_lease_expires
    ON agents(lease_expires_at)
    WHERE lease_expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agents_lease_expires;
DROP INDEX IF EXISTS idx_agents_free;
DROP TRIGGER IF EXISTS agents_current_assignment_ck_update;
DROP TRIGGER IF EXISTS agents_current_assignment_ck_insert;
-- +goose StatementEnd
-- +goose StatementBegin
-- SQLite prior to 3.35 cannot DROP COLUMN; we no-op here. If you need to
-- revert the columns, recreate the agents table from a prior snapshot.
-- +goose StatementEnd
```

Design note: the spec expresses this as a single CHECK constraint. SQLite
only accepts table CHECK constraints at `CREATE TABLE` time; since we're
adding via ALTER, triggers are the equivalent portable mechanism.
Equivalent CHECK goes into postgres in Task 5.

**Step 2: Verify migration loads**

```
mise run test -- -run TestAgent_ModelAndRuntime_Roundtrip ./internal/infra/sqlite/...
```
Expected: still passes (migration ran against in-memory DB; existing tests untouched).

**Step 3: Commit**

```
git add internal/infra/sqlite/migrations/003_agent_current_assignment.sql
git commit -m "feat(sqlite): migration for agent current-assignment columns"
```

---

### Task 4: Wire the SQLite store to read/write the new columns

**Files:**
- Modify: `internal/infra/sqlite/agents.go`

**Step 1: Update `agentCols`, `scanAgent`, `CreateAgent`, `UpdateAgent`, `LookupAgentByName`**

Replace the `agentCols` constant:

```go
const agentCols = "id, name, team_id, model, runtime, current_role, current_project, current_workflow_id, lease_expires_at, created_at, updated_at"
```

Update `scanAgent` to read the four new columns. They are nullable so
use `sql.NullString` / `sql.NullTime` and copy into domain fields only
if `.Valid`:

```go
func scanAgent(row interface {
    Scan(dest ...any) error
}) (*domain.Agent, error) {
    var a domain.Agent
    var role, project, workflowID sql.NullString
    var leaseExpires sql.NullTime
    if err := row.Scan(
        &a.ID, &a.Name, &a.TeamID, &a.Model, &a.Runtime,
        &role, &project, &workflowID, &leaseExpires,
        &a.CreatedAt, &a.UpdatedAt,
    ); err != nil {
        return nil, err
    }
    if role.Valid {
        a.CurrentRole = role.String
    }
    if project.Valid {
        a.CurrentProject = project.String
    }
    if workflowID.Valid {
        a.CurrentWorkflowID = workflowID.String
    }
    if leaseExpires.Valid {
        a.LeaseExpiresAt = leaseExpires.Time.UTC()
    }
    return &a, nil
}
```

Update `CreateAgent` and `UpdateAgent` to pass the four new values. Helper:

```go
func toNullable(a *domain.Agent) (any, any, any, any) {
    var role, project, workflowID any
    var leaseExpires any
    if a.CurrentRole != "" {
        role = a.CurrentRole
    }
    if a.CurrentProject != "" {
        project = a.CurrentProject
    }
    if a.CurrentWorkflowID != "" {
        workflowID = a.CurrentWorkflowID
    }
    if !a.LeaseExpiresAt.IsZero() {
        leaseExpires = a.LeaseExpiresAt.UTC()
    }
    return role, project, workflowID, leaseExpires
}
```

Then use in both INSERT and UPDATE. Insert SQL becomes 11 `?` placeholders matching the new `agentCols`. Update SQL adds `current_role = ?, current_project = ?, current_workflow_id = ?, lease_expires_at = ?`.

**Step 2: Run the failing test; confirm it now passes**

```
mise run test -- -run TestAgent_CurrentAssignment_Roundtrip ./internal/infra/sqlite/...
```
Expected: PASS.

**Step 3: Run the full sqlite package**

```
mise run test -- ./internal/infra/sqlite/...
```
Expected: all tests pass (no regressions in existing agent/task/role tests).

**Step 4: Commit**

```
git add internal/infra/sqlite/agents.go
git commit -m "feat(sqlite): round-trip current-assignment columns on agents"
```

---

### Task 5: Parallel test, migration, and store changes for Postgres

**Files:**
- Create: `internal/infra/postgres/migrations/003_agent_current_assignment.sql`
- Modify: `internal/infra/postgres/agents.go`
- Optional test: `internal/infra/postgres/` has no round-trip agent test today; if the existing test harness skips when postgres is unavailable, add an equivalent test behind that guard. Otherwise skip. (Inspect `internal/infra/postgres/` for a `*_test.go` harness first.)

**Step 1: Write the postgres migration**

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN current_role TEXT;
ALTER TABLE agents ADD COLUMN current_project TEXT;
ALTER TABLE agents ADD COLUMN current_workflow_id TEXT;
ALTER TABLE agents ADD COLUMN lease_expires_at TIMESTAMPTZ;
ALTER TABLE agents ADD CONSTRAINT agents_current_assignment_ck CHECK (
    (current_role IS NULL
        AND current_project IS NULL
        AND current_workflow_id IS NULL
        AND lease_expires_at IS NULL)
    OR
    (current_role IS NOT NULL
        AND current_project IS NOT NULL
        AND current_workflow_id IS NOT NULL
        AND lease_expires_at IS NOT NULL)
);
CREATE INDEX idx_agents_free ON agents(id) WHERE current_workflow_id IS NULL;
CREATE INDEX idx_agents_lease_expires ON agents(lease_expires_at) WHERE lease_expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agents_lease_expires;
DROP INDEX IF EXISTS idx_agents_free;
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_current_assignment_ck;
ALTER TABLE agents DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE agents DROP COLUMN IF EXISTS current_workflow_id;
ALTER TABLE agents DROP COLUMN IF EXISTS current_project;
ALTER TABLE agents DROP COLUMN IF EXISTS current_role;
-- +goose StatementEnd
```

**Step 2: Update `internal/infra/postgres/agents.go`**

Mirror the sqlite changes exactly; use `$N` placeholders instead of `?`.
`scanAgent` uses the same nullable pattern. `toNullable` helper returns
`any` values (untyped `nil` becomes NULL in pgx).

**Step 3: Verify build**

```
mise run lint
mise run build:dev
```
Expected: both succeed.

**Step 4: Commit**

```
git add internal/infra/postgres/migrations/003_agent_current_assignment.sql internal/infra/postgres/agents.go
git commit -m "feat(postgres): round-trip current-assignment columns on agents"
```

---

### Task 6: Write failing SQLite test for the all-or-nothing trigger

**Files:**
- Test: `internal/infra/sqlite/agents_test.go`

**Step 1: Add test**

Append `TestAgent_CurrentAssignment_AllOrNothingRejected`:

```go
func TestAgent_CurrentAssignment_AllOrNothingRejected(t *testing.T) {
    store := newTestStore(t)
    ctx := context.Background()
    store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

    // Insert with partial assignment (role set, others empty) should fail.
    partial := &domain.Agent{
        ID: "a_001", Name: "alice", TeamID: "t_001",
        CurrentRole: "developer", // others deliberately left zero
    }
    if err := store.CreateAgent(ctx, partial); err == nil {
        t.Fatalf("CreateAgent with partial current_* unexpectedly succeeded")
    }

    // Fully free agent succeeds.
    free := &domain.Agent{ID: "a_002", Name: "bob", TeamID: "t_001"}
    if err := store.CreateAgent(ctx, free); err != nil {
        t.Fatalf("CreateAgent with free agent failed: %v", err)
    }

    // Now try to update it to a partial state — should fail.
    free.CurrentWorkflowID = "wf-xyz"
    if err := store.UpdateAgent(ctx, free); err == nil {
        t.Fatalf("UpdateAgent with partial current_* unexpectedly succeeded")
    }
}
```

**Step 2: Run**

```
mise run test -- -run TestAgent_CurrentAssignment_AllOrNothingRejected ./internal/infra/sqlite/...
```
Expected: PASS (the trigger created in Task 3 rejects the partial write).

**Step 3: Commit**

```
git add internal/infra/sqlite/agents_test.go
git commit -m "test(sqlite): verify all-or-nothing trigger on current assignment"
```

---

### Task 7: `tasks` table — add `flow_task_ref` migration (SQLite + Postgres)

**Files:**
- Create: `internal/infra/sqlite/migrations/004_task_flow_task_ref.sql`
- Create: `internal/infra/postgres/migrations/004_task_flow_task_ref.sql`

**Step 1: Write both migrations**

SQLite (`004_task_flow_task_ref.sql`):
```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE tasks ADD COLUMN flow_task_ref TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite cannot DROP COLUMN on older versions; no-op.
-- +goose StatementEnd
```

Postgres (identical filename):
```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE tasks ADD COLUMN flow_task_ref TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tasks DROP COLUMN IF EXISTS flow_task_ref;
-- +goose StatementEnd
```

**Step 2: Sanity check builds**

```
mise run lint
```

**Step 3: Commit**

```
git add internal/infra/sqlite/migrations/004_task_flow_task_ref.sql internal/infra/postgres/migrations/004_task_flow_task_ref.sql
git commit -m "feat(db): add flow_task_ref column to tasks"
```

---

### Task 8: Expose `FlowTaskRef` on `domain.Task` and round-trip it in both stores

**Files:**
- Modify: `internal/domain/types.go` (Task struct)
- Modify: `internal/infra/sqlite/tasks.go`
- Modify: `internal/infra/postgres/tasks.go`
- Test: `internal/infra/sqlite/tasks_test.go` (add new test first)

**Step 1: Write failing test** (append to `tasks_test.go`)

`TestTask_FlowTaskRef_Roundtrip`:
1. Create team + agent + task with `FlowTaskRef: "flow-task-117"`.
2. `GetTask` — assert `FlowTaskRef == "flow-task-117"`.
3. Update task with `FlowTaskRef: ""`; re-fetch; assert cleared.

**Step 2: Run, confirm fail**

```
mise run test -- -run TestTask_FlowTaskRef_Roundtrip ./internal/infra/sqlite/...
```
Expected: compile error — `FlowTaskRef` unknown.

**Step 3: Add field on domain.Task**

In `internal/domain/types.go`, append to Task:

```go
// FlowTaskRef is a free-form string referencing the originating Flow
// workflow task. Opaque to Hive — Hive does not parse or validate it.
FlowTaskRef string
```

**Step 4: Update sqlite store**

In `tasks.go`: add `flow_task_ref` to the column list in INSERT / SELECT / UPDATE. Use `sql.NullString` on read, untyped `nil` on write when empty (mirror `agentID` pattern already in this file).

**Step 5: Update postgres store**

Same changes, `$N` placeholders.

**Step 6: Run all tests**

```
mise run test
```
Expected: all green.

**Step 7: Commit**

```
git add internal/domain/types.go internal/infra/sqlite/tasks.go internal/infra/postgres/tasks.go internal/infra/sqlite/tasks_test.go
git commit -m "feat(domain,db): round-trip flow_task_ref on tasks"
```

---

### Task 9: Plumb `flow_task_ref` through REST input/output and client

**Files:**
- Modify: `internal/daemon/rest_types.go` (`CreateTaskInput`, `UpdateTaskInput`, `taskResponse`)
- Modify: `internal/daemon/rest_huma.go` (create-task and update-task handlers, `taskToResponse`)
- Modify: `client/tasks.go` (`CreateTaskInput`, `UpdateTaskInput`)
- Modify: `client/types.go` (`Task`)
- Test: `tests/e2e/tasks_test.go` (failing test first)

Background: Task 8 added the `FlowTaskRef` field to `domain.Task` and round-tripped it through the sqlite/postgres stores, but the REST layer and the Go client still ignore it. Flow needs to set this when creating a Hive task via REST — this task wires it end-to-end.

**Step 1: Write failing E2E test**

Append `TestTask_FlowTaskRef_RoundTrip_REST` to `tests/e2e/tasks_test.go`. Use the existing harness pattern (`newHarness(t)` + `h.Client`):

1. Create a team via the client.
2. `h.Client.CreateTask(ctx, client.CreateTaskInput{TeamID: team.ID, Title: "t", FlowTaskRef: "flow-task-117"})` — assert returned task's `FlowTaskRef == "flow-task-117"`.
3. `h.Client.GetTask(ctx, created.ID)` — assert `FlowTaskRef` survives the round trip.
4. `h.Client.UpdateTask(ctx, created.ID, client.UpdateTaskInput{AgentID: "", FlowTaskRef: ""})` — assert the returned task has `FlowTaskRef == ""`.

**Step 2: Run, confirm fail**

```
mise run test -- -run TestTask_FlowTaskRef_RoundTrip_REST ./tests/e2e/...
```
Expected: compile error — `FlowTaskRef` is not a field on `client.CreateTaskInput`, `client.UpdateTaskInput`, or `client.Task`.

**Step 3: Add `FlowTaskRef` to client types**

In `client/types.go`, append to `Task`:

```go
FlowTaskRef string    `json:"FlowTaskRef,omitempty"` // free-form Flow workflow task reference
```

In `client/tasks.go`, append to both `CreateTaskInput` and `UpdateTaskInput`:

```go
FlowTaskRef string `json:"flow_task_ref,omitempty"`
```

(Use `omitempty` on both the request and response sides so existing callers keep working unchanged. The existing field convention here is snake_case JSON for request bodies — `team_id`, `agent_id` — and PascalCase for response bodies, matching what the server emits.)

**Step 4: Add `FlowTaskRef` to REST input/output types**

In `internal/daemon/rest_types.go`:

- Append to `CreateTaskInput.Body`:

  ```go
  FlowTaskRef string `json:"flow_task_ref,omitempty" doc:"Free-form reference to the originating Flow workflow task"`
  ```

- Append to `UpdateTaskInput.Body`:

  ```go
  FlowTaskRef string `json:"flow_task_ref,omitempty" doc:"Free-form reference to the originating Flow workflow task (empty to clear)"`
  ```

  Note: `UpdateTaskInput.Body.AgentID` is documented as "always sent; empty string clears assignment". `FlowTaskRef` follows the same partial-update sentinel: empty string clears it. This matches the body field semantics already established for `AgentID`.

- Append to `taskResponse`:

  ```go
  FlowTaskRef string    `json:"FlowTaskRef,omitempty" doc:"Free-form reference to the originating Flow workflow task"`
  ```

**Step 5: Wire the field through the handlers in `rest_huma.go`**

- In `taskToResponse`, copy the new field:

  ```go
  return taskResponse{
      ID: t.ID, TeamID: t.TeamID, AgentID: t.AgentID,
      Title: t.Title, Description: t.Description, Status: string(t.Status),
      FlowTaskRef: t.FlowTaskRef,
      CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
  }
  ```

- In the `create-task` handler (around line 594), set `FlowTaskRef` on the constructed `domain.Task`:

  ```go
  task := &domain.Task{
      ID: NewID("tk"), TeamID: input.Body.TeamID, AgentID: input.Body.AgentID,
      Title: input.Body.Title, Description: input.Body.Description, Status: status,
      FlowTaskRef: input.Body.FlowTaskRef,
  }
  ```

- In the `update-task` handler (around line 638), apply the field unconditionally — same pattern as `existing.AgentID = input.Body.AgentID`:

  ```go
  existing.FlowTaskRef = input.Body.FlowTaskRef
  ```

**Step 6: Run the failing test; confirm it now passes**

```
mise run test -- -run TestTask_FlowTaskRef_RoundTrip_REST ./tests/e2e/...
```
Expected: PASS.

**Step 7: Run the full unit suite to confirm no regressions**

```
mise run lint
mise run test
```

**Step 8: Commit**

```
git add internal/daemon/rest_types.go internal/daemon/rest_huma.go client/tasks.go client/types.go tests/e2e/tasks_test.go
git commit -m "feat(rest,client): plumb flow_task_ref through CreateTask/UpdateTask"
```

---

### Task 10: Verify E2E harness still works end-to-end

**Files:** none — smoke test only.

**Step 1: Run the full test suite**

```
mise run lint
mise run test
mise run e2e
```
Expected: all pass. E2E runs against a real daemon binary built by the `build:dev` dependency; if `mise run e2e` fails because the E2E harness can't reach Passport at `http://passport.nexus:3000`, that is an environmental prerequisite, not a code defect — surface the failure in the plan completion report and stop.

**Step 2: Commit (docs touch-up if anything drifted)**

No source changes expected; skip commit unless something was updated.

---

## Completion Criteria

- `mise run lint` passes.
- `mise run test` passes, including the new tests:
  `TestAgent_CurrentAssignment_Roundtrip`,
  `TestAgent_CurrentAssignment_AllOrNothingRejected`,
  `TestTask_FlowTaskRef_Roundtrip`,
  `TestTask_FlowTaskRef_RoundTrip_REST`.
- `mise run e2e` passes (or the only failure is an environmental one unrelated to these changes).
- `domain.Agent` has `CurrentRole`, `CurrentProject`, `CurrentWorkflowID`, `LeaseExpiresAt` fields.
- `domain.Task` has `FlowTaskRef` field, and the REST `CreateTask`/`UpdateTask` plus the Go client accept and return it.
- Migrations `003_agent_current_assignment.sql` and `004_task_flow_task_ref.sql` exist in both `sqlite/migrations/` and `postgres/migrations/`.

No REST/MCP behavior changes yet. Those land in plan `-02-endpoints.md`.
