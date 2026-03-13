# Passport Auth Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Hive's custom API key middleware with Passport's shared auth SDK, providing JWT and API key validation on all endpoints via `github.com/Work-Fort/Passport/go/service-auth`.

**Architecture:** Passport middleware wraps the entire HTTP mux with a public-path-skip wrapper exempting `/v1/health`, `/openapi`, and `/docs`. Agent identity flows from Passport's `auth.Identity` context instead of `X-Agent-Id` headers. Agent IDs become Passport UUIDs.

**Tech Stack:** Go, `github.com/Work-Fort/Passport/go/service-auth` (JWT + API key validators), Viper, Cobra

**Spec:** `docs/2026-03-13-passport-auth-design.md`

---

## Chunk 1: Core Auth Infrastructure

**Task order is sequential.** Tasks 1→2→3 must be done in order (each depends on the prior). Tasks 2 and 3 are committed together because the ServerConfig struct change in server.go must happen in the same commit as the daemon.go flag changes that reference it. Tasks 4 and 5 can proceed independently after Task 3.

### Task 1: Add Passport SDK dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the Passport SDK module**

```bash
go get github.com/Work-Fort/Passport/go/service-auth@latest
```

This pulls in the `auth`, `jwt`, and `apikey` packages plus the transitive `github.com/lestrrat-go/jwx/v2` dependency.

- [ ] **Step 2: Verify the module resolves**

```bash
go mod tidy
```

Expected: `go.mod` and `go.sum` updated with `github.com/Work-Fort/Passport/go/service-auth` and its transitive deps.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add Passport service-auth SDK"
```

---

### Task 2: Replace config defaults and CLI flags

**Files:**
- Modify: `internal/config/config.go:83` — replace `api-key` default with `passport-url` and `passport-token`
- Modify: `cmd/daemon/daemon.go:29,44,54` — replace `apiKey` var and `--api-key` flag with `passportURL` and `--passport-url`
- Modify: `cmd/mcpbridge/mcp_bridge.go:19,31,35,40,62` — replace `agentID` var, `--agent-id` flag with `passportToken` and `--passport-token`
- Modify: `cmd/export/export.go:19,33-34,50,67` — replace `apiKey` var and `--api-key` flag with `passportToken` and `--passport-token`
- Modify: `cmd/importcmd/importcmd.go:19,35-36,52,86` — same as export

- [ ] **Step 1: Update config defaults**

In `internal/config/config.go`, replace line 83:

```go
// Before
viper.SetDefault("api-key", "")

// After
viper.SetDefault("passport-url", "http://passport.nexus:3000")
viper.SetDefault("passport-token", "")
```

- [ ] **Step 2: Update daemon command**

In `cmd/daemon/daemon.go`:

Replace the `apiKey` variable and flag (lines 29, 44-46, 54, 106):

```go
// Before (line 29)
var apiKey string

// After
var passportURL string
```

```go
// Before (lines 44-46)
if !cmd.Flags().Changed("api-key") {
    apiKey = viper.GetString("api-key")
}

// After
if !cmd.Flags().Changed("passport-url") {
    passportURL = viper.GetString("passport-url")
}
```

```go
// Before (line 54)
cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for REST authentication")

// After
cmd.Flags().StringVar(&passportURL, "passport-url", "http://passport.nexus:3000",
    "Passport auth service URL")
```

```go
// Before (line 47)
return run(bind, port, db, apiKey)

