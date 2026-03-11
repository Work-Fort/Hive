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

func newTestRESTWithPerms(t *testing.T) (*daemon.REST, string) {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
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
	body := `{"permissions":[{"permission":"role:read"}]}`
	req := httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/permissions", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
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
