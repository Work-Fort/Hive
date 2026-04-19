---
type: plan
step: "1"
title: "Passport scheme split — Hive consumer migration"
status: approved
assessment_status: complete
provenance:
  source: cross-repo-coordination
  issue_id: "Cluster 3b (Passport, 2026-04-19)"
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: null
related_plans:
  - passport/lead/docs/plans/2026-04-19-auth-scheme-dispatch.md
  - sharkfin/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - flow/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - pylon/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - combine/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
---

# Hive — Passport Scheme Split Consumer Migration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update Hive's HTTP client (`client/client.go`) and the export/import/mcp-bridge subcommands to send API keys under the new `Authorization: ApiKey-v1 <key>` scheme. Outbound consumer clients are API-key-only — no JWT-sending path is added. The fix is a single-constructor rename + flag rename + transport edit.

**Background / Why:** Per TPM clarification 2026-04-19: only web browser clients use JWT; agents and services use API keys. Hive's CLI tools (export, import, mcp-bridge) are agent/service callers — they never send JWTs outbound. The current `client.New(baseURL, token)` constructor takes an opaque `token` and prefixes it with `Bearer`, which (a) is type-dishonest about what `token` actually is in practice (always an API key) and (b) breaks against passport's new scheme-dispatch middleware (Bearer is now JWT-only). Renaming to `client.New(baseURL, apiKey)` and switching the header to `ApiKey-v1` fixes both. Inbound middleware in Hive's daemon still needs both schemes for browser-routed traffic; that path is untouched here.

**Architecture:** Three call sites today take a "Passport token" string and blindly prefix it with `Bearer`:

1. `client.New(baseURL, token)` — used by `cmd/export`, `cmd/importcmd`, and **Flow's adapter** (which passes a service token = API key).
2. `cmd/mcpbridge/mcp_bridge.go` — flag is `--passport-token`; in production it always carries an API key.

The fix per call site is mechanical:

- `client/client.go`: rename the parameter `token string` → `apiKey string`, update the field name, and switch the header to `ApiKey-v1`. The Go type signature `func New(string, string) *Client` is technically unchanged — Go parameter names are not part of the type — but the **wire format** flips from `Bearer` to `ApiKey-v1`. That wire-format change is the actual breaking change and the commit message tags it `BREAKING CHANGE` accordingly. The parameter rename is for type honesty: every caller in production was passing an API key into a slot called `token`, and the docstring previously claimed it accepted "a Passport JWT or API key" — narrowed here to "API key only" per TPM clarification.
- `cmd/export`, `cmd/importcmd`, `cmd/mcpbridge`: rename the `--passport-token` flag to `--passport-api-key` (env var: `HIVE_PASSPORT_API_KEY`). No JWT-side flag is added. This is operator-visible: anyone consuming hive's CLIs must update their flags / env / `.mcp.json`.

**Tech Stack:** Go 1.x, cobra, viper. No new dependencies.

---

## Conventions

- Conventional Commits multi-line + `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer per commit.
- `mise run test`, `mise run e2e`, `mise run lint` for verification.
- Pin `service-auth` to local passport branch via `replace` for the duration of this work; drop in Task 4.

---

## Pre-flight: pin to local passport branch

`client/` is a separate Go module (`module github.com/Work-Fort/Hive/client`) but its `client/go.mod` does NOT directly depend on `service-auth` — verified at planning time (the file is bare: `module …; go 1.26`). Only the root module needs the `replace` directive. Re-run `grep service-auth client/go.mod` to confirm before editing — if a dep ever appears there, add a second `replace` to the client module too.

Add to root `go.mod`:

```
replace github.com/Work-Fort/Passport/go/service-auth => /home/kazw/Work/WorkFort/passport/lead/go/service-auth
```

`go mod tidy` (in the root only), commit:

```bash
git commit -m "$(cat <<'EOF'
chore: pin passport service-auth to local branch for scheme-split work

Temporary replace directive — removed in the final task of this plan.
Lets us verify the API-key client rename against passport's
pre-release scheme-dispatch branch.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 1: Inventory the operator-facing surface (env vars, flags, configs)

