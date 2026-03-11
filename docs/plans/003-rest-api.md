# REST API Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete REST API layer over the domain store, exposing all CRUD endpoints for teams, roles, agents, documents, tasks, and permissions with API key authentication.

**Architecture:** A single `REST` struct in `internal/daemon/` holds a `domain.Store` reference and exposes handler methods registered on the stdlib `http.ServeMux`. Handlers are split into focused files (one per entity) mirroring the SQLite store layout. API key middleware protects all `/v1/*` routes except health.

**Tech Stack:** Go stdlib `net/http`, `encoding/json`, `crypto/rand`, `net/http/httptest`

**Depends on:** Plan 002 (domain model + SQLite store) must be complete.

---

## Chunk 1: Foundation (ID gen, JSON helpers, middleware, server wiring)

### Task 1: ID generation utility

**Files:**
- Create: `internal/daemon/id.go`
- Create: `internal/daemon/id_test.go`

- [ ] **Step 1: Create ID generator**

Create `internal/daemon/id.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"crypto/rand"
	"fmt"
)

// NewID generates a prefixed random ID: "<prefix>_<16 hex chars>".
// Prefixes: tm (team), rl (role), ag (agent), doc (document), tk (task).
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("%s_%x", prefix, b)
}
```

- [ ] **Step 2: Create ID tests**

Create `internal/daemon/id_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"strings"
	"testing"
)

func TestNewID_Format(t *testing.T) {
	tests := []struct {
		prefix string
	}{
		{"tm"},
		{"rl"},
		{"ag"},
		{"doc"},
		{"tk"},
	}
	for _, tt := range tests {
		id := NewID(tt.prefix)
		if !strings.HasPrefix(id, tt.prefix+"_") {
			t.Errorf("NewID(%q) = %q, missing prefix", tt.prefix, id)
		}
		// prefix + "_" + 16 hex chars
		wantLen := len(tt.prefix) + 1 + 16
		if len(id) != wantLen {
			t.Errorf("NewID(%q) = %q, len %d, want %d", tt.prefix, id, len(id), wantLen)
		}
	}
}

func TestNewID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewID("tm")
		if seen[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go build ./internal/daemon/...
go test ./internal/daemon/ -run TestNewID -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add prefixed ID generation utility

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 2: JSON helpers and REST struct

**Files:**
- Create: `internal/daemon/rest.go`

- [ ] **Step 1: Create REST struct with JSON helpers**

Create `internal/daemon/rest.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// REST holds shared dependencies for all REST handlers.
type REST struct {
	store domain.Store
}

// NewREST creates a new REST handler group.
func NewREST(store domain.Store) *REST {
	return &REST{store: store}
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// readJSON decodes the request body into v.
func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// mapDomainError maps a domain error to an HTTP status code and writes the response.
// Returns true if an error was handled (caller should return).
func mapDomainError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrHasDependencies):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrDepthExceeded):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrCycleDetected):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
	return true
}
```

- [ ] **Step 2: Build**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go build ./internal/daemon/...
```

- [ ] **Step 3: Commit**

```
feat(daemon): add REST struct and JSON helper functions

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 3: API key auth middleware

**Files:**
- Create: `internal/daemon/middleware.go`
- Create: `internal/daemon/middleware_test.go`

- [ ] **Step 1: Create middleware**

Create `internal/daemon/middleware.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"
	"strings"
)

