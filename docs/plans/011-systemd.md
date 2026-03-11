# Systemd Service Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a systemd user service file that starts the Hive daemon on login, restarts it on failure, and provides readiness signalling via the existing `/v1/health` endpoint.

**Architecture:** One `.service` file at `deploy/hive.service` and one `mise` install task. The daemon already handles XDG paths — no code changes are needed. The service file references the binary by its installed path (`%h/.local/bin/hive`) and passes configuration via `Environment=` directives. The API key is loaded from a separate credential file so it is never visible in `systemctl status` output.

**Tech Stack:** systemd user unit, shell (mise task)

**Depends on:** Plan 001 (daemon binary) must be complete.

---

## Chunk 1: Service File

### Task 1: Write the systemd user service file

**Files:**
- Create: `deploy/hive.service`

- [ ] **Step 1: Create `deploy/hive.service`**

Create `deploy/hive.service`:

```ini
# SPDX-License-Identifier: GPL-3.0-or-later
[Unit]
Description=Hive agent provisioning daemon
Documentation=https://github.com/Work-Fort/Hive
After=network.target

[Service]
Type=simple
ExecStart=%h/.local/bin/hive daemon
ExecStartPost=/bin/sh -c 'for i in $(seq 1 30); do curl -sf http://127.0.0.1:17000/v1/health && exit 0; sleep 1; done; exit 1'
Restart=on-failure
RestartSec=5s

# Load the API key from a file so it is not exposed in `systemctl status`.
# Create the file with:  install -m 600 /dev/null ~/.config/hive/api-key
# Then write your key:   echo -n 'your-key-here' > ~/.config/hive/api-key
EnvironmentFile=-%h/.config/hive/env

# Override defaults via environment (matches HIVE_* prefix in config.go).
# These can be overridden by placing the same vars in ~/.config/hive/env.
Environment=HIVE_LOG_LEVEL=info

[Install]
WantedBy=default.target
```

Notes on design decisions:

- `Type=simple` is used rather than `Type=notify` because the daemon does not call `sd_notify`. The `ExecStartPost` poll loop bridges the gap: it retries `GET /v1/health` up to 30 times (30 seconds) and exits non-zero if the daemon never becomes ready. systemd marks the service start as failed if `ExecStartPost` exits non-zero, which is the desired behaviour.
- `%h` expands to the user's home directory inside a user unit (equivalent to `$HOME`).
- `EnvironmentFile=` with a leading `-` means the file is optional — the service starts even if `~/.config/hive/env` does not exist.
- `~/.config/hive/env` should contain `HIVE_API_KEY=<value>` (and optionally other `HIVE_*` overrides). This file must be mode `600`.
- The database path defaults to `~/.local/state/hive/hive.db` via the daemon's XDG logic (`internal/config/config.go` — `StateDir` resolves to `$XDG_STATE_HOME/hive`, which defaults to `~/.local/state/hive`).

- [ ] **Step 2: Verify the unit file parses cleanly (if systemd is available)**

```bash
systemd-analyze verify deploy/hive.service 2>&1 || true
```

This is best-effort; CI environments may not have systemd. The `|| true` prevents a hard failure in those cases.

- [ ] **Step 3: Commit**

```bash
git add deploy/hive.service
git commit -m "feat: add systemd user service file"
```

---

## Chunk 2: Installation Task

### Task 2: Add a mise install task

**Files:**
- Modify: `mise.toml`

The install task copies the binary and service file to their standard locations, then reloads the systemd user daemon so the new unit is picked up without requiring a logout.

- [ ] **Step 1: Add `install` and `uninstall` tasks to `mise.toml`**

Add to `mise.toml`:

```toml
[tasks.install]
description = "Install hive binary and systemd user service"
run = """
set -euo pipefail
install -Dm755 build/hive "$HOME/.local/bin/hive"
install -Dm644 deploy/hive.service "$HOME/.config/systemd/user/hive.service"
systemctl --user daemon-reload
echo "Installed. Enable with: systemctl --user enable --now hive"
echo "Set API key: echo 'HIVE_API_KEY=<key>' > ~/.config/hive/env && chmod 600 ~/.config/hive/env"
"""

[tasks.uninstall]
description = "Stop, disable, and remove hive binary and service"
run = """
set -euo pipefail
systemctl --user stop hive 2>/dev/null || true
systemctl --user disable hive 2>/dev/null || true
rm -f "$HOME/.local/bin/hive"
rm -f "$HOME/.config/systemd/user/hive.service"
systemctl --user daemon-reload
echo "Uninstalled."
"""
```

- [ ] **Step 2: Verify `mise.toml` is valid TOML**

```bash
mise tasks ls
```

- [ ] **Step 3: Commit**

```bash
git add mise.toml
git commit -m "feat: add mise install/uninstall tasks for systemd service"
```

---

## Summary

### Files Created
| File | Purpose |
|---|---|
| `deploy/hive.service` | Systemd user unit: starts daemon, polls health, restarts on failure |

### Files Modified
| File | Change |
|---|---|
| `mise.toml` | Add `install` and `uninstall` tasks |

### Key Design Decisions

1. **`Type=simple` + `ExecStartPost` poll.** The daemon does not call `sd_notify`, so `Type=notify` would stall forever. The poll loop gives systemd a concrete failure signal if the daemon never becomes healthy, and it exits immediately on first success so there is no unnecessary delay.

2. **API key in `EnvironmentFile`, not inline.** `Environment=HIVE_API_KEY=secret` appears in `systemctl status` output. `EnvironmentFile=` with a `600`-mode file keeps the key out of logs and process listings. The leading `-` makes the file optional so the service boots without it (useful during initial setup or when the key is set via the config file instead).

3. **No code changes needed.** The daemon already reads XDG paths, respects `HIVE_*` env vars, and defaults the database to `~/.local/state/hive/hive.db`. The service file is pure configuration on top of the existing binary.

4. **`WantedBy=default.target`.** This is the standard target for user services that should start on login. The user enables autostart with `systemctl --user enable --now hive`.
