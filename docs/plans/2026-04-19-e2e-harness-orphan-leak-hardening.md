---
type: plan
step: "1"
title: "hive e2e harness — orphan-leak hardening"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: null
  completed: null
related_plans: []
---

# Hive E2E Harness — Orphan-Leak Hardening

**Goal:** Stop the e2e harness from leaking orphan processes when the
`hive daemon` subprocess exits before its descendants do, and add a
graceful SIGTERM step so daemons get a chance to flush state before
being killed. The current `tests/e2e/harness_test.go` is the only
WorkFort e2e harness that uses bare `cmd.Process.Kill()` with no
SIGTERM at all (line 162) — every other repo at least sends SIGTERM
first. It also captures stdout/stderr to a `*os.File` log already
(line 110-117) but that file is closed before the daemon exits
(line 147), so a daemon that writes after the harness `lf.Close()`
silently writes to a closed fd. None of the four canonical fixes are
in place.

**Canonical fix** (see `/home/kazw/Work/WorkFort/skills/lead/go-service-architecture/references/architecture-reference.md` — section
"Orphan-Process Hardening (Required)"):

1. **`Setpgid: true`** in `cmd.SysProcAttr`.
2. **`*os.File` for stdout/stderr** held open until after `cmd.Wait`
   returns, then read for failure-dump and DATA RACE detection.
3. **Negative-pid kill** (`syscall.Kill(-pgid, sig)`).
4. **`cmd.WaitDelay = 10 * time.Second`** safety net.

All four parts are load-bearing.

**Repo specifics.** Hive's harness is the largest refactor of the six:
- The log file (`*os.File`) is closed inside `newHarness` before
  `Close` runs, so `Close` cannot read it back. Move the close into
  `Close` and read the bytes there.
- `Close` does `Process.Kill()` directly with no SIGTERM. Add the
  SIGTERM-then-deadline-then-SIGKILL pattern that every other repo
  has.
- The harness has no DATA RACE check today. Add one alongside the
  log-dump (this is a SHOULD-FIX for parity with the rest of
  WorkFort's e2e suites — keep it scoped to "if the file already
  contains DATA RACE, fail the test"; do not introduce race-detector
  flags or other coverage changes).
- The package is `e2e_test` (an external test package). The leak test
  goes in the same package and reuses `hiveBin` from `TestMain`.

**Tech stack:** Go 1.26 (e2e nested module), `os/exec`, `syscall`.
No new dependencies.

**Commands:** `mise run e2e` (the existing task at `.mise/tasks/e2e`)
runs `mise run build:dev` then `cd tests/e2e && go test -v -race
-count=1 -timeout 120s ./...`. Targeted runs use `cd tests/e2e &&
go test -run TestX -count=1 ./...`.

---

## Prerequisites

- `tests/e2e/go.mod` (Go 1.26) — `cmd.WaitDelay` (Go 1.20+) is
  available.
- `hiveBin` is set in `TestMain` (existing setup) so the new test can
  reuse it.

---

## Conventions

- Run all build/test commands via `mise run <task>` from `hive/lead/`.
  Targeted go test runs are permitted from inside `tests/e2e/`.
- Commit after each task with the multi-line conventional-commits
  HEREDOC and the Co-Authored-By trailer below.

