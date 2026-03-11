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
