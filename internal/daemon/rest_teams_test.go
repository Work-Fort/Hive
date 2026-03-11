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
	for _, name := range []string{"alpha", "beta"} {
		body := `{"name":"` + name + `"}`
		req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: status %d", name, rr.Code)
		}
	}
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
	req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(`{"name":"alpha"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)
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
	req := httptest.NewRequest("POST", "/v1/teams", bytes.NewBufferString(`{"name":"alpha"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := created["ID"].(string)
	req = httptest.NewRequest("DELETE", "/v1/teams/"+id, nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: got status %d", rr.Code)
	}
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
