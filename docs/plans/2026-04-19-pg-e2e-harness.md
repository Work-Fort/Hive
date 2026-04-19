---
type: plan
step: "2026-04-19-pg-e2e-harness"
title: "Hive PG e2e harness"
status: complete
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "2026-04-19-pg-e2e-harness"
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: "2026-04-19"
related_plans:
  - 008-postgres.md
  - 010-e2e-tests.md
---

# Hive PG e2e harness

## Overview

Hive's e2e suite (`tests/e2e/`) currently runs only against SQLite — `harness_test.go:103` hardcodes `dbPath := filepath.Join(stateDir, "hive.db")` and passes it via `--db`. Hive's domain `Store` already has a parallel PostgreSQL adapter at `internal/infra/postgres/`, dispatched by `internal/infra/open.go` based on DSN prefix. The Postgres path is exercised by adapter-level unit tests but never by the 28 e2e tests.

This plan brings Hive's e2e harness in line with the Sharkfin / Combine pattern landed earlier in 2026-04: backend selection via env var, per-test PG schema reset, an `AltDB(t)` helper for tests that spin up a second store, a `mise run e2e` task that stays a single command, and a parallel `e2e-postgres` CI job in a new `.github/workflows/ci.yaml` (Hive currently ships only `release.yml`).

After this plan all 28 e2e tests run unchanged on both SQLite and PostgreSQL. No silent skips. No backend-conditional `t.Skip` calls.

## Prerequisites

- Hive `master` clean working tree.
- Local Postgres 17 reachable for manual verification (`createdb hive_test` once).
- `mise run build:dev` succeeds.

## Reference patterns

The harness pattern closely follows Sharkfin's single-env-var convention because Hive's daemon already speaks the same DSN-dispatch language as Sharkfin's:

- Hive `--db <dsn>` accepts either a SQLite file path or a `postgres://` DSN. Auto-dispatch lives in `internal/infra/open.go`.
- Hive's viper config uses `EnvPrefix=HIVE` with `_` ↔ `-` replacement, so the env var `HIVE_DB` already feeds the `db` viper key. We rely on this directly.
- Sharkfin uses a single `SHARKFIN_DB` env var; Combine uses a two-var split (`COMBINE_DB_DRIVER` + `COMBINE_DB_DATA_SOURCE`) because Combine's daemon takes the driver and DSN as separate config keys. **Hive matches Sharkfin's single-var shape**, not Combine's, because Hive's daemon uses one `--db` flag.

The PG schema reset, `AltDB(t)` helper, CI service block, and `pgx/v5/stdlib` import all mirror the constellation.

## Task breakdown

### Task 1: Add pgx driver dependency to e2e module

**Files:**
- Modify: `tests/e2e/go.mod`
- Modify: `tests/e2e/go.sum`

**Step 1: Add the pgx driver import to a new helpers file (forces `go mod tidy` to record the dep)**

Create `tests/e2e/pg_helpers_test.go` with the import only (the rest of the file lands in Task 2):

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	_ "github.com/jackc/pgx/v5/stdlib"
)
```

**Step 2: Tidy the e2e module**

Run: `cd tests/e2e && go mod tidy`
Expected: `go.mod` gains `github.com/jackc/pgx/v5 v5.x.x` in the `require` block; `go.sum` updated.

**Step 3: Verify the e2e suite still builds**

Run: `mise run build:dev && cd tests/e2e && go test -count=1 -run='^$' ./...`
Expected: PASS (no tests selected, package builds clean).

**Step 4: Commit**

```bash
git add tests/e2e/go.mod tests/e2e/go.sum tests/e2e/pg_helpers_test.go
git commit -m "$(cat <<'EOF'
chore(e2e): add pgx stdlib driver to e2e module

Pull in github.com/jackc/pgx/v5/stdlib so the e2e harness can open a
PostgreSQL connection for per-test schema reset. Driver registration
is required before any database/sql code can dial postgres://.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Backend selection in harness

**Depends on:** Task 1 (pgx driver registered)

**Files:**
- Modify: `tests/e2e/harness_test.go:102-118` (replace hardcoded SQLite path with env-driven DSN)
- Modify: `tests/e2e/pg_helpers_test.go` (add `resetPostgres` helper)

**Step 1: Write the failing test — env-driven backend selection**

Add to `tests/e2e/pg_helpers_test.go` (alongside the import from Task 1):

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"database/sql"
	"fmt"
	"os"
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
```

(Add `path/filepath` to the imports.)

**Step 2: Run the test against PG to verify it fails**

Run:
```
cd tests/e2e && \
  HIVE_DB="postgres://hive:hive@localhost:5432/hive_test?sslmode=disable" \
  go test -v -count=1 -run TestHarness_UsesPostgresWhenHiveDBSet ./...
