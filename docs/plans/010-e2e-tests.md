# E2E Test Suite Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a black-box end-to-end test suite in a separate Go module at `tests/e2e/`. TestMain builds the `hive` binary with `-race`, a test harness starts and stops the daemon on a random port, and test files exercise every REST API resource group using the `client/` package.

**Architecture:** Separate `go.mod` in `tests/e2e/` that replaces `github.com/Work-Fort/Hive/client` with a local path. TestMain compiles the binary once into a temp directory. Each test (or sub-test) gets its own `Harness` which starts a fresh daemon process with isolated XDG directories and a new random port, then stops it after the test.

**Tech Stack:** Go 1.26, `testing`, `os/exec`, `net/http`, `github.com/Work-Fort/Hive/client`

**Depends on:** All previous plans (001-009) must be complete.

---

## Chunk 1: Module Bootstrap

### Task 1: Create the separate Go module

**Files:**
- Create: `tests/e2e/go.mod`
- Create: `tests/e2e/go.sum` (generated)

- [ ] **Step 1: Create go.mod with replace directive**

Create `tests/e2e/go.mod`:

```
module github.com/Work-Fort/Hive/tests/e2e

go 1.26

require github.com/Work-Fort/Hive/client v0.0.0

replace github.com/Work-Fort/Hive/client => ../../client
```

The `replace` directive points at the local `client/` tree so no version tag is needed. The `client` package has zero `internal/` imports, so the go tool accepts this cross-module reference.

- [ ] **Step 2: Initialize go.sum**

```bash
cd tests/e2e && go mod tidy
```

- [ ] **Step 3: Verify the module resolves**

```bash
cd tests/e2e && go list ./...
```

- [ ] **Step 4: Commit**

```bash
git add tests/e2e/go.mod tests/e2e/go.sum
git commit -m "feat: add e2e test module with client replace directive"
```

---

## Chunk 2: Test Harness

### Task 2: TestMain — binary build

**Files:**
- Create: `tests/e2e/main_test.go`

TestMain is the entry point for the test binary. It compiles the `hive` binary once with `-race` into a temp directory shared by all tests in the run. If the build fails, all tests are skipped with a clear error.

- [ ] **Step 1: Create main_test.go**

Create `tests/e2e/main_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// hiveBin is the path to the compiled hive binary, set by TestMain.
var hiveBin string

// TestMain compiles the hive binary once with -race before running any tests.
// All tests in the package share the same binary.
func TestMain(m *testing.M) {
	// Resolve the repository root relative to this file's location.
	// tests/e2e/ is two levels below the repo root.
	repoRoot, err := filepath.Abs("../../")
	if err != nil {
		panic("resolve repo root: " + err.Error())
	}

	// Build into a temp dir so we don't dirty the working tree.
	tmpDir, err := os.MkdirTemp("", "hive-e2e-*")
	if err != nil {
		panic("create temp dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	hiveBin = filepath.Join(tmpDir, "hive")

	cmd := exec.Command("go", "build", "-race", "-o", hiveBin, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stderr // build output goes to stderr so test runner shows it
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Print a clear message and exit — don't try to run tests against a
		// missing binary.
		os.Stderr.WriteString("FATAL: failed to build hive binary: " + err.Error() + "\n")
		os.Exit(1)
	}

	os.Exit(m.Run())
}
```

- [ ] **Step 2: Verify it compiles (no tests yet)**

```bash
cd tests/e2e && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/main_test.go
git commit -m "feat: add TestMain that builds hive binary with -race"
```

### Task 3: Harness — daemon lifecycle manager

**Files:**
- Create: `tests/e2e/harness_test.go`

The `Harness` struct starts a daemon process on a random free port, waits for the health endpoint to respond, and exposes a configured `*client.Client`. Cleanup shuts down the process and removes temp directories.

- [ ] **Step 1: Create harness_test.go**

