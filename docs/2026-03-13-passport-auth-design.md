# Passport Auth Integration Design

**Goal:** Replace Hive's custom API key middleware with Passport's shared auth SDK, providing JWT and API key validation on all endpoints (REST and MCP) via `github.com/Work-Fort/Passport/go/service-auth`.

**Passport server:** `http://passport.nexus:3000` (configurable via `--passport-url` / `HIVE_PASSPORT_URL`)

---

## 1. Auth Middleware Replacement

Remove `internal/daemon/middleware.go` (custom `APIKeyAuth` function). Replace with Passport's `auth.NewFromValidators()`.

On daemon startup:

```go
import (
    "github.com/Work-Fort/Passport/go/service-auth"
    "github.com/Work-Fort/Passport/go/service-auth/jwt"
    "github.com/Work-Fort/Passport/go/service-auth/apikey"
)

opts := auth.DefaultOptions(passportURL)
jwtV, err := jwt.New(ctx, opts.JWKSURL, opts.JWKSRefreshInterval)
akV := apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL)
passportAuth := auth.NewFromValidators(jwtV, akV)
```

The middleware wraps the entire mux. A thin path-skip wrapper exempts public routes (`/health`, `/openapi`) before delegating to Passport. All other paths (`/v1/*`, `/mcp`) require a valid `Authorization: Bearer <token>` header.

On success: `auth.Identity` is stored in request context with fields `ID` (UUID), `Username`, `Name`, `DisplayName`, `Type` ("user", "agent", "service").

On failure: HTTP 401 Unauthorized.

### ServerConfig changes

```go
// Before
type ServerConfig struct {
    APIKey string
    // ...
}

// After
type ServerConfig struct {
    PassportURL string
    // ...
}
```

### Public path exemption

```go
func publicPathSkip(passport http.Handler, unprotected http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.URL.Path == "/v1/health",
             r.URL.Path == "/openapi",
             strings.HasPrefix(r.URL.Path, "/docs"):
            unprotected.ServeHTTP(w, r)
        default:
            passport.ServeHTTP(w, r)
        }
    })
}
```

Note: The current `APIKeyAuth` middleware only protects `/v1/*` paths, leaving `/mcp` unauthenticated (relying solely on the `X-Agent-Id` header). The new wrapper is a deliberate security improvement — `/mcp` now requires a valid Passport Bearer token.

---

## 2. Identity Resolution

### REST path

Huma handlers call `auth.IdentityFromContext(ctx)` to get the caller's identity. Handlers needing agent-specific data (team, roles, permissions) look up the agent by Passport UUID in the store.

### MCP path

The `X-Agent-Id` header is removed. The `httpContextFunc` passed to mcp-go's `WithHTTPContextFunc` extracts the Passport `auth.Identity` already in context (placed there by the middleware). `resolveAgent()` changes from header-based lookup to `auth.IdentityFromContext(ctx)` → store lookup by `Identity.ID`.

`toolFilter()` and per-tool `requireAgent()` calls stay structurally identical — they just get the agent ID from `Identity.ID` instead of the header.

### session.go changes

```go
// Before
func httpContextFunc() func(ctx context.Context, r *http.Request) context.Context {
    return func(ctx context.Context, r *http.Request) context.Context {
        agentID := r.Header.Get("X-Agent-Id")
        if agentID != "" {
            ctx = contextWithAgentID(ctx, agentID)
        }
        return ctx
    }
}

// After
func httpContextFunc() func(ctx context.Context, r *http.Request) context.Context {
    return func(ctx context.Context, r *http.Request) context.Context {
        id, ok := auth.IdentityFromContext(ctx)
        if ok {
            ctx = contextWithAgentID(ctx, id.ID)
        }
        return ctx
    }
}
```

`resolveAgent()` error message changes from `"missing X-Agent-Id header"` to `"missing agent identity"`.

### MCP bridge changes

The `mcp-bridge` command drops `--agent-id` and adds `--passport-token`:

```
--passport-token  Passport JWT or API key (Viper: passport-token, env: HIVE_PASSPORT_TOKEN)
```

It sends `Authorization: Bearer <token>` on every request instead of `X-Agent-Id`. This is MCP spec compliant (MCP requires OAuth 2.1 Bearer tokens on HTTP Streamable transport).

---

## 3. Agent Identity — Passport UUIDs

`domain.Agent.ID` changes from `ag_xxxx` format to Passport UUIDs. The agent table becomes a local extension of Passport identity — Hive stores team assignment, roles, and permissions keyed on the Passport UUID.

`NewID("ag")` calls in agent creation code are removed. Agent records are created with the Passport UUID as their primary key.

### Agent creation flow

`POST /v1/agents` changes to accept an `id` field (the Passport UUID) instead of auto-generating one:

```go
// Before
agent := &domain.Agent{ID: NewID("ag"), Name: input.Body.Name, TeamID: input.Body.TeamID}

// After
agent := &domain.Agent{ID: input.Body.ID, Name: input.Body.Name, TeamID: input.Body.TeamID}
```

The `CreateAgentInput.Body` adds a required `ID string` field (Passport UUID). The caller provisions the identity in Passport first, then registers the agent in Hive with that UUID. This keeps Passport as the source of truth for identity while Hive stores the team/role/permission extensions.

### REST handler identity note

`rest_huma.go` changes are limited to the agent creation endpoint above. The REST handlers are admin endpoints that take explicit IDs in the URL — they do not need `auth.IdentityFromContext` for their current logic.

Other entity IDs (`tm_`, `rl_`, `doc_`, `tk_`) remain unchanged — they are Hive-internal.

All foreign keys referencing agent IDs follow the UUID format:
- `tasks.agent_id`
- `documents.agent_id`
- `agent_roles.agent_id`
- `agent_permissions.agent_id`

