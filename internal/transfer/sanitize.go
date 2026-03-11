// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"regexp"
	"strings"
)

var nonAlphanumHyphen = regexp.MustCompile(`[^a-z0-9-]+`)
var multiHyphen = regexp.MustCompile(`-{3,}`)

// SanitizeName converts an entity name to a safe filename component.
// Lowercase, spaces to hyphens, non-alphanumeric stripped.
func SanitizeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumHyphen.ReplaceAllString(s, "")
	s = multiHyphen.ReplaceAllString(s, "--")
	s = strings.Trim(s, "-")
	if s == "" {
		return "_unnamed"
	}
	return s
}