**Files:** read-only inventory pass; produce a list before any rename edits.

**Step 1: Run the operator-surface greps**

```bash
cd /home/kazw/Work/WorkFort/hive/lead

# Every place the old env var name appears (must all migrate to HIVE_PASSPORT_API_KEY):
grep -rn 'HIVE_PASSPORT_TOKEN' --include='*.go' --include='*.json' --include='*.service' --include='*.openrc' --include='*.md' --include='*.sh' .

# Every place the old flag/viper key appears:
grep -rn '\-\-passport-token\|"passport-token"\|passport-token' --include='*.go' --include='*.json' --include='*.md' --include='*.sh' .
```

**Step 2: Snapshot at planning time (re-verify before editing — files may drift)**

`HIVE_PASSPORT_TOKEN` (env var rename target → `HIVE_PASSPORT_API_KEY`):

| File | Line | Context |
| --- | --- | --- |
| `.mcp.json` | 11 | `"HIVE_PASSPORT_TOKEN": "${HIVE_PASSPORT_TOKEN}"` — both the JSON key and the `${VAR}` reference flip together |
| `cmd/mcpbridge/mcp_bridge.go` | 32 | error message text "set --passport-token, HIVE_PASSPORT_TOKEN, or passport-token" |
| `dist/hive.system.service` | (no direct reference; only `HIVE_PASSPORT_URL`. The unit loads `~/.config/hive/env` — operators may have set `HIVE_PASSPORT_TOKEN` there. The unit itself needs no edit, but the deploy runbook calls this out as an operator-visible env var rename.) | — |
| `docs/2026-03-13-passport-auth-design.md`, `docs/plans/015-passport-auth.md` | various | historical design docs — leave untouched (they document pre-rename state) |

`--passport-token` flag / `passport-token` viper key:

| File | Lines |
| --- | --- |
| `cmd/export/export.go` | 33-34, 67 |
| `cmd/importcmd/importcmd.go` | 35-36, 86 |
| `cmd/mcpbridge/mcp_bridge.go` | 28-29, 32, 38 |
| `internal/config/config.go` | 84 (viper default) |
| `tests/e2e/export_import_test.go` | 75, 125, 173, 186, 199 (5 sites) |

If the live grep finds additional hits (e.g., a new operator-facing script, or `HIVE_PASSPORT_TOKEN` referenced in a CI workflow file), include them in the rename pass. The plan is wrong, not the source.

**Step 3: No commit — this task produces only the inventory.**

---

### Task 2: Rename `client.New(baseURL, token)` → `client.New(baseURL, apiKey)` and send `ApiKey-v1`

**Files:**
- Modify: `client/client.go`
- Modify: `client/client_test.go` (verified to exist via `ls client/`)

**Step 1: Add a failing test**

```go
func TestClient_APIKeySendsApiKeyV1(t *testing.T) {
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "wf-svc_secret")
	_, _ = c.GetMe(context.Background())

	if gotAuth != "ApiKey-v1 wf-svc_secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "ApiKey-v1 wf-svc_secret")
	}
}
```

