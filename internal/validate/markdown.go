// SPDX-License-Identifier: GPL-3.0-or-later
package validate

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
)

// Markdown validates that content can be parsed as CommonMark.
//
// CommonMark is very permissive — almost any string is valid. This function
// currently catches encoding issues and serves as the integration point for
// future AST-based linting rules (no raw HTML, heading structure, etc.).
func Markdown(content string) error {
	md := goldmark.New()
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return fmt.Errorf("invalid markdown: %w", err)
	}
	return nil
}