// After
return run(bind, port, db, passportURL)
```

Update the `run` function signature and ServerConfig (lines 59, 103-110):

```go
// Before (line 59)
func run(bind string, port int, db, apiKey string) error {

// After
func run(bind string, port int, db, passportURL string) error {
```

```go
// Before (line 106)
APIKey:       apiKey,

// After
PassportURL:  passportURL,
```

- [ ] **Step 3: Update mcp-bridge command**

In `cmd/mcpbridge/mcp_bridge.go`:

```go
// Before (line 19)
var agentID string

// After
var passportToken string
```

```go
// Before (line 27)
return run(agentID, host, port)

// After
return run(passportToken, host, port)
```

```go
// Before (lines 31, 35)
cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID this bridge serves (required)")
cmd.MarkFlagRequired("agent-id")

// After
cmd.Flags().StringVar(&passportToken, "passport-token", "", "Passport JWT or API key")
// No MarkFlagRequired — token can come from env (HIVE_PASSPORT_TOKEN) or config file via Viper
```

Add Viper resolution to the RunE (before `return run(...)`) :

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if !cmd.Flags().Changed("passport-token") {
        passportToken = viper.GetString("passport-token")
    }
    if passportToken == "" {
        return fmt.Errorf("passport token required: set --passport-token, HIVE_PASSPORT_TOKEN, or passport-token in config")
    }
    return run(passportToken, host, port)
},
```

Add `"fmt"` to the import block (already has `"fmt"`).

Update run signature and the header (lines 40, 62):

```go
// Before (line 40)
func run(agentID, host string, port int) error {

// After
func run(passportToken, host string, port int) error {
```

```go
// Before (line 62)
req.Header.Set("X-Agent-Id", agentID)

// After
req.Header.Set("Authorization", "Bearer "+passportToken)
```

Remove the `"github.com/spf13/cobra"` import — wait, it's already used. Add `"github.com/spf13/viper"` to imports.

- [ ] **Step 4: Update export command**

In `cmd/export/export.go`:

```go
// Before (line 19)
var apiKey string

// After
var passportToken string
```

```go
// Before (lines 33-34)
if !cmd.Flags().Changed("api-key") {
    apiKey = viper.GetString("api-key")
}

// After
if !cmd.Flags().Changed("passport-token") {
    passportToken = viper.GetString("passport-token")
}
```

```go
// Before (line 50)
ds = transfer.NewRESTDataSource(client.New(baseURL, apiKey))

// After
ds = transfer.NewRESTDataSource(client.New(baseURL, passportToken))
```

```go
// Before (line 67)
cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for REST authentication")

// After
cmd.Flags().StringVar(&passportToken, "passport-token", "", "Passport JWT or API key")
```

- [ ] **Step 5: Update import command**

In `cmd/importcmd/importcmd.go`, apply the same changes as export:

Replace `apiKey` → `passportToken`, `"api-key"` → `"passport-token"` in lines 19, 35-36, 52, 86. Same pattern as step 4.

- [ ] **Step 6: Do NOT build yet**

The build will fail because `ServerConfig.PassportURL` doesn't exist yet — Task 3 adds it. Proceed directly to Task 3. These two tasks are committed together.

---

### Task 3: Replace middleware and server wiring

**Files:**
- Modify: `internal/daemon/middleware.go` — replace `APIKeyAuth` with `publicPathSkip`
- Modify: `internal/daemon/server.go:17-24,57` — `ServerConfig.APIKey` → `PassportURL`, Passport middleware wiring
- Delete: `internal/daemon/middleware_test.go` — old tests no longer apply

- [ ] **Step 1: Replace middleware.go**

Replace the entire content of `internal/daemon/middleware.go` with:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"
	"strings"
)

// publicPathSkip routes public paths (health, OpenAPI docs) to the unprotected
// handler, and all other paths through the Passport auth middleware.
func publicPathSkip(passport http.Handler, unprotected http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/health",
			r.URL.Path == "/openapi",
			strings.HasPrefix(r.URL.Path, "/docs"):
			unprotected.ServeHTTP(w, r)
		default:
			passport.ServeHTTP(w, r)
		}
	})
}
```

- [ ] **Step 2: Update ServerConfig and NewServer**

In `internal/daemon/server.go`:

Update imports — add the Passport SDK packages:

```go
import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	auth "github.com/Work-Fort/Passport/go/service-auth"
	"github.com/Work-Fort/Passport/go/service-auth/apikey"
	"github.com/Work-Fort/Passport/go/service-auth/jwt"

	"github.com/Work-Fort/Hive/internal/domain"
)
```

Replace `ServerConfig` (lines 17-24):

```go
// Before
type ServerConfig struct {
	Bind         string
	Port         int
	APIKey       string
	Health       *HealthService
	Store        domain.Store
	Provisioning *ProvisioningService
}