```
Expected: FAIL — `daemon created SQLite file at /tmp/hive-e2e-harness-XXX/state/hive.db while HIVE_DB=postgres://...` (because `harness_test.go:103` still hardcodes the SQLite path).

**Step 3: Modify `tests/e2e/harness_test.go` — replace the hardcoded `--db` block**

Replace the block currently at lines 102-118 (the `// SQLite database inside the state directory.` block through the `cmd.Env = append(...)`):

```go
	// Backend selection. Default: SQLite file inside the per-harness
	// state dir. If HIVE_DB is set to a postgres://... DSN we use that
	// instead and reset its schema before the daemon comes up so each
	// test starts from a clean store; the daemon's DSN-dispatch (see
	// internal/infra/open.go) routes the rest. SQLite tempfiles cannot
	// collide because each harness gets its own t.TempDir-rooted state
	// directory.
	dbDSN := os.Getenv("HIVE_DB")
	if dbDSN == "" {
		dbDSN = filepath.Join(stateDir, "hive.db")
	} else if strings.HasPrefix(dbDSN, "postgres://") || strings.HasPrefix(dbDSN, "postgresql://") {
		if err := resetPostgres(dbDSN); err != nil {
			stubStop()
			os.RemoveAll(dir)
			t.Fatalf("reset postgres: %v", err)
		}
	}

	cmd := exec.Command(
		hiveBin,
		"daemon",
		"--bind", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--db", dbDSN,
		"--passport-url", "http://"+stubAddr,
		"--log-level", "disabled",
		"--sweeper-interval", "200ms",
	)
	cmd.Env = append(os.Environ(),
		"XDG_STATE_HOME="+stateDir,
		"XDG_CONFIG_HOME="+configDir,
	)
```

Add `"strings"` to the import block.

**Step 4: Run the test to verify it passes on PG**

Run:
```
cd tests/e2e && \
  HIVE_DB="postgres://hive:hive@localhost:5432/hive_test?sslmode=disable" \
  go test -v -count=1 -run TestHarness_UsesPostgresWhenHiveDBSet ./...
```
Expected: PASS.

**Step 5: Run the test to verify it skips on SQLite (the default)**

Run: `mise run build:dev && cd tests/e2e && go test -v -count=1 -run TestHarness_UsesPostgresWhenHiveDBSet ./...`
Expected: SKIP with `HIVE_DB not set to a postgres:// DSN` message.

> Note: the `t.Skip` here is environment-impossible by design — when SQLite is the chosen backend, asserting that PG was selected is meaningless. This is the only legitimate skip the plan introduces. Per `feedback_no_test_failures.md`, this counts as an environment-conditional skip (analogous to OS-conditional), not a silent backend gate. Task 7 audits the rest of the suite to confirm no other test silently no-ops on backend.

**Step 6: Commit**

```bash
git add tests/e2e/harness_test.go tests/e2e/pg_helpers_test.go
git commit -m "$(cat <<'EOF'
feat(e2e): backend selection via HIVE_DB env var

Replace the hardcoded SQLite path in newHarness with an env-driven
DSN: when HIVE_DB is set to a postgres:// DSN the harness resets the
public schema before daemon startup and forwards the DSN through the
existing --db flag. Default behaviour (HIVE_DB unset) is unchanged —
each harness still gets its own per-test SQLite tempfile.

Hive's --db flag already auto-dispatches by DSN prefix in
internal/infra/open.go, and the viper config maps HIVE_DB onto the
db key automatically; no daemon-side wiring is needed.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: AltDB helper for tests that need a second store

**Depends on:** Task 2 (resetPostgres + usingPostgres helpers exist)

**Background:** `TestExportImportRoundTrip` and `TestExportImportDryRun` (in `tests/e2e/export_import_test.go`) build an SQLite tempfile path and pass it to `hive import --db <path>` to import into a fresh database. Under PG that hardcoded `freshDBPath` is a SQLite path the daemon cannot reach, so those two tests would crash on the PG backend. We need an `AltDB(t)` helper that returns a sibling DSN appropriate for the active backend.

**Files:**
- Modify: `tests/e2e/pg_helpers_test.go` (add `AltDB`)
- Modify: `tests/e2e/export_import_test.go:93` and `:135` (use `AltDB(t)` instead of hardcoded SQLite tempfile)

**Step 1: Write the failing test — AltDB returns a sibling PG DSN**

Add to `tests/e2e/pg_helpers_test.go`:

```go
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
```

Add `"net/url"` and `"path/filepath"` to imports.

Add a unit test below it:

```go
func TestAltDB_PostgresReturnsSibling(t *testing.T) {
	if !usingPostgres() {
		t.Skipf("HIVE_DB not set to a postgres:// DSN; sibling-DB shape only checked under PG")
	}
	dsn := AltDB(t)
	if !strings.HasSuffix(dsn, "_b?sslmode=disable") && !strings.Contains(dsn, "_b?") {
		t.Fatalf("expected sibling DSN ending with database name +_b, got %q", dsn)
	}
}
```

**Step 2: Run the test to verify it fails (compile error: undefined: AltDB)**

Run: `cd tests/e2e && go test -count=1 -run TestAltDB_PostgresReturnsSibling ./...`
Expected: FAIL — undefined `AltDB` (only after Step 1 lands the helper this passes; if instead you write the test first and the helper second, the failure mode is "undefined: AltDB").

**Step 3: Run the test to verify it passes under PG**

Run:
```
cd tests/e2e && \
  HIVE_DB="postgres://hive:hive@localhost:5432/hive_test?sslmode=disable" \
  go test -v -count=1 -run TestAltDB_PostgresReturnsSibling ./...
