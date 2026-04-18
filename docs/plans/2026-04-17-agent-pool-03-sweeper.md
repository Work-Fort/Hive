---
type: plan
step: "3"
title: "Agent pool — lease-expiry sweeper"
status: approved
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "3"
dates:
  created: "2026-04-17"
  approved: "2026-04-18"
  completed: null
related_plans:
  - 2026-04-17-agent-pool-01-schema.md
  - 2026-04-17-agent-pool-02-endpoints.md
  - 2026-04-17-agent-pool-04-get-provisioning.md
---

# Agent Pool — Step 3: Lease-Expiry Sweeper Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Run a background goroutine in the Hive daemon that periodically (every ~30s) clears `current_*` assignment columns on any agent whose `lease_expires_at` is in the past, emitting one audit log line per cleared agent.

**Architecture:** Step 3 in the four-plan series for
`docs/2026-04-18-agent-assignment-schema.md`. Prerequisites: plans
`-01-schema.md` and `-02-endpoints.md`. The sweeper depends on the
columns (plan 01) and complements claim/release (plan 02) by recovering
from Flow crashes — when Flow dies mid-workflow, no one renews; the
sweeper returns the agent to the pool after the lease expires.

We follow the exact pattern `cmd/daemon/daemon.go` already uses for the
periodic health checks: a `context.WithCancel` started inside `run()`,
a goroutine ticking on a `time.Ticker`, graceful shutdown on SIGTERM.
One new domain-layer service (`SweeperService`) so it's testable in
isolation, wired into `daemon.go` near the existing
`health.StartPeriodic` call.

**Tech Stack:** Go 1.26, `time.Ticker`, `context.Context`, `charmbracelet/log`.

---

## Conventions (apply to every task)

- Never run `go build`/`go test` directly — use `mise run <task>` from repo root.
- `mise run test` for unit tests; `mise run e2e` for end-to-end.
- Use `github.com/charmbracelet/log` (already in use throughout
  `internal/daemon/`) for log output. Match the log key/value style:
  `log.Info("sweeper: released expired claim", "agent_id", id, "workflow_id", wf)`.
- Commit after each task.

---

### Task 1: Domain-layer store method — batch clear expired leases

**Files:**
- Modify: `internal/domain/ports.go` (AgentStore)
- Modify: `internal/infra/sqlite/agents.go`
- Modify: `internal/infra/postgres/agents.go`
- Test: `internal/infra/sqlite/agents_test.go` (failing test first)

**Step 1: Write failing test**

Append `TestSweepExpiredLeases`:

```go
func TestSweepExpiredLeases(t *testing.T) {
    store := newTestStore(t)
    ctx := context.Background()
    store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
    store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})
    store.CreateAgent(ctx, &domain.Agent{ID: "a_002", Name: "bob",   TeamID: "t_001"})

    past   := time.Now().UTC().Add(-time.Minute)
    future := time.Now().UTC().Add(time.Hour)

    // a_001 has an expired lease; a_002 has a fresh one.
    a1 := &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001",
        CurrentRole: "developer", CurrentProject: "flow",
        CurrentWorkflowID: "wf-1", LeaseExpiresAt: past}
    if err := store.UpdateAgent(ctx, a1); err != nil {
        t.Fatalf("update a_001: %v", err)
    }
    a2 := &domain.Agent{ID: "a_002", Name: "bob", TeamID: "t_001",
        CurrentRole: "reviewer", CurrentProject: "flow",
        CurrentWorkflowID: "wf-2", LeaseExpiresAt: future}
    if err := store.UpdateAgent(ctx, a2); err != nil {
        t.Fatalf("update a_002: %v", err)
    }

    released, err := store.SweepExpiredLeases(ctx, time.Now().UTC())
    if err != nil {
        t.Fatalf("sweep: %v", err)
    }
    if len(released) != 1 || released[0].ID != "a_001" {
        t.Fatalf("expected a_001 released, got %+v", released)
    }

    got1, _ := store.GetAgent(ctx, "a_001")
    if got1.CurrentWorkflowID != "" {
        t.Errorf("a_001 still claimed: %+v", got1)
    }
    got2, _ := store.GetAgent(ctx, "a_002")
    if got2.CurrentWorkflowID != "wf-2" {
        t.Errorf("a_002 unexpectedly released: %+v", got2)
    }
}
```

**Step 2: Run, confirm fail**

