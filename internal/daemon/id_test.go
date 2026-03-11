// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"strings"
	"testing"
)

func TestNewID_Format(t *testing.T) {
	tests := []struct {
		prefix string
	}{
		{"tm"},
		{"rl"},
		{"ag"},
		{"doc"},
		{"tk"},
	}
	for _, tt := range tests {
		id := NewID(tt.prefix)
		if !strings.HasPrefix(id, tt.prefix+"_") {
			t.Errorf("NewID(%q) = %q, missing prefix", tt.prefix, id)
		}
		// prefix + "_" + 16 hex chars
		wantLen := len(tt.prefix) + 1 + 16
		if len(id) != wantLen {
			t.Errorf("NewID(%q) = %q, len %d, want %d", tt.prefix, id, len(id), wantLen)
		}
	}
}

func TestNewID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewID("tm")
		if seen[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
}
