# Project Skeleton Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bootstrap the Hive Go project with CLI framework, config system, HTTP server, empty MCP endpoint, health check, and build tooling — producing a runnable binary.

**Architecture:** Single Go binary using Cobra for CLI, Viper for config, charmbracelet/log for logging. HTTP server with stdlib `net/http` serving `/v1/health` and `/mcp` (empty placeholder). Follows Nexus conventions: `internal/` for private packages, hexagonal layout, XDG paths, SPDX headers. Daemon and bridge live in separate subpackages under `cmd/`.

**Tech Stack:** Go 1.26+, Cobra, Viper, charmbracelet/log, mcp-go, mise

---

## Chunk 1: Build Tooling, Go Module, and CLI Root

### Task 1: mise.toml and .gitignore

**Files:**
- Modify: `mise.toml`
- Create: `.gitignore`

- [ ] **Step 1: Configure mise.toml with Go toolchain and build tasks**

Replace `mise.toml` contents with:

```toml
[tools]
go = "1.26.0"

[tasks.build]
description = "Build the hive binary"
run = "go build -o build/hive ."

[tasks.test]
description = "Run unit tests"
run = "go test ./..."

[tasks.lint]
description = "Run go vet"
run = "go vet ./..."

[tasks.clean]
description = "Remove build artifacts"
run = "rm -rf build/"
```

- [ ] **Step 2: Create .gitignore**

Create `.gitignore`:

```
build/
```

- [ ] **Step 3: Ensure Go 1.26 is available**

```bash
mise install
go version
```

Expected: `go version go1.26.0 ...` (no output from mise install if already cached).

- [ ] **Step 4: Commit**

```bash
git add mise.toml .gitignore
git commit -m "chore: configure mise with Go 1.26 and add .gitignore"
```

### Task 2: Initialize Go module, config, root command, and main.go

All four files are created together so the first commit compiles.

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `internal/config/config.go`
- Create: `cmd/root.go`

- [ ] **Step 1: Initialize Go module**

```bash
go mod init github.com/Work-Fort/Hive
go mod edit -go=1.26
```

- [ ] **Step 2: Create internal/config/config.go**

Create `internal/config/config.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	EnvPrefix           = "HIVE"
	ConfigFileName      = "config"
	ConfigType          = "yaml"
	DefaultBind         = "127.0.0.1"
	DefaultPort         = 17000
	DefaultMaxRoleDepth = 10
)

// Paths holds XDG-compliant directory paths.
type Paths struct {
	ConfigDir string
	StateDir  string
}

var GlobalPaths *Paths

func init() {
	GlobalPaths = GetPaths()
}

// GetPaths returns XDG-compliant directory paths.
func GetPaths() *Paths {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get home directory: %v\n", err)
			os.Exit(1)
		}
		configHome = filepath.Join(home, ".config")
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get home directory: %v\n", err)
			os.Exit(1)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}

	return &Paths{
		ConfigDir: filepath.Join(configHome, "hive"),
		StateDir:  filepath.Join(stateHome, "hive"),
	}
}

// InitDirs creates all necessary directories.
func InitDirs() error {
	dirs := []string{
		GlobalPaths.ConfigDir,
		GlobalPaths.StateDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}

// InitViper sets up viper defaults and config file search paths.
func InitViper() {
	viper.SetDefault("bind", DefaultBind)
	viper.SetDefault("port", DefaultPort)
	viper.SetDefault("log-level", "debug")
	viper.SetDefault("db", "")
	viper.SetDefault("api-key", "")
	viper.SetDefault("max-role-depth", DefaultMaxRoleDepth)

	viper.SetConfigName(ConfigFileName)
	viper.SetConfigType(ConfigType)
	viper.AddConfigPath(GlobalPaths.ConfigDir)

	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

// LoadConfig reads the config file if present.
func LoadConfig() error {
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}
	return nil
}

// BindFlags binds cobra flags to viper.
func BindFlags(flags *pflag.FlagSet) error {
	flagsToBind := []string{"log-level"}
	for _, name := range flagsToBind {
		if err := viper.BindPFlag(name, flags.Lookup(name)); err != nil {
			return fmt.Errorf("bind flag %s: %w", name, err)
		}
	}
	return nil
}
```

- [ ] **Step 3: Create cmd/root.go**

