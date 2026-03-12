// SPDX-License-Identifier: GPL-3.0-or-later
package validate_test

import (
	"testing"

	"github.com/Work-Fort/Hive/internal/validate"
)

func TestMarkdown_ValidContent(t *testing.T) {
	cases := []string{
		"# Hello\n\nSome text.",
		"Plain text without any formatting.",
		"- item 1\n- item 2\n- item 3",
		"```go\nfunc main() {}\n```",
		"",
	}
	for _, content := range cases {
		if err := validate.Markdown(content); err != nil {
			t.Errorf("Markdown(%q) = %v, want nil", content, err)
		}
	}
}

func TestMarkdown_EmptyString(t *testing.T) {
	if err := validate.Markdown(""); err != nil {
		t.Fatalf("empty string should be valid: %v", err)
	}
}
