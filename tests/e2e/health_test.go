// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"fmt"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestHealth(t *testing.T) {
	h := newHarness(t)

	report, err := h.Client.Health(ctx())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}

	if report.Status != "healthy" {
		t.Errorf("expected status %q, got %q", "healthy", report.Status)
	}
}

// TestHealthUnauthenticated verifies that the health endpoint does not require
// an API key — it is intentionally public.
func TestHealthUnauthenticated(t *testing.T) {
	h := newHarness(t)

	// Create a client with no API key targeting the same port.
	unauthClient := client.New(
		fmt.Sprintf("http://127.0.0.1:%d", h.port),
		"",
	)

	report, err := unauthClient.Health(ctx())
	if err != nil {
		t.Fatalf("Health without API key: %v", err)
	}
	if report.Status == "" {
		t.Error("expected non-empty status in health report")
	}
}