// APIKeyAuth returns middleware that checks the Authorization header for a
// Bearer token matching apiKey. Skips authentication if apiKey is empty.
func APIKeyAuth(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if no key configured
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth for health endpoint
		if r.URL.Path == "/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Only protect /v1/* routes
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		token, found := strings.CutPrefix(auth, "Bearer ")
		if !found || token != apiKey {
			writeError(w, http.StatusForbidden, "invalid api key")
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 2: Create middleware tests**

Create `internal/daemon/middleware_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyAuth_NoKeyConfigured(t *testing.T) {
	handler := APIKeyAuth("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/teams", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAPIKeyAuth_HealthSkipped(t *testing.T) {
	handler := APIKeyAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	handler := APIKeyAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/v1/teams", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAPIKeyAuth_WrongKey(t *testing.T) {
	handler := APIKeyAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/v1/teams", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	handler := APIKeyAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/teams", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAPIKeyAuth_NonV1Skipped(t *testing.T) {
	handler := APIKeyAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestAPIKeyAuth -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add API key auth middleware

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 4: Wire routes into server.go and update daemon.go

**Files:**
- Modify: `internal/daemon/server.go`
- Modify: `cmd/daemon/daemon.go`

- [ ] **Step 1: Update ServerConfig and NewServer**

Replace the contents of `internal/daemon/server.go` with:

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
	Bind   string
	Port   int
	APIKey string
	Health *HealthService
	Store  domain.Store
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// REST API routes
	rest := NewREST(cfg.Store)

	// Teams
	mux.HandleFunc("GET /v1/teams", rest.ListTeams)
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)
	mux.HandleFunc("GET /v1/teams/{id}", rest.GetTeam)
	mux.HandleFunc("PUT /v1/teams/{id}", rest.UpdateTeam)
	mux.HandleFunc("DELETE /v1/teams/{id}", rest.DeleteTeam)

	// Roles
	mux.HandleFunc("GET /v1/roles", rest.ListRoles)
	mux.HandleFunc("POST /v1/roles", rest.CreateRole)
	mux.HandleFunc("GET /v1/roles/{id}", rest.GetRole)
	mux.HandleFunc("PUT /v1/roles/{id}", rest.UpdateRole)
	mux.HandleFunc("DELETE /v1/roles/{id}", rest.DeleteRole)

	// Role documents
	mux.HandleFunc("GET /v1/roles/{id}/documents", rest.ListRoleDocuments)
	mux.HandleFunc("POST /v1/roles/{id}/documents", rest.CreateRoleDocument)

	// Standalone documents
	mux.HandleFunc("GET /v1/documents/{id}", rest.GetDocument)
	mux.HandleFunc("PUT /v1/documents/{id}", rest.UpdateDocument)
	mux.HandleFunc("DELETE /v1/documents/{id}", rest.DeleteDocument)

	// Agents
	mux.HandleFunc("GET /v1/agents", rest.ListAgents)
	mux.HandleFunc("POST /v1/agents", rest.CreateAgent)
	mux.HandleFunc("GET /v1/agents/{id}", rest.GetAgent)
	mux.HandleFunc("PUT /v1/agents/{id}", rest.UpdateAgent)
	mux.HandleFunc("DELETE /v1/agents/{id}", rest.DeleteAgent)

	// Agent roles
	mux.HandleFunc("PUT /v1/agents/{id}/roles", rest.SetAgentRoles)

	// Agent memory
	mux.HandleFunc("GET /v1/agents/{id}/memory", rest.ListAgentMemory)
	mux.HandleFunc("POST /v1/agents/{id}/memory", rest.CreateAgentMemory)

	// Tasks
	mux.HandleFunc("GET /v1/teams/{id}/tasks", rest.ListTeamTasks)
	mux.HandleFunc("POST /v1/tasks", rest.CreateTask)
	mux.HandleFunc("GET /v1/tasks/{id}", rest.GetTask)
	mux.HandleFunc("PUT /v1/tasks/{id}", rest.UpdateTask)
	mux.HandleFunc("DELETE /v1/tasks/{id}", rest.DeleteTask)

	// Permissions
	mux.HandleFunc("GET /v1/agents/{id}/permissions", rest.GetAgentPermissions)
	mux.HandleFunc("PUT /v1/agents/{id}/permissions", rest.SetAgentPermissions)

	// MCP placeholder
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "MCP server not yet implemented", http.StatusNotImplemented)
	})

	// Wrap with API key auth middleware
	handler := APIKeyAuth(cfg.APIKey, mux)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      handler,
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

- [ ] **Step 2: Update daemon.go to pass APIKey**

In `cmd/daemon/daemon.go`, update the `NewServer` call to include the `APIKey` field:

```go
	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:   bind,
		Port:   port,
		APIKey: apiKey,
		Health: health,
		Store:  store,
	})
