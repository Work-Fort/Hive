// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"database/sql"
	"fmt"
	"net/url"
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

// AltDB returns a second DB DSN for tests that need a fresh store
// alongside the harness daemon's primary store.
//
// Under SQLite (HIVE_DB unset) it returns a fresh tempfile path inside
// t.TempDir() — the file does not yet exist; the daemon (or import
// CLI) seeds it on first open.
//
// Under PostgreSQL it constructs a sibling database DSN by appending
// "_b" to the database name in HIVE_DB (e.g.
// hive_test → hive_test_b), creates the sibling DB if absent, resets
// its schema (DROP/CREATE public), and registers a t.Cleanup that
// resets it again. We do not DROP DATABASE because that races with
// daemon shutdown; leaving a clean schema is sufficient.
func AltDB(t *testing.T) string {
	t.Helper()
	if !usingPostgres() {
		return filepath.Join(t.TempDir(), "alt.db")
	}

	envDSN := os.Getenv("HIVE_DB")
	u, err := url.Parse(envDSN)
	if err != nil {
		t.Fatalf("AltDB: parse HIVE_DB %q: %v", envDSN, err)
	}
	origDB := strings.TrimPrefix(u.Path, "/")
	siblingDB := origDB + "_b"
	u.Path = "/" + siblingDB
	siblingDSN := u.String()

	adminDB, err := sql.Open("pgx", envDSN)
	if err != nil {
		t.Fatalf("AltDB: open admin connection: %v", err)
	}
	defer adminDB.Close()

	var exists bool
	if err := adminDB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", siblingDB,
	).Scan(&exists); err != nil {
		t.Fatalf("AltDB: check sibling db existence: %v", err)
	}
	if !exists {
		if _, err := adminDB.Exec("CREATE DATABASE " + siblingDB); err != nil {
			t.Fatalf("AltDB: create sibling db %q: %v", siblingDB, err)
		}
	}

	if err := resetPostgres(siblingDSN); err != nil {
		t.Fatalf("AltDB: reset sibling postgres %q: %v", siblingDSN, err)
	}
	t.Cleanup(func() {
		if err := resetPostgres(siblingDSN); err != nil {
			t.Logf("AltDB cleanup: reset sibling postgres: %v", err)
		}
	})
	return siblingDSN
}

func TestAltDB_PostgresReturnsSibling(t *testing.T) {
	// SQLite branch of AltDB is exercised by TestExportImportRoundTrip
	// when HIVE_DB is unset.
	if !usingPostgres() {
		t.Skipf("HIVE_DB not set to a postgres:// DSN; sibling-DB shape only checked under PG")
	}
	dsn := AltDB(t)
	if !strings.HasSuffix(dsn, "_b?sslmode=disable") && !strings.Contains(dsn, "_b?") {
		t.Fatalf("expected sibling DSN ending with database name +_b, got %q", dsn)
	}
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
