// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import "strings"

// isUniqueViolation returns true if the error is a SQLite unique constraint
// violation. modernc.org/sqlite wraps errors as strings.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
