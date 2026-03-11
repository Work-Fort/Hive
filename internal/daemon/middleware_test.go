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
