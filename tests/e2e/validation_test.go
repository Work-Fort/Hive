// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"encoding/json"
	"errors"
	"os/exec"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestCreateTask_InvalidStatus_REST(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "eng")
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.CreateTask(ctx(), client.CreateTaskInput{
		TeamID: team.ID,
		Title:  "bad task",
		Status: "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !errors.Is(err, client.ErrUnprocessable) {
		t.Errorf("expected ErrUnprocessable, got %v", err)
	}
}

func TestSchemaCommand(t *testing.T) {
	// No harness needed — schema is a local command.
	// Pass --log-level disabled to avoid PersistentPreRunE side effects.
	cmd := exec.Command(hiveBin, "schema", "task", "--log-level", "disabled")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema command failed: %s\n%s", err, out)
	}

	var schema map[string]any
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if schema["title"] != "Hive Task" {
		t.Errorf("title = %v", schema["title"])
	}

	// Invalid entity should fail
	cmd = exec.Command(hiveBin, "schema", "nonexistent", "--log-level", "disabled")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected error for unknown entity")
	}
}
