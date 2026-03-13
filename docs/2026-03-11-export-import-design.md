# Export/Import Design Spec

## Goal

Enable full-state export of Hive entities to a filesystem directory with YAML front matter, and import from that directory back into Hive — preserving timestamps, references, and content.

**Critical requirement:** Export/import serves as the database migration path between backends (SQLite ↔ Postgres). Timestamps, IDs, and all state must be faithfully preserved to enable lossless migration.

## Scope

All domain entities: teams, roles, permissions, agents (with role assignments and permission grants), documents, and tasks.

## CLI Commands

### `hive export <dir>`

Exports all entities to the target directory, creating it if needed. Overwrites existing files.

**Flags:**
- `--host` / `--port` — daemon address (defaults from viper)
- `--api-key` — auth token (defaults from viper)
- `--db <dsn>` — skip REST API, use direct database access

### `hive import <dir>`

Imports entities from the target directory in dependency order. Fails on name conflicts by default.

**Flags:**
- `--host` / `--port` / `--api-key` — same as export
- `--db <dsn>` — direct database access
- `--upsert` — update existing entities instead of failing on conflicts
- `--dry-run` — validate and report what would happen without making changes

### Data Source Resolution

1. If `--db` is provided: open the database directly using `domain.Store`
2. Otherwise: use `client.Client` to talk to the daemon over REST

Both paths go through a common `DataSource` interface so export/import logic is backend-agnostic.

**REST limitations:** The current REST API does not support setting timestamps on creation. REST-based import generates fresh timestamps. For lossless migration between database backends, use `--db` on both ends:

```bash
# Migration: SQLite → Postgres
hive export ./backup --db sqlite:///old/hive.db
hive import ./backup --db postgres://user:pass@host/hive

# Migration: Postgres → SQLite
hive export ./backup --db postgres://user:pass@host/hive
hive import ./backup --db sqlite:///new/hive.db
```

### Database Migration

Export/import is the canonical way to move between database backends. The `--db` path preserves all state losslessly: UUIDs, timestamps, relationships, and content. The daemon must be stopped before direct database operations.

## File Format

### Folder Layout

```
<dir>/
  teams/
    engineering.yaml
  roles/
    base-role.yaml
    developer.yaml
  permissions/
    read-docs.yaml
    write-docs.yaml
  agents/
    claude-dev.yaml
  documents/
    role-developer--coding-standards.md
    agent-claude-dev--project-notes.md
  tasks/
    engineering--fix-auth-bug.yaml
```

### Filename Convention

Entity name sanitized: lowercase, spaces replaced with hyphens, special characters stripped. Filenames are cosmetic — the authoritative entity name comes from the `name:` field inside the file.

- **Documents:** `{kind}-{owner-name}--{title}.md` — prefixed with `role-` or `agent-` to prevent collisions when a role and agent share a name.
- **Tasks:** `{team}--{title}.yaml` — prefixed with team name since task titles are not globally unique.

### Entity File Formats

**Teams** — `teams/{name}.yaml`:
```yaml
name: engineering
created_at: "2026-03-10T14:30:00Z"
updated_at: "2026-03-11T09:15:00Z"
```

**Roles** — `roles/{name}.yaml`:
```yaml
name: developer
parent: base-role
created_at: "2026-03-10T14:30:00Z"
updated_at: "2026-03-11T09:15:00Z"
```

`parent` is an empty string for root roles.

**Permissions** — `permissions/{name}.yaml`:
```yaml
name: read-docs
```

Permissions are simple name-only entities. Exporting them explicitly ensures a fresh database has the correct permission set before agents reference them.

**Agents** — `agents/{name}.yaml`:
```yaml
name: claude-dev
team: engineering
created_at: "2026-03-10T14:30:00Z"
updated_at: "2026-03-11T09:15:00Z"
roles:
  - role: developer
    priority: 1
  - role: reviewer
    priority: 2
permissions:
  - permission: read-docs
    scope_team: engineering
  - permission: write-docs
```

`scope_team` is omitted for global permissions. The `permission` field references a permission by name; on import, it is resolved to a permission ID. The `scope_team` field references a team by name; on import, it is resolved to a team ID.