```
Expected: PASS, with `psql -l` showing `hive_test_b` exists after the run.

**Step 4: Modify `tests/e2e/export_import_test.go:93` (TestExportImportRoundTrip)**

Replace:
```go
	freshDBPath := filepath.Join(t.TempDir(), "fresh.db")
```
with:
```go
	freshDBPath := AltDB(t)
```

**Step 5: Modify `tests/e2e/export_import_test.go:135` (TestExportImportDryRun)**

Replace:
```go
	freshDBPath := filepath.Join(t.TempDir(), "dry-run.db")
```
with:
```go
	freshDBPath := AltDB(t)
```

**Step 6: Run the full e2e suite under SQLite**

Run: `mise run e2e`
Expected: PASS — all 28+1 tests green (the new TestAltDB_PostgresReturnsSibling skips under SQLite; the new TestHarness_UsesPostgresWhenHiveDBSet skips too).

**Step 7: Run the full e2e suite under PG**

Run:
```
mise run build:dev && \
  cd tests/e2e && \
  HIVE_DB="postgres://hive:hive@localhost:5432/hive_test?sslmode=disable" \
  go test -v -race -count=1 -timeout 600s ./...
```
Expected: PASS for every test except those legitimately skipped (the SQLite-only skip does not apply here; the new `TestHarness_UsesPostgresWhenHiveDBSet` and `TestAltDB_PostgresReturnsSibling` actively run).

**Step 8: Commit**

```bash
git add tests/e2e/pg_helpers_test.go tests/e2e/export_import_test.go
git commit -m "$(cat <<'EOF'
feat(e2e): add AltDB helper for second-store tests

Mirror the Sharkfin/Combine AltDB pattern: under SQLite return a fresh
tempfile, under PostgreSQL construct a sibling database DSN (suffix
_b), create it if absent, reset its public schema, and clean up after
the test. Wire TestExportImportRoundTrip and TestExportImportDryRun
through the helper so the import CLI no longer hardcodes a SQLite
path that the daemon cannot reach when HIVE_DB selects PG.

The helper does not DROP DATABASE — that races with daemon shutdown.
A clean schema is sufficient for test isolation, and the sibling DB
persists between runs without growing.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Document the e2e task and how to run it against PG

**Depends on:** Task 3 (suite passes on both backends)

The current `.mise/tasks/e2e` is a single-line `go test` invocation. The constellation pattern (Sharkfin, Combine) keeps the mise task as a single command and selects the backend purely via env vars in CI. We follow that pattern — no `--backend` flag is needed because `HIVE_DB` is the existing knob and the daemon already dispatches on it. The change here is purely a description update so `mise tasks` shows how to point it at PG.

**Files:**
- Modify: `.mise/tasks/e2e`

**Step 1: Update the description and add a comment block explaining the env-var contract**

Replace the contents of `.mise/tasks/e2e` with:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#MISE description="Run end-to-end tests (backend selected by HIVE_DB env var)"
#MISE depends=["build:dev"]
#MISE dir="tests/e2e"
set -euo pipefail