```

- [ ] **Step 3: Build (will fail until handler files exist — that's expected)**

This step is deferred until all handler files are created. The server wiring is complete but needs the handler methods to compile.

- [ ] **Step 4: Commit** (deferred to end of Chunk 2 when all handlers compile)

---

## Chunk 2: Entity Handlers (teams, roles, documents)

### Task 5: Team handlers

**Files:**
- Create: `internal/daemon/rest_teams.go`
- Create: `internal/daemon/rest_teams_test.go`

- [ ] **Step 1: Create team handlers**

Create `internal/daemon/rest_teams.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ListTeams handles GET /v1/teams.
func (h *REST) ListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.store.ListTeams(r.Context())
	if mapDomainError(w, err) {
		return
	}
	if teams == nil {
		teams = []*domain.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

// CreateTeam handles POST /v1/teams.
func (h *REST) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	team := &domain.Team{
		ID:   NewID("tm"),
		Name: req.Name,
	}
	if mapDomainError(w, h.store.CreateTeam(r.Context(), team)) {
		return
	}

	// Re-read to get timestamps
	created, err := h.store.GetTeam(r.Context(), team.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetTeam handles GET /v1/teams/{id}.
func (h *REST) GetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	team, err := h.store.GetTeam(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, team)
}

// UpdateTeam handles PUT /v1/teams/{id}.
func (h *REST) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if mapDomainError(w, h.store.UpdateTeam(r.Context(), id, req.Name)) {
		return
	}

	updated, err := h.store.GetTeam(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteTeam handles DELETE /v1/teams/{id}.
func (h *REST) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteTeam(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Create team handler tests**

Create `internal/daemon/rest_teams_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

func newTestREST(t *testing.T) *daemon.REST {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return daemon.NewREST(store)
}

func TestTeams_CreateAndGet(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)
	mux.HandleFunc("GET /v1/teams/{id}", rest.GetTeam)

	// Create
	body := `{"name":"alpha"}`
	req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: got status %d, want %d, body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)
	if id == "" {
		t.Fatal("created team has no ID")
	}

	// Get
	req = httptest.NewRequest("GET", "/v1/teams/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: got status %d, want %d", rr.Code, http.StatusOK)
	}

	var got map[string]any
	json.NewDecoder(rr.Body).Decode(&got)
	if got["Name"] != "alpha" {
		t.Errorf("got name %v, want alpha", got["Name"])
	}
}

func TestTeams_List(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)
	mux.HandleFunc("GET /v1/teams", rest.ListTeams)

	// Create two teams
	for _, name := range []string{"alpha", "beta"} {
		body := `{"name":"` + name + `"}`
		req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: status %d", name, rr.Code)
		}
	}

	// List
	req := httptest.NewRequest("GET", "/v1/teams", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: got status %d", rr.Code)
	}

	var teams []map[string]any
	json.NewDecoder(rr.Body).Decode(&teams)
	if len(teams) != 2 {
		t.Errorf("got %d teams, want 2", len(teams))
	}
}

func TestTeams_Update(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)
	mux.HandleFunc("PUT /v1/teams/{id}", rest.UpdateTeam)

	// Create
	req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(`{"name":"alpha"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)

	// Update
	req = httptest.NewRequest("PUT", "/v1/teams/"+id, bytes.NewBufferString(`{"name":"renamed"}`))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: got status %d, body: %s", rr.Code, rr.Body.String())
	}

	var updated map[string]any
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated["Name"] != "renamed" {
		t.Errorf("got name %v, want renamed", updated["Name"])
	}
}

func TestTeams_Delete(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)
	mux.HandleFunc("DELETE /v1/teams/{id}", rest.DeleteTeam)
	mux.HandleFunc("GET /v1/teams/{id}", rest.GetTeam)

	// Create
	req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(`{"name":"alpha"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)

	// Delete
	req = httptest.NewRequest("DELETE", "/v1/teams/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: got status %d", rr.Code)
	}

	// Verify gone
	req = httptest.NewRequest("GET", "/v1/teams/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: got status %d, want 404", rr.Code)
	}
}

func TestTeams_CreateMissingName(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)

	req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestTeams_GetNotFound(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/teams/{id}", rest.GetTeam)

	req := httptest.NewRequest("GET", "/v1/teams/tm_nonexistent", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestTeams_ListEmpty(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/teams", rest.ListTeams)

	req := httptest.NewRequest("GET", "/v1/teams", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got status %d", rr.Code)
	}

	var teams []map[string]any
	json.NewDecoder(rr.Body).Decode(&teams)
	if len(teams) != 0 {
		t.Errorf("got %d teams, want 0", len(teams))
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestTeams -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add team REST handlers with tests

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 6: Role handlers

**Files:**
- Create: `internal/daemon/rest_roles.go`
- Create: `internal/daemon/rest_roles_test.go`

- [ ] **Step 1: Create role handlers**

Create `internal/daemon/rest_roles.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ListRoles handles GET /v1/roles.
func (h *REST) ListRoles(w http.ResponseWriter, r *http.Request) {
	parentID := r.URL.Query().Get("parent_id")
	roles, err := h.store.ListRoles(r.Context(), parentID)
	if mapDomainError(w, err) {
		return
	}
	if roles == nil {
		roles = []*domain.Role{}
	}
	writeJSON(w, http.StatusOK, roles)
}

// CreateRole handles POST /v1/roles.
func (h *REST) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	role := &domain.Role{
		ID:       NewID("rl"),
		Name:     req.Name,
		ParentID: req.ParentID,
	}
	if mapDomainError(w, h.store.CreateRole(r.Context(), role)) {
		return
	}

	created, err := h.store.GetRole(r.Context(), role.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetRole handles GET /v1/roles/{id}.
func (h *REST) GetRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	role, err := h.store.GetRole(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// UpdateRole handles PUT /v1/roles/{id}.
func (h *REST) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if mapDomainError(w, h.store.UpdateRole(r.Context(), id, req.Name, req.ParentID)) {
		return
	}

	updated, err := h.store.GetRole(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteRole handles DELETE /v1/roles/{id}.
func (h *REST) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteRole(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Create role handler tests**

Create `internal/daemon/rest_roles_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoles_CRUD(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/roles", rest.CreateRole)
	mux.HandleFunc("GET /v1/roles/{id}", rest.GetRole)
	mux.HandleFunc("GET /v1/roles", rest.ListRoles)
	mux.HandleFunc("PUT /v1/roles/{id}", rest.UpdateRole)
	mux.HandleFunc("DELETE /v1/roles/{id}", rest.DeleteRole)

	// Create
	req := httptest.NewRequest("POST", "/v1/roles", bytes.NewBufferString(`{"name":"engineer"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)

	// Get
	req = httptest.NewRequest("GET", "/v1/roles/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: status %d", rr.Code)
	}

	// Update
	req = httptest.NewRequest("PUT", "/v1/roles/"+id, bytes.NewBufferString(`{"name":"senior-engineer"}`))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var updated map[string]any
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated["Name"] != "senior-engineer" {
		t.Errorf("got name %v, want senior-engineer", updated["Name"])
	}

	// List
	req = httptest.NewRequest("GET", "/v1/roles", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: status %d", rr.Code)
	}

	var roles []map[string]any
	json.NewDecoder(rr.Body).Decode(&roles)
	if len(roles) != 1 {
		t.Errorf("got %d roles, want 1", len(roles))
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/v1/roles/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", rr.Code)
	}
}

func TestRoles_WithParent(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/roles", rest.CreateRole)
	mux.HandleFunc("GET /v1/roles", rest.ListRoles)

	// Create parent
	req := httptest.NewRequest("POST", "/v1/roles", bytes.NewBufferString(`{"name":"base"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var parent map[string]any
	json.NewDecoder(rr.Body).Decode(&parent)
	parentID := parent["ID"].(string)

	// Create child
	body := `{"name":"child","parent_id":"` + parentID + `"}`
	req = httptest.NewRequest("POST", "/v1/roles", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create child: status %d, body: %s", rr.Code, rr.Body.String())
	}

	// List with parent_id filter
	req = httptest.NewRequest("GET", "/v1/roles?parent_id="+parentID, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var roles []map[string]any
	json.NewDecoder(rr.Body).Decode(&roles)
	if len(roles) != 1 {
		t.Errorf("got %d roles with parent filter, want 1", len(roles))
	}
}

func TestRoles_CreateMissingName(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/roles", rest.CreateRole)

	req := httptest.NewRequest("POST", "/v1/roles", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestRoles -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add role REST handlers with tests

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 7: Document handlers (role docs, agent memory, standalone)

**Files:**
- Create: `internal/daemon/rest_documents.go`
- Create: `internal/daemon/rest_documents_test.go`

- [ ] **Step 1: Create document handlers**

Create `internal/daemon/rest_documents.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ListRoleDocuments handles GET /v1/roles/{id}/documents.
func (h *REST) ListRoleDocuments(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")

	// Verify role exists
	if _, err := h.store.GetRole(r.Context(), roleID); mapDomainError(w, err) {
		return
	}

	docs, err := h.store.ListRoleDocuments(r.Context(), roleID)
	if mapDomainError(w, err) {
		return
	}
	if docs == nil {
		docs = []*domain.Document{}
	}
	writeJSON(w, http.StatusOK, docs)
}

// CreateRoleDocument handles POST /v1/roles/{id}/documents.
func (h *REST) CreateRoleDocument(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")

	// Verify role exists
	if _, err := h.store.GetRole(r.Context(), roleID); mapDomainError(w, err) {
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	doc := &domain.Document{
		ID:      NewID("doc"),
		Kind:    domain.DocumentKindRole,
		Title:   req.Title,
		Content: req.Content,
		RoleID:  roleID,
	}
	if mapDomainError(w, h.store.CreateDocument(r.Context(), doc)) {
		return
	}

	created, err := h.store.GetDocument(r.Context(), doc.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetDocument handles GET /v1/documents/{id}.
func (h *REST) GetDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := h.store.GetDocument(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// UpdateDocument handles PUT /v1/documents/{id}.
func (h *REST) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	if mapDomainError(w, h.store.UpdateDocument(r.Context(), id, req.Title, req.Content)) {
		return
	}

	updated, err := h.store.GetDocument(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteDocument handles DELETE /v1/documents/{id}.
func (h *REST) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteDocument(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListAgentMemory handles GET /v1/agents/{id}/memory.
func (h *REST) ListAgentMemory(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	// Verify agent exists
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}

	docs, err := h.store.ListAgentMemory(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if docs == nil {
		docs = []*domain.Document{}
	}
	writeJSON(w, http.StatusOK, docs)
}

// CreateAgentMemory handles POST /v1/agents/{id}/memory.
func (h *REST) CreateAgentMemory(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	// Verify agent exists
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	doc := &domain.Document{
		ID:      NewID("doc"),
		Kind:    domain.DocumentKindMemory,
		Title:   req.Title,
		Content: req.Content,
		AgentID: agentID,
	}
	if mapDomainError(w, h.store.CreateDocument(r.Context(), doc)) {
		return
	}

	created, err := h.store.GetDocument(r.Context(), doc.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
```

- [ ] **Step 2: Create document handler tests**

Create `internal/daemon/rest_documents_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// newTestRESTWithData creates a REST handler with a team, agent, and role pre-seeded.
// Returns (rest, teamID, agentID, roleID).
func newTestRESTWithData(t *testing.T) (*daemon.REST, string, string, string) {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	team := &domain.Team{ID: "tm_test01", Name: "testteam"}
	store.CreateTeam(ctx, team)

	role := &domain.Role{ID: "rl_test01", Name: "testrole"}
	store.CreateRole(ctx, role)

	agent := &domain.Agent{ID: "ag_test01", Name: "testagent", TeamID: "tm_test01"}
	store.CreateAgent(ctx, agent)

	return daemon.NewREST(store), team.ID, agent.ID, role.ID
}

func TestDocuments_RoleCRUD(t *testing.T) {
	rest, _, _, roleID := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/roles/{id}/documents", rest.ListRoleDocuments)
	mux.HandleFunc("POST /v1/roles/{id}/documents", rest.CreateRoleDocument)
	mux.HandleFunc("GET /v1/documents/{id}", rest.GetDocument)
	mux.HandleFunc("PUT /v1/documents/{id}", rest.UpdateDocument)
	mux.HandleFunc("DELETE /v1/documents/{id}", rest.DeleteDocument)

	// List empty
	req := httptest.NewRequest("GET", "/v1/roles/"+roleID+"/documents", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list empty: status %d", rr.Code)
	}

	// Create
	body := `{"title":"setup","content":"# Setup\nDo this."}`
	req = httptest.NewRequest("POST", "/v1/roles/"+roleID+"/documents", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	docID := created["ID"].(string)

	// Get
	req = httptest.NewRequest("GET", "/v1/documents/"+docID, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get: status %d", rr.Code)
	}

	// Update
	req = httptest.NewRequest("PUT", "/v1/documents/"+docID, bytes.NewBufferString(`{"title":"updated","content":"new"}`))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var updated map[string]any
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated["Title"] != "updated" {
		t.Errorf("got title %v, want updated", updated["Title"])
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/v1/documents/"+docID, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", rr.Code)
	}

	// Verify gone
	req = httptest.NewRequest("GET", "/v1/documents/"+docID, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: status %d, want 404", rr.Code)
	}
}

func TestDocuments_AgentMemory(t *testing.T) {
	rest, _, agentID, _ := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agents/{id}/memory", rest.ListAgentMemory)
	mux.HandleFunc("POST /v1/agents/{id}/memory", rest.CreateAgentMemory)
	mux.HandleFunc("GET /v1/documents/{id}", rest.GetDocument)

	// Create memory doc
	body := `{"title":"learned","content":"I learned something."}`
	req := httptest.NewRequest("POST", "/v1/agents/"+agentID+"/memory", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create memory: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	if created["Kind"] != "memory" {
		t.Errorf("got kind %v, want memory", created["Kind"])
	}

	// List memory
	req = httptest.NewRequest("GET", "/v1/agents/"+agentID+"/memory", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list memory: status %d", rr.Code)
	}

	var docs []map[string]any
	json.NewDecoder(rr.Body).Decode(&docs)
	if len(docs) != 1 {
		t.Errorf("got %d memory docs, want 1", len(docs))
	}
}

func TestDocuments_RoleNotFound(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/roles/{id}/documents", rest.CreateRoleDocument)

	req := httptest.NewRequest("POST", "/v1/roles/rl_nonexistent/documents", bytes.NewBufferString(`{"title":"x","content":"y"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rr.Code)
	}
}

func TestDocuments_CreateMissingTitle(t *testing.T) {
	rest, _, _, roleID := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/roles/{id}/documents", rest.CreateRoleDocument)

	req := httptest.NewRequest("POST", "/v1/roles/"+roleID+"/documents", bytes.NewBufferString(`{"content":"no title"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
```

Note: The `newTestRESTWithData` helper uses a package import. The test file imports need adjusting since it's in `daemon_test` package. Fix by adding the import:

```go
import (
	"github.com/Work-Fort/Hive/internal/daemon"
)
```

The `newTestRESTWithData` function returns `*daemon.REST` so the full import block for `rest_documents_test.go` is:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestDocuments -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add document REST handlers with tests

Covers role documents, agent memory, and standalone document CRUD.

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

## Chunk 3: Entity Handlers (agents, tasks, permissions)

### Task 8: Agent handlers

**Files:**
- Create: `internal/daemon/rest_agents.go`
- Create: `internal/daemon/rest_agents_test.go`

- [ ] **Step 1: Create agent handlers**

Create `internal/daemon/rest_agents.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ListAgents handles GET /v1/agents.
func (h *REST) ListAgents(w http.ResponseWriter, r *http.Request) {
	teamID := r.URL.Query().Get("team_id")
	agents, err := h.store.ListAgents(r.Context(), teamID)
	if mapDomainError(w, err) {
		return
	}
	if agents == nil {
		agents = []*domain.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

// CreateAgent handles POST /v1/agents.
func (h *REST) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		TeamID string `json:"team_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}

	// Verify team exists
	if _, err := h.store.GetTeam(r.Context(), req.TeamID); mapDomainError(w, err) {
		return
	}

	agent := &domain.Agent{
		ID:     NewID("ag"),
		Name:   req.Name,
		TeamID: req.TeamID,
	}
	if mapDomainError(w, h.store.CreateAgent(r.Context(), agent)) {
		return
	}

	created, err := h.store.GetAgent(r.Context(), agent.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetAgent handles GET /v1/agents/{id}.
func (h *REST) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := h.store.GetAgent(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}

	// Include role assignments
	roles, err := h.store.GetAgentRoles(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	if roles == nil {
		roles = []domain.AgentRole{}
	}

	resp := struct {
		*domain.Agent
		Roles []domain.AgentRole `json:"roles"`
	}{
		Agent: agent,
		Roles: roles,
	}
	writeJSON(w, http.StatusOK, resp)
}

// UpdateAgent handles PUT /v1/agents/{id}.
func (h *REST) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name   string `json:"name"`
		TeamID string `json:"team_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}

	if mapDomainError(w, h.store.UpdateAgent(r.Context(), id, req.Name, req.TeamID)) {
		return
	}

	updated, err := h.store.GetAgent(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteAgent handles DELETE /v1/agents/{id}.
func (h *REST) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteAgent(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetAgentRoles handles PUT /v1/agents/{id}/roles.
func (h *REST) SetAgentRoles(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	// Verify agent exists
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}

	var req struct {
		Roles []struct {
			RoleID   string `json:"role_id"`
			Priority int    `json:"priority"`
		} `json:"roles"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	roles := make([]domain.AgentRole, len(req.Roles))
	for i, r := range req.Roles {
		if r.RoleID == "" {
			writeError(w, http.StatusBadRequest, "role_id is required for each role")
			return
		}
		roles[i] = domain.AgentRole{
			AgentID:  agentID,
			RoleID:   r.RoleID,
			Priority: r.Priority,
		}
	}

	if mapDomainError(w, h.store.SetAgentRoles(r.Context(), agentID, roles)) {
		return
	}

	// Return updated assignments
	updated, err := h.store.GetAgentRoles(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if updated == nil {
		updated = []domain.AgentRole{}
	}
	writeJSON(w, http.StatusOK, updated)
}
```

- [ ] **Step 2: Create agent handler tests**

Create `internal/daemon/rest_agents_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgents_CRUD(t *testing.T) {
	rest, teamID, _, _ := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agents", rest.CreateAgent)
	mux.HandleFunc("GET /v1/agents/{id}", rest.GetAgent)
	mux.HandleFunc("GET /v1/agents", rest.ListAgents)
	mux.HandleFunc("PUT /v1/agents/{id}", rest.UpdateAgent)
	mux.HandleFunc("DELETE /v1/agents/{id}", rest.DeleteAgent)

	// Create
	body := `{"name":"bob","team_id":"` + teamID + `"}`
	req := httptest.NewRequest("POST", "/v1/agents", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)

	// Get (should include roles array)
	req = httptest.NewRequest("GET", "/v1/agents/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: status %d", rr.Code)
	}

	var got map[string]any
	json.NewDecoder(rr.Body).Decode(&got)
	if got["Name"] != "bob" {
		t.Errorf("got name %v, want bob", got["Name"])
	}
	if _, ok := got["roles"]; !ok {
		t.Error("get agent response missing roles field")
	}

	// List
	req = httptest.NewRequest("GET", "/v1/agents", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var agents []map[string]any
	json.NewDecoder(rr.Body).Decode(&agents)
	// Should include the pre-seeded agent + new one
	if len(agents) < 2 {
		t.Errorf("got %d agents, want at least 2", len(agents))
	}

	// List with team_id filter
	req = httptest.NewRequest("GET", "/v1/agents?team_id="+teamID, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	json.NewDecoder(rr.Body).Decode(&agents)
	if len(agents) < 2 {
		t.Errorf("got %d agents for team, want at least 2", len(agents))
	}

	// Update
	body = `{"name":"bob-updated","team_id":"` + teamID + `"}`
	req = httptest.NewRequest("PUT", "/v1/agents/"+id, bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: status %d, body: %s", rr.Code, rr.Body.String())
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/v1/agents/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", rr.Code)
	}
}

func TestAgents_SetRoles(t *testing.T) {
	rest, _, agentID, roleID := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/agents/{id}/roles", rest.SetAgentRoles)

	body := `{"roles":[{"role_id":"` + roleID + `","priority":1}]}`
	req := httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/roles", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("set roles: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var roles []map[string]any
	json.NewDecoder(rr.Body).Decode(&roles)
	if len(roles) != 1 {
		t.Errorf("got %d roles, want 1", len(roles))
	}
}

func TestAgents_SetRolesEmpty(t *testing.T) {
	rest, _, agentID, roleID := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/agents/{id}/roles", rest.SetAgentRoles)

	// Set a role first
	body := `{"roles":[{"role_id":"` + roleID + `","priority":1}]}`
	req := httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/roles", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Clear all roles
	body = `{"roles":[]}`
	req = httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/roles", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("clear roles: status %d", rr.Code)
	}

	var roles []map[string]any
	json.NewDecoder(rr.Body).Decode(&roles)
	if len(roles) != 0 {
		t.Errorf("got %d roles after clear, want 0", len(roles))
	}
}

func TestAgents_CreateMissingTeam(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agents", rest.CreateAgent)

	req := httptest.NewRequest("POST", "/v1/agents", bytes.NewBufferString(`{"name":"x","team_id":"tm_nonexistent"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rr.Code)
	}
}

func TestAgents_CreateMissingFields(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agents", rest.CreateAgent)

	// Missing team_id
	req := httptest.NewRequest("POST", "/v1/agents", bytes.NewBufferString(`{"name":"x"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing team_id: got status %d, want 400", rr.Code)
	}

	// Missing name
	req = httptest.NewRequest("POST", "/v1/agents", bytes.NewBufferString(`{"team_id":"tm_1"}`))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing name: got status %d, want 400", rr.Code)
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestAgents -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add agent REST handlers with tests

Includes CRUD, team_id filter, and role assignment endpoints.

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 9: Task handlers

**Files:**
- Create: `internal/daemon/rest_tasks.go`
- Create: `internal/daemon/rest_tasks_test.go`

- [ ] **Step 1: Create task handlers**

Create `internal/daemon/rest_tasks.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ListTeamTasks handles GET /v1/teams/{id}/tasks.
func (h *REST) ListTeamTasks(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")

	// Verify team exists
	if _, err := h.store.GetTeam(r.Context(), teamID); mapDomainError(w, err) {
		return
	}

	tasks, err := h.store.ListTeamTasks(r.Context(), teamID)
	if mapDomainError(w, err) {
		return
	}
	if tasks == nil {
		tasks = []*domain.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

// CreateTask handles POST /v1/tasks.
func (h *REST) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamID      string `json:"team_id"`
		AgentID     string `json:"agent_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}

	// Verify team exists
	if _, err := h.store.GetTeam(r.Context(), req.TeamID); mapDomainError(w, err) {
		return
	}

	status := domain.TaskStatus(req.Status)
	if status == "" {
		status = domain.TaskStatusPending
	}

	task := &domain.Task{
		ID:          NewID("tk"),
		TeamID:      req.TeamID,
		AgentID:     req.AgentID,
		Title:       req.Title,
		Description: req.Description,
		Status:      status,
	}
	if mapDomainError(w, h.store.CreateTask(r.Context(), task)) {
		return
	}

	created, err := h.store.GetTask(r.Context(), task.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetTask handles GET /v1/tasks/{id}.
func (h *REST) GetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := h.store.GetTask(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// UpdateTask handles PUT /v1/tasks/{id}.
func (h *REST) UpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Get existing task to preserve unchanged fields
	existing, err := h.store.GetTask(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		AgentID     string `json:"agent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	// Apply updates to existing
	if req.Title != "" {
		existing.Title = req.Title
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Status != "" {
		existing.Status = domain.TaskStatus(req.Status)
	}
	// AgentID can be set to empty to unassign
	existing.AgentID = req.AgentID

	if mapDomainError(w, h.store.UpdateTask(r.Context(), id, existing)) {
		return
	}

	updated, err := h.store.GetTask(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteTask handles DELETE /v1/tasks/{id}.
func (h *REST) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteTask(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Create task handler tests**

Create `internal/daemon/rest_tasks_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTasks_CRUD(t *testing.T) {
	rest, teamID, _, _ := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tasks", rest.CreateTask)
	mux.HandleFunc("GET /v1/tasks/{id}", rest.GetTask)
	mux.HandleFunc("GET /v1/teams/{id}/tasks", rest.ListTeamTasks)
	mux.HandleFunc("PUT /v1/tasks/{id}", rest.UpdateTask)
	mux.HandleFunc("DELETE /v1/tasks/{id}", rest.DeleteTask)

	// Create
	body := `{"title":"fix bug","team_id":"` + teamID + `","description":"important"}`
	req := httptest.NewRequest("POST", "/v1/tasks", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)
	if created["Status"] != "pending" {
		t.Errorf("default status: got %v, want pending", created["Status"])
	}

	// Get
	req = httptest.NewRequest("GET", "/v1/tasks/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: status %d", rr.Code)
	}

	// List team tasks
	req = httptest.NewRequest("GET", "/v1/teams/"+teamID+"/tasks", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: status %d", rr.Code)
	}

	var tasks []map[string]any
	json.NewDecoder(rr.Body).Decode(&tasks)
	if len(tasks) != 1 {
		t.Errorf("got %d tasks, want 1", len(tasks))
	}

	// Update status
	body = `{"title":"fix bug","status":"in_progress"}`
	req = httptest.NewRequest("PUT", "/v1/tasks/"+id, bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var updated map[string]any
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated["Status"] != "in_progress" {
		t.Errorf("got status %v, want in_progress", updated["Status"])
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/v1/tasks/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", rr.Code)
	}
}

func TestTasks_CreateMissingFields(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tasks", rest.CreateTask)

	// Missing title
	req := httptest.NewRequest("POST", "/v1/tasks", bytes.NewBufferString(`{"team_id":"tm_1"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing title: got status %d, want 400", rr.Code)
	}

	// Missing team_id
	req = httptest.NewRequest("POST", "/v1/tasks", bytes.NewBufferString(`{"title":"x"}`))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing team_id: got status %d, want 400", rr.Code)
	}
}

func TestTasks_TeamNotFound(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tasks", rest.CreateTask)

	req := httptest.NewRequest("POST", "/v1/tasks", bytes.NewBufferString(`{"title":"x","team_id":"tm_nonexistent"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rr.Code)
	}
}

func TestTasks_WithAgent(t *testing.T) {
	rest, teamID, agentID, _ := newTestRESTWithData(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tasks", rest.CreateTask)
	mux.HandleFunc("GET /v1/tasks/{id}", rest.GetTask)

	body := `{"title":"assigned task","team_id":"` + teamID + `","agent_id":"` + agentID + `"}`
	req := httptest.NewRequest("POST", "/v1/tasks", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	if created["AgentID"] != agentID {
		t.Errorf("got agent_id %v, want %s", created["AgentID"], agentID)
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestTasks -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add task REST handlers with tests

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 10: Permission handlers

**Files:**
- Create: `internal/daemon/rest_permissions.go`
- Create: `internal/daemon/rest_permissions_test.go`

- [ ] **Step 1: Create permission handlers**

Create `internal/daemon/rest_permissions.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// GetAgentPermissions handles GET /v1/agents/{id}/permissions.
func (h *REST) GetAgentPermissions(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	// Verify agent exists
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}

	perms, err := h.store.GetAgentPermissions(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if perms == nil {
		perms = []domain.AgentPermission{}
	}
	writeJSON(w, http.StatusOK, perms)
}

// SetAgentPermissions handles PUT /v1/agents/{id}/permissions.
func (h *REST) SetAgentPermissions(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	// Verify agent exists
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}

	var req struct {
		Permissions []struct {
			Permission  string `json:"permission"`
			ScopeTeamID string `json:"scope_team_id"`
		} `json:"permissions"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	perms := make([]domain.AgentPermission, len(req.Permissions))
	for i, p := range req.Permissions {
		if p.Permission == "" {
			writeError(w, http.StatusBadRequest, "permission name is required for each entry")
			return
		}
		perms[i] = domain.AgentPermission{
			AgentID:      agentID,
			PermissionID: p.Permission,
			ScopeTeamID:  p.ScopeTeamID,
		}
	}

	if mapDomainError(w, h.store.SetAgentPermissions(r.Context(), agentID, perms)) {
		return
	}

	// Return updated permissions
	updated, err := h.store.GetAgentPermissions(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if updated == nil {
		updated = []domain.AgentPermission{}
	}
	writeJSON(w, http.StatusOK, updated)
}
```

- [ ] **Step 2: Create permission handler tests**

Create `internal/daemon/rest_permissions_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// newTestRESTWithPerms creates a REST handler with seeded permissions.
func newTestRESTWithPerms(t *testing.T) (*daemon.REST, string) {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	// Seed permissions
	if err := store.SeedPermissions(ctx, []string{"role:read", "role:write", "task:read"}); err != nil {
		t.Fatalf("seed permissions: %v", err)
	}

	team := &domain.Team{ID: "tm_perm01", Name: "permteam"}
	store.CreateTeam(ctx, team)

	agent := &domain.Agent{ID: "ag_perm01", Name: "permagent", TeamID: "tm_perm01"}
	store.CreateAgent(ctx, agent)

	return daemon.NewREST(store), agent.ID
}

func TestPermissions_SetAndGet(t *testing.T) {
	rest, agentID := newTestRESTWithPerms(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agents/{id}/permissions", rest.GetAgentPermissions)
	mux.HandleFunc("PUT /v1/agents/{id}/permissions", rest.SetAgentPermissions)

	// Get empty
	req := httptest.NewRequest("GET", "/v1/agents/"+agentID+"/permissions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get empty: status %d", rr.Code)
	}

	var empty []map[string]any
	json.NewDecoder(rr.Body).Decode(&empty)
	if len(empty) != 0 {
		t.Errorf("got %d permissions, want 0", len(empty))
	}

	// Set permissions
	body := `{"permissions":[{"permission":"role:read"},{"permission":"task:read","scope_team_id":"tm_perm01"}]}`
	req = httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/permissions", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("set: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var perms []map[string]any
	json.NewDecoder(rr.Body).Decode(&perms)
	if len(perms) != 2 {
		t.Errorf("got %d permissions, want 2", len(perms))
	}

	// Get updated
	req = httptest.NewRequest("GET", "/v1/agents/"+agentID+"/permissions", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	json.NewDecoder(rr.Body).Decode(&perms)
	if len(perms) != 2 {
		t.Errorf("got %d permissions after re-get, want 2", len(perms))
	}
}

func TestPermissions_ClearAll(t *testing.T) {
	rest, agentID := newTestRESTWithPerms(t)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/agents/{id}/permissions", rest.SetAgentPermissions)

	// Set
	body := `{"permissions":[{"permission":"role:read"}]}`
	req := httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/permissions", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Clear
	body = `{"permissions":[]}`
	req = httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/permissions", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("clear: status %d", rr.Code)
	}

	var perms []map[string]any
	json.NewDecoder(rr.Body).Decode(&perms)
	if len(perms) != 0 {
		t.Errorf("got %d permissions after clear, want 0", len(perms))
	}
}

func TestPermissions_AgentNotFound(t *testing.T) {
	rest := newTestREST(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agents/{id}/permissions", rest.GetAgentPermissions)

	req := httptest.NewRequest("GET", "/v1/agents/ag_nonexistent/permissions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rr.Code)
	}
}
```

- [ ] **Step 3: Build and test**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -run TestPermissions -v
```

- [ ] **Step 4: Commit**

```
feat(daemon): add permission REST handlers with tests

Co-Authored-By: Claude <noreply@anthropic.com>
```

---

### Task 11: Full build, server wiring, and smoke test

**Files:**
- Modify: `internal/daemon/server.go` (already updated in Task 4)
- Modify: `cmd/daemon/daemon.go` (already updated in Task 4)

- [ ] **Step 1: Verify complete build**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go build ./...
```

- [ ] **Step 2: Run all REST tests**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./internal/daemon/ -v
```

- [ ] **Step 3: Run all project tests**

```bash
cd /home/kazw/Work/WorkFort/hive/lead
go test ./...
```

- [ ] **Step 4: Commit server wiring**

```
feat(daemon): wire all REST routes into server with API key auth

Registers all team, role, document, agent, task, and permission
routes on the HTTP mux with Bearer token middleware.

Co-Authored-By: Claude <noreply@anthropic.com>
```
