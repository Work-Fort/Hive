// SPDX-License-Identifier: GPL-3.0-or-later
package validate_test

import (
	"testing"

	"github.com/Work-Fort/Hive/internal/validate"
)

func TestValidateTeam_Valid(t *testing.T) {
	if err := validate.ValidateTeam(map[string]any{"name": "eng"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTeam_MissingName(t *testing.T) {
	if err := validate.ValidateTeam(map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateTask_InvalidStatus(t *testing.T) {
	err := validate.ValidateTask(map[string]any{
		"title": "fix", "team": "eng", "status": "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestValidateTask_ValidOptionalFields(t *testing.T) {
	err := validate.ValidateTask(map[string]any{
		"title": "fix", "team": "eng", "status": "pending",
		"agent": "claude", "description": "fix the bug",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateDocument_InvalidKind(t *testing.T) {
	err := validate.ValidateDocument(map[string]any{
		"title": "doc", "kind": "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestValidateDocument_ValidRole(t *testing.T) {
	err := validate.ValidateDocument(map[string]any{
		"title": "doc", "kind": "role",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateAgent_MissingTeam(t *testing.T) {
	err := validate.ValidateAgent(map[string]any{"name": "claude"})
	if err == nil {
		t.Fatal("expected error for missing team")
	}
}

func TestValidatePermission_Valid(t *testing.T) {
	if err := validate.ValidatePermission(map[string]any{"name": "read"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRole_Valid(t *testing.T) {
	if err := validate.ValidateRole(map[string]any{"name": "dev"}); err != nil {
		t.Fatal(err)
	}
}