Create `tests/e2e/harness_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/client"
)

const (
	// testAPIKey is the shared API key used by every harness in the test run.
	testAPIKey = "e2e-test-key-do-not-use-in-prod"

	// startupTimeout is how long to wait for the daemon health endpoint to
	// respond before declaring startup a failure.
	startupTimeout = 15 * time.Second

	// pollInterval is how often to probe the health endpoint during startup.
	pollInterval = 50 * time.Millisecond
)

// Harness manages a single daemon process for a test. Each call to
// newHarness starts a fresh daemon with isolated XDG directories and a
// random port. Call h.Close() (or register it with t.Cleanup) to stop the
// daemon and remove temp files.
type Harness struct {
	t      *testing.T
	dir    string         // root temp directory for this harness
	cmd    *exec.Cmd      // the running hive process
	port   int            // the port the daemon is listening on
	Client *client.Client // pre-configured client for this harness
}

// newHarness creates a Harness, starts the daemon, and waits for it to be
// healthy. The harness is automatically closed via t.Cleanup.
func newHarness(t *testing.T) *Harness {
	t.Helper()

	// Per-test temp directory tree.
	dir, err := os.MkdirTemp("", "hive-e2e-harness-*")
	if err != nil {
		t.Fatalf("create harness temp dir: %v", err)
	}

	// XDG directories inside the temp tree.
	stateDir := filepath.Join(dir, "state")
	configDir := filepath.Join(dir, "config")
	for _, d := range []string{stateDir, configDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			os.RemoveAll(dir)
			t.Fatalf("create xdg dir %s: %v", d, err)
		}
	}

	// Pick a free TCP port.
	port, err := freePort()
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("pick free port: %v", err)
	}

	// SQLite database inside the state directory.
	dbPath := filepath.Join(stateDir, "hive.db")

	cmd := exec.Command(
		hiveBin,
		"daemon",
		"--bind", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--db", dbPath,
		"--api-key", testAPIKey,
		"--log-level", "disabled",
	)
	// XDG env vars scope config/state to our temp dirs.
	cmd.Env = append(os.Environ(),
		"XDG_STATE_HOME="+stateDir,
		"XDG_CONFIG_HOME="+configDir,
	)
	// Capture daemon output to a file so failures can be diagnosed.
	logFile := filepath.Join(dir, "daemon.log")
	lf, err := os.Create(logFile)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("create daemon log: %v", err)
	}
	cmd.Stdout = lf
	cmd.Stderr = lf

	if err := cmd.Start(); err != nil {
		lf.Close()
		os.RemoveAll(dir)
		t.Fatalf("start daemon: %v", err)
	}

	h := &Harness{
		t:      t,
		dir:    dir,
		cmd:    cmd,
		port:   port,
		Client: client.New(fmt.Sprintf("http://127.0.0.1:%d", port), testAPIKey),
	}

	// Wait for the health endpoint to respond.
	if err := h.waitHealthy(); err != nil {
		h.Close()
		// Print the daemon log to help diagnose startup failures.
		lf.Close()
		if b, readErr := os.ReadFile(logFile); readErr == nil {
			t.Logf("daemon log:\n%s", b)
		}
		t.Fatalf("daemon did not become healthy: %v", err)
	}

	lf.Close()
	t.Cleanup(h.Close)
	return h
}

// Close stops the daemon process and removes the temp directory.
func (h *Harness) Close() {
	h.t.Helper()
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
	}
	if h.dir != "" {
		os.RemoveAll(h.dir)
	}
}

// waitHealthy polls the health endpoint until it returns 2xx or the
// startupTimeout is exceeded.
func (h *Harness) waitHealthy() error {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", h.port)
	deadline := time.Now().Add(startupTimeout)
	httpClient := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			// Any 2xx response (including 218 Degraded) is a successful start.
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				return nil
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timed out after %s waiting for health endpoint %s", startupTimeout, url)
}

// freePort asks the OS for an available TCP port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

// ctx returns a background context. Helper so test bodies stay concise.
func ctx() context.Context {
	return context.Background()
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd tests/e2e && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/harness_test.go
git commit -m "feat: add e2e test harness that manages daemon lifecycle"
```

---

## Chunk 3: Health and Smoke Tests

### Task 4: Health endpoint test

**Files:**
- Create: `tests/e2e/health_test.go`

A minimal test that verifies the daemon starts cleanly and reports healthy status. This test also acts as a smoke test for the whole test infrastructure (binary build, harness, client).

- [ ] **Step 1: Create health_test.go**

Create `tests/e2e/health_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"fmt"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestHealth(t *testing.T) {
	h := newHarness(t)

	report, err := h.Client.Health(ctx())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if report.Status != "healthy" {
		t.Errorf("expected status %q, got %q", "healthy", report.Status)
	}
}

// TestHealthUnauthenticated verifies that the health endpoint does not require
// an API key — it is intentionally public.
func TestHealthUnauthenticated(t *testing.T) {
	h := newHarness(t)

	// Create a client with no API key targeting the same port.
	unauthClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"",
	)

	report, err := unauthClient.Health(ctx())
	if err != nil {
		t.Fatalf("Health without API key: %v", err)
	}
	if report.Status == "" {
		t.Error("expected non-empty status in health report")
	}
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd tests/e2e && go build ./...
```

