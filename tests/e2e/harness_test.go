// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	// testPassportURL is the Passport instance used for E2E tests.
	testPassportURL = "http://passport.nexus:3000"

	// Test user credentials for Passport.
	testEmail    = "e2e-test@workfort.dev"
	testPassword = "e2e-test-password-2026"
	testUsername = "e2e-test"
	testName     = "E2E Test User"

	// startupTimeout is how long to wait for the daemon health endpoint to
	// respond before declaring startup a failure.
	startupTimeout = 15 * time.Second

	// pollInterval is how often to probe the health endpoint during startup.
	pollInterval = 50 * time.Millisecond
)

// testPassportToken is a JWT obtained from Passport in TestMain.
// JWTs are validated locally via JWKS (no rate limit), unlike API keys.
var testPassportToken string

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
		"--passport-url", testPassportURL,
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
		Client: client.New(fmt.Sprintf("http://127.0.0.1:%d", port), testPassportToken),
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

// obtainPassportJWT signs in to Passport and returns a JWT. The JWT is
// validated locally by daemons via JWKS (no rate limit, unlike API keys).
func obtainPassportJWT() (string, error) {
	hc := &http.Client{Timeout: 10 * time.Second}

	// Sign in to get a session cookie.
	signInBody, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": testPassword,
	})
	req, _ := http.NewRequest("POST", testPassportURL+"/v1/sign-in/email", bytes.NewReader(signInBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("sign-in request: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("sign-in returned %d", resp.StatusCode)
	}

	// Extract session cookie.
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "better-auth.session_token" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		return "", fmt.Errorf("no session cookie in sign-in response")
	}

	// Get JWT from the token endpoint.
	tokenReq, _ := http.NewRequest("GET", testPassportURL+"/v1/token", nil)
	tokenReq.AddCookie(sessionCookie)
	tokenResp, err := hc.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer tokenResp.Body.Close()
	if tokenResp.StatusCode != 200 {
		return "", fmt.Errorf("token endpoint returned %d", tokenResp.StatusCode)
	}

	var tokenBody struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenBody); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	return tokenBody.Token, nil
}
