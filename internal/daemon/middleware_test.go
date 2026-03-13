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