```
mise run test -- -run TestSweepExpiredLeases ./internal/infra/sqlite/...
```
Expected: undefined method error.

**Step 3: Add to `AgentStore`**

```go
// SweepExpiredLeases clears the current assignment on any agent whose
// lease_expires_at is non-NULL and earlier than `now`. Returns the
// agents that were released (for audit logging).
SweepExpiredLeases(ctx context.Context, now time.Time) ([]*Agent, error)
```

**Step 4: Implement in SQLite**

Use a single transaction:
1. `SELECT` the rows matching `lease_expires_at IS NOT NULL AND lease_expires_at < ?` (capture full agent records so we can log them).
2. `UPDATE agents SET current_role = NULL, current_project = NULL, current_workflow_id = NULL, lease_expires_at = NULL, updated_at = datetime('now') WHERE lease_expires_at IS NOT NULL AND lease_expires_at < ?` with the same `?`.
3. Return the pre-sweep agent records.

Scan carefully — since the UPDATE clears fields, scan before the UPDATE so `released[i]` still shows what the claim was at sweep time.

**Step 5: Implement in Postgres**

Same logic, `$1` placeholder, `NOW()` — or better, use the `now` arg passed in for determinism across both backends. Prefer passing the argument: both DBs behave identically, and tests can inject a fixed time.

**Step 6: Run test**

```
mise run test -- -run TestSweepExpiredLeases ./internal/infra/sqlite/...
```
Expected: PASS.

**Step 7: Commit**

```
git add internal/domain/ports.go internal/infra/sqlite/agents.go internal/infra/postgres/agents.go internal/infra/sqlite/agents_test.go
git commit -m "feat(db): SweepExpiredLeases clears stale claims across backends"
```

---

### Task 2: Create `SweeperService` with unit tests

**Files:**
- Create: `internal/daemon/sweeper.go`
- Create: `internal/daemon/sweeper_test.go`

**Step 1: Write failing test**

`sweeper_test.go`:

```go
package daemon_test

import (
    "context"
    "testing"
    "time"

    "github.com/Work-Fort/Hive/internal/daemon"
    "github.com/Work-Fort/Hive/internal/domain"
    "github.com/Work-Fort/Hive/internal/infra/sqlite"
)

func TestSweeperReleasesOneExpiredAgent(t *testing.T) {
    store, err := sqlite.Open("")
    if err != nil { t.Fatalf("open: %v", err) }
    t.Cleanup(func() { store.Close() })

    ctx := context.Background()
    store.CreateTeam(ctx, &domain.Team{ID: "t", Name: "t"})
    store.CreateAgent(ctx, &domain.Agent{ID: "a", Name: "a", TeamID: "t"})
    past := time.Now().UTC().Add(-time.Minute)
    store.UpdateAgent(ctx, &domain.Agent{
        ID: "a", Name: "a", TeamID: "t",
        CurrentRole: "r", CurrentProject: "p",
        CurrentWorkflowID: "wf", LeaseExpiresAt: past,
    })

    sw := daemon.NewSweeperService(store)
    count, err := sw.SweepOnce(ctx)
    if err != nil { t.Fatalf("sweep: %v", err) }
    if count != 1 { t.Errorf("released count = %d, want 1", count) }

    got, _ := store.GetAgent(ctx, "a")
    if got.CurrentWorkflowID != "" {
        t.Errorf("agent still claimed: %+v", got)
    }
}
```

**Step 2: Run, confirm fail**

```
mise run test -- -run TestSweeperReleasesOneExpiredAgent ./internal/daemon/...
```
Expected: undefined `SweeperService` / `NewSweeperService`.

**Step 3: Implement `sweeper.go`**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
    "context"
    "time"

    "github.com/charmbracelet/log"

    "github.com/Work-Fort/Hive/internal/domain"
)

// SweeperService periodically clears expired agent leases.
type SweeperService struct {
    store domain.Store
}

// NewSweeperService constructs a SweeperService.
func NewSweeperService(store domain.Store) *SweeperService {
    return &SweeperService{store: store}
}