```bash
git add <files>
git commit -m "$(cat <<'EOF'
<type>(<scope>): <description>

<body explaining why, not what>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task Breakdown

### Task 1: Write the failing leak-detection test

**Files:**
- Create: `tests/e2e/harness_leak_test.go`

**Step 1: Write the test**

The test starts a harness, calls `Close` explicitly, then asserts no
live process remains in the daemon's pgid. Without `Setpgid`,
`Getpgid(daemonPID)` returns the harness's group, not the daemon's
PID — the test fails.

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestHarnessClose_KillsProcessGroup(t *testing.T) {
	h := newHarness(t)
	pid := h.cmd.Process.Pid

	// pgid must equal pid because newHarness sets Setpgid.
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid(%d): %v", pid, err)
	}
	if pgid != pid {
		t.Fatalf("daemon pgid = %d, want %d (Setpgid not set)", pgid, pid)
	}
	// Defence against the (vanishingly rare) case where the test
	// process itself is in a group whose id equals the daemon PID —
	// pgid == pid would pass spuriously.
	if pgid == os.Getpid() {
		t.Fatalf("daemon pgid (%d) equals harness pid; daemon inherited harness group", pgid)
	}

	// newHarness already registers t.Cleanup(h.Close); call it
	// explicitly so the assertion below runs after the daemon is
	// gone, not after the test function returns.
	h.Close()

	// Use errors.Is (not direct ==) because syscall.Errno implements
	// the errors.Is contract and errors.Is is the idiomatic Go choice.
	if err := syscall.Kill(-pgid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill(-%d, 0) = %v, want ESRCH (group still has live members)", pgid, err)
	}
}
```

`newHarness` already registers a `t.Cleanup(h.Close)` callback. The
test calling `h.Close()` directly is safe because `Close` is
idempotent in Task 2's rewrite (the second invocation finds
`h.cmd.Process == nil` after the first set it to nil, or the second
SIGKILL is a harmless ESRCH). Verify idempotency in Task 2 Step 5.

**Step 2: Run the test to verify it fails**

Run from `hive/lead/`:

```
mise run build:dev
cd tests/e2e && go test -run TestHarnessClose_KillsProcessGroup -count=1 ./...
```

Expected: FAIL with `daemon pgid = <harness_pgid>, want <daemon_pid>
(Setpgid not set)`.

**Step 3: Commit the failing test**

```bash
git add tests/e2e/harness_leak_test.go
git commit -m "$(cat <<'EOF'
test(e2e): add failing TestHarnessClose_KillsProcessGroup

Asserts the daemon spawns into its own process group and that Close
empties the group. Currently fails because newHarness does not set
Setpgid; the next task fixes the harness.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Apply the four-part canonical fix and add SIGTERM path

**Depends on:** Task 1

**Files:**
- Modify: `tests/e2e/harness_test.go` — `Harness` struct, `newHarness`,
  `Close`.

**Step 1: Update the imports**

Add `bytes` and `syscall`. Keep `time` (already imported). The result
should include:

```go
import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/client"
)
```

**Step 2: Add `logFile` to the `Harness` struct**

The struct (lines 31-39) drops the `dir` field's role as the only
post-cleanup-readable handle for daemon output. Add a `*os.File` so
`Close` can read the captured log back.

```go
type Harness struct {
	t        *testing.T
	dir      string
	cmd      *exec.Cmd
	port     int
	Client   *client.Client
	stubStop func()
	signJWT  func(id, username, name, userType string) string
	logFile  *os.File // stdout+stderr capture; closed in Close after wait
}
```

**Step 3: Rewrite the spawn block in `newHarness`**

Replace lines 105-124 (from `cmd := exec.Command(` through the
`cmd.Start()` block and the literal that builds `h`) with:

```go
	cmd := exec.Command(
		hiveBin,
		"daemon",
		"--bind", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--db", dbPath,
		"--passport-url", "http://"+stubAddr,
		"--log-level", "disabled",
		"--sweeper-interval", "200ms",
	)
	cmd.Env = append(os.Environ(),
		"XDG_STATE_HOME="+stateDir,
		"XDG_CONFIG_HOME="+configDir,
	)

	logFile := filepath.Join(dir, "daemon.log")
	lf, err := os.Create(logFile)
	if err != nil {
		stubStop()
		os.RemoveAll(dir)
		t.Fatalf("create daemon log: %v", err)
	}
	// *os.File for stdout/stderr (not io.Writer) so exec.Cmd does
	// not create a copy goroutine; Setpgid puts the daemon and any
	// descendants in a fresh process group; WaitDelay force-closes
	// any inherited fds after the daemon exits. See the orphan-
	// process hardening section of go-service-architecture.
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		lf.Close()
		stubStop()
		os.RemoveAll(dir)
		t.Fatalf("start daemon: %v", err)
	}

	h := &Harness{
		t:        t,
		dir:      dir,
		cmd:      cmd,
		port:     port,
		Client:   client.New(fmt.Sprintf("http://127.0.0.1:%d", port), harnessToken),
		stubStop: stubStop,
		signJWT:  signJWT,
		logFile:  lf,
	}
