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

	req := httptest.NewRequest("POST", "/v1/roles", bytes.NewBufferString(`{"name":"base"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var parent map[string]any
	json.NewDecoder(rr.Body).Decode(&parent)
	parentID := parent["ID"].(string)

	body := `{"name":"child","parent_id":"` + parentID + `"}`
	req = httptest.NewRequest("POST", "/v1/roles", bytes.NewBufferString(body))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create child: status %d, body: %s", rr.Code, rr.Body.String())
	}

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
