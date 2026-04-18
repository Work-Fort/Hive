// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// hiveBin is the path to the compiled hive binary, set by TestMain.
var hiveBin string

// TestMain compiles the hive binary once with -race before running any tests.
// All tests in the package share the same binary.
func TestMain(m *testing.M) {
	// Resolve the repository root relative to this file's location.
	// tests/e2e/ is two levels below the repo root.
	repoRoot, err := filepath.Abs("../../")
	if err != nil {
		panic("resolve repo root: " + err.Error())
	}

	// Build into a temp dir so we don't dirty the working tree.
	tmpDir, err := os.MkdirTemp("", "hive-e2e-*")
	if err != nil {
		panic("create temp dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	hiveBin = filepath.Join(tmpDir, "hive")

	cmd := exec.Command("go", "build", "-race", "-o", hiveBin, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stderr // build output goes to stderr so test runner shows it
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Print a clear message and exit — don't try to run tests against a
		// missing binary.
		os.Stderr.WriteString("FATAL: failed to build hive binary: " + err.Error() + "\n")
		os.Exit(1)
	}

	os.Exit(m.Run())
}
