// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

// REST holds shared dependencies for all REST handlers.
type REST struct {
	store domain.Store
}

// NewREST creates a new REST handler group.
func NewREST(store domain.Store) *REST {
	return &REST{store: store}
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// readJSON decodes the request body into v.
func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// mapDomainError maps a domain error to an HTTP status code and writes the response.
// Returns true if an error was handled (caller should return).
func mapDomainError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrHasDependencies):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrDepthExceeded):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrCycleDetected):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
	return true
}