**Documents** — `documents/{kind}-{owner}--{title}.md`:
```yaml
---
title: Coding Standards
kind: role
role: developer
created_at: "2026-03-10T14:30:00Z"
updated_at: "2026-03-11T09:15:00Z"
---
The actual markdown content goes here...
```

For agent memory documents, `role` is replaced with `agent`:
```yaml
---
title: Project Notes
kind: memory
agent: claude-dev
created_at: "2026-03-10T14:30:00Z"
updated_at: "2026-03-11T09:15:00Z"
---
Memory content here...
```

**Tasks** — `tasks/{team}--{title}.yaml`:
```yaml
title: Fix auth bug
team: engineering
agent: claude-dev
status: pending
created_at: "2026-03-10T14:30:00Z"
updated_at: "2026-03-11T09:15:00Z"
description: |
  The authentication middleware fails when...
```

`agent` is omitted for unassigned tasks.

## Export Logic

1. Create target directory structure (`teams/`, `roles/`, `permissions/`, `agents/`, `documents/`, `tasks/`)
2. Fetch and write in dependency order:
   - List all teams, write each as `teams/{name}.yaml`
   - List all roles, write each as `roles/{name}.yaml`
   - List all permissions, write each as `permissions/{name}.yaml`
   - List all agents; for each, fetch role assignments and permissions; write as `agents/{name}.yaml`
   - For each role, list documents; for each agent, list memory; write as `documents/{kind}-{owner}--{title}.md`
   - For each team, list tasks; write as `tasks/{team}--{title}.yaml`
3. Print summary: `Exported 3 teams, 5 roles, 4 permissions, 2 agents, 8 documents, 12 tasks`

## Import Logic

1. Scan directory, parse all files, build in-memory manifest
2. Validate all references before creating anything:
   - Role `parent` → must exist in `roles/` directory or already in system
   - Agent `team` → must exist in `teams/` directory or already in system
   - Agent `roles[].role` → must exist in `roles/` directory or already in system
   - Agent `permissions[].permission` → must exist in `permissions/` directory or already in system
   - Agent `permissions[].scope_team` → must exist in `teams/` directory or already in system
   - Document `role` or `agent` → must exist in respective directory or already in system
   - Task `team` → must exist in `teams/` directory or already in system
   - Task `agent` → must exist in `agents/` directory or already in system
3. Create in dependency order:
   - **Teams** — no dependencies
   - **Roles** — may reference parent roles; process roots first, then children (topological sort on the parent forest)
   - **Permissions** — no dependencies (but must exist before agents)
   - **Agents** — reference teams, roles, and permissions; create agent first, then set role assignments and permissions in separate calls
   - **Documents** — reference roles or agents
   - **Tasks** — reference teams, optionally agents
4. For each entity:
   - **Default:** Look up by name. If exists, fail with error listing all conflicts.
   - **`--upsert`:** Look up by name. If exists, update. If not, create.

### Timestamp Handling

- **New entity (create):** Use `created_at` and `updated_at` from the file as-is
- **Existing entity (upsert):** Keep the database's existing timestamps, ignore the file's

### Dry Run

`--dry-run` runs full validation and reference resolution but skips create/update calls. Reports what would be created, what would be updated (with `--upsert`), and what conflicts exist.

### Error Handling

Fail fast on the first error. Since entities are created in dependency order, anything already created is valid and consistent. The user can fix the issue and re-run with `--upsert` to continue where they left off.

## Architecture

### DataSource Interface

```go
type DataSource interface {
    // Teams
    ListTeams(ctx context.Context) ([]*domain.Team, error)
    CreateTeam(ctx context.Context, t *domain.Team) error
    UpdateTeam(ctx context.Context, id, name string) error
    LookupTeamByName(ctx context.Context, name string) (*domain.Team, error)

    // Roles
    ListAllRoles(ctx context.Context) ([]*domain.Role, error)
    CreateRole(ctx context.Context, r *domain.Role) error
    UpdateRole(ctx context.Context, id, name, parentID string) error
    LookupRoleByName(ctx context.Context, name string) (*domain.Role, error)

    // Permissions
    ListPermissions(ctx context.Context) ([]*domain.Permission, error)
    EnsurePermission(ctx context.Context, name string) error // idempotent create-if-not-exists
    LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error)

    // Agents
    ListAllAgents(ctx context.Context) ([]*domain.Agent, error)
    CreateAgent(ctx context.Context, a *domain.Agent) error
    UpdateAgent(ctx context.Context, id, name, teamID string) error
    LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error)
    GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error)
    SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error
    GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error)
    SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error

    // Documents
    ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error)
    ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error)
    CreateDocument(ctx context.Context, d *domain.Document) error
    UpdateDocument(ctx context.Context, id, title, content string) error
    LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error)

    // Tasks
    ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error)
    CreateTask(ctx context.Context, t *domain.Task) error
    UpdateTask(ctx context.Context, id string, t *domain.Task) error
    LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error)
}
```

