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

	req = httptest.NewRequest("GET", "/v1/tasks/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get: status %d", rr.Code)
	}

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
	req := httptest.NewRequest("POST", "/v1/tasks", bytes.NewBufferString(`{"team_id":"tm_1"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing title: got status %d, want 400", rr.Code)
	}
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