# Backend selection
# -----------------
# Default: SQLite (per-test tempfile). No env var needed.
# Postgres:
#   HIVE_DB="postgres://hive:hive@localhost:5432/hive_test?sslmode=disable" \
#     mise run e2e
#
# The harness reads HIVE_DB once per test, resets the schema if it is a
# postgres:// DSN, and forwards it via the daemon's --db flag (which
# auto-dispatches in internal/infra/open.go).
#
# Per-backend CI runs are split in .github/workflows/ci.yaml:
#   - "ci"            -> SQLite via mise run ci
#   - "e2e-postgres"  -> mise run e2e with HIVE_DB set to a service container
#
# Both backends MUST stay green. mise run ci does NOT chain both —
# that is CI's job (parallel jobs), not the local task runner's.

go test -v -race -count=1 -timeout 600s ./...
```

Note: the timeout grows from `120s` → `600s` to match Combine's e2e budget; PG migrations + schema resets cost real time per test (and CI runners are slower than dev boxes).

**Step 2: Verify mise still parses and runs the task**

Run: `mise tasks | grep e2e`
Expected: shows the new description.

Run: `mise run e2e`
Expected: PASS (SQLite path; same as Task 3 step 6).

**Step 3: Commit**

```bash
git add .mise/tasks/e2e
git commit -m "$(cat <<'EOF'
chore(e2e): document HIVE_DB backend selection in mise task

Update the e2e task description and add a comment block explaining
the SQLite-default / HIVE_DB-postgres contract that the harness
already implements. Bump the test timeout from 120s to 600s so PG
runs (schema resets per test, goose migrations on every daemon boot)
have headroom on CI runners.

mise run e2e remains a single command; CI splits the two backends
into parallel jobs rather than chaining them locally.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Add ci.yaml with parallel e2e-postgres job

**Depends on:** Task 4 (e2e task documented)

Hive currently has only `release.yml`, which runs on push-to-master and is gated by `Work-Fort/github-tag-action`. There is no PR / push CI that runs lint + tests + e2e. Following Sharkfin's pattern (which has both `ci.yaml` and `release.yaml`) we add a new `ci.yaml` covering the SQLite default and the PG backend in parallel. `release.yml` stays unchanged.

**Files:**
- Create: `.github/workflows/ci.yaml`

**Step 1: Create the workflow**

```yaml
# SPDX-License-Identifier: GPL-3.0-or-later
name: CI

on:
  push:
    branches: [master]
  pull_request:

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v3
      - run: mise run ci

  e2e-postgres:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_DB: hive_test
          POSTGRES_USER: hive
          POSTGRES_PASSWORD: hive
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v3
      - run: mise run build:dev
      - run: mise run e2e
        env:
          HIVE_DB: postgres://hive:hive@localhost:5432/hive_test?sslmode=disable
```

**Step 2: Lint the workflow locally**

Run: `mise run lint` (golangci-lint) and verify nothing complains about the workflow.
Expected: PASS (golangci-lint does not parse YAML, but this catches Go regressions from the harness changes).

If `actionlint` is available locally: `actionlint .github/workflows/ci.yaml`. Optional — CI itself is the source of truth.

**Step 3: Commit**

