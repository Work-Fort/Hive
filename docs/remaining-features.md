# Remaining Features

Tracking document for features needed before Hive is production-ready as the
agent provisioning service for WorkFort teams.

## ~~1. Project Skeleton~~ DONE

[Design](hive-design.md) · [Plan](plans/001-project-skeleton.md)

Go module, Cobra CLI, Viper config, XDG paths, logging, mise tasks. Daemon
binds to `--bind`/`--port` and serves an empty `/v1/health` endpoint. MCP bridge
connects to daemon with `--agent-id`/`--host`/`--port`. Builds a single binary.

## ~~2. Domain Model + SQLite Store~~ DONE

[Design](hive-design.md) · [Plan](plans/002-domain-sqlite.md)

Domain types and port interfaces in `internal/domain/`. SQLite store
implementing all ports with goose migrations. Tables: teams, roles, agents,
agent_roles, documents, tasks, permissions, agent_permissions. Recursive CTE
for role chain queries verified with EXPLAIN QUERY PLAN (indexed lookups).

## ~~3. REST API~~ DONE

[Design](hive-design.md) · [Plan](plans/003-rest-api.md)

Full CRUD endpoints under `/v1` for teams, roles, documents, agents, tasks,
and permissions. API key authentication. JSON request/response bodies.

## ~~4. Provisioning Resolution~~ DONE

[Design](hive-design.md) · [Plan](plans/004-provisioning.md)

Role composition engine: recursive CTE walks inheritance chains, collects
documents hierarchically, respects priority ordering and configurable depth
limit. Cycle detection on write. Depth audit on boot.

## ~~5. MCP Server~~ DONE

[Design](hive-design.md) · [Plan](plans/005-mcp-server.md)

MCP tool registration on `POST /mcp`. Session-aware tool filtering based on
agent permissions — agents only see tools they can use. Tools: get_provisioning,
memory CRUD, task CRUD.

## ~~6. RBAC~~ DONE

[Design](hive-design.md) · [Plan](plans/006-rbac.md)

Permission enforcement across REST and MCP. Per-agent grants with optional
team scope. Permission checks in REST middleware and MCP auth middleware.
Seeded permission set on first run.

## ~~7. Health Service~~ DONE

[Design](hive-design.md) · [Plan](plans/007-health-service.md)

Global health registry with boot-time and periodic checks. Role depth audit,
database connectivity. Health endpoint returns healthy/degraded/unhealthy.

## ~~8. PostgreSQL Store~~ DONE

[Design](hive-design.md) · [Plan](plans/008-postgres.md)

PostgreSQL store mirroring SQLite implementation. Same port interfaces, pgx
driver, own goose migrations. DSN auto-detection in `internal/infra/open.go`.

## ~~9. Public Client Package~~ DONE

[Design](hive-design.md) · [Plan](plans/009-client-package.md)

Go HTTP client at `client/` with zero internal imports. Own response types
mirroring API JSON shapes. Methods for all REST endpoints. Sentinel errors
for common HTTP status codes.

## 10. E2E Test Suite

[Design](hive-design.md) · [Plan](plans/010-e2e-tests.md)

Separate Go module in `tests/e2e/` following Nexus/Sharkfin pattern. TestMain
builds binary with `-race`, harness manages daemon lifecycle, temp XDG dirs
per test. HTTP client exercises REST API and MCP bridge.

## 11. Systemd Service

[Plan](plans/011-systemd.md)

Systemd user service file. Binary at `~/.local/bin/hive`, database at
`~/.local/state/hive/hive.db`. Health check integration for readiness.