(`GetMe(ctx)` is the canonical zero-arg GET on `*Client` — defined at `client/agents.go:51`. It's the simplest method that exercises `do(...)` through the auth header logic.)

**Step 2: Run and confirm failure**

```
mise run test -- -run TestClient_APIKey -v ./client/...
```

Expected: FAIL — `Authorization = "Bearer wf-svc_secret"`.

**Step 3: Implement the rename + scheme switch**

In `client/client.go`, rename the field and parameter for type honesty:

```go
// Client is an HTTP client for the Hive REST API. It authenticates
// every request with a Passport API key under the ApiKey-v1 scheme.
type Client struct {
	http    http.Client
	baseURL string
	apiKey  string
}

// New creates a Client that authenticates with a Passport API key
// (Authorization: ApiKey-v1 <key>). API keys are recognizable by
// the wf-agent_ or wf-svc_ prefix.
//
// Hive's outbound clients are API-key-only — JWTs are reserved for
// browser-routed traffic which never originates here.
func New(baseURL, apiKey string) *Client {
	return &Client{
		http:    http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}
```

In the `do` method (around line 59-61), replace:

```go
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
```

with:

```go
	if c.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey-v1 "+c.apiKey)
	}
```

**Step 4: Run tests**

```
mise run test -- -run TestClient_ -v ./client/...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add client/client.go client/client_test.go
git commit -m "$(cat <<'EOF'
feat(client)!: rename token → apiKey; send ApiKey-v1 scheme

BREAKING CHANGE: client.New's second parameter is renamed from
"token" to "apiKey" (no signature change, but the wire format
changes from "Authorization: Bearer <key>" to "Authorization:
ApiKey-v1 <key>").

Per TPM clarification 2026-04-19: only web browser clients use JWT;
agents and services use API keys. Hive's outbound clients are
caller-side agents/services, so the parameter is now type-honest
about what it actually carries. Required by passport's
scheme-dispatch middleware (Bearer is now JWT-only).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Rename `--passport-token` → `--passport-api-key` in `cmd/export` and `cmd/importcmd`

**Files:**
- Modify: `cmd/export/export.go`
- Modify: `cmd/importcmd/importcmd.go`
- Modify: `internal/config/config.go`

**Step 1: Rename the flag**

In each command, rename `--passport-token` to `--passport-api-key` and the variable `passportToken` → `passportAPIKey`. The construction is now a single line:

```go
hiveClient := client.New(baseURL, passportAPIKey)
ds = transfer.NewRESTDataSource(hiveClient)

// ... flag declaration:
cmd.Flags().StringVar(&passportAPIKey, "passport-api-key", "", "Passport API key (env: HIVE_PASSPORT_API_KEY)")
```

Apply the same pattern to `cmd/importcmd/importcmd.go`.

**Step 2: Update `internal/config/config.go`**

Replace `viper.SetDefault("passport-token", "")` with:

```go
viper.SetDefault("passport-api-key", "")
```

(Env var binding follows: `HIVE_PASSPORT_API_KEY`.)

**Step 3: Build and run unit tests**

```
mise run lint && mise run test
```

Expected: PASS.

**Step 4: Commit**

```bash
git add cmd/export/export.go cmd/importcmd/importcmd.go internal/config/config.go
git commit -m "$(cat <<'EOF'
feat(cli)!: rename --passport-token → --passport-api-key

BREAKING CHANGE: the export and import subcommands renamed
--passport-token to --passport-api-key (env: HIVE_PASSPORT_API_KEY).
The previous "token" name was misleading — agents and services
always send API keys, never JWTs (TPM clarification 2026-04-19).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Rename the same flag in `cmd/mcpbridge/mcp_bridge.go`, update `.mcp.json` env var, switch header to `ApiKey-v1`

**Files:**
- Modify: `cmd/mcpbridge/mcp_bridge.go` (lines 28-29, 32, 38)
- Modify: `.mcp.json` (line 11 — both the JSON env-var key and the `${VAR}` reference)
- Modify: `tests/e2e/export_import_test.go` (5 sites: lines 75, 125, 173, 186, 199)

**Step 1: Rename flag, switch header, update error message**

The bridge currently has `--passport-token` and `passportToken` variable; the bridge's request setter prefixes with `Bearer`. Rename the flag/variable and switch the header:

```go
var passportAPIKey string
// flag declaration (was line 38):
cmd.Flags().StringVar(&passportAPIKey, "passport-api-key", "", "Passport API key (env: HIVE_PASSPORT_API_KEY, sent as ApiKey-v1)")

// viper read (was lines 28-29):
if !cmd.Flags().Changed("passport-api-key") {
    passportAPIKey = viper.GetString("passport-api-key")
}

// error message (was line 32):
return fmt.Errorf("passport API key required: set --passport-api-key, HIVE_PASSPORT_API_KEY, or passport-api-key in config")

// header:
req.Header.Set("Authorization", "ApiKey-v1 "+passportAPIKey)
```

(Cobra's viper-binding convention is to derive the env var name by upper-casing the key and prefixing the binary name — `HIVE_PASSPORT_API_KEY` follows from `passport-api-key` once the viper key is renamed in `internal/config/config.go` Task 3.)

**Step 2: Update `.mcp.json`**

`.mcp.json:11` is currently:

```json
"HIVE_PASSPORT_TOKEN": "${HIVE_PASSPORT_TOKEN}"
```

Change to:

```json
"HIVE_PASSPORT_API_KEY": "${HIVE_PASSPORT_API_KEY}"
```

(Both the JSON key and the shell-style `${VAR}` reference flip — the shell variable on the right is what an operator must export in their environment for the MCP host to inject it; both must be the new name for the substitution to resolve.)

**Step 3: Build, run unit tests**

```
mise run lint && mise run test && mise run build:dev
```

Expected: PASS, binary builds.

**Step 4: Update e2e tests**

Apply mechanically per Task 1 inventory — 5 sites in `tests/e2e/export_import_test.go` (lines 75, 125, 173, 186, 199). Each is `"--passport-token", <token>` → `"--passport-api-key", <token>`. The tokens passed are already API keys (issued by passport-stub for the export/import service identity), so this is a pure flag rename.

```
mise run e2e
```

Expected: PASS.

**Step 5: Verify the rename is exhaustive**

```
grep -rn 'HIVE_PASSPORT_TOKEN\|--passport-token\|"passport-token"' --include='*.go' --include='*.json' --include='*.service' --include='*.openrc' --include='*.sh' .
```

Expected: no matches under `cmd/`, `client/`, `internal/`, `tests/`, `dist/`, or `.mcp.json`. Matches inside `docs/2026-03-13-passport-auth-design.md` and `docs/plans/015-passport-auth.md` are historical and stay.

**Step 6: Commit**

```bash
git add cmd/mcpbridge/mcp_bridge.go .mcp.json tests/e2e/export_import_test.go
git commit -m "$(cat <<'EOF'
feat(mcpbridge)!: rename --passport-token → --passport-api-key; send ApiKey-v1

BREAKING CHANGE: hive mcp-bridge now uses --passport-api-key
(env: HIVE_PASSPORT_API_KEY) and sends the value under the
Authorization: ApiKey-v1 scheme. Required by passport's
scheme-dispatch middleware.

Updates the e2e suite invocations (5 sites in
tests/e2e/export_import_test.go) and .mcp.json (env-var key + ${VAR}
reference) to match. The error message in mcp_bridge.go also flips so
operators see the new name when the API key is missing.

Operator-visible env var rename — operators consuming hive's
mcp-bridge must update their environment from HIVE_PASSPORT_TOKEN
to HIVE_PASSPORT_API_KEY (see ~/Work/WorkFort/passport-scheme-split-deploy.md
§ Operator-visible breaking changes).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Drop the local `replace`, bump dependency, push

(Same shape as the sharkfin plan's Task 5. Sequencing: only after passport's plan has been pushed and the new `service-auth` tag is published.)

```bash
# 1. Remove replace from go.mod
# 2. Bump:
go get github.com/Work-Fort/Passport/go/service-auth@<new-tag>
go mod tidy
# 3. Verify:
mise run lint && mise run test && mise run e2e
# 4. Commit + push
git commit -m "$(cat <<'EOF'
chore(deps): bump passport service-auth to <new-tag> (scheme dispatch)

Drops the local replace directive used during the consumer migration.
Hive's client now sends API keys under ApiKey-v1, aligned with
passport's scheme-dispatch middleware.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Dependents that need a coordinated bump

After this plan's push, **flow** consumes `hiveclient.New(baseURL, token)` in `flow/lead/internal/infra/hive/adapter.go`. Flow's plan is a pure no-op rename at that call site (the parameter name changed, signature didn't) — but flow MUST bump the hive dep tag after this push. See `flow/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md`.

## Verification checklist

- [ ] `mise run lint` clean
- [ ] `mise run test` PASS
- [ ] `mise run e2e` PASS
- [ ] No `Bearer ` left in `client/` or `cmd/` (`grep -rn 'Bearer' client/ cmd/ --include='*.go'`)
- [ ] No `--passport-token` remains anywhere (`grep -rn 'passport-token' --include='*.go' --include='*.json'`)
- [ ] `client.New` has at least one test asserting `ApiKey-v1 <key>` on the wire
