// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common HTTP status codes. Use errors.Is to check.
var (
	// ErrBadRequest is returned when the server responds with 400.
	ErrBadRequest = errors.New("bad request")

	// ErrUnauthorized is returned when the server responds with 401.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when the server responds with 403.
	ErrForbidden = errors.New("forbidden")

	// ErrNotFound is returned when the server responds with 404.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when the server responds with 409.
	ErrConflict = errors.New("conflict")

	// ErrUnprocessable is returned when the server responds with 422.
	ErrUnprocessable = errors.New("unprocessable entity")
)

// APIError is returned for any non-2xx HTTP response. It carries the HTTP
// status code and the error message decoded from the server's JSON body.
type APIError struct {
	StatusCode int
	Message    string
	// sentinel wraps one of the package-level sentinel errors when the status
	// code matches a known value.
	sentinel error
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("hive api error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("hive api error %d", e.StatusCode)
}

// Unwrap allows errors.Is to match sentinel errors.
func (e *APIError) Unwrap() error { return e.sentinel }

// decodeAPIError reads the response body and returns an *APIError. It maps
// known status codes to sentinel errors via Unwrap.
func decodeAPIError(resp *http.Response) *APIError {
	var body struct {
		Error string `json:"error"`
	}
	// Best-effort decode; ignore errors (body may be empty or non-JSON).
	json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck

	ae := &APIError{StatusCode: resp.StatusCode, Message: body.Error}
	switch resp.StatusCode {
	case http.StatusBadRequest:
		ae.sentinel = ErrBadRequest
	case http.StatusUnauthorized:
		ae.sentinel = ErrUnauthorized
	case http.StatusForbidden:
		ae.sentinel = ErrForbidden
	case http.StatusNotFound:
		ae.sentinel = ErrNotFound
	case http.StatusConflict:
		ae.sentinel = ErrConflict
	case http.StatusUnprocessableEntity:
		ae.sentinel = ErrUnprocessable
	}
	return ae
}
