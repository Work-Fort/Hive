# Health Service Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the existing `HealthService` stub into a full registry with named, typed health checks (boot-time and periodic), a `Ping` port on the `Store` interface for database connectivity checks, and a richer `GET /v1/health` JSON response that includes per-check results alongside the overall status.

**Architecture:** The existing `HealthService` in `internal/daemon/health.go` already exposes `AddWarning` / `AddError` and drives the HTTP handler. This plan replaces the raw warning/error string lists with a named check registry. Each check is a `func(ctx) CheckResult`. Boot-time checks run immediately at startup; periodic checks run on a configurable ticker. The HTTP handler serialises all check results into the response JSON. Domain-level change: add `Ping(ctx) error` to the `Store` interface and implement it on the SQLite store.

**Tech Stack:** Go, `net/http`, `sync`, `time`

**Depends on:** Plan 002 (SQLite store) must be complete.

---

## Chunk 1: Domain — Ping port

### Task 1: Add Ping to the Store interface and SQLite store

**Files:**
- Modify: `internal/domain/ports.go`
- Modify: `internal/infra/sqlite/store.go`

- [ ] **Step 1: Add Ping to the Store interface**

In `internal/domain/ports.go`, add `Ping` to the combined `Store` interface. The method belongs on `Store` directly (not a sub-interface) because it is a connectivity check rather than a domain operation.

```go
// Store combines all storage interfaces.
type Store interface {
	TeamStore
	RoleStore
	AgentStore
	DocumentStore
	TaskStore
	PermissionStore
	// Ping verifies that the underlying storage is reachable.
	Ping(ctx context.Context) error
	io.Closer
}
```

- [ ] **Step 2: Implement Ping on the SQLite store**

In `internal/infra/sqlite/store.go`, add:

```go
// Ping verifies that the SQLite connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/ports.go internal/infra/sqlite/store.go
git commit -m "feat: add Ping to Store interface and SQLite implementation"
```

---

## Chunk 2: HealthService registry

### Task 2: Rewrite HealthService with named check registry

**Files:**
- Modify: `internal/daemon/health.go`

The existing `HealthService` uses raw `[]string` warning/error buckets. Replace this with a named check registry. The `AddWarning` / `AddError` methods used by `AuditRoleDepths` in `provisioning.go` must remain — they become convenience wrappers that register a result under a generated name. The periodic ticker is started by the daemon after all checks are registered.

- [ ] **Step 1: Rewrite internal/daemon/health.go**

Full file replacement:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the overall system health.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// CheckSeverity classifies a check result.
type CheckSeverity string

const (
	SeverityOK      CheckSeverity = "ok"
	SeverityWarning CheckSeverity = "warning"
	SeverityError   CheckSeverity = "error"
)

// CheckResult holds the outcome of a single health check.
type CheckResult struct {
	Name     string        `json:"name"`
	Severity CheckSeverity `json:"severity"`
	Message  string        `json:"message,omitempty"`
}

// HealthReport is returned by the health endpoint.
type HealthReport struct {
	Status string        `json:"status"`
	Checks []CheckResult `json:"checks"`
}

// CheckFunc is a health check function. It returns a CheckResult.
type CheckFunc func(ctx context.Context) CheckResult

// registeredCheck pairs a name and function for a single health check.
type registeredCheck struct {
	name string
	fn   CheckFunc
}

// HealthService is a registry for named health checks. Checks are either
// boot-time (run once at startup) or periodic (run on a ticker).
type HealthService struct {
	mu       sync.RWMutex
	results  map[string]CheckResult // latest result per check name
	periodic []registeredCheck
}

// NewHealthService creates a new HealthService.
func NewHealthService() *HealthService {
	return &HealthService{
		results: make(map[string]CheckResult),
	}
}

// RegisterBootCheck runs fn immediately and records the result under name.
// Call this during daemon startup before the HTTP server begins accepting
// requests.
func (h *HealthService) RegisterBootCheck(name string, fn CheckFunc) {
	result := fn(context.Background())
	result.Name = name
	h.mu.Lock()
	h.results[name] = result
	h.mu.Unlock()
}

// RegisterPeriodicCheck records a check that will be run by StartPeriodic.
// It also runs the check immediately so the result is available before the
// first tick fires.
func (h *HealthService) RegisterPeriodicCheck(name string, fn CheckFunc) {
	result := fn(context.Background())
	result.Name = name
	h.mu.Lock()
	h.results[name] = result
	h.periodic = append(h.periodic, registeredCheck{name: name, fn: fn})
	h.mu.Unlock()
}

// StartPeriodic runs all periodic checks on the given interval until ctx is
// cancelled. It is intended to be called in a goroutine after the server
// starts listening.
func (h *HealthService) StartPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.mu.RLock()
			checks := make([]registeredCheck, len(h.periodic))
			copy(checks, h.periodic)
			h.mu.RUnlock()

			for _, c := range checks {
				result := c.fn(ctx)
				result.Name = c.name
				h.mu.Lock()
				h.results[c.name] = result
				h.mu.Unlock()
			}
		}
	}
}

