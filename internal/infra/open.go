// SPDX-License-Identifier: GPL-3.0-or-later
package infra

import (
	"fmt"
	"strings"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

// Open auto-detects the database backend from the DSN and returns a Store.
//
// DSN formats:
//   - postgres://... or postgresql://... -> PostgreSQL (not yet implemented)
//   - Any file path or empty string      -> SQLite (empty = :memory:)
func Open(dsn string) (domain.Store, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return nil, fmt.Errorf("postgres not yet implemented")
	}
	return sqlite.Open(dsn)
}
