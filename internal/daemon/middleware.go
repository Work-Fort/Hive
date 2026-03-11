// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"
	"strings"
)

// APIKeyAuth returns middleware that checks the Authorization header for a
// Bearer token matching apiKey. Skips authentication if apiKey is empty.
func APIKeyAuth(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		token, found := strings.CutPrefix(auth, "Bearer ")
		if !found || token != apiKey {
			writeError(w, http.StatusForbidden, "invalid api key")
			return
		}

		next.ServeHTTP(w, r)
	})
}
