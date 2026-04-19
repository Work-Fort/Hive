// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/client"
)

const (
	// startupTimeout is how long to wait for the daemon health endpoint to
	// respond before declaring startup a failure.
	startupTimeout = 15 * time.Second

	// pollInterval is how often to probe the health endpoint during startup.
	pollInterval = 50 * time.Millisecond
)

// Harness manages a single daemon process for a test. Each call to
// newHarness starts a fresh daemon with isolated XDG directories and a
// random port. Call h.Close() (or register it with t.Cleanup) to stop the
// daemon and remove temp files.
type Harness struct {
	t          *testing.T
	dir        string         // root temp directory for this harness
	cmd        *exec.Cmd      // the running hive process
	port       int            // the port the daemon is listening on
	Client     *client.Client // pre-configured client for this harness
	stubStop   func()         // stops the JWKS stub server
	signJWT    func(id, username, name, userType string) string
	mintAPIKey func(id, username, name, userType string) string
	logFile    *os.File // stdout+stderr capture; closed in Close after wait
}

// SignJWT creates a signed JWT with the given identity claims.
// The token is valid for 1 hour and signed with the JWKS stub's private key.
// id should be the agent's UUID as stored in the Hive database.
func (h *Harness) SignJWT(id, username, name, userType string) string {
	return h.signJWT(id, username, name, userType)
}

// MintAPIKey registers a new API key with the JWKS stub bound to the given
// identity claims, and returns the key string. Pass it into client.New —
// it travels under the ApiKey-v1 scheme and the daemon scheme-dispatches
// to the API-key validator.
func (h *Harness) MintAPIKey(id, username, name, userType string) string {
	return h.mintAPIKey(id, username, name, userType)
}

// newHarness creates a Harness, starts the daemon, and waits for it to be
// healthy. The harness is automatically closed via t.Cleanup.
func newHarness(t *testing.T) *Harness {
	t.Helper()

	// Per-test temp directory tree.
	dir, err := os.MkdirTemp("", "hive-e2e-harness-*")
	if err != nil {
		t.Fatalf("create harness temp dir: %v", err)
	}

	// XDG directories inside the temp tree.
	stateDir := filepath.Join(dir, "state")
	configDir := filepath.Join(dir, "config")
	for _, d := range []string{stateDir, configDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			os.RemoveAll(dir)
			t.Fatalf("create xdg dir %s: %v", d, err)
		}
	}

	// Start the JWKS stub before the daemon so the initial JWKS fetch succeeds.
	stubAddr, stubStop, signJWT, mintAPIKey := startJWKSStub()

	// Mint a per-harness API key bound to a stable identity. The Hive Go
	// client sends every credential under ApiKey-v1, so the harness uses
	// API keys (not JWTs) for its default authenticated client.
	harnessToken := mintAPIKey(
		"00000000-0000-0000-0000-000000000000",
		"e2e-test",
		"E2E Test User",
		"user",
	)

	// Pick a free TCP port.
	port, err := freePort()
	if err != nil {
		stubStop()
		os.RemoveAll(dir)
		t.Fatalf("pick free port: %v", err)
	}

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

	logFile := filepath.Join(dir, "daemon.log")
	lf, err := os.Create(logFile)
	if err != nil {
		stubStop()
		os.RemoveAll(dir)
		t.Fatalf("create daemon log: %v", err)
	}
	// *os.File for stdout/stderr (not io.Writer) so exec.Cmd does
	// not create a copy goroutine; Setpgid puts the daemon and any
	// descendants in a fresh process group; WaitDelay force-closes
	// any inherited fds after the daemon exits. See the orphan-
	// process hardening section of go-service-architecture.
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		lf.Close()
		stubStop()
		os.RemoveAll(dir)
		t.Fatalf("start daemon: %v", err)
	}

	h := &Harness{
		t:          t,
		dir:        dir,
		cmd:        cmd,
		port:       port,
		Client:     client.New(fmt.Sprintf("http://127.0.0.1:%d", port), harnessToken),
		stubStop:   stubStop,
		signJWT:    signJWT,
		mintAPIKey: mintAPIKey,
		logFile:    lf,
	}

	if err := h.waitHealthy(); err != nil {
		// Read what the daemon managed to write before death.
		if b, readErr := os.ReadFile(logFile); readErr == nil {
			t.Logf("daemon log:\n%s", b)
		}
		h.Close()
		t.Fatalf("daemon did not become healthy: %v", err)
	}

	t.Cleanup(h.Close)
	return h
}

// Close stops the daemon process (SIGTERM, then SIGKILL after 10s),
// shuts down the JWKS stub, and removes the temp directory. Reads
// the captured stdout/stderr after the daemon exits, dumps it on
// test failure, and fails the test if it contains DATA RACE.
//
// Idempotent under sequential calls (the test calls Close explicitly
// and t.Cleanup runs Close again on test exit; the second call is a
// no-op). NOT safe to call concurrently — fields are zeroed without
// locking. The t.Cleanup contract is sequential, so this is fine.
func (h *Harness) Close() {
	h.t.Helper()
	if h.stubStop != nil {
		h.stubStop()
		h.stubStop = nil
	}
	if h.cmd != nil && h.cmd.Process != nil {
		pgid := h.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- h.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-done
		}
		// Mark process as reaped so a second Close is a no-op.
		h.cmd.Process = nil
	}

	var logBytes []byte
	if h.logFile != nil {
		// Read whatever the daemon wrote, then close and unlink.
		// (We can't unlink while open on Windows, but the e2e
		// harness is Linux-only.)
		logBytes, _ = os.ReadFile(h.logFile.Name())
		h.logFile.Close()
		h.logFile = nil
	}

	if h.t.Failed() && len(logBytes) > 0 {
		h.t.Logf("daemon log:\n%s", logBytes)
	}
	if bytes.Contains(logBytes, []byte("DATA RACE")) {
		h.t.Errorf("data race detected in daemon log:\n%s", logBytes)
	}

	if h.dir != "" {
		os.RemoveAll(h.dir)
		h.dir = ""
	}
}

// waitHealthy polls the health endpoint until it returns 2xx or the
// startupTimeout is exceeded.
func (h *Harness) waitHealthy() error {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", h.port)
	deadline := time.Now().Add(startupTimeout)
	httpClient := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			// Any 2xx response (including 218 Degraded) is a successful start.
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				return nil
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timed out after %s waiting for health endpoint %s", startupTimeout, url)
}

// freePort asks the OS for an available TCP port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

// ctx returns a background context. Helper so test bodies stay concise.
func ctx() context.Context {
	return context.Background()
}