// SweepOnce runs a single sweep pass. Returns the number of agents
// whose claim was cleared. Logs one line per released agent.
func (s *SweeperService) SweepOnce(ctx context.Context) (int, error) {
    released, err := s.store.SweepExpiredLeases(ctx, time.Now().UTC())
    if err != nil {
        return 0, err
    }
    for _, a := range released {
        log.Info("sweeper: released expired claim",
            "agent_id", a.ID,
            "agent_name", a.Name,
            "workflow_id", a.CurrentWorkflowID,
            "role", a.CurrentRole,
            "project", a.CurrentProject,
            "lease_expired_at", a.LeaseExpiresAt.Format(time.RFC3339),
        )
    }
    return len(released), nil
}

// Start runs SweepOnce on `interval` until ctx is cancelled. Errors from
// individual sweeps are logged but do not stop the loop — the sweeper
// must survive transient DB issues.
func (s *SweeperService) Start(ctx context.Context, interval time.Duration) {
    if interval <= 0 {
        interval = 30 * time.Second
    }
    // Run once immediately so a freshly-started daemon cleans stragglers.
    if _, err := s.SweepOnce(ctx); err != nil {
        log.Warn("sweeper: initial sweep failed", "err", err)
    }
    t := time.NewTicker(interval)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            log.Debug("sweeper: shutting down")
            return
        case <-t.C:
            if _, err := s.SweepOnce(ctx); err != nil {
                log.Warn("sweeper: sweep failed", "err", err)
            }
        }
    }
}
```

**Step 4: Run**

```
mise run test -- ./internal/daemon/...
```
Expected: test passes.

**Step 5: Commit**

```
git add internal/daemon/sweeper.go internal/daemon/sweeper_test.go
git commit -m "feat(daemon): SweeperService with SweepOnce + Start loop"
```

---

### Task 3: Add test for the Start loop's graceful shutdown

**Files:**
- Modify: `internal/daemon/sweeper_test.go`

**Step 1: Append test**

```go
func TestSweeperStart_StopsOnContextCancel(t *testing.T) {
    store, _ := sqlite.Open("")
    t.Cleanup(func() { store.Close() })

    sw := daemon.NewSweeperService(store)

    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan struct{})
    go func() { sw.Start(ctx, 10*time.Millisecond); close(done) }()

    // Let the ticker fire at least once.
    time.Sleep(50 * time.Millisecond)
    cancel()

    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("sweeper did not stop within 1s of cancel")
    }
}
```

**Step 2: Run**

```
mise run test -- -run TestSweeperStart_StopsOnContextCancel ./internal/daemon/...
```
Expected: PASS.

**Step 3: Commit**

```
git add internal/daemon/sweeper_test.go
git commit -m "test(daemon): sweeper exits promptly on context cancel"
```

---

### Task 4: Wire the sweeper into the daemon boot sequence

**Files:**
- Modify: `cmd/daemon/daemon.go`

**Step 1: Add a viper-configurable interval**

Near the existing viper reads (around `maxRoleDepth := viper.GetInt("max-role-depth")`), add:

```go
sweeperInterval := viper.GetDuration("sweeper-interval")
if sweeperInterval <= 0 {
    sweeperInterval = 30 * time.Second
}
```

**Step 2: Start the sweeper alongside `health.StartPeriodic`**

Find the existing:

```go
healthCtx, healthCancel := context.WithCancel(context.Background())
defer healthCancel()
go health.StartPeriodic(healthCtx, 30*time.Second)
```

Add immediately after:

```go
sweeper := hiveDaemon.NewSweeperService(store)
go sweeper.Start(healthCtx, sweeperInterval)
```

Reuse `healthCtx` — it's already cancelled on shutdown and the sweeper
has the same lifetime as the periodic health checks.

**Step 3: Verify**

```
mise run lint
mise run build:dev
```

**Step 4: Commit**

```
git add cmd/daemon/daemon.go
git commit -m "feat(daemon): run sweeper alongside periodic health checks"
```

---

### Task 5: E2E test — expired lease is swept within one interval

**Files:**
- Create: `tests/e2e/agents_sweeper_test.go` (or extend `agents_pool_test.go` if that exists from plan 02)

**Step 1: Write the test**

Use the harness; claim an agent with TTL of 1 second; sleep 2 seconds;
list with `assigned=true` and assert zero agents. Because the default
sweeper interval in the daemon is 30 s, **this test needs the daemon
run with a shorter interval**. Update `tests/e2e/harness_test.go` to
pass `--sweeper-interval=500ms` — or expose a harness option.

Simplest approach: extend the `cmd/daemon/daemon.go` flag surface with
a `--sweeper-interval` flag bound to viper, and pass that flag from the
harness (`"--sweeper-interval", "200ms"`).

```go
cmd.Flags().Duration("sweeper-interval", 30*time.Second, "Lease sweeper interval")
viper.BindPFlag("sweeper-interval", cmd.Flags().Lookup("sweeper-interval"))
```

Re-read in the `run()` body via `viper.GetDuration("sweeper-interval")`.

**Step 2: Harness change**

In `tests/e2e/harness_test.go`, add `"--sweeper-interval", "200ms"` to
the `exec.Command` args.

**Step 3: Run**

```
mise run e2e
```
Expected: new test passes. If sweeper interval flag wasn't wired, debug
and fix. If environmental (Passport), surface and stop.

**Step 4: Commit**

```
git add cmd/daemon/daemon.go tests/e2e/harness_test.go tests/e2e/agents_sweeper_test.go
git commit -m "feat(daemon,e2e): configurable sweeper interval + expiry test"
```

---

### Task 6: Race-coverage unit test — sweeper clears, concurrent renew loses cleanly

**Files:**
- Modify: `internal/infra/sqlite/agents_test.go`

This test exists specifically to lock in the CAS contract on
`RenewAgentLease`: when the sweeper clears an expired claim before a
renew lands, the renew must fail with `ErrWorkflowMismatch` rather than
silently re-establishing the claim. The happy-path sweep test in Task 5
does NOT cover this — keep this case as a separate, named test so a
future editor doesn't deduplicate it against the happy-path sweep.

**Step 1: Append failing test**

```go
func TestSweepRaceWithRenewLosesGracefully(t *testing.T) {
    store := newTestStore(t)
    ctx := context.Background()
    store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
    store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

    // Claim with a TTL that has already expired.
    past := time.Now().UTC().Add(-time.Second)
    if _, err := store.ClaimAgent(ctx, "developer", "flow", "wf-1", past); err != nil {
        t.Fatalf("claim: %v", err)
    }

    // Sweeper clears the expired row.
    released, err := store.SweepExpiredLeases(ctx, time.Now().UTC())
    if err != nil {
        t.Fatalf("sweep: %v", err)
    }
    if len(released) != 1 || released[0].ID != "a_001" {
        t.Fatalf("expected a_001 released, got %+v", released)
    }

    // Renew arrives after the sweep — must NOT silently re-claim. The CAS
    // on current_workflow_id no longer matches because the row was cleared.
    err = store.RenewAgentLease(ctx, "a_001", "wf-1", time.Now().UTC().Add(time.Hour))
    if !errors.Is(err, domain.ErrWorkflowMismatch) {
        t.Fatalf("expected ErrWorkflowMismatch, got %v", err)
    }

    // Confirm the agent really is free — not silently re-leased.
    got, _ := store.GetAgent(ctx, "a_001")
    if got.CurrentWorkflowID != "" {
        t.Errorf("agent still claimed after sweep+renew race: %+v", got)
    }
}
```

Add `"errors"` to the test file's imports if not already present.

**Step 2: Run**

```
mise run test -- -run TestSweepRaceWithRenewLosesGracefully ./internal/infra/sqlite/...
```
Expected: PASS — the existing CAS in `RenewAgentLease` (added in plan
02 Task 3) already enforces this. This task is locking the behavior in
with a regression test, not changing implementation. If it fails,
treat that as a real regression in the CAS implementation and fix the
store before the test passes.

**Step 3: Commit**

```
git add internal/infra/sqlite/agents_test.go
git commit -m "test(sqlite): renew loses cleanly when sweeper wins the race"
```

---

### Task 7: Full green run

**Step 1:**

```
mise run lint
mise run test
mise run e2e
```
Expected: all green.

**Step 2: No commit unless final fix-ups needed.**

---

## Completion Criteria

- `SweeperService.Start` runs on daemon boot with a configurable interval (default 30s; override via `--sweeper-interval` / `HIVE_SWEEPER_INTERVAL`).
- When `lease_expires_at < now`, the four `current_*` columns are reset to NULL and an info log line is emitted per released agent.
- The sweeper exits cleanly on SIGTERM (shares `healthCtx` lifetime).
- Unit tests verify SweepOnce releases expired claims and skips fresh ones.
- E2E test verifies end-to-end: claim with short TTL → wait → agent is back in the free pool.

Plan `-04-get-provisioning.md` remains to teach MCP `get_provisioning` about the assignment.