// AddWarning is a compatibility shim used by provisioning.AuditRoleDepths.
// It records a degraded result under an auto-generated name so existing
// callers do not need to be changed.
func (h *HealthService) AddWarning(msg string) {
	name := fmt.Sprintf("warning:%s", msg)
	result := CheckResult{Name: name, Severity: SeverityWarning, Message: msg}
	h.mu.Lock()
	h.results[name] = result
	h.mu.Unlock()
}

// AddError is a compatibility shim used by provisioning.AuditRoleDepths.
// It records an unhealthy result under an auto-generated name.
func (h *HealthService) AddError(msg string) {
	name := fmt.Sprintf("error:%s", msg)
	result := CheckResult{Name: name, Severity: SeverityError, Message: msg}
	h.mu.Lock()
	h.results[name] = result
	h.mu.Unlock()
}

// Report computes and returns the current health report.
func (h *HealthService) Report() HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()

	checks := make([]CheckResult, 0, len(h.results))
	overall := StatusHealthy

	for _, r := range h.results {
		checks = append(checks, r)
		switch r.Severity {
		case SeverityError:
			overall = StatusUnhealthy
		case SeverityWarning:
			if overall == StatusHealthy {
				overall = StatusDegraded
			}
		}
	}

	return HealthReport{
		Status: string(overall),
		Checks: checks,
	}
}

// HandleHealth returns an http.HandlerFunc for the GET /v1/health endpoint.
func HandleHealth(health *HealthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.Report()

		var statusCode int
		switch report.Status {
		case string(StatusHealthy):
			statusCode = http.StatusOK
		case string(StatusDegraded):
			statusCode = 218
		default:
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(report) //nolint:errcheck
	}
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/health.go
git commit -m "feat: replace HealthService string buckets with named check registry"
```

---

## Chunk 3: Built-in checks and daemon wiring

### Task 3: Implement built-in checks and wire periodic runner in daemon

**Files:**
- Modify: `cmd/daemon/daemon.go`

The two built-in checks are:

1. **Database connectivity** — `Ping` the store; runs as a periodic check so the health endpoint reflects live db state.
2. **Role depth audit** — already implemented in `provisioning.AuditRoleDepths`; currently called once at boot via direct call in `daemon.go`. Migrate it to `RegisterBootCheck` so it is recorded as a named check result.

- [ ] **Step 1: Update cmd/daemon/daemon.go to register checks and start periodic runner**

Replace the boot-time checks section and add the periodic start. The relevant section of `run()` changes from:

```go
// Boot-time checks
provisioning.AuditRoleDepths(context.Background())
```

to:

```go
// Boot-time check: role depth audit
health.RegisterBootCheck("role_depth_audit", func(ctx context.Context) hiveDaemon.CheckResult {
    // AuditRoleDepths reports directly to health via AddWarning/AddError.
    // Run it and return an ok result — any violations are already recorded
    // as individual warning results by AddWarning inside AuditRoleDepths.
    provisioning.AuditRoleDepths(ctx)
    return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityOK, Message: "role depth audit complete"}
})