// After
type ServerConfig struct {
	Bind         string
	Port         int
	PassportURL  string
	Health       *HealthService
	Store        domain.Store
	Provisioning *ProvisioningService
}
```

Replace the middleware wiring in `NewServer` (line 55-57):

```go
// Before
// Wrap with API key auth middleware.
// Middleware already skips non-/v1/ paths, so /openapi and /docs are public.
handler := APIKeyAuth(cfg.APIKey, mux)

// After
// Passport auth middleware — validates JWT and API key tokens.
var handler http.Handler
if cfg.PassportURL != "" {
    opts := auth.DefaultOptions(cfg.PassportURL)
    jwtV, err := jwt.New(context.Background(), opts.JWKSURL, opts.JWKSRefreshInterval)
    if err != nil {
        log.Warn("jwt validator init failed, falling back to API key only", "err", err)
    }

    var validators []auth.Validator
    if jwtV != nil {
        validators = append(validators, jwtV)
    }
    validators = append(validators, apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL))

    passportMW := auth.NewFromValidators(validators...)
    handler = publicPathSkip(passportMW(mux), mux)
} else {
    handler = mux
}
```

- [ ] **Step 3: Delete old middleware tests**

```bash
git rm internal/daemon/middleware_test.go
```

- [ ] **Step 4: Write new middleware test**

Create `internal/daemon/middleware_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicPathSkip_HealthSkipped(t *testing.T) {
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	unprotected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := publicPathSkip(protected, unprotected)

	for _, path := range []string{"/v1/health", "/openapi", "/docs", "/docs/index.html"} {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("path %q: got status %d, want %d (should skip auth)", path, rr.Code, http.StatusOK)
		}
	}
}

func TestPublicPathSkip_ProtectedPaths(t *testing.T) {
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	unprotected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := publicPathSkip(protected, unprotected)

	for _, path := range []string{"/v1/teams", "/v1/agents", "/mcp"} {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("path %q: got status %d, want %d (should require auth)", path, rr.Code, http.StatusUnauthorized)
		}
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/daemon/ -run TestPublicPathSkip -v
```

Expected: Both tests pass.

- [ ] **Step 6: Verify build**

```bash
go build ./...
```

Expected: Compiles cleanly.

- [ ] **Step 7: Commit (includes Task 2 files)**

```bash
git add internal/config/config.go cmd/daemon/daemon.go cmd/mcpbridge/mcp_bridge.go cmd/export/export.go cmd/importcmd/importcmd.go internal/daemon/middleware.go internal/daemon/middleware_test.go internal/daemon/server.go
git commit -m "feat: replace APIKeyAuth with Passport middleware and CLI flags"
```

---

### Task 4: Update session.go — identity from Passport context

**Files:**
- Modify: `internal/daemon/session.go:4-9,42-52,56-66` — import Passport SDK, update `httpContextFunc` and `resolveAgent`

- [ ] **Step 1: Update httpContextFunc**

In `internal/daemon/session.go`:

Add the Passport import:

```go
import (
	"context"
	"fmt"
	"net/http"

	auth "github.com/Work-Fort/Passport/go/service-auth"

	"github.com/Work-Fort/Hive/internal/domain"
)
```

Replace `httpContextFunc` (lines 42-52):

```go
// Before
func httpContextFunc() func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		agentID := r.Header.Get("X-Agent-Id")
		if agentID != "" {
			ctx = contextWithAgentID(ctx, agentID)
		}
		return ctx
	}
}

