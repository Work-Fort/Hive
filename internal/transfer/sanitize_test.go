package transfer

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Engineering", "engineering"},
		{"My Role!", "my-role"},
		{"hello world", "hello-world"},
		{"a--b", "a--b"},
		{"  spaces  ", "spaces"},
		{"café", "caf"},
		{"!!!", "_unnamed"},
		{"", "_unnamed"},
	}
	for _, tt := range tests {
		got := SanitizeName(tt.in)
		if got != tt.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