// Periodic check: database connectivity (runs every 30 seconds)
health.RegisterPeriodicCheck("database", func(ctx context.Context) hiveDaemon.CheckResult {
    if err := store.Ping(ctx); err != nil {
        return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityError, Message: err.Error()}
    }
    return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityOK}
})
```

Then, after the server goroutine is launched, start the periodic runner:

```go
// Start periodic health checks (cancelled on shutdown)
healthCtx, healthCancel := context.WithCancel(context.Background())
defer healthCancel()
go health.StartPeriodic(healthCtx, 30*time.Second)
```

Full `run()` function after the change:

```go
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

	// Provisioning
	maxRoleDepth := viper.GetInt("max-role-depth")
	provisioning := hiveDaemon.NewProvisioningService(store, health, maxRoleDepth)

	// Boot-time check: role depth audit
	health.RegisterBootCheck("role_depth_audit", func(ctx context.Context) hiveDaemon.CheckResult {
		provisioning.AuditRoleDepths(ctx)
		return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityOK, Message: "role depth audit complete"}
	})

	// Periodic check: database connectivity
	health.RegisterPeriodicCheck("database", func(ctx context.Context) hiveDaemon.CheckResult {
		if err := store.Ping(ctx); err != nil {
			return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityError, Message: err.Error()}
		}
		return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityOK}
	})

	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:         bind,
		Port:         port,
		APIKey:       apiKey,
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

	// Start periodic health checks
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go health.StartPeriodic(healthCtx, 30*time.Second)

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

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/daemon/daemon.go
git commit -m "feat: wire built-in health checks and periodic runner in daemon"
```

---

## Chunk 4: Tests

### Task 4: Unit tests for HealthService

**Files:**
- Create: `internal/daemon/health_test.go`

- [ ] **Step 1: Create health_test.go**

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthService_InitiallyHealthy(t *testing.T) {
	h := NewHealthService()
	r := h.Report()
	if r.Status != string(StatusHealthy) {
		t.Errorf("expected healthy, got %s", r.Status)
	}
	if len(r.Checks) != 0 {
		t.Errorf("expected no checks, got %d", len(r.Checks))
	}
}

func TestHealthService_RegisterBootCheck_OK(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("db", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityOK}
	})
	r := h.Report()
	if r.Status != string(StatusHealthy) {
		t.Errorf("expected healthy, got %s", r.Status)
	}
	if len(r.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(r.Checks))
	}
	if r.Checks[0].Name != "db" {
		t.Errorf("expected check name 'db', got %s", r.Checks[0].Name)
	}
}

func TestHealthService_RegisterBootCheck_Warning(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("audit", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityWarning, Message: "chain too deep"}
	})
	r := h.Report()
	if r.Status != string(StatusDegraded) {
		t.Errorf("expected degraded, got %s", r.Status)
	}
}

func TestHealthService_RegisterBootCheck_Error(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("db", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityError, Message: "connection refused"}
	})
	r := h.Report()
	if r.Status != string(StatusUnhealthy) {
		t.Errorf("expected unhealthy, got %s", r.Status)
	}
}

func TestHealthService_ErrorDominatesWarning(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("audit", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityWarning, Message: "depth warning"}
	})
	h.RegisterBootCheck("db", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityError, Message: "db down"}
	})
	r := h.Report()
	if r.Status != string(StatusUnhealthy) {
		t.Errorf("expected unhealthy when error present, got %s", r.Status)
	}
}

func TestHealthService_AddWarningCompat(t *testing.T) {
	h := NewHealthService()
	h.AddWarning("role too deep")
	r := h.Report()
	if r.Status != string(StatusDegraded) {
		t.Errorf("expected degraded via AddWarning, got %s", r.Status)
	}
}

func TestHealthService_AddErrorCompat(t *testing.T) {
	h := NewHealthService()
	h.AddError("db failed")
	r := h.Report()
	if r.Status != string(StatusUnhealthy) {
		t.Errorf("expected unhealthy via AddError, got %s", r.Status)
	}
}

func TestHealthService_PeriodicCheckUpdatesResult(t *testing.T) {
	h := NewHealthService()
	calls := 0
	h.RegisterPeriodicCheck("ticker", func(_ context.Context) CheckResult {
		calls++
		if calls == 1 {
			return CheckResult{Severity: SeverityOK}
		}
		return CheckResult{Severity: SeverityWarning, Message: "flapped"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.StartPeriodic(ctx, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	cancel()

	r := h.Report()
	// After at least one tick the warning must have been recorded.
	if r.Status == string(StatusHealthy) && calls > 1 {
		t.Errorf("expected degraded after periodic tick updated the result")
	}
}

func TestHandleHealth_HTTP_StatusCodes(t *testing.T) {
	cases := []struct {
		name       string
		severity   CheckSeverity
		wantStatus int
	}{
		{"healthy", SeverityOK, http.StatusOK},
		{"degraded", SeverityWarning, 218},
		{"unhealthy", SeverityError, http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHealthService()
			if tc.severity != SeverityOK {
				h.RegisterBootCheck("test", func(_ context.Context) CheckResult {
					return CheckResult{Severity: tc.severity, Message: "test"}
				})
			}
			handler := HandleHealth(h)
			req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
			rr := httptest.NewRecorder()
			handler(rr, req)
			if rr.Code != tc.wantStatus {
				t.Errorf("want status %d, got %d", tc.wantStatus, rr.Code)
			}
			var report HealthReport
			if err := json.NewDecoder(rr.Body).Decode(&report); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if report.Status == "" {
				t.Error("response missing status field")
			}
		})
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/daemon/... -run TestHealth -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/health_test.go
git commit -m "test: add HealthService unit tests"
```

---

## Expected Health Endpoint Response

`GET /v1/health` returns JSON with overall status and per-check results.

Healthy example (HTTP 200):

```json
{
  "status": "healthy",
  "checks": [
    {"name": "database", "severity": "ok"},
    {"name": "role_depth_audit", "severity": "ok", "message": "role depth audit complete"}
  ]
}
```

Degraded example (HTTP 218):

```json
{
  "status": "degraded",
  "checks": [
    {"name": "database", "severity": "ok"},
    {"name": "role_depth_audit", "severity": "ok", "message": "role depth audit complete"},
    {"name": "warning:role \"deep-role\" has chain depth 11 (max 10)", "severity": "warning", "message": "role \"deep-role\" has chain depth 11 (max 10)"}
  ]
}
```

Unhealthy example (HTTP 503):

```json
{
  "status": "unhealthy",
  "checks": [
    {"name": "database", "severity": "error", "message": "sql: database is closed"},
    {"name": "role_depth_audit", "severity": "ok", "message": "role depth audit complete"}
  ]
}
```