// After
func httpContextFunc() func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		id, ok := auth.IdentityFromContext(ctx)
		if ok {
			ctx = contextWithAgentID(ctx, id.ID)
		}
		return ctx
	}
}
```

The `net/http` import is still needed — `httpContextFunc` returns a function taking `*http.Request`.

- [ ] **Step 2: Update resolveAgent error message**

In `internal/daemon/session.go`, line 59:

```go
// Before
return ctx, fmt.Errorf("missing X-Agent-Id header")

// After
return ctx, fmt.Errorf("missing agent identity")
```

- [ ] **Step 3: Run unit tests**

```bash
go test ./internal/daemon/ -v -count=1
```

Expected: All existing MCP tool tests still pass — they inject agent IDs via `contextWithAgentID` directly, so the `httpContextFunc` change doesn't affect them.

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/session.go
git commit -m "feat: resolve agent identity from Passport context instead of X-Agent-Id"
```

---

### Task 5: Update client library

**Files:**
- Modify: `client/client.go:21-35,59-61` — rename `apiKey` field to `token`, update constructor

- [ ] **Step 1: Rename apiKey to token**

In `client/client.go`:

```go
// Before (lines 21-25)
type Client struct {
	http    http.Client
	baseURL string
	apiKey  string
}

// After
type Client struct {
	http    http.Client
	baseURL string
	token   string
}
```

```go
// Before (lines 27-36)
// New creates a new Client. baseURL should be the scheme+host+port of the
// Hive daemon (e.g., "http://127.0.0.1:17000"). apiKey is sent as a Bearer
// token on every authenticated request.
func New(baseURL string, apiKey string) *Client {
	return &Client{
		http:    http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

// After
// New creates a new Client. baseURL should be the scheme+host+port of the
// Hive daemon (e.g., "http://127.0.0.1:17000"). token is a Passport JWT or
// API key sent as a Bearer token on every authenticated request.
func New(baseURL string, token string) *Client {
	return &Client{
		http:    http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
	}
}
```

```go
// Before (lines 59-61)
if c.apiKey != "" {
    req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// After
if c.token != "" {
    req.Header.Set("Authorization", "Bearer "+c.token)
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: Compiles cleanly. The `client.New` signature is `(baseURL, token string)` — same arity, callers don't break.

- [ ] **Step 3: Commit**

```bash
git add client/client.go
git commit -m "feat: rename client apiKey field to token for Passport auth"
```

---

## Chunk 2: Agent Identity & REST Changes

### Task 6: Update agent creation to accept Passport UUIDs

**Files:**
- Modify: `internal/daemon/rest_types.go:189-194` — add `ID` field to `CreateAgentInput`
- Modify: `internal/daemon/rest_huma.go:438` — use `input.Body.ID` instead of `NewID("ag")`
- Modify: `client/agents.go:28-31` — update `CreateAgent` to accept ID parameter

- [ ] **Step 1: Add ID field to CreateAgentInput**

In `internal/daemon/rest_types.go`, update `CreateAgentInput` (lines 189-194):

```go
// Before
type CreateAgentInput struct {
	Body struct {
		Name   string `json:"name" doc:"Agent name" minLength:"1"`
		TeamID string `json:"team_id" doc:"Team ID" minLength:"1"`
	}
}

// After
type CreateAgentInput struct {
	Body struct {
		ID     string `json:"id" doc:"Passport UUID" minLength:"1"`
		Name   string `json:"name" doc:"Agent name" minLength:"1"`
		TeamID string `json:"team_id" doc:"Team ID" minLength:"1"`
	}
}
```

- [ ] **Step 2: Update REST handler**

In `internal/daemon/rest_huma.go`, line 438:

```go
// Before
agent := &domain.Agent{ID: NewID("ag"), Name: input.Body.Name, TeamID: input.Body.TeamID}

// After
agent := &domain.Agent{ID: input.Body.ID, Name: input.Body.Name, TeamID: input.Body.TeamID}
```

- [ ] **Step 3: Update client CreateAgent**

In `client/agents.go`, update `CreateAgent` (lines 28-31):

```go
// Before
func (c *Client) CreateAgent(ctx context.Context, name, teamID string) (*Agent, error) {
	body := map[string]string{"name": name, "team_id": teamID}
	var out Agent
	return &out, c.do(ctx, http.MethodPost, "/v1/agents", body, &out)
}

