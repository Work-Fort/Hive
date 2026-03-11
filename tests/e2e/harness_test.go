// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/client"
)

const (
	// testAPIKey is the shared API key used by every harness in the test run.
	testAPIKey = "e2e-test-key-do-not-use-in-prod"

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
	t      *testing.T
	dir    string         // root temp directory for this harness
	cmd    *exec.Cmd      // the running hive process
	port   int            // the port the daemon is listening on
	Client *client.Client // pre-configured client for this harness
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

	// Pick a free TCP port.
	port, err := freePort()
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("pick free port: %v", err)
	}

	// SQLite database inside the state directory.
	dbPath := filepath.Join(stateDir, "hive.db")

	cmd := exec.Command(
		hiveBin,
		"daemon",
		"--bind", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--db", dbPath,
		"--api-key", testAPIKey,
		"--log-level", "disabled",
	)
	// XDG env vars scope config/state to our temp dirs.
	cmd.Env = append(os.Environ(),
		"XDG_STATE_HOME="+stateDir,
		"XDG_CONFIG_HOME="+configDir,
	)
	// Capture daemon output to a file so failures can be diagnosed.
	logFile := filepath.Join(dir, "daemon.log")
	lf, err := os.Create(logFile)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("create daemon log: %v", err)
	}
	cmd.Stdout = lf
	cmd.Stderr = lf

	if err := cmd.Start(); err != nil {
		lf.Close()
		os.RemoveAll(dir)
		t.Fatalf("start daemon: %v", err)
	}

	h := &Harness{
		t:      t,
		dir:    dir,
		cmd:    cmd,
		port:   port,
		Client: client.New(fmt.Sprintf("http://127.0.0.1:%d", port), testAPIKey),
	}

	// Wait for the health endpoint to respond.
	if err := h.waitHealthy(); err != nil {
		h.Close()
		// Print the daemon log to help diagnose startup failures.
		lf.Close()
		if b, readErr := os.ReadFile(logFile); readErr == nil {
			t.Logf("daemon log:\n%s", b)
		}
		t.Fatalf("daemon did not become healthy: %v", err)
	}

	lf.Close()
	t.Cleanup(h.Close)
	return h
}

// Close stops the daemon process and removes the temp directory.
func (h *Harness) Close() {
	h.t.Helper()
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
	}
	if h.dir != "" {
		os.RemoveAll(h.dir)
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