- [ ] **Step 3: Run the health tests (requires the main module to be built)**

```bash
cd tests/e2e && go test -v -run TestHealth ./...
```

- [ ] **Step 4: Commit**

```bash
git add tests/e2e/health_test.go
git commit -m "test: add e2e health endpoint tests"
```

---

## Chunk 4: Resource CRUD Tests

### Task 5: Teams CRUD tests

**Files:**
- Create: `tests/e2e/teams_test.go`

- [ ] **Step 1: Create teams_test.go**

Create `tests/e2e/teams_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestTeams(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Create
	team, err := c.CreateTeam(ctx(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.Name != "alpha" {
		t.Errorf("Name: got %q, want %q", team.Name, "alpha")
	}
	if team.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Get
	got, err := c.GetTeam(ctx(), team.ID)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.ID != team.ID {
		t.Errorf("GetTeam ID mismatch: %q vs %q", got.ID, team.ID)
	}

	// List
	teams, err := c.ListTeams(ctx())
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if !containsTeam(teams, team.ID) {
		t.Errorf("ListTeams: created team %q not found in list", team.ID)
	}

	// Update
	updated, err := c.UpdateTeam(ctx(), team.ID, "alpha-renamed")
	if err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	if updated.Name != "alpha-renamed" {
		t.Errorf("UpdateTeam Name: got %q, want %q", updated.Name, "alpha-renamed")
	}

	// Delete
	if err := c.DeleteTeam(ctx(), team.ID); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}

	// Confirm gone
	_, err = c.GetTeam(ctx(), team.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetTeam after delete: expected ErrNotFound, got %v", err)
	}
}

// TestTeamDuplicateName verifies that creating two teams with the same name
// returns a conflict error.
func TestTeamDuplicateName(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	if _, err := c.CreateTeam(ctx(), "beta"); err != nil {
		t.Fatalf("first CreateTeam: %v", err)
	}

	_, err := c.CreateTeam(ctx(), "beta")
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("duplicate team name: expected ErrConflict, got %v", err)
	}
}

// TestTeamDeleteWithDependencies verifies that a team with agents cannot be
// deleted.
func TestTeamDeleteWithDependencies(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "gamma")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := c.CreateAgent(ctx(), "dep-agent", team.ID); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	err = c.DeleteTeam(ctx(), team.ID)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("delete team with agent: expected ErrConflict, got %v", err)
	}
}

func containsTeam(teams []client.Team, id string) bool {
	for _, t := range teams {
		if t.ID == id {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests**

```bash
cd tests/e2e && go test -v -run TestTeam ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/teams_test.go
git commit -m "test: add e2e teams CRUD tests"
```

### Task 6: Roles CRUD tests

**Files:**
- Create: `tests/e2e/roles_test.go`

- [ ] **Step 1: Create roles_test.go**

Create `tests/e2e/roles_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestRoles(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Create root role
	root, err := c.CreateRole(ctx(), "developer", "")
	if err != nil {
		t.Fatalf("CreateRole root: %v", err)
	}
	if root.Name != "developer" {
		t.Errorf("Name: got %q, want %q", root.Name, "developer")
	}
	if root.ParentID != "" {
		t.Errorf("ParentID: expected empty, got %q", root.ParentID)
	}

	// Create child role
	child, err := c.CreateRole(ctx(), "frontend-developer", root.ID)
	if err != nil {
		t.Fatalf("CreateRole child: %v", err)
	}
	if child.ParentID != root.ID {
		t.Errorf("ParentID: got %q, want %q", child.ParentID, root.ID)
	}

	// Get
	got, err := c.GetRole(ctx(), root.ID)
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got.ID != root.ID {
		t.Errorf("GetRole ID: got %q, want %q", got.ID, root.ID)
	}

	// List (all)
	roles, err := c.ListRoles(ctx(), "")
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if !containsRole(roles, root.ID) || !containsRole(roles, child.ID) {
		t.Errorf("ListRoles missing created roles")
	}

	// List filtered by parent
	children, err := c.ListRoles(ctx(), root.ID)
	if err != nil {
		t.Fatalf("ListRoles by parent: %v", err)
	}
	if len(children) != 1 || children[0].ID != child.ID {
		t.Errorf("ListRoles(parent=%s): expected [%s], got %v", root.ID, child.ID, children)
	}

	// Update
	updated, err := c.UpdateRole(ctx(), root.ID, "engineer", "")
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updated.Name != "engineer" {
		t.Errorf("UpdateRole Name: got %q, want %q", updated.Name, "engineer")
	}

	// Delete child first (parent still has a child, can't delete parent yet)
	if err := c.DeleteRole(ctx(), child.ID); err != nil {
		t.Fatalf("DeleteRole child: %v", err)
	}

	// Delete parent
	if err := c.DeleteRole(ctx(), root.ID); err != nil {
		t.Fatalf("DeleteRole root: %v", err)
	}

	// Confirm gone
	_, err = c.GetRole(ctx(), root.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetRole after delete: expected ErrNotFound, got %v", err)
	}
}