```

**Step 4: Update the wait-healthy failure path**

Lines 137-148 read the log file for diagnostics if startup fails. With
`logFile` now stored on the struct, the code is simpler:

```go
	if err := h.waitHealthy(); err != nil {
		// Read what the daemon managed to write before death.
		if b, readErr := os.ReadFile(logFile); readErr == nil {
			t.Logf("daemon log:\n%s", b)
		}
		h.Close()
		t.Fatalf("daemon did not become healthy: %v", err)
	}

	t.Cleanup(h.Close)
	return h
}
```

The bare `lf.Close()` at line 147 is gone — `Close` owns the log file
lifecycle now. Calling `os.ReadFile(logFile)` while `lf` is still open
is safe; the OS returns whatever bytes have been written through the
fd.

**Step 5: Rewrite `Close` to graceful-stop the process group**

Replace `Close` (lines 154-167) with:

```go
// Close stops the daemon process (SIGTERM, then SIGKILL after 10s),
// shuts down the JWKS stub, and removes the temp directory. Reads
// the captured stdout/stderr after the daemon exits, dumps it on
// test failure, and fails the test if it contains DATA RACE.
//
// Idempotent under sequential calls (the test calls Close explicitly
// and t.Cleanup runs Close again on test exit; the second call is a
// no-op). NOT safe to call concurrently — fields are zeroed without
// locking. The t.Cleanup contract is sequential, so this is fine.
func (h *Harness) Close() {
	h.t.Helper()
	if h.stubStop != nil {
		h.stubStop()
		h.stubStop = nil
	}
	if h.cmd != nil && h.cmd.Process != nil {
		pgid := h.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- h.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-done
		}
		// Mark process as reaped so a second Close is a no-op.
		h.cmd.Process = nil
	}

	var logBytes []byte
	if h.logFile != nil {
		// Read whatever the daemon wrote, then close and unlink.
		// (We can't unlink while open on Windows, but the e2e
		// harness is Linux-only.)
		logBytes, _ = os.ReadFile(h.logFile.Name())
		h.logFile.Close()
		h.logFile = nil
	}

	if h.t.Failed() && len(logBytes) > 0 {
		h.t.Logf("daemon log:\n%s", logBytes)
	}
	if bytes.Contains(logBytes, []byte("DATA RACE")) {
		h.t.Errorf("data race detected in daemon log:\n%s", logBytes)
	}

	if h.dir != "" {
		os.RemoveAll(h.dir)
		h.dir = ""
	}
}
```

The DATA RACE check uses `t.Errorf` (not `t.Fatalf`) because `Close`
runs in a `t.Cleanup` callback where `Fatalf` would not actually halt
the test — the test is already done. `Errorf` correctly marks the
test as failed.

**Step 6: Run the leak test to verify it passes**

Run: `mise run e2e` filtered to the leak test:

```
mise run build:dev
cd tests/e2e && go test -run TestHarnessClose_KillsProcessGroup -count=1 ./...
```

Expected: PASS.

**Step 7: Run the full e2e suite to verify no regression**

Run: `mise run e2e`

Expected: PASS. Existing harness tests still see the daemon start,
hit endpoints, and shut down cleanly. The graceful-SIGTERM step is
a new behaviour but should be transparent — the daemon receives
SIGTERM where it previously received SIGKILL, and ends up in the
same state.

**Step 8: Commit**

```bash
git add tests/e2e/harness_test.go
git commit -m "$(cat <<'EOF'
fix(e2e): harden harness against orphan leaks; add graceful SIGTERM

