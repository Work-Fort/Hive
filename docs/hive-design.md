# Hive Design

Agent provisioning microservice that dynamically serves role definitions, memory
documents, and task lists to AI agents. Agents connect via MCP to receive their
composed roles and manage their own memory and tasks at runtime.

## Architecture

Go binary, hexagonal architecture matching Sharkfin and Nexus conventions.
Single binary serves as both the HTTP daemon (REST + MCP) and a stdio MCP
bridge for Claude Code integration.

### Binary Structure

```
hive daemon  [--bind 127.0.0.1] [--port 17000]
hive mcp-bridge [--passport-token TOKEN] [--host 127.0.0.1] [--port 17000]
hive version
```

Global flags: `--log-level disabled|debug|info|warn|error`

Config hierarchy: CLI flags > env vars (`HIVE_*`) > config file
(`$XDG_CONFIG_HOME/hive/config.yaml`) > defaults.

### Route Layout

```
/v1/*    REST API (JSON, Passport Bearer auth)
/mcp     MCP server (JSON-RPC 2.0, Passport Bearer auth)
/v1/health  Health check
```

## Data Model

### ID Strategy

All entity IDs are randomly generated text IDs (e.g., `hv_01abc...`). Format
follows the pattern used in Nexus: short prefix + random component. IDs are
generated at the application layer, not auto-incremented by the database.

### Teams

```sql
teams (id TEXT PRIMARY KEY, name TEXT UNIQUE NOT NULL, created_at, updated_at)
```

Organizational unit. Agents belong to exactly one team.

### Roles

```sql
roles (id, name, parent_id NULLABLE REFERENCES roles, created_at, updated_at)
```

Reusable capability definitions with optional single-parent inheritance.
Examples: `developer`, `frontend-developer` (parent: `developer`),
`security-reviewer`.

### Agents

```sql
agents (id, name, team_id REFERENCES teams, created_at, updated_at)
```

A provisioned identity. Belongs to one team.

### Agent-Role Assignments

```sql
agent_roles (
  agent_id REFERENCES agents,
  role_id REFERENCES roles,
  priority INTEGER NOT NULL,
  PRIMARY KEY (agent_id, role_id),
  UNIQUE (agent_id, priority)
)
```

Many-to-many with explicit ordering. Lower priority number = higher precedence.
Each agent-role pair is unique, and no two roles can share the same priority
for a given agent.

### Documents

```sql
documents (
  id, kind TEXT, title TEXT, content TEXT,
  role_id NULLABLE REFERENCES roles,
  agent_id NULLABLE REFERENCES agents,
  created_at, updated_at
)
```

- `kind`: `role` (attached to a role via `role_id`) or `memory` (attached to an
  agent via `agent_id`)
- `content` is markdown text
- CHECK constraint: exactly one of `role_id` or `agent_id` must be set
  (`(role_id IS NULL) != (agent_id IS NULL)`)

### Tasks

```sql
tasks (
  id, team_id REFERENCES teams, agent_id NULLABLE REFERENCES agents,
  title TEXT, description TEXT, status TEXT,
  created_at, updated_at
)
```

- `status`: `pending`, `in_progress`, `completed`
- Team-scoped visibility — agents see their team's tasks
- `agent_id` nullable — tasks can be unassigned

### RBAC

```sql
permissions (id, name TEXT UNIQUE)
```

Seeded permissions:

| Permission | Description |
|---|---|
| `role:read` | Read role documents |
| `role:write` | Edit role documents |
| `memory:read` | Read memory documents |
| `memory:write` | Edit memory documents |
| `task:read` | Read tasks |
| `task:write` | Create/update tasks |
| `agent:manage` | Assign roles to agents |
| `team:manage` | Manage target teams |

```sql
agent_permissions (
  agent_id REFERENCES agents,
  permission_id REFERENCES permissions,
  scope_team_id NULLABLE REFERENCES teams,
  PRIMARY KEY (agent_id, permission_id, scope_team_id)
)
```

- `scope_team_id = NULL` means global scope
- `scope_team_id` set means permission applies only to that team
- All permission checks are per-agent, per-scope — no blanket team-level grants

## Provisioning Resolution

The core operation: when an agent calls `get_provisioning`, Hive composes all
role documents and memory into a hierarchical response.

### Algorithm

1. Resolve agent's role assignments ordered by priority
2. For each role, walk the inheritance chain to root (recursive CTE)
3. Collect documents per role, grouped hierarchically
4. Append agent's own memory documents
5. Return structured response

### Depth Limit

Configurable via `max_role_depth` (config file) or `HIVE_MAX_ROLE_DEPTH` (env).
Default: 10.

Enforced in two places:

- **On write** — role creation/update rejects if adding a parent would exceed
  the limit
- **On read** — recursive CTE includes a depth counter, stops at the limit as a
  safety net

### Response Structure