// TestRoleDeleteWithChildren verifies that a role with child roles cannot be
// deleted.
func TestRoleDeleteWithChildren(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	parent, err := c.CreateRole(ctx(), "base", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if _, err := c.CreateRole(ctx(), "derived", parent.ID); err != nil {
		t.Fatalf("CreateRole child: %v", err)
	}

	err = c.DeleteRole(ctx(), parent.ID)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("delete role with children: expected ErrConflict, got %v", err)
	}
}

func containsRole(roles []client.Role, id string) bool {
	for _, r := range roles {
		if r.ID == id {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests**

```bash
cd tests/e2e && go test -v -run TestRole ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/roles_test.go
git commit -m "test: add e2e roles CRUD tests"
```

### Task 7: Agents CRUD tests

**Files:**
- Create: `tests/e2e/agents_test.go`

- [ ] **Step 1: Create agents_test.go**

Create `tests/e2e/agents_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestAgents(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Setup: team and role required for agents and role assignment.
	team, err := c.CreateTeam(ctx(), "agents-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	role, err := c.CreateRole(ctx(), "agents-role", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Create
	agent, err := c.CreateAgent(ctx(), "alice", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if agent.Name != "alice" {
		t.Errorf("Name: got %q, want %q", agent.Name, "alice")
	}
	if agent.TeamID != team.ID {
		t.Errorf("TeamID: got %q, want %q", agent.TeamID, team.ID)
	}

	// Get (includes roles slice, initially empty)
	got, err := c.GetAgent(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.ID != agent.ID {
		t.Errorf("GetAgent ID: got %q, want %q", got.ID, agent.ID)
	}
	if len(got.Roles) != 0 {
		t.Errorf("GetAgent Roles: expected empty, got %v", got.Roles)
	}

	// List
	agents, err := c.ListAgents(ctx(), "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if !containsAgent(agents, agent.ID) {
		t.Errorf("ListAgents: agent %q not found", agent.ID)
	}

	// List filtered by team
	teamAgents, err := c.ListAgents(ctx(), team.ID)
	if err != nil {
		t.Fatalf("ListAgents by team: %v", err)
	}
	if !containsAgent(teamAgents, agent.ID) {
		t.Errorf("ListAgents(team=%s): agent not found", team.ID)
	}

	// Update
	updated, err := c.UpdateAgent(ctx(), agent.ID, "alice-updated", team.ID)
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if updated.Name != "alice-updated" {
		t.Errorf("UpdateAgent Name: got %q, want %q", updated.Name, "alice-updated")
	}

	// Set role assignments
	assignments := []client.RoleAssignment{
		{RoleID: role.ID, Priority: 1},
	}
	agentRoles, err := c.SetAgentRoles(ctx(), agent.ID, assignments)
	if err != nil {
		t.Fatalf("SetAgentRoles: %v", err)
	}
	if len(agentRoles) != 1 || agentRoles[0].RoleID != role.ID {
		t.Errorf("SetAgentRoles: expected 1 assignment for role %s, got %v", role.ID, agentRoles)
	}

	// Get after role assignment
	withRoles, err := c.GetAgent(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after SetAgentRoles: %v", err)
	}
	if len(withRoles.Roles) != 1 {
		t.Errorf("GetAgent Roles: expected 1, got %d", len(withRoles.Roles))
	}

	// Clear role assignments
	if _, err := c.SetAgentRoles(ctx(), agent.ID, []client.RoleAssignment{}); err != nil {
		t.Fatalf("SetAgentRoles (clear): %v", err)
	}

	// Delete
	if err := c.DeleteAgent(ctx(), agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// Confirm gone
	_, err = c.GetAgent(ctx(), agent.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetAgent after delete: expected ErrNotFound, got %v", err)
	}
}

func containsAgent(agents []client.Agent, id string) bool {
	for _, a := range agents {
		if a.ID == id {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests**

```bash
cd tests/e2e && go test -v -run TestAgent ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/agents_test.go
git commit -m "test: add e2e agents CRUD and role assignment tests"
```

### Task 8: Documents CRUD tests

**Files:**
- Create: `tests/e2e/documents_test.go`

- [ ] **Step 1: Create documents_test.go**

Create `tests/e2e/documents_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestRoleDocuments(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	role, err := c.CreateRole(ctx(), "docs-role", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Create
	doc, err := c.CreateRoleDocument(ctx(), role.ID, "Dev Guide", "# Dev Guide\nContent here.")
	if err != nil {
		t.Fatalf("CreateRoleDocument: %v", err)
	}
	if doc.Title != "Dev Guide" {
		t.Errorf("Title: got %q, want %q", doc.Title, "Dev Guide")
	}
	if doc.Kind != "role" {
		t.Errorf("Kind: got %q, want %q", doc.Kind, "role")
	}
	if doc.RoleID != role.ID {
		t.Errorf("RoleID: got %q, want %q", doc.RoleID, role.ID)
	}

	// List
	docs, err := c.ListRoleDocuments(ctx(), role.ID)
	if err != nil {
		t.Fatalf("ListRoleDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != doc.ID {
		t.Errorf("ListRoleDocuments: expected [%s], got %v", doc.ID, docs)
	}

	// Get
	got, err := c.GetDocument(ctx(), doc.ID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.Content != "# Dev Guide\nContent here." {
		t.Errorf("Content mismatch")
	}

	// Update
	updated, err := c.UpdateDocument(ctx(), doc.ID, "Dev Guide v2", "# Updated")
	if err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}
	if updated.Title != "Dev Guide v2" {
		t.Errorf("UpdateDocument Title: got %q, want %q", updated.Title, "Dev Guide v2")
	}

	// Delete
	if err := c.DeleteDocument(ctx(), doc.ID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	// Confirm gone
	_, err = c.GetDocument(ctx(), doc.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetDocument after delete: expected ErrNotFound, got %v", err)
	}
}

func TestAgentMemoryDocuments(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "mem-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "mem-agent", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Create memory document
	doc, err := c.CreateAgentMemory(ctx(), agent.ID, "Notes", "Some notes.")
	if err != nil {
		t.Fatalf("CreateAgentMemory: %v", err)
	}
	if doc.Kind != "memory" {
		t.Errorf("Kind: got %q, want %q", doc.Kind, "memory")
	}
	if doc.AgentID != agent.ID {
		t.Errorf("AgentID: got %q, want %q", doc.AgentID, agent.ID)
	}

	// List
	docs, err := c.ListAgentMemory(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("ListAgentMemory: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != doc.ID {
		t.Errorf("ListAgentMemory: expected [%s], got %v", doc.ID, docs)
	}

	// Delete
	if err := c.DeleteDocument(ctx(), doc.ID); err != nil {
		t.Fatalf("DeleteDocument (memory): %v", err)
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd tests/e2e && go test -v -run TestDocument ./... && go test -v -run TestAgentMemory ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/documents_test.go
git commit -m "test: add e2e role document and agent memory tests"
```

### Task 9: Tasks CRUD tests

**Files:**
- Create: `tests/e2e/tasks_test.go`

- [ ] **Step 1: Create tasks_test.go**

Create `tests/e2e/tasks_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestTasks(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Setup: tasks require a team; agent is optional.
	team, err := c.CreateTeam(ctx(), "tasks-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "task-agent", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Create (unassigned)
	task, err := c.CreateTask(ctx(), client.CreateTaskInput{
		TeamID:      team.ID,
		Title:       "Write tests",
		Description: "Cover all endpoints",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.Title != "Write tests" {
		t.Errorf("Title: got %q, want %q", task.Title, "Write tests")
	}
	if task.Status != "pending" {
		t.Errorf("Status: got %q, want %q", task.Status, "pending")
	}
	if task.AgentID != "" {
		t.Errorf("AgentID: expected empty (unassigned), got %q", task.AgentID)
	}

	// Get
	got, err := c.GetTask(ctx(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("GetTask ID: got %q, want %q", got.ID, task.ID)
	}

	// List by team
	tasks, err := c.ListTeamTasks(ctx(), team.ID)
	if err != nil {
		t.Fatalf("ListTeamTasks: %v", err)
	}
	if !containsTask(tasks, task.ID) {
		t.Errorf("ListTeamTasks: task %q not found", task.ID)
	}

	// Update — assign agent, change status
	updated, err := c.UpdateTask(ctx(), task.ID, client.UpdateTaskInput{
		Status:  "in_progress",
		AgentID: agent.ID,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Status != "in_progress" {
		t.Errorf("Status after update: got %q, want %q", updated.Status, "in_progress")
	}
	if updated.AgentID != agent.ID {
		t.Errorf("AgentID after update: got %q, want %q", updated.AgentID, agent.ID)
	}

	// Delete — must unassign first (task has no assignment restriction per design,
	// but agent deletion is blocked by assigned tasks, not task deletion)
	if err := c.DeleteTask(ctx(), task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	// Confirm gone
	_, err = c.GetTask(ctx(), task.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetTask after delete: expected ErrNotFound, got %v", err)
	}
}

// TestAgentDeleteBlockedByTask verifies that an agent with an assigned task
// cannot be deleted.
func TestAgentDeleteBlockedByTask(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "block-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "block-agent", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	_, err = c.CreateTask(ctx(), client.CreateTaskInput{
		TeamID:  team.ID,
		AgentID: agent.ID,
		Title:   "blocking task",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	err = c.DeleteAgent(ctx(), agent.ID)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("delete agent with task: expected ErrConflict, got %v", err)
	}
}

func containsTask(tasks []client.Task, id string) bool {
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests**

```bash
cd tests/e2e && go test -v -run TestTask ./... && go test -v -run TestAgentDelete ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/tasks_test.go
git commit -m "test: add e2e tasks CRUD and deletion constraint tests"
```

### Task 10: Permissions tests

**Files:**
- Create: `tests/e2e/permissions_test.go`

- [ ] **Step 1: Create permissions_test.go**

Create `tests/e2e/permissions_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestPermissions(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "perm-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "perm-agent", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Initially no permissions.
	perms, err := c.GetAgentPermissions(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgentPermissions (empty): %v", err)
	}
	if len(perms) != 0 {
		t.Errorf("expected 0 permissions initially, got %d", len(perms))
	}

	// Set global permissions.
	grants := []client.PermissionGrant{
		{Permission: "role:read"},
		{Permission: "memory:read"},
		{Permission: "memory:write"},
		{Permission: "task:read"},
	}
	set, err := c.SetAgentPermissions(ctx(), agent.ID, grants)
	if err != nil {
		t.Fatalf("SetAgentPermissions: %v", err)
	}
	if len(set) != len(grants) {
		t.Errorf("SetAgentPermissions: got %d, want %d", len(set), len(grants))
	}

	// Read back.
	perms, err = c.GetAgentPermissions(ctx(), agent.ID)
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}
	if len(perms) != len(grants) {
		t.Errorf("GetAgentPermissions after set: got %d, want %d", len(perms), len(grants))
	}

	// Overwrite with a scoped grant.
	scopedGrants := []client.PermissionGrant{
		{Permission: "task:write", ScopeTeamID: team.ID},
	}
	scoped, err := c.SetAgentPermissions(ctx(), agent.ID, scopedGrants)
	if err != nil {
		t.Fatalf("SetAgentPermissions (scoped): %v", err)
	}
	if len(scoped) != 1 {
		t.Errorf("scoped SetAgentPermissions: got %d, want 1", len(scoped))
	}
	if scoped[0].ScopeTeamID != team.ID {
		t.Errorf("ScopeTeamID: got %q, want %q", scoped[0].ScopeTeamID, team.ID)
	}

	// Revoke all.
	revoked, err := c.SetAgentPermissions(ctx(), agent.ID, []client.PermissionGrant{})
	if err != nil {
		t.Fatalf("SetAgentPermissions (revoke all): %v", err)
	}
	if len(revoked) != 0 {
		t.Errorf("after revoke: expected 0, got %d", len(revoked))
	}
}

// TestUnauthorizedRequest verifies that requests with a wrong API key are
// rejected with ErrUnauthorized.
func TestUnauthorizedRequest(t *testing.T) {
	h := newHarness(t)

	badClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"wrong-key",
	)

	_, err := badClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("wrong API key: expected ErrUnauthorized, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd tests/e2e && go test -v -run TestPermission ./... && go test -v -run TestUnauthorized ./...
```

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/permissions_test.go
git commit -m "test: add e2e permissions and auth tests"
```

---

## Chunk 5: Full Suite Run and CI Integration

### Task 11: Run the full test suite

This task verifies the entire suite passes end-to-end.

- [ ] **Step 1: Run all e2e tests with race detector**

```bash
cd tests/e2e && go test -race -timeout 120s -v ./...
```

The `-race` flag on the test binary itself (in addition to the race-instrumented daemon binary) catches any races in the test harness itself.

- [ ] **Step 2: Run with -count=1 to bypass cache**

```bash
cd tests/e2e && go test -race -count=1 -timeout 120s ./...
```

- [ ] **Step 3: Commit (no file changes — just confirm green)**

If all tests pass, no additional commit is needed. If any test revealed a bug in the harness or a test assertion, fix and commit with message:

```bash
git commit -m "fix: correct e2e test assertion for <specific issue>"
```

### Task 12: Add mise task for e2e tests

**Files:**
- Modify: `mise.toml`

- [ ] **Step 1: Add e2e test task to mise.toml**

Read the existing `mise.toml` first to understand the current task format, then add an `e2e` task:

```toml
[tasks.e2e]
description = "Run e2e test suite (builds binary with -race first)"
run = "cd tests/e2e && go test -race -count=1 -timeout 120s ./..."
```

- [ ] **Step 2: Verify the task runs**

```bash
mise run e2e
```

- [ ] **Step 3: Commit**

```bash
git add mise.toml
git commit -m "chore: add mise e2e task for end-to-end test suite"
```

---

## Summary

### Files Created

| File | Purpose |
|---|---|
| `tests/e2e/go.mod` | Separate module; replaces `client/` with local path |
| `tests/e2e/go.sum` | Dependency checksums |
| `tests/e2e/main_test.go` | TestMain: builds hive binary with `-race` |
| `tests/e2e/harness_test.go` | Harness: starts/stops daemon, exposes client |
| `tests/e2e/health_test.go` | Health endpoint smoke tests |
| `tests/e2e/teams_test.go` | Teams CRUD + constraint tests |
| `tests/e2e/roles_test.go` | Roles CRUD + parent filter + constraint tests |
| `tests/e2e/agents_test.go` | Agents CRUD + role assignment tests |
| `tests/e2e/documents_test.go` | Role documents + agent memory CRUD tests |
| `tests/e2e/tasks_test.go` | Tasks CRUD + deletion constraint tests |
| `tests/e2e/permissions_test.go` | Permission set/get/revoke + auth rejection |

### Files Modified

| File | Change |
|---|---|
| `mise.toml` | Add `mise run e2e` task |

### Design Decisions

1. **One harness per test function.** Each top-level `Test*` function calls `newHarness(t)` and gets an isolated daemon process with its own SQLite database and XDG directories. Tests are fully independent and can run with `t.Parallel()` if needed in the future.

2. **Binary compiled once per test run.** TestMain builds the binary and stores the path in the package-level `hiveBin` variable. All harnesses in a single `go test` invocation reuse the same binary, keeping the build cost at one compile.

3. **Race detection at two layers.** The daemon binary is compiled with `-race` so the Go runtime detects races inside the daemon. The test binary itself is also run with `-race` (via `go test -race`) to catch races in the harness and test logic.

4. **Health polling for startup.** The harness polls `/v1/health` at 50 ms intervals for up to 15 seconds. This is more reliable than sleeping a fixed duration and works across fast and slow CI machines.

5. **Daemon log captured to file.** Each harness writes daemon stdout/stderr to `<tmpdir>/daemon.log`. If `waitHealthy` fails, the harness reads and prints this file before calling `t.Fatalf`, giving immediate visibility into daemon startup errors.

6. **Tests target smoke coverage, not exhaustive validation.** Each resource group tests happy-path CRUD plus the most important constraint (e.g., conflict on duplicate name, conflict on delete with dependencies). This is sufficient to verify the REST API is wired end-to-end through the store.