// After
func (c *Client) CreateAgent(ctx context.Context, id, name, teamID string) (*Agent, error) {
	body := map[string]string{"id": id, "name": name, "team_id": teamID}
	var out Agent
	return &out, c.do(ctx, http.MethodPost, "/v1/agents", body, &out)
}
```

- [ ] **Step 4: Fix all callers of CreateAgent**

The signature changed from `(ctx, name, teamID)` to `(ctx, id, name, teamID)`. Find and fix all callers:

In `tests/e2e/permissions_test.go:20`:
```go
// Before
agent, err := c.CreateAgent(ctx(), "perm-agent", team.ID)

// After — generate a UUID for tests
agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000001", "perm-agent", team.ID)
```

In `tests/e2e/export_import_test.go:41`:
```go
// Before
agent, err := c.CreateAgent(ctx(), "claude", team.ID)

// After
agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000002", "claude", team.ID)
```

In `tests/e2e/tasks_test.go:20`:
```go
// Before
agent, err := c.CreateAgent(ctx(), "task-agent", team.ID)

// After
agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000003", "task-agent", team.ID)
```

Same for `tests/e2e/tasks_test.go:100`:
```go
// Before
agent, err := c.CreateAgent(ctx(), "block-agent", team.ID)

// After
agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000004", "block-agent", team.ID)
```

In `tests/e2e/documents_test.go:82`:
```go
// Before
agent, err := c.CreateAgent(ctx(), "mem-agent", team.ID)

// After
agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000005", "mem-agent", team.ID)
```

In `tests/e2e/agents_test.go:26`:
```go
// Before
agent, err := c.CreateAgent(ctx(), "alice", team.ID)

// After
agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000006", "alice", team.ID)
```

In `tests/e2e/teams_test.go:92`:
```go
// Before
if _, err := c.CreateAgent(ctx(), "dep-agent", team.ID); err != nil {

// After
if _, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000007", "dep-agent", team.ID); err != nil {
```

In `internal/transfer/rest_source.go:134-136`:
```go
// Before
func (r *restDataSource) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := r.c.CreateAgent(ctx, a.Name, a.TeamID)
	return err
}

// After
func (r *restDataSource) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := r.c.CreateAgent(ctx, a.ID, a.Name, a.TeamID)
	return err
}
```

- [ ] **Step 5: Build and run unit tests**

```bash
go build ./...
go test ./internal/daemon/ -v -count=1
```

Expected: Compiles and unit tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/rest_types.go internal/daemon/rest_huma.go client/agents.go internal/transfer/rest_source.go tests/e2e/permissions_test.go tests/e2e/export_import_test.go tests/e2e/tasks_test.go tests/e2e/documents_test.go tests/e2e/agents_test.go tests/e2e/teams_test.go
git commit -m "feat: agent creation accepts Passport UUID instead of auto-generating ID"
```

---

### Task 7: Update MCP tool tests for UUID agent IDs

**Files:**
- Modify: `internal/daemon/mcp_tools_test.go:37,245-253,361-367,425-431,489-495` — use UUID strings instead of `NewID("ag")`

- [ ] **Step 1: Update setupTestEnv to use UUIDs**

In `internal/daemon/mcp_tools_test.go`:

Add `"github.com/google/uuid"` to imports (already a transitive dep from google/uuid in go.mod).

Line 37:
```go
// Before
agent := &domain.Agent{ID: NewID("ag"), Name: "test-agent", TeamID: team.ID}

// After
agent := &domain.Agent{ID: uuid.New().String(), Name: "test-agent", TeamID: team.ID}
```

Lines 245-253 (TestMemoryIsolation, agent2):
```go
// Before
agent2 := &domain.Agent{ID: NewID("ag"), Name: "agent2", TeamID: team2.ID}

// After
agent2 := &domain.Agent{ID: uuid.New().String(), Name: "agent2", TeamID: team2.ID}
```