```json
{
  "roles": [
    {
      "priority": 1,
      "chain": [
        {
          "role": "frontend-specialist",
          "documents": [{"title": "Frontend Standards", "content": "..."}]
        },
        {
          "role": "developer",
          "documents": [{"title": "Dev Practices", "content": "..."}]
        }
      ]
    },
    {
      "priority": 2,
      "chain": [
        {
          "role": "security-reviewer",
          "documents": [{"title": "Security Checklist", "content": "..."}]
        }
      ]
    }
  ],
  "memory": [
    {"title": "Learned Patterns", "content": "..."},
    {"title": "Project Notes", "content": "..."}
  ]
}
```

### Rules

- **Priority ordering** — lower number = higher precedence, resolved first
- **Chain order is leaf-to-root** — the `chain` array starts at the leaf role
  (index 0) and walks toward the root ancestor (last element). Most specific
  documents appear first.
- **Cycle detection** — enforced on role creation/update, not at query time
- **No deduplication** — if two role chains share an ancestor, its documents
  appear in both chains

## REST API

All endpoints under `/v1`. JSON request/response bodies. Authenticated via
Passport-issued JWT or API key (`Authorization: Bearer <token>`), validated
against the Passport service at the configured `HIVE_PASSPORT_URL`.

### Pagination

List endpoints return all results in v1. No pagination. This is acceptable for
the expected scale (tens of teams, hundreds of agents/roles, thousands of
documents/tasks). Pagination will be added if needed.

### Deletion Behavior

All deletes reject when dependencies exist (no cascading):

- **Delete team** → rejected if team has agents or tasks
- **Delete role** → rejected if role has child roles or agent assignments
- **Delete agent** → rejected if agent has tasks assigned; cascades
  agent_roles and agent_permissions (these are pure metadata)
- **Delete document** → always allowed (orphan cleanup)

### Teams

- `GET /v1/teams` — list teams
- `POST /v1/teams` — create team
- `GET /v1/teams/:id` — get team
- `PUT /v1/teams/:id` — update team
- `DELETE /v1/teams/:id` — delete team

### Roles

- `GET /v1/roles` — list roles (optional `?parent_id=` filter)
- `POST /v1/roles` — create role (with optional `parent_id`)
- `GET /v1/roles/:id` — get role with documents
- `PUT /v1/roles/:id` — update role
- `DELETE /v1/roles/:id` — delete role

### Documents

- `GET /v1/roles/:id/documents` — list role's documents
- `POST /v1/roles/:id/documents` — add document to role
- `GET /v1/documents/:id` — get document
- `PUT /v1/documents/:id` — update document content
- `DELETE /v1/documents/:id` — delete document
- `GET /v1/agents/:id/memory` — list agent's memory docs
- `POST /v1/agents/:id/memory` — add memory doc to agent

### Agents

- `GET /v1/agents` — list agents (optional `?team_id=` filter)
- `POST /v1/agents` — create agent
- `GET /v1/agents/:id` — get agent with role assignments
- `PUT /v1/agents/:id` — update agent
- `DELETE /v1/agents/:id` — delete agent
- `PUT /v1/agents/:id/roles` — set role assignments (with priorities)

### Tasks

- `GET /v1/teams/:id/tasks` — list team's tasks
- `POST /v1/tasks` — create task
- `GET /v1/tasks/:id` — get task
- `PUT /v1/tasks/:id` — update task
- `DELETE /v1/tasks/:id` — delete task

### Permissions

- `GET /v1/agents/:id/permissions` — list agent's permissions
- `PUT /v1/agents/:id/permissions` — set agent's permissions

### Health

- `GET /v1/health` — health check (200 healthy, 218 degraded, 503 unhealthy)

## MCP Tools

Tools exposed to agents via the MCP bridge. The tool list is dynamically
filtered per-session based on the agent's resolved permissions — agents only
see tools they have access to.

Agent identity comes from the Passport Bearer token on the mcp-bridge's HTTP
requests, resolved to an agent record by Passport UUID.

| Tool | Description | Permission |
|---|---|---|
| `get_provisioning` | Get composed role docs + memory (hierarchical) | `role:read` + `memory:read` |
| `list_memory` | List own memory docs | `memory:read` |
| `get_memory` | Get a specific memory doc | `memory:read` |
| `create_memory` | Create a new memory doc | `memory:write` |
| `update_memory` | Update a memory doc | `memory:write` |
| `delete_memory` | Delete a memory doc | `memory:write` |
| `list_tasks` | List team tasks | `task:read` |
| `get_task` | Get a specific task | `task:read` |
| `create_task` | Create a task | `task:write` |
| `update_task` | Update task status/details | `task:write` |
| `delete_task` | Delete a task | `task:write` |

Cross-team management tools (for agents with `team:manage` or `role:write`
scoped to other teams) are deferred to a future design iteration. v1 focuses
on self-service provisioning and task management.

## Authentication

### REST API

Authenticated via Passport-issued JWT or API key. The daemon validates tokens
against the Passport service (JWKS for JWTs, verification endpoint for API
keys). Passport is the source of truth for identity.

### MCP

The mcp-bridge speaks stdio (JSON-RPC) to Claude Code and forwards requests to
the daemon over HTTP (`POST /mcp`) with `Authorization: Bearer <token>`.
The daemon validates the token via Passport, resolves the caller's identity,
and looks up the agent record by Passport UUID. No `X-Agent-Id` header.