Create `cmd/root.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/internal/config"
)

// Version is set at build time via ldflags.
var Version string

var rootCmd = &cobra.Command{
	Use:   "hive",
	Short: "Hive agent provisioning daemon",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.InitDirs(); err != nil {
			return err
		}
		if err := config.LoadConfig(); err != nil {
			return err
		}

		ll := viper.GetString("log-level")
		if ll == "disabled" {
			log.SetOutput(io.Discard)
			return nil
		}

		var level log.Level
		switch ll {
		case "debug":
			level = log.DebugLevel
		case "info":
			level = log.InfoLevel
		case "warn":
			level = log.WarnLevel
		case "error":
			level = log.ErrorLevel
		default:
			level = log.DebugLevel
		}

		logPath := filepath.Join(config.GlobalPaths.StateDir, "debug.log")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}

		logger := log.NewWithOptions(f, log.Options{
			ReportTimestamp: true,
			TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
			Level:           level,
			ReportCaller:    true,
			Formatter:       log.JSONFormatter,
		})
		log.SetDefault(logger)

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	config.InitViper()

	rootCmd.PersistentFlags().StringP("log-level", "l", "debug",
		"Log level: disabled, debug, info, warn, error")

	if err := config.BindFlags(rootCmd.PersistentFlags()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if Version != "" {
		rootCmd.Version = Version
	} else {
		rootCmd.Version = "dev"
	}
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
}
```

- [ ] **Step 4: Create main.go**

Create `main.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"github.com/Work-Fort/Hive/cmd"
)

func main() {
	cmd.Execute()
}
```

- [ ] **Step 5: Resolve dependencies and verify compilation**

```bash
go mod tidy
go build ./...
```

Expected: no output (exit code 0). Binary compiles successfully.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum main.go cmd/root.go internal/config/config.go
git commit -m "feat: add Go module, config package, and Cobra root command"
```

## Chunk 2: Daemon Command and HTTP Server

### Task 3: Health handler

**Files:**
- Create: `internal/daemon/health.go`

- [ ] **Step 1: Create health handler**

Create `internal/daemon/health.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"encoding/json"
	"net/http"
	"sync"
)

// HealthStatus represents the overall system health.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// HealthReport is returned by the health endpoint.
type HealthReport struct {
	Status   HealthStatus `json:"status"`
	Warnings []string     `json:"warnings,omitempty"`
	Errors   []string     `json:"errors,omitempty"`
}

// HealthService tracks system health warnings and errors.
type HealthService struct {
	mu       sync.RWMutex
	warnings []string
	errors   []string
}

// NewHealthService creates a new HealthService.
func NewHealthService() *HealthService {
	return &HealthService{}
}

// AddWarning adds a health warning.
func (h *HealthService) AddWarning(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.warnings = append(h.warnings, msg)
}

// AddError adds a health error.
func (h *HealthService) AddError(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errors = append(h.errors, msg)
}

// Status returns the current health report.
func (h *HealthService) Status() HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()

	report := HealthReport{Status: StatusHealthy}

	if len(h.warnings) > 0 {
		report.Status = StatusDegraded
		report.Warnings = make([]string, len(h.warnings))
		copy(report.Warnings, h.warnings)
	}

	if len(h.errors) > 0 {
		report.Status = StatusUnhealthy
		report.Errors = make([]string, len(h.errors))
		copy(report.Errors, h.errors)
	}

	return report
}

// HandleHealth returns an http.HandlerFunc for the health endpoint.
func HandleHealth(health *HealthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.Status()

		var statusCode int
		switch report.Status {
		case StatusHealthy:
			statusCode = http.StatusOK
		case StatusDegraded:
			statusCode = 218
		default:
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(report)
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/daemon/health.go
git commit -m "feat: add health service and handler"
```

### Task 4: HTTP server setup

**Files:**
- Create: `internal/daemon/server.go`

- [ ] **Step 1: Create server setup**

Create `internal/daemon/server.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind   string
	Port   int
	Health *HealthService
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// MCP placeholder — will be wired to mcp-go StreamableHTTPServer in plan 005.
	// Full MCP spec requires GET /mcp (SSE) and DELETE /mcp (session termination)
	// in addition to POST /mcp, which will be handled by the library.
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "MCP server not yet implemented", http.StatusNotImplemented)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// ListenAndServe starts the server on the configured address.
func ListenAndServe(srv *http.Server) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	fmt.Printf("Hive daemon listening on %s\n", ln.Addr())
	return srv.Serve(ln)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/daemon/server.go
git commit -m "feat: add HTTP server with health and MCP placeholder"
```

### Task 5: Daemon subcommand

**Files:**
- Create: `cmd/daemon/daemon.go`

Note: The daemon command lives in its own subpackage `cmd/daemon/` per the spec's
project layout. The root command imports and registers it.

- [ ] **Step 1: Create daemon subcommand**

Create `cmd/daemon/daemon.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	hiveDaemon "github.com/Work-Fort/Hive/internal/daemon"
)

