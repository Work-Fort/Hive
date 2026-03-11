// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthService_InitiallyHealthy(t *testing.T) {
	h := NewHealthService()
	r := h.Report()
	if r.Status != string(StatusHealthy) {
		t.Errorf("expected healthy, got %s", r.Status)
	}
	if len(r.Checks) != 0 {
		t.Errorf("expected no checks, got %d", len(r.Checks))
	}
}

func TestHealthService_RegisterBootCheck_OK(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("db", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityOK}
	})
	r := h.Report()
	if r.Status != string(StatusHealthy) {
		t.Errorf("expected healthy, got %s", r.Status)
	}
	if len(r.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(r.Checks))
	}
	if r.Checks[0].Name != "db" {
		t.Errorf("expected check name 'db', got %s", r.Checks[0].Name)
	}
}

func TestHealthService_RegisterBootCheck_Warning(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("audit", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityWarning, Message: "chain too deep"}
	})
	r := h.Report()
	if r.Status != string(StatusDegraded) {
		t.Errorf("expected degraded, got %s", r.Status)
	}
}

func TestHealthService_RegisterBootCheck_Error(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("db", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityError, Message: "connection refused"}
	})
	r := h.Report()
	if r.Status != string(StatusUnhealthy) {
		t.Errorf("expected unhealthy, got %s", r.Status)
	}
}

func TestHealthService_ErrorDominatesWarning(t *testing.T) {
	h := NewHealthService()
	h.RegisterBootCheck("audit", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityWarning, Message: "depth warning"}
	})
	h.RegisterBootCheck("db", func(_ context.Context) CheckResult {
		return CheckResult{Severity: SeverityError, Message: "db down"}
	})
	r := h.Report()
	if r.Status != string(StatusUnhealthy) {
		t.Errorf("expected unhealthy when error present, got %s", r.Status)
	}
}

func TestHealthService_AddWarningCompat(t *testing.T) {
	h := NewHealthService()
	h.AddWarning("role too deep")
	r := h.Report()
	if r.Status != string(StatusDegraded) {
		t.Errorf("expected degraded via AddWarning, got %s", r.Status)
	}
}

func TestHealthService_AddErrorCompat(t *testing.T) {
	h := NewHealthService()
	h.AddError("db failed")
	r := h.Report()
	if r.Status != string(StatusUnhealthy) {
		t.Errorf("expected unhealthy via AddError, got %s", r.Status)
	}
}

func TestHealthService_PeriodicCheckUpdatesResult(t *testing.T) {
	h := NewHealthService()
	calls := 0
	h.RegisterPeriodicCheck("ticker", func(_ context.Context) CheckResult {
		calls++
		if calls == 1 {
			return CheckResult{Severity: SeverityOK}
		}
		return CheckResult{Severity: SeverityWarning, Message: "flapped"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.StartPeriodic(ctx, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	cancel()

	r := h.Report()
	// After at least one tick the warning must have been recorded.
	if r.Status == string(StatusHealthy) && calls > 1 {
		t.Errorf("expected degraded after periodic tick updated the result")
	}
}

func TestHandleHealth_HTTP_StatusCodes(t *testing.T) {
	cases := []struct {
		name       string
		severity   CheckSeverity
		wantStatus int
	}{
		{"healthy", SeverityOK, http.StatusOK},
		{"degraded", SeverityWarning, 218},
		{"unhealthy", SeverityError, http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHealthService()
			if tc.severity != SeverityOK {
				h.RegisterBootCheck("test", func(_ context.Context) CheckResult {
					return CheckResult{Severity: tc.severity, Message: "test"}
				})
			}
			handler := HandleHealth(h)
			req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
			rr := httptest.NewRecorder()
			handler(rr, req)
			if rr.Code != tc.wantStatus {
				t.Errorf("want status %d, got %d", tc.wantStatus, rr.Code)
			}
			var report HealthReport
			if err := json.NewDecoder(rr.Body).Decode(&report); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if report.Status == "" {
				t.Error("response missing status field")
			}
		})
	}
}