Lines 361-367 (TestTaskTeamIsolation, agent2):
```go
// Before
agent2 := &domain.Agent{ID: NewID("ag"), Name: "agent2", TeamID: team2.ID}

// After
agent2 := &domain.Agent{ID: uuid.New().String(), Name: "agent2", TeamID: team2.ID}
```

Lines 425-431 (TestToolFilterHidesUnpermittedTools):
```go
// Before
agent := &domain.Agent{ID: NewID("ag"), Name: "limited-agent", TeamID: team.ID}

// After
agent := &domain.Agent{ID: uuid.New().String(), Name: "limited-agent", TeamID: team.ID}
```

Lines 489-495 (TestToolFilterNoPermissions):
```go
// Before
agent := &domain.Agent{ID: NewID("ag"), Name: "noperm-agent", TeamID: team.ID}

// After
agent := &domain.Agent{ID: uuid.New().String(), Name: "noperm-agent", TeamID: team.ID}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/daemon/ -v -count=1 -race
```

Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/mcp_tools_test.go
git commit -m "test: use UUID agent IDs in MCP tool tests"
```

---

## Chunk 3: E2E Tests & Deployment

### Task 8: Update E2E test harness for Passport

**Files:**
- Modify: `tests/e2e/harness_test.go:18-28,73-81,108` — replace `testAPIKey` with Passport token, `--api-key` with `--passport-url`
- Modify: `tests/e2e/permissions_test.go:86-112` — update auth error assertions
- Modify: `tests/e2e/export_import_test.go:71-76,120-125,167-172,180-184,193-199` — replace `--api-key` with `--passport-token`

- [ ] **Step 1: Update harness constants and daemon startup**

In `tests/e2e/harness_test.go`:

```go
// Before (lines 18-21)
const (
	testAPIKey = "e2e-test-key-do-not-use-in-prod"
	startupTimeout = 15 * time.Second
	pollInterval = 50 * time.Millisecond
)

// After
const (
	// testPassportURL is the Passport instance used for E2E tests.
	testPassportURL = "http://passport.nexus:3000"
	// testPassportToken is a Passport-issued API key for E2E tests.
	// Must be pre-provisioned in the Passport instance.
	testPassportToken = "PLACEHOLDER_REPLACE_WITH_REAL_TOKEN"
	startupTimeout    = 15 * time.Second
	pollInterval      = 50 * time.Millisecond
)
```

Note: The `testPassportToken` value must be replaced with a real Passport-issued API key before E2E tests can pass. This is provisioned in the Passport instance at `passport.nexus:3000`.

Update the daemon command args (lines 73-81):

```go
// Before
cmd := exec.Command(
    hiveBin,
    "daemon",
    "--bind", "127.0.0.1",
    "--port", fmt.Sprintf("%d", port),
    "--db", dbPath,
    "--api-key", testAPIKey,
    "--log-level", "disabled",
)

// After
cmd := exec.Command(
    hiveBin,
    "daemon",
    "--bind", "127.0.0.1",
    "--port", fmt.Sprintf("%d", port),
    "--db", dbPath,
    "--passport-url", testPassportURL,
    "--log-level", "disabled",
)
```

Update the client creation (line 108):

```go
// Before
Client: client.New(fmt.Sprintf("http://127.0.0.1:%d", port), testAPIKey),

// After
Client: client.New(fmt.Sprintf("http://127.0.0.1:%d", port), testPassportToken),
```

- [ ] **Step 2: Update permissions test assertions**

In `tests/e2e/permissions_test.go`, update `TestUnauthorizedRequest` (lines 86-112):

```go
// Before
// TestUnauthorizedRequest verifies that requests with a wrong API key are
// rejected with ErrForbidden (HTTP 403), and that requests with a missing
// Authorization header are rejected with ErrUnauthorized (HTTP 401).
func TestUnauthorizedRequest(t *testing.T) {
	h := newHarness(t)

	// Wrong key: server returns 403 Forbidden.
	badClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"wrong-key",
	)
	_, err := badClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrForbidden) {
		t.Errorf("wrong API key: expected ErrForbidden, got %v", err)
	}

	// No key at all: server returns 401 Unauthorized.
	noKeyClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"",
	)
	_, err = noKeyClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("no API key: expected ErrUnauthorized, got %v", err)
	}
}