Spawn the hive daemon into its own process group (Setpgid), keep the
stdout/stderr log file open through Close so the captured output can
be read back, signal the whole group on shutdown (kill(-pgid, ...))
with SIGTERM-then-SIGKILL after a 10s grace, and set WaitDelay so
cmd.Wait force-closes any inherited fd after the daemon exits.

Hive was the only WorkFort e2e harness using a bare Process.Kill
with no SIGTERM, and the only one closing the log file before
cmd.Wait. The combination meant a daemon that took longer than the
harness deadline to flush state was killed mid-write to a closed
fd, and any descendant inheriting the log fd kept cmd.Wait blocked
until the workflow timeout fired. Adds a DATA RACE check on the
captured log for parity with the other repos.

Implements the canonical e2e-harness orphan-leak hardening pattern
documented in skills/lead/go-service-architecture/references/architecture-reference.md
(section "Orphan-Process Hardening (Required)").

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Verify cleanup is bounded under simulated test failure

**Depends on:** Task 2

**Files:**
- (Temporary, reverted) inject `t.Fatal` into the cheapest existing
  e2e test.

**Step 1: Confirm working tree is clean**

Run `git status`. Expected: clean. The next step injects a temporary
edit; a clean tree before this step ensures revert is unambiguous.

**Step 2: Inject a forced failure**

Pick a small existing test in `tests/e2e/` that calls `newHarness`
and add `t.Fatal("synthetic failure to verify cleanup bound")`
immediately after `newHarness` returns. Do not commit. Optionally
`git stash push -k -m "synthetic-failure"` then `git stash pop` so
the diff is recoverable if the timing run is interrupted.

**Step 3: Time `mise run e2e`**

Run from `hive/lead/`:

```
time mise run e2e
```

Expected:

- The synthetic test FAILs.
- Total wall clock under 30 seconds (typically 10-15s including the
  build:dev step). The harness's 10-second SIGKILL deadline is the
  worst case; the daemon should respond to SIGTERM in well under a
  second.

If the run exceeds 30 seconds, inspect with `ps -o pid,pgid,cmd
-p $(pgrep -f hive.*daemon)` and re-check the four parts.

**Step 4: Revert the synthetic failure**

`git checkout -- <test_file>` to restore. Run `git status` and
confirm the working tree is clean.

**Step 5: Final regression run**

Run: `mise run e2e`
Expected: PASS, all tests green.

No commit for this task — verification only.

---

## Verification Checklist

After all tasks complete:

- [ ] `mise run e2e` passes from `hive/lead/`.
- [ ] `TestHarnessClose_KillsProcessGroup` passes; reverting the
  `Setpgid` line in `harness_test.go` makes it fail with the expected
  message.
- [ ] `Harness.logFile` is `*os.File` and is closed inside `Close`
  after `cmd.Wait` returns, not in `newHarness`.
- [ ] `cmd.SysProcAttr.Setpgid == true`, `cmd.WaitDelay == 10s`,
  `cmd.Stdout`/`cmd.Stderr` both `*os.File`.
- [ ] `Close` uses `syscall.Kill(-pgid, sig)` with SIGTERM then
  SIGKILL after a 10s deadline, never `cmd.Process.Kill` directly.
- [ ] DATA RACE check runs against the read-back log bytes (new
  behaviour for hive).
- [ ] `Close` is idempotent — calling it twice does not panic.
- [ ] `time mise run e2e` with an injected `t.Fatal` returns in
  under 30 seconds (Task 3 spot check).

## Out of Scope

- Adding race-detector flags or other coverage changes beyond the
  DATA RACE-string check on the log.
- Refactoring how `dir`, `dbPath`, or the JWKS stub are wired.
- Cross-repo coordination — each affected harness gets its own plan.
