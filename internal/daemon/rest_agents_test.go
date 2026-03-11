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
	body := `{"roles":[{"role_id":"` + roleID + `","priority":1}]}`
	req := httptest.NewRequest("PUT", "/v1/agents/"+agentID+"/roles", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
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
	req := httptest.NewRequest("POST", "/v1/agents", bytes.NewBufferString(`{"name":"x"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing team_id: got status %d, want 400", rr.Code)
	}
	req = httptest.NewRequest("POST", "/v1/agents", bytes.NewBufferString(`{"team_id":"tm_1"}`))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing name: got status %d, want 400", rr.Code)
	}
}