// After
// TestUnauthorizedRequest verifies that requests with an invalid token are
// rejected with ErrUnauthorized (HTTP 401), and that requests with no
// Authorization header are also rejected with ErrUnauthorized.
func TestUnauthorizedRequest(t *testing.T) {
	h := newHarness(t)

	// Invalid token: Passport returns 401 Unauthorized.
	badClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"invalid-token",
	)
	_, err := badClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("invalid token: expected ErrUnauthorized, got %v", err)
	}

	// No token: Passport returns 401 Unauthorized.
	noTokenClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"",
	)
	_, err = noTokenClient.ListTeams(ctx())
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("no token: expected ErrUnauthorized, got %v", err)
	}
}
```

- [ ] **Step 3: Update export/import test CLI flags**

In `tests/e2e/export_import_test.go`, replace every `"--api-key", testAPIKey` with `"--passport-token", testPassportToken`:

Line 74: `"--api-key", testAPIKey,` → `"--passport-token", testPassportToken,`
Line 124: same
Line 172: same
Line 184: same
Line 197: same

- [ ] **Step 4: Build E2E tests to verify compilation**

```bash
cd tests/e2e && go vet ./... && cd ../..
```

Expected: Compiles and vets cleanly. Tests won't run without a Passport instance.

- [ ] **Step 5: Commit**

```bash
git add tests/e2e/harness_test.go tests/e2e/permissions_test.go tests/e2e/export_import_test.go
git commit -m "test: update E2E tests for Passport auth"
```

---

### Task 9: Flatten database migrations

**Files:**
- Modify: `internal/infra/sqlite/migrations/001_init.sql` — this is already a single migration, no flattening needed. The column types (`TEXT`) already support UUIDs. No schema change required.
- Modify: `internal/infra/postgres/migrations/001_init.sql` — same, already single migration with `TEXT` columns.

- [ ] **Step 1: Verify migrations already support UUIDs**

The `agents.id` column is `TEXT PRIMARY KEY` in both SQLite and Postgres. UUIDs are valid TEXT values. No migration change is needed — the format change from `ag_xxxx` to UUIDs is application-level only.

Read both files to confirm:

```bash
head -5 internal/infra/sqlite/migrations/001_init.sql
head -5 internal/infra/postgres/migrations/001_init.sql
```

Expected: Both show `id TEXT PRIMARY KEY` for the agents table.

- [ ] **Step 2: Add a comment to the migration noting UUID format**

In `internal/infra/sqlite/migrations/001_init.sql`, add a comment before the agents table:

```sql
-- Agent IDs are Passport UUIDs (not auto-generated prefixed IDs).
CREATE TABLE agents (
```

Same in `internal/infra/postgres/migrations/001_init.sql`.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/sqlite/migrations/001_init.sql internal/infra/postgres/migrations/001_init.sql
git commit -m "docs: note UUID format for agent IDs in migrations"
```

---

### Task 10: Update deployment files and docs

**Files:**
- Modify: `deploy/hive.service:14-16,19` — replace API key references with Passport URL
- Modify: `.mise/tasks/install/local:12` — update echo message
- Modify: `.mcp.json` — replace `--agent-id` with `--passport-token`
- Modify: `docs/hive-design.md:17,29,219-221,297-298,328-331` — update auth references

- [ ] **Step 1: Update systemd service**

Replace `deploy/hive.service` lines 14-16:

```ini
# Before
# Load the API key from a file so it is not exposed in `systemctl status`.
# Create the file with:  install -m 600 /dev/null ~/.config/hive/api-key
# Then write your key:   echo -n 'your-key-here' > ~/.config/hive/api-key

# After
# Load environment overrides (e.g. HIVE_PASSPORT_URL).
# Create the file with:  install -m 600 /dev/null ~/.config/hive/env
# Example:  echo 'HIVE_PASSPORT_URL=http://passport.nexus:3000' > ~/.config/hive/env
```

- [ ] **Step 2: Update install task**

In `.mise/tasks/install/local`, line 12:

```bash
# Before
echo "Set API key: echo 'HIVE_API_KEY=<key>' > ~/.config/hive/env && chmod 600 ~/.config/hive/env"

# After
echo "Configure: echo 'HIVE_PASSPORT_URL=http://passport.nexus:3000' > ~/.config/hive/env && chmod 600 ~/.config/hive/env"
```

- [ ] **Step 3: Update .mcp.json**

Replace `.mcp.json`:

```json
{
  "mcpServers": {
    "hive": {
      "command": "build/hive",
      "args": [
        "mcp-bridge",
        "--log-level",
        "disabled"
      ],
      "env": {
        "HIVE_PASSPORT_TOKEN": "${HIVE_PASSPORT_TOKEN}"
      }
    }
  }
}
```

The token comes from the environment — no hardcoded values.

- [ ] **Step 4: Update design doc**

In `docs/hive-design.md`:

Line 17 — update mcp-bridge usage:
```
hive mcp-bridge [--passport-token TOKEN] [--host 127.0.0.1] [--port 17000]
```

Lines 29 — update route layout auth note:
```
/v1/*    REST API (JSON, Passport Bearer auth)
/mcp     MCP server (JSON-RPC 2.0, Passport Bearer auth)
```

Lines 219-221 — update REST auth description:
```
All endpoints under `/v1`. JSON request/response bodies. Authenticated via
Passport-issued JWT or API key (`Authorization: Bearer <token>`), validated
against the Passport service at the configured `HIVE_PASSPORT_URL`.
```

Lines 297-298 — update MCP identity source:
```
Agent identity comes from the Passport Bearer token on the mcp-bridge's HTTP
requests, resolved to an agent record by Passport UUID.
```

Lines 318-331 — update Authentication section:
```
## Authentication

### REST API

Authenticated via Passport-issued JWT or API key. The daemon validates tokens
against the Passport service (JWKS for JWTs, verification endpoint for API
keys). Passport is the source of truth for identity.

### MCP

The mcp-bridge speaks stdio (JSON-RPC) to Claude Code and forwards requests to
the daemon over HTTP (`POST /mcp`) with `Authorization: Bearer <token>`.
The daemon validates the token via Passport, resolves the caller's identity,
and looks up the agent record by Passport UUID. No `X-Agent-Id` header.
```

- [ ] **Step 5: Commit**

```bash
git add deploy/hive.service .mise/tasks/install/local .mcp.json docs/hive-design.md
git commit -m "docs: update deployment files and design doc for Passport auth"
```

---

### Task 11: Update validate entities for UUID agent IDs

**Files:**
- Modify: `internal/validate/entities.go:34-42` — no structural change needed

- [ ] **Step 1: Verify validation still works**

The `AgentSchema` has a `name` (required string) and `team` (required string) field. The ID is not validated by the entity schema — it's a primary key set by the caller. UUID format doesn't affect the validation schema.

No changes needed. Skip this task.

---

### Task 12: Final verification

- [ ] **Step 1: Run all unit tests**

```bash
go test ./... -race -count=1
```

Expected: All unit tests pass.

- [ ] **Step 2: Run go vet and build**

```bash
go vet ./...
go build ./...
```

Expected: No warnings, clean build.

- [ ] **Step 3: Verify E2E test compilation**

```bash
cd tests/e2e && go vet ./... && cd ../..
```

Expected: Compiles cleanly. E2E tests require a running Passport instance to actually execute.

- [ ] **Step 4: Final commit if any loose changes**

```bash
git status
```

If clean, done. If there are uncommitted changes, stage and commit them.
