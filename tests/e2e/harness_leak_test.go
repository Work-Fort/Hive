// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestHarnessClose_KillsProcessGroup(t *testing.T) {
	h := newHarness(t)
	pid := h.cmd.Process.Pid

	// pgid must equal pid because newHarness sets Setpgid.
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid(%d): %v", pid, err)
	}
	if pgid != pid {
		t.Fatalf("daemon pgid = %d, want %d (Setpgid not set)", pgid, pid)
	}
	// Defence against the (vanishingly rare) case where the test
	// process itself is in a group whose id equals the daemon PID —
	// pgid == pid would pass spuriously.
	if pgid == os.Getpid() {
		t.Fatalf("daemon pgid (%d) equals harness pid; daemon inherited harness group", pgid)
	}

	// newHarness already registers t.Cleanup(h.Close); call it
	// explicitly so the assertion below runs after the daemon is
	// gone, not after the test function returns.
	h.Close()

	// Use errors.Is (not direct ==) because syscall.Errno implements
	// the errors.Is contract and errors.Is is the idiomatic Go choice.
	if err := syscall.Kill(-pgid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill(-%d, 0) = %v, want ESRCH (group still has live members)", pgid, err)
	}
}
