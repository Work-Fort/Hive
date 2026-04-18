# Agent Assignment Schema and Endpoints

Hive's component-level work for the agent pool design. The
cross-cutting design lives in
`flow/lead/docs/2026-04-18-agent-pool.md`; this doc is the spec for
what Hive specifically needs to add.

## Schema additions

### `agents` table — current assignment

Four nullable columns, all-or-nothing:

| Column | Type | Purpose |
|---|---|---|
| `current_role` | TEXT NULL | Role the agent is filling (e.g. `developer`, `reviewer`). |
| `current_project` | TEXT NULL | Project the agent is working on (e.g. `flow`, `nexus`). |
| `current_workflow_id` | TEXT NULL | Flow workflow ID holding the lease. |
| `lease_expires_at` | DATETIME NULL | When the lease auto-releases if not renewed. |

DB constraint: either all four are NULL (agent is free) or all four
are set (agent is claimed). Express as a CHECK constraint:

```sql
CHECK (
  (current_role IS NULL
    AND current_project IS NULL
    AND current_workflow_id IS NULL
    AND lease_expires_at IS NULL)
  OR
  (current_role IS NOT NULL
    AND current_project IS NOT NULL
    AND current_workflow_id IS NOT NULL
    AND lease_expires_at IS NOT NULL)
)
```

### `tasks` table — Flow workflow reference

| Column | Type | Purpose |
|---|---|---|
| `flow_task_ref` | TEXT NULL | Free-form string referencing the originating Flow workflow task. Opaque to Hive — Hive does not parse or validate it. |

## New endpoints

All are scoped to the calling identity via the existing Passport
auth middleware. Workflow-mediated calls come from Flow's service
token; agent-self calls (none in this design) would come from the
agent's own token.

### `POST /v1/agents/claim`

Body:
```json
{
  "role": "developer",
  "project": "flow",
  "workflow_id": "flow-task-117",
  "lease_ttl_seconds": 120
}
```

Atomically picks one free agent and sets its assignment. Returns
the chosen agent (full record) on success, 409 on pool exhaustion.

Selection: among free agents, picks any. The design defers
preference logic (e.g. "prefer agents who recently did this role")
to future work.

### `POST /v1/agents/{id}/release`

Body:
```json
{ "workflow_id": "flow-task-117" }
```

Atomically clears the assignment if `current_workflow_id` matches
`workflow_id`. Prevents one workflow from accidentally releasing
another workflow's claim.

Returns 204 on success, 409 if the workflow_id doesn't match, 404
if the agent doesn't exist.

### `POST /v1/agents/{id}/renew`

Body:
```json
{ "workflow_id": "flow-task-117", "lease_ttl_seconds": 120 }
```

Updates `lease_expires_at` to `now + lease_ttl_seconds` if
`current_workflow_id` matches. Used by Flow's background renewer
to keep its own claims alive.

Returns 204 on success, 409 if the workflow_id doesn't match.

### `GET /v1/agents?assigned={true|false}`

Optional query params:
- `assigned=false` — list free agents only
- `assigned=true&workflow_id=...` — list agents claimed by a specific workflow
- `role=` and `project=` — filter further

Used by Flow's scheduler to find candidates and by Virgil-style
operators to inspect pool state.

## Background sweeper

Goroutine in the daemon that runs periodically (every ~30s) and
runs:

```sql
UPDATE agents
SET current_role = NULL,
    current_project = NULL,
    current_workflow_id = NULL,
    lease_expires_at = NULL
WHERE lease_expires_at IS NOT NULL
  AND lease_expires_at < datetime('now');
```

Recovers from Flow crashes — when Flow stops renewing, claims
auto-expire and agents return to the pool. Sweep emits a log line
per release for audit visibility.

## `get_provisioning` extension

Today: returns the persistent role-set assigned to the agent.

New behavior: if `current_assignment` is set, return the role
documents for `current_role` instead of the persistent role-set.
Project is exposed as additional context (probably as a memory
document, since the role doc itself shouldn't change per project).

This way an agent's MCP tool `get_provisioning` returns "you are
acting as a `developer` for project `flow`" when claimed, and
returns idle/empty (or the persistent role-set) when free.

The existing permission-filtering for tools continues to apply per
the assignment's role — so a developer claimed for `flow` only
sees the tools the `developer` role permits.

## Migration order

1. Add columns to `agents` and `tasks`.
2. Add `claim` / `release` / `renew` / pool-list endpoints.
3. Add sweeper.
4. Update `get_provisioning` to honor `current_assignment`.

Each step is independently shippable; Flow can start using claim/
release as soon as step 2 lands.

## Out of scope

- Hive does not start or stop VMs. Flow does that.
- Hive does not know about drives or btrfs. Flow does that.
- Hive does not interpret `flow_task_ref` — it's a string Hive
  stores and returns verbatim.
- Hive does not pick "warm" agents based on prior assignments.
  Flow may layer that on top later.
