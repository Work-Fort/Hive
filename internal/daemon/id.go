// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"crypto/rand"
	"fmt"
)

// NewID generates a prefixed random ID: "<prefix>_<16 hex chars>".
// Prefixes: tm (team), rl (role), ag (agent), doc (document), tk (task).
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("%s_%x", prefix, b)
}
