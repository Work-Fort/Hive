// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// openTestStore opens an in-memory SQLite store for testing.
func openTestStore(dsn string) (domain.Store, error) {
	return sqlite.Open(dsn)
}
