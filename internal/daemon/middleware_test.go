// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	auth "github.com/Work-Fort/Passport/go/service-auth"
)

type stubJWTValidator struct{ calls atomic.Int32 }

func (s *stubJWTValidator) Validate(_ context.Context, token string) (auth.Identity, error) {
	s.calls.Add(1)
	if len(token) >= 4 && token[:4] == "jwt-" {
		return auth.Identity{ID: "u1", Username: "user", Type: auth.TypeUser}, nil
	}
	return auth.Identity{}, auth.ErrInvalidToken
}

type stubAPIKeyValidator struct{ calls atomic.Int32 }

func (s *stubAPIKeyValidator) Validate(_ context.Context, token string) (auth.Identity, error) {
	s.calls.Add(1)
	if len(token) >= 7 && token[:7] == "wf-svc_" {
		return auth.Identity{ID: "s1", Username: "svc", Type: auth.TypeService}, nil
	}
	return auth.Identity{}, auth.ErrInvalidToken
}

func buildSchemeHandler(jwtV, akV auth.Validator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agents", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := auth.NewSchemeDispatch(jwtV, akV)
	return publicPathSkip(mw(mux), mux)
}

// Cluster 3b regression: a Bearer token failing JWT validation must NOT fall
// through to the API-key validator. Without scheme dispatch, a malformed JWT
// hits the API-key validator's network call to passport.
func TestSchemeDispatch_BearerMalformedJWTDoesNotFallThrough(t *testing.T) {
	jwtV := &stubJWTValidator{}
	akV := &stubAPIKeyValidator{}
	h := buildSchemeHandler(jwtV, akV)

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.jwt")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if akV.calls.Load() != 0 {
		t.Errorf("api-key validator called %d times; expected 0", akV.calls.Load())
	}
}

func TestSchemeDispatch_ApiKeyV1Routes(t *testing.T) {
	jwtV := &stubJWTValidator{}
	akV := &stubAPIKeyValidator{}
	h := buildSchemeHandler(jwtV, akV)

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req.Header.Set("Authorization", "ApiKey-v1 wf-svc_x")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if jwtV.calls.Load() != 0 {
		t.Errorf("jwt validator called %d times for ApiKey-v1 request; expected 0", jwtV.calls.Load())
	}
}

// Bearer-prefixed API key must be rejected — closes the latent-bug class
// where consumers passed a wf-svc_ key into a Bearer header and got accepted
// via validator fallthrough.
func TestSchemeDispatch_BearerForAPIKeyReturns401(t *testing.T) {
	jwtV := &stubJWTValidator{}
	akV := &stubAPIKeyValidator{}
	h := buildSchemeHandler(jwtV, akV)

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer wf-svc_x")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if akV.calls.Load() != 0 {
		t.Errorf("api-key validator called %d times; expected 0 (no fallthrough)", akV.calls.Load())
	}
}

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