**Signature notes:**
- `ListAllRoles` and `ListAllAgents` are named differently from the store's `ListRoles(parentID)` / `ListAgents(teamID)` to clarify they return all entities. The `dbDataSource` passes `""` to the underlying store methods. The `restDataSource` calls the client's list methods with `""`.
- `CreateDocument` dispatches to `client.CreateRoleDocument` or `client.CreateAgentMemory` based on whether `d.RoleID` or `d.AgentID` is set.
- `EnsurePermission` is idempotent (create-if-not-exists). The `dbDataSource` wraps `SeedPermissions(ctx, []string{name})`. The `restDataSource` uses a new `POST /v1/permissions` endpoint.
- Return types use pointer slices (`[]*domain.T`) for top-level entities to match the store interface. Junction types (`AgentRole`, `AgentPermission`) use value slices since they are small structs.

**Two implementations:**
- `restDataSource` — wraps `client.Client`, talks to daemon over REST. Cannot preserve timestamps on create (REST API limitation).
- `dbDataSource` — wraps `domain.Store`, uses direct database access. Full timestamp preservation.

**Store additions needed for `dbDataSource`:**
- `LookupTeamByName`, `LookupRoleByName`, `LookupAgentByName` — indexed name lookups
- `ListPermissions` — list all permission entities
- `LookupPermissionByName` — resolve permission name to ID
- `LookupDocumentByOwnerAndTitle` — find document by owner + title
- `LookupTaskByTeamAndTitle` — find task by team + title

**Client additions needed for `restDataSource`:**
- `ListPermissions` — new endpoint `GET /v1/permissions`
- `CreatePermission` — new endpoint `POST /v1/permissions`
- Lookup methods use client-side filtering on list results (no new endpoints needed)

**Note:** The export logic iterates teams (fetched with IDs) and calls `ListTeamTasks(teamID)` using the team's ID, not its name. Same pattern for `ListRoleDocuments(roleID)` and `ListAgentMemory(agentID)` — always use IDs from previously fetched entities.

### Package Layout

```
cmd/
  export/
    export.go           # CLI command, flag parsing, DataSource selection
  import/
    import.go           # CLI command, flag parsing, DataSource selection
internal/
  transfer/
    datasource.go       # DataSource interface
    rest_source.go      # REST implementation (wraps client.Client)
    db_source.go        # Direct DB implementation (wraps domain.Store)
    exporter.go         # Export logic (fetch + serialize + write)
    importer.go         # Import logic (parse + validate + create)
    yaml.go             # YAML/front-matter marshaling/unmarshaling
    sanitize.go         # Filename sanitization
```

### Name-Based Lookups

The DataSource interface includes `Lookup*ByName` methods not in the current store or client. These are needed for:
- Resolving references during import (`team: engineering` → team ID)
- Detecting conflicts (entity with same name already exists)

The `restDataSource` implements lookups by fetching the full list and filtering client-side. The `dbDataSource` requires new indexed name-lookup queries in the store.

### Dry Run Output

When `--dry-run` is set, import prints a summary table:

```
Dry run results:
  teams:       2 create, 0 update, 0 conflict
  roles:       3 create, 1 update, 0 conflict
  permissions: 4 create, 0 update, 0 conflict
  agents:      1 create, 0 update, 1 conflict (claude-dev)
  documents:   5 create, 0 update, 0 conflict
  tasks:       8 create, 0 update, 0 conflict
```

With `--upsert`, "conflict" becomes "update". Without `--upsert`, conflicts cause a non-zero exit code.