### MCP Session Lifecycle

One mcp-bridge process = one session. The session starts when the bridge
connects and ends when the bridge process exits. No timeout, no persistent
state beyond the agent ID. The MCP library (`mcp-go`) manages session IDs
via the `Mcp-Session-Id` HTTP header automatically.

## Health Service

Global health registry that tracks warnings and errors. Checked at daemon boot
and periodically thereafter.

### Boot-Time Checks

- **Role depth audit** — query all role chains, warn if any exceed
  `max_role_depth`
- **Database connectivity** — verify store is reachable

### Health Statuses

- `healthy` — all checks pass
- `degraded` — warnings present (e.g., deep role chains)
- `unhealthy` — critical failures (e.g., database unreachable)

## Project Layout

```
hive/
├── main.go
├── go.mod                        # github.com/Work-Fort/Hive
├── cmd/
│   ├── root.go
│   ├── daemon/
│   │   └── daemon.go
│   └── mcpbridge/
│       └── mcp_bridge.go
├── client/                       # PUBLIC Go HTTP client (zero internal imports)
│   ├── client.go                 # Client struct, New(), Option funcs
│   ├── errors.go                 # APIError, sentinel errors
│   ├── teams.go                  # Team operations
│   ├── roles.go                  # Role operations
│   ├── agents.go                 # Agent operations
│   ├── documents.go              # Document operations
│   ├── tasks.go                  # Task operations
│   └── permissions.go            # Permission operations
├── internal/
│   ├── config/                   # Viper + XDG paths
│   ├── domain/
│   │   ├── types.go              # Team, Role, Agent, Document, Task, Permission
│   │   └── ports.go              # Store interfaces (hexagonal ports)
│   ├── daemon/
│   │   ├── server.go             # HTTP server setup, REST + MCP routing
│   │   ├── rest.go               # REST API handlers
│   │   ├── mcp_server.go         # MCP tool registration + session-aware filtering
│   │   ├── mcp_tools.go          # Tool definitions
│   │   ├── provisioning.go       # Role composition + document resolution logic
│   │   ├── session.go            # Agent session management
│   │   └── health.go             # Health service + checks
│   └── infra/
│       ├── open.go               # DSN → Store router
│       ├── sqlite/
│       │   ├── store.go
│       │   ├── teams.go, roles.go, agents.go
│       │   ├── documents.go, tasks.go, permissions.go
│       │   └── migrations/
│       └── postgres/
│           └── [mirrors sqlite]
├── tests/e2e/                    # Separate Go module, black-box tests
├── docs/
└── mise.toml
```

### Package Visibility

- `client/` — public, importable as `github.com/Work-Fort/Hive/client`. Defines
  its own response types mirroring API JSON shapes. Zero dependency on `internal/`.
- `internal/` — private, Go compiler enforces no external imports. Contains
  domain logic, infrastructure adapters, daemon, and config.
- `cmd/` — CLI wiring only, delegates to `internal/`.

## OpenAPI

The REST API is documented with an OpenAPI 3.1 spec generated at runtime by
[Huma v2](https://huma.rocks/) (`github.com/danielgtaylor/huma/v2`). This
follows the same pattern used in Nexus.

### Library Choice

Huma v2 with the `humago` adapter for Go's stdlib `net/http`. Huma generates
the OpenAPI spec from typed Go structs at startup — no code generation step,
no YAML files to maintain. Input structs use `doc:` tags for field descriptions,
`path:`/`query:` tags for parameter binding.

### Endpoints

- `GET /openapi` — OpenAPI 3.1 JSON spec (served by Huma automatically)
- `GET /docs` — Interactive Swagger UI documentation (served by Huma)

Both endpoints are unauthenticated — they sit outside the Passport auth middleware.

### Handler Pattern

All REST handlers migrate from `func(w http.ResponseWriter, r *http.Request)`
to Huma's typed handler pattern:

```go
huma.Register(api, huma.Operation{
    OperationID: "create-team",
    Method:      http.MethodPost,
    Path:        "/v1/teams",
    Summary:     "Create a team",
    Tags:        []string{"Teams"},
}, func(ctx context.Context, input *CreateTeamInput) (*TeamOutput, error) {
    // ...
})
```

### Exceptions

Two route groups stay as raw `HandleFunc` registrations on the mux:

- **Health** (`GET /v1/health`) — returns conditional status codes
  (200/218/503) from a single handler, which Huma operations don't support.
- **MCP** (`/mcp`) — JSON-RPC 2.0, not REST. Handled by mcp-go.

### Error Mapping

Domain errors map to Huma errors via `huma.NewError()`, replacing the manual
`writeError` helper. Same status code mapping as before (404, 409, 422, 500).

## Deployment

Systemd user service:

- Binary: `~/.local/bin/hive`
- Service: `~/.config/systemd/user/hive.service`
- Database: `~/.local/state/hive/hive.db` (SQLite default)
- Config: `~/.config/hive/config.yaml`
- Logs: `$XDG_STATE_HOME/hive/debug.log`
