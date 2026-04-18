// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", path)
	}
}

// TestExportImportRoundTrip seeds a full set of entities, exports them via the
// CLI, verifies the directory layout, then imports into a fresh database and
// checks that no errors occur.
func TestExportImportRoundTrip(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// --- Seed data ---

	team, err := c.CreateTeam(ctx(), "engineering")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	role, err := c.CreateRole(ctx(), "developer", "")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000002", "claude", team.ID, "", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	_, err = c.SetAgentRoles(ctx(), agent.ID, []client.RoleAssignment{
		{RoleID: role.ID, Priority: 1},
	})
	if err != nil {
		t.Fatalf("SetAgentRoles: %v", err)
	}

	_, err = c.CreateRoleDocument(ctx(), role.ID, "Standards", "# Coding Standards")
	if err != nil {
		t.Fatalf("CreateRoleDocument: %v", err)
	}

	_, err = c.CreateTask(ctx(), client.CreateTaskInput{
		TeamID: team.ID,
		Title:  "Fix bug",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// --- Export via CLI ---

	exportDir := filepath.Join(t.TempDir(), "export")
	portStr := fmt.Sprintf("%d", h.port)

	exportToken := h.SignJWT("00000000-0000-0000-0000-000000000000", "e2e-test", "E2E Test User", "user")
	exportCmd := exec.Command(hiveBin, "export", exportDir,
		"--host", "127.0.0.1",
		"--port", portStr,
		"--passport-token", exportToken,
		"--log-level", "disabled",
	)
	exportOut, err := exportCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export command failed: %v\noutput: %s", err, exportOut)
	}

	// --- Verify export directory structure ---

	assertFileExists(t, filepath.Join(exportDir, "teams", "engineering.yaml"))
	assertFileExists(t, filepath.Join(exportDir, "roles", "developer.yaml"))
	assertFileExists(t, filepath.Join(exportDir, "agents", "claude.yaml"))
	assertFileExists(t, filepath.Join(exportDir, "documents", "role-developer--standards.md"))
	assertFileExists(t, filepath.Join(exportDir, "tasks", "engineering--fix-bug.yaml"))

	// --- Import into fresh DB ---

	freshDBPath := filepath.Join(t.TempDir(), "fresh.db")

	importCmd := exec.Command(hiveBin, "import", exportDir,
		"--db", freshDBPath,
		"--log-level", "disabled",
	)
	importOut, err := importCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("import command failed: %v\noutput: %s", err, importOut)
	}
}

// TestExportImportDryRun verifies that --dry-run validates the import without
// actually writing any data.
func TestExportImportDryRun(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	_, err := c.CreateTeam(ctx(), "eng")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	// --- Export ---

	exportDir := filepath.Join(t.TempDir(), "export")
	portStr := fmt.Sprintf("%d", h.port)

	exportToken := h.SignJWT("00000000-0000-0000-0000-000000000000", "e2e-test", "E2E Test User", "user")
	exportCmd := exec.Command(hiveBin, "export", exportDir,
		"--host", "127.0.0.1",
		"--port", portStr,
		"--passport-token", exportToken,
		"--log-level", "disabled",
	)
	exportOut, err := exportCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export command failed: %v\noutput: %s", err, exportOut)
	}

	// --- Import with --dry-run ---

	freshDBPath := filepath.Join(t.TempDir(), "dry-run.db")

	importCmd := exec.Command(hiveBin, "import", exportDir,
		"--db", freshDBPath,
		"--dry-run",
		"--log-level", "disabled",
	)
	importOut, err := importCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("import --dry-run command failed: %v\noutput: %s", err, importOut)
	}

	output := string(importOut)
	if !strings.Contains(output, "Dry run") {
		t.Errorf("expected output to contain 'Dry run', got: %s", output)
	}
}

// TestImportConflictAndUpsert verifies that importing duplicate data into the
// same daemon fails without --upsert, and succeeds with --upsert.
func TestImportConflictAndUpsert(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	_, err := c.CreateTeam(ctx(), "eng")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	// --- Export ---

	exportDir := filepath.Join(t.TempDir(), "export")
	portStr := fmt.Sprintf("%d", h.port)
	conflictToken := h.SignJWT("00000000-0000-0000-0000-000000000000", "e2e-test", "E2E Test User", "user")

	exportCmd := exec.Command(hiveBin, "export", exportDir,
		"--host", "127.0.0.1",
		"--port", portStr,
		"--passport-token", conflictToken,
		"--log-level", "disabled",
	)
	exportOut, err := exportCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export command failed: %v\noutput: %s", err, exportOut)
	}

	// --- Import into same daemon (should fail — conflict) ---

	importConflict := exec.Command(hiveBin, "import", exportDir,
		"--host", "127.0.0.1",
		"--port", portStr,
		"--passport-token", conflictToken,
		"--log-level", "disabled",
	)
	conflictOut, err := importConflict.CombinedOutput()
	if err == nil {
		t.Fatalf("expected import without --upsert to fail, but it succeeded\noutput: %s", conflictOut)
	}

	// --- Import with --upsert into same daemon (should succeed) ---

	importUpsert := exec.Command(hiveBin, "import", exportDir,
		"--host", "127.0.0.1",
		"--port", portStr,
		"--passport-token", conflictToken,
		"--upsert",
		"--log-level", "disabled",
	)
	upsertOut, err := importUpsert.CombinedOutput()
	if err != nil {
		t.Fatalf("import --upsert command failed: %v\noutput: %s", err, upsertOut)
	}
}
