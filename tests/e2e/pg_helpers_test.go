// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// usingPostgres reports whether HIVE_DB selects a PostgreSQL backend.
func usingPostgres() bool {
	dsn := os.Getenv("HIVE_DB")
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// resetPostgres drops and recreates the public schema so each test
// starts from a clean database. Goose migrations re-run on daemon
// startup. All three DDL statements run in a single Exec so we don't
// leave a window where another connection sees the new schema before
// permissions are restored.
func resetPostgres(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	_, err = db.Exec(`
		DROP SCHEMA IF EXISTS public CASCADE;
		CREATE SCHEMA public;
		GRANT ALL ON SCHEMA public TO PUBLIC;
	`)
	if err != nil {
		return fmt.Errorf("reset schema: %w", err)
	}
	return nil
}

// TestHarness_UsesPostgresWhenHiveDBSet boots the harness with HIVE_DB
// pointed at PG and verifies that the daemon's database file is NOT
// created on disk (proving the daemon dialled PG, not the SQLite
// fallback path).
func TestHarness_UsesPostgresWhenHiveDBSet(t *testing.T) {
	if !usingPostgres() {
		t.Skipf("HIVE_DB not set to a postgres:// DSN; this test runs in the e2e-postgres CI job and locally with HIVE_DB=postgres://...")
	}
	h := newHarness(t)
	sqlitePath := filepath.Join(h.dir, "state", "hive.db")
	if _, err := os.Stat(sqlitePath); err == nil {
		t.Fatalf("daemon created SQLite file at %s while HIVE_DB=postgres://...; backend selection broken", sqlitePath)
	}
}
