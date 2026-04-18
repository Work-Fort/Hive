// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// startJWKSStub starts a JWKS stub server that serves:
//   - GET /v1/jwks — the public key in JWKS format
//   - POST /v1/verify-api-key — rejects all API keys with 401 (the e2e suite
//     only uses JWT auth via signJWT; a permissive stub defeats negative-auth
//     tests like TestUnauthorizedRequest)
//
// It returns:
//   - addr: the server address (host:port)
//   - stop: function to stop the server
//   - signJWT: function to create signed JWTs with the expected claims
func startJWKSStub() (addr string, stop func(), signJWT func(id, username, name, userType string) string) {
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

	// Reject all API keys. The e2e harness only uses JWT auth (via signJWT),
	// so any bearer token that falls through to API key validation is by
	// definition not valid. Returning 401 here is what lets negative-auth
	// tests (TestUnauthorizedRequest) work.
	mux.HandleFunc("POST /v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "unknown api key"}) //nolint:errcheck
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

	return ln.Addr().String(), stopFn, signFn
}