---

## 4. Client Library

`client.Client` replaces `apiKey string` with `token string`. Constructor becomes `New(baseURL, token string)`. The `do()` method continues setting `Authorization: Bearer <token>` — same header, different token source.

---

## 5. CLI Commands

### daemon

- Remove: `--api-key`
- Add: `--passport-url` (default `http://passport.nexus:3000`, Viper key `passport-url`, env `HIVE_PASSPORT_URL`)

### mcp-bridge

- Remove: `--agent-id`
- Add: `--passport-token` (Viper key `passport-token`, env `HIVE_PASSPORT_TOKEN`)
- Keep: `--host`, `--port`

### export / import

- Remove: `--api-key`
- Add: `--passport-token` (Viper key `passport-token`, env `HIVE_PASSPORT_TOKEN`)
- When using `--db` mode (direct SQLite), no token needed.

### Config defaults (internal/config/config.go)

- Remove: `api-key`
- Add: `passport-url` (default `http://passport.nexus:3000`)
- Add: `passport-token` (default `""`)

All flags follow Viper conventions: flag → env var → config file → default.

---

## 6. Database Migration

Hive is not yet deployed. All existing migrations are flattened into a single initial migration reflecting the final schema.

Key change: agent IDs are UUIDs instead of `ag_xxxx` prefixed strings. The column type (`TEXT`) stays the same — only the content format changes. All foreign keys referencing agents naturally follow.

---

## 7. Test Changes

### Unit tests — middleware

Delete `internal/daemon/middleware_test.go`. The custom middleware is gone. Add tests for the public-path-skip wrapper only.

### Unit tests — MCP tools

`setupTestEnv` creates agents with UUID strings instead of `NewID("ag")`. Context injection changes from `contextWithAgentID` using a raw string to injecting a Passport `auth.Identity` into context. Tool invocation and result checking stay the same.

### E2E tests — harness

The harness connects to the real Passport instance at `passport.nexus:3000`. It passes `--passport-url http://passport.nexus:3000` to the daemon process. The client is created with a real Passport API key or JWT token.

`testAPIKey` constant is replaced with a Passport-issued test token.

### E2E tests — permissions

Wrong-key and no-key tests adapt to use invalid/missing Bearer tokens. Assertions change from `ErrForbidden` to `ErrUnauthorized` (Passport returns 401 for all auth failures).

### E2E tests — export/import

Replace `--api-key` with `--passport-token` on CLI invocations.

---

## 8. Deployment

### Systemd service (deploy/hive.service)

Environment file changes from `HIVE_API_KEY` to `HIVE_PASSPORT_URL`. The daemon validates incoming tokens — it doesn't need a token itself.

### Install task (.mise/tasks/install/local)

Update echo to reference Passport URL configuration instead of API key setup.

### MCP config (.mcp.json)

Bridge command changes to `mcp-bridge --passport-token <token>`. Token comes from environment (`HIVE_PASSPORT_TOKEN`).

### Design doc (docs/hive-design.md)

Auth section updates from "single shared API key" to "Passport-issued JWT or API key, validated via JWKS and API key verification endpoints."

---

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/daemon/middleware.go` | Delete | Custom APIKeyAuth removed |
| `internal/daemon/middleware_test.go` | Delete | Replaced by skip-wrapper tests |
| `internal/daemon/server.go` | Modify | Passport middleware wiring, ServerConfig changes |
| `internal/daemon/session.go` | Modify | Identity from Passport context, remove X-Agent-Id |
| `internal/daemon/mcp_server.go` | Modify | Remove X-Agent-Id references |
| `internal/daemon/rest_huma.go` | Modify | Agent creation accepts Passport UUID instead of NewID("ag") |
| `internal/daemon/authz.go` | Modify | Agent ID is now UUID format |
| `internal/daemon/mcp_tools.go` | Modify | Agent resolution from Passport identity |
| `internal/daemon/mcp_tools_test.go` | Modify | UUID agents, Passport identity in context |
| `cmd/daemon/daemon.go` | Modify | Replace --api-key with --passport-url |
| `cmd/mcpbridge/mcp_bridge.go` | Modify | Replace --agent-id with --passport-token |
| `cmd/export/export.go` | Modify | Replace --api-key with --passport-token |
| `cmd/importcmd/importcmd.go` | Modify | Replace --api-key with --passport-token |
| `client/client.go` | Modify | apiKey → token |
| `internal/config/config.go` | Modify | New config defaults |
| `internal/infra/sqlite/migrations.go` | Modify | Flatten to single migration with UUID agent IDs |
| `tests/e2e/harness_test.go` | Modify | Passport URL, real token |
| `tests/e2e/permissions_test.go` | Modify | 401 assertions |
| `tests/e2e/export_import_test.go` | Modify | --passport-token flag |
| `deploy/hive.service` | Modify | HIVE_PASSPORT_URL |
| `.mise/tasks/install/local` | Modify | Update echo message |
| `.mcp.json` | Modify | mcp-bridge with --passport-token |
| `docs/hive-design.md` | Modify | Auth section |
| `go.mod` | Modify | Add Passport SDK dependency |

---

## Dependencies

- `github.com/Work-Fort/Passport/go/service-auth` (published on GitHub with tags)
- Transitive: `github.com/lestrrat-go/jwx/v2` (JWT validation)
- Passport server running at `passport.nexus:3000` (for E2E tests and production)

---

## Out of Scope

- OAuth 2.1 dynamic client registration (Passport handles this)
- User management UI (Passport owns identity CRUD)
- Authorization changes (Hive's AuthzService and RBAC stay as-is)
- Token issuance (Passport handles this — Hive only validates)
