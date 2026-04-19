// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// startJWKSStub starts a JWKS stub server that serves:
//   - GET /v1/jwks — the public key in JWKS format (for any JWT-side
//     negative-auth tests that still construct Bearer tokens directly)
//   - POST /v1/verify-api-key — accepts only API keys registered via
//     mintAPIKey; rejects everything else with 401 so negative-auth tests
//     keep working.
//
// Returns:
//   - addr: the server address (host:port)
//   - stop: function to stop the server
//   - signJWT: function to create signed JWTs with the expected claims
//     (kept for negative-auth tests that construct raw Authorization headers)
//   - mintAPIKey: function that registers a new API key with the stub and
//     returns the key string. Pass this string into client.New — it travels
//     under the ApiKey-v1 scheme, the daemon scheme-dispatches to the API-key
//     validator, the validator calls /v1/verify-api-key here, and the
//     identity assembled from the registered claims is propagated.
func startJWKSStub() (addr string, stop func(), signJWT func(id, username, name, userType string) string, mintAPIKey func(id, username, name, userType string) string) {
	// Generate RSA key pair.
	rawKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: generate RSA key: %v", err))
	}

	// Build JWK from the private key with kid and algorithm set.
	privJWK, err := jwk.FromRaw(rawKey)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: create JWK from private key: %v", err))
	}
	_ = privJWK.Set(jwk.KeyIDKey, "test-key-1")
	_ = privJWK.Set(jwk.AlgorithmKey, jwa.RS256)

	privSet := jwk.NewSet()
	_ = privSet.AddKey(privJWK)

	pubSet, err := jwk.PublicSetOf(privSet)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: derive public key set: %v", err))
	}

	// Pre-marshal the public JWKS response.
	jwksBytes, err := json.Marshal(pubSet)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: marshal JWKS: %v", err))
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksBytes) //nolint:errcheck
	})

	// Registered API keys → identity claims. Populated by mintAPIKey.
	type apiKeyIdentity struct{ id, username, name, userType string }
	registry := make(map[string]apiKeyIdentity)
	var registryMu sync.Mutex

	// /v1/verify-api-key honours registered keys, rejects everything else.
	// Negative-auth tests (TestUnauthorizedRequest) rely on the rejection path.
	// Request body: {"key": "<api-key-string>"} (matches apikey.Validator in
	// service-auth v0.1.0 which marshals the field as "key", not "api_key").
	// Response body mirrors the better-auth /api/auth/get-api-key shape that
	// service-auth@v0.1.0 decodes: {"valid":true,"key":{"userId":"...","metadata":{...}}}
	mux.HandleFunc("POST /v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		registryMu.Lock()
		ident, ok := registry[body.Key]
		registryMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "unknown api key"}) //nolint:errcheck
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"valid": true,
			"key": map[string]any{
				"userId": ident.id,
				"metadata": map[string]any{
					"username":     ident.username,
					"name":         ident.name,
					"display_name": ident.name,
					"type":         ident.userType,
				},
			},
		}) //nolint:errcheck
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: listen: %v", err))
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln) //nolint:errcheck

	stopFn := func() {
		srv.Close()
	}

	// signJWT creates a signed RS256 JWT with the claims expected by the
	// Passport service-auth library (Subject → id.ID, plus username, name,
	// display_name, type custom claims).
	signFn := func(id, username, name, userType string) string {
		now := time.Now()
		tok, err := jwt.NewBuilder().
			Subject(id).
			Issuer("passport-stub").
			Audience([]string{"hive"}).
			IssuedAt(now).
			Expiration(now.Add(1*time.Hour)).
			Claim("username", username).
			Claim("name", name).
			Claim("display_name", name).
			Claim("type", userType).
			Build()
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: build JWT: %v", err))
		}

		// Sign using the JWK private key which carries kid and alg.
		signedBytes, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: sign JWT: %v", err))
		}
		return string(signedBytes)
	}

	mintFn := func(id, username, name, userType string) string {
		key := fmt.Sprintf("wf-svc_e2e-%s-%d", id, time.Now().UnixNano())
		registryMu.Lock()
		registry[key] = apiKeyIdentity{id: id, username: username, name: name, userType: userType}
		registryMu.Unlock()
		return key
	}

	return ln.Addr().String(), stopFn, signFn, mintFn
}