```bash
git add .github/workflows/ci.yaml
git commit -m "$(cat <<'EOF'
ci: add ci.yaml with parallel e2e-postgres job

Hive previously had only release.yml, which runs after a tag and does
not exercise the e2e suite. Add a CI workflow that runs on every push
to master and on PRs:

  - ci: mise run ci (lint + unit + SQLite e2e)
  - e2e-postgres: mise run e2e with HIVE_DB pointed at a Postgres 17
    service container

This matches the constellation pattern (Sharkfin, Combine). Both
backends MUST stay green; the parallel layout means a regression on
either backend blocks merge without forcing local devs to spin up PG
for the default flow.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Push and confirm both jobs go green

**Depends on:** Task 5

**Step 1: Push the branch**

Run: `git push origin <branch>`

**Step 2: Watch the workflow runs**

Run: `gh run watch --exit-status`
Expected: both `ci` and `e2e-postgres` complete successfully.

If `e2e-postgres` is hung past the e2e timeout, cancel and treat it as a defect to fix — never as "flaky" or "pre-existing" (per `feedback_no_test_failures.md`, tightened 2026-04-19).

**Step 3: No commit. Just verification.**

---

### Task 7: No-silent-skips audit

**Depends on:** Task 6 (CI green)

Per the tightened `feedback_no_test_failures.md`, every `t.Skip*` call in `tests/e2e/` must either:

a. Be environment-impossible (e.g. OS-conditional, or "this test only makes sense when backend X is selected" with the inverse case covered by another test), or
b. Be replaced with proper setup so the test runs.

**Files:**
- Audit: `tests/e2e/**/*.go`

**Step 1: Enumerate every skip**

Run: `grep -rn "t\.Skip\|t\.SkipNow\|Skipf" tests/e2e/`

After Tasks 1-3 the only matches must be:
- `tests/e2e/pg_helpers_test.go: TestHarness_UsesPostgresWhenHiveDBSet` — skips under SQLite. Justified: asserts a PG-only invariant; the SQLite path is exercised by every other test in the suite under the default backend.
- `tests/e2e/pg_helpers_test.go: TestAltDB_PostgresReturnsSibling` — skips under SQLite. Justified: asserts a PG-only DSN shape; the SQLite branch of `AltDB` is exercised by `TestExportImportRoundTrip` and `TestExportImportDryRun` when run under SQLite.

**Step 2: For each skip, document the inverse**

Add a one-line comment above each `t.Skip*` pointing at the test that exercises the other branch, so a future reviewer can verify coverage at a glance. Example:

```go
// SQLite branch of AltDB is exercised by TestExportImportRoundTrip
// when HIVE_DB is unset.
if !usingPostgres() {
    t.Skipf("HIVE_DB not set to a postgres:// DSN; ...")
}
```

**Step 3: Run both backends one more time**

Run: `mise run e2e`
Expected: PASS, with the two PG-only tests printing SKIP.

Run:
```
HIVE_DB="postgres://hive:hive@localhost:5432/hive_test?sslmode=disable" mise run e2e
```
Expected: PASS, with no skips printed (all tests run on PG).

**Step 4: Commit**

```bash
git add tests/e2e/pg_helpers_test.go
git commit -m "$(cat <<'EOF'
docs(e2e): annotate backend-conditional skips with their inverse

Both PG-only tests in pg_helpers_test.go skip when HIVE_DB is unset.
Add a one-line comment above each Skipf pointing at the test that
exercises the other backend, so a reviewer can confirm there is no
silent gap in coverage.

Per feedback_no_test_failures.md (tightened 2026-04-19), every skip
must be either environment-impossible or replaced with proper setup.
These two skips are environment-impossible (PG-DSN-only invariants)
and the inverse paths are covered by the existing suite under the
SQLite default.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Verification checklist

- [ ] `tests/e2e/go.mod` lists `github.com/jackc/pgx/v5` in `require`.
- [ ] `tests/e2e/harness_test.go` no longer hardcodes `dbPath := filepath.Join(stateDir, "hive.db")`. The DSN comes from `HIVE_DB` with the SQLite tempfile as the default.
- [ ] `tests/e2e/pg_helpers_test.go` defines `usingPostgres`, `resetPostgres`, and `AltDB(t)`.
- [ ] `tests/e2e/export_import_test.go` calls `AltDB(t)` for both fresh-DB sites.
- [ ] `mise run e2e` (no env) — every test that runs PASSes; only the two PG-only tests SKIP, each with a comment naming the inverse coverage.
- [ ] `HIVE_DB=postgres://... mise run e2e` — every test PASSes, no skips printed.
- [ ] `.mise/tasks/e2e` description mentions `HIVE_DB`; timeout is 600s.
- [ ] `.github/workflows/ci.yaml` exists with `ci` and `e2e-postgres` parallel jobs.
- [ ] CI run triggered by the push: both jobs green.
- [ ] `grep -rn "t\.Skip" tests/e2e/` returns at most the two annotated skips and nothing else.
- [ ] `internal/infra/postgres/migrations/` was not modified (no schema regressions sneaked in).
- [ ] `cmd/daemon/daemon.go` was not modified (the daemon side already supports both backends; this plan is harness-only).

## Decisions recorded

1. **Postgres version pinned to `postgres:17` in CI.** Matches Sharkfin and Combine. Local dev points at the host's peer-trust PG 18 directly; the CI/host divergence (CI on 17, host on 18) is tracked as a cross-cutting workstream item at `WorkFort/TOOLING-BASELINE-REMAINING-WORK.md` and is explicitly out of scope here.
2. **No `mise run pg:start` helper.** Skipped — no constellation peer ships one. Local dev points at the host PG; CI uses the service container. A start helper would cross-cut repos and is not this plan's scope.
3. **`hive_test_b` cleanup mirrors Combine's `combine_e2e_b` pattern.** `AltDB(t)` runs the same `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO PUBLIC` reset on the sibling DB that the harness runs on the primary, both before the test (in `AltDB`) and after (in the registered `t.Cleanup`). The two lifecycles are symmetric — no special orchestration is needed and the sibling DB does not need to be dropped between runs.
