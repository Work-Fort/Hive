// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"encoding/json"
	"net/http"
	"sync"
)

// HealthStatus represents the overall system health.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// HealthReport is returned by the health endpoint.
type HealthReport struct {
	Status   HealthStatus `json:"status"`
	Warnings []string     `json:"warnings,omitempty"`
	Errors   []string     `json:"errors,omitempty"`
}

// HealthService tracks system health warnings and errors.
type HealthService struct {
	mu       sync.RWMutex
	warnings []string
	errors   []string
}

// NewHealthService creates a new HealthService.
func NewHealthService() *HealthService {
	return &HealthService{}
}

// AddWarning adds a health warning.
func (h *HealthService) AddWarning(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.warnings = append(h.warnings, msg)
}

// AddError adds a health error.
func (h *HealthService) AddError(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errors = append(h.errors, msg)
}

// Status returns the current health report.
func (h *HealthService) Status() HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()

	report := HealthReport{Status: StatusHealthy}

	if len(h.warnings) > 0 {
		report.Status = StatusDegraded
		report.Warnings = make([]string, len(h.warnings))
		copy(report.Warnings, h.warnings)
	}

	if len(h.errors) > 0 {
		report.Status = StatusUnhealthy
		report.Errors = make([]string, len(h.errors))
		copy(report.Errors, h.errors)
	}

	return report
}

// HandleHealth returns an http.HandlerFunc for the health endpoint.
func HandleHealth(health *HealthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.Status()

		var statusCode int
		switch report.Status {
		case StatusHealthy:
			statusCode = http.StatusOK
		case StatusDegraded:
			statusCode = 218
		default:
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(report)
	}
}
