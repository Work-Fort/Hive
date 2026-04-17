# Bug: SQLite Missing SetMaxOpenConns(1)

## Problem

`internal/infra/sqlite/store.go` — no `db.SetMaxOpenConns(1)` call after
opening the database. PRAGMAs are set via DSN query parameters, but the
connection pool size is unconstrained. SQLite requires single-writer
serialization. Without this constraint, concurrent requests will cause
`SQLITE_BUSY` errors.

## Fix

Add after `sql.Open`:

```go
db.SetMaxOpenConns(1)
```

See `codex/src/architecture/go-service-patterns.md` for the standard pattern.

## Severity

Medium — causes intermittent failures under concurrent load.