// NewCmd returns the daemon cobra command.
func NewCmd() *cobra.Command {
	var bind string
	var port int
	var db string
	var apiKey string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Hive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(bind, port, db, apiKey)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", 17000, "Listen port")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN (postgres://... or SQLite file path)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for REST authentication")

	return cmd
}

func run(bind string, port int, db, apiKey string) error {
	health := hiveDaemon.NewHealthService()

	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:   bind,
		Port:   port,
		Health: health,
	})

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := hiveDaemon.ListenAndServe(srv); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http shutdown", "err", err)
	}

	return nil
}
```

- [ ] **Step 2: Register daemon command in root**

Add to `cmd/root.go` in the `init()` function, after the version setup:

```go
import daemonCmd "github.com/Work-Fort/Hive/cmd/daemon"
```

And in `init()`:

```go
rootCmd.AddCommand(daemonCmd.NewCmd())
```

- [ ] **Step 3: Verify build and run**

```bash
go mod tidy
go build -o build/hive .
./build/hive daemon --port 17001 &
sleep 1
curl -s http://127.0.0.1:17001/v1/health | jq .
kill %1
```

Expected: `{"status":"healthy"}`

- [ ] **Step 4: Commit**

```bash
git add cmd/daemon/daemon.go cmd/root.go
git commit -m "feat: add daemon subcommand with signal handling"
```

## Chunk 3: MCP Bridge

### Task 6: MCP bridge subcommand

**Files:**
- Create: `cmd/mcpbridge/mcp_bridge.go`

Note: Like the daemon, the bridge lives in its own subpackage. Flags use local
variables (not global viper bindings) to avoid key collisions with daemon flags.

- [ ] **Step 1: Create MCP bridge subcommand**

Create `cmd/mcpbridge/mcp_bridge.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package mcpbridge

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

// NewCmd returns the mcp-bridge cobra command.
func NewCmd() *cobra.Command {
	var agentID string
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "mcp-bridge",
		Short: "Stdio-to-HTTP MCP bridge",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(agentID, host, port)
		},
	}

	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID this bridge serves (required)")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Daemon host")
	cmd.Flags().IntVar(&port, "port", 17000, "Daemon port")

	cmd.MarkFlagRequired("agent-id")

	return cmd
}

func run(agentID, host string, port int) error {
	mcpURL := fmt.Sprintf("http://%s:%d/mcp", host, port)

	client := &http.Client{Timeout: 60 * time.Second}
	var sessionID string

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		req, err := http.NewRequest("POST", mcpURL, bytes.NewReader(line))
		if err != nil {
			log.Error("create request", "err", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Agent-Id", agentID)
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Error("forward request", "err", err)
			continue
		}

		// Capture session ID from response
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			sessionID = sid
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Error("read response", "err", err)
			continue
		}

		os.Stdout.Write(body)
		os.Stdout.Write([]byte("\n"))
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read: %w", err)
	}

	return nil
}
```

- [ ] **Step 2: Register bridge command in root**

Add to `cmd/root.go`:

```go
import mcpBridgeCmd "github.com/Work-Fort/Hive/cmd/mcpbridge"
```

And in `init()`:

```go
rootCmd.AddCommand(mcpBridgeCmd.NewCmd())
```

- [ ] **Step 3: Verify build**

```bash
go mod tidy
go build -o build/hive .
./build/hive mcp-bridge --help
```

Expected: shows usage with `--agent-id`, `--host`, `--port` flags.

- [ ] **Step 4: Commit**

```bash
git add cmd/mcpbridge/mcp_bridge.go cmd/root.go
git commit -m "feat: add MCP bridge subcommand (stdio-to-HTTP)"
```

### Task 7: Smoke test — full build, daemon start, health check

Manual verification that the skeleton works end-to-end.

- [ ] **Step 1: Clean build and start daemon**

```bash
mise run build
./build/hive daemon --bind 127.0.0.1 --port 17099 &
sleep 1
```

- [ ] **Step 2: Verify health endpoint**

```bash
curl -s http://127.0.0.1:17099/v1/health | jq .
```

Expected:

```json
{
  "status": "healthy"
}
```

- [ ] **Step 3: Verify MCP placeholder**

```bash
curl -s -X POST http://127.0.0.1:17099/mcp
```

Expected: `MCP server not yet implemented` with 501 status.

- [ ] **Step 4: Verify version**

```bash
./build/hive --version
```

Expected: `hive version dev`

- [ ] **Step 5: Stop daemon and clean up**

```bash
kill %1
```

- [ ] **Step 6: Final commit if any loose files**

```bash
git status
# If go.sum was updated:
git add go.sum
git commit -m "chore: update go.sum after dependency resolution"
```
