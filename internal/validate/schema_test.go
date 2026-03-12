// SPDX-License-Identifier: GPL-3.0-or-later
package validate_test

import (
	"strings"
	"testing"

	"github.com/Work-Fort/Hive/internal/validate"
)

var testSchema = validate.EntitySchema{
	Name: "widget",
	Fields: []validate.FieldDef{
		{Name: "name", Type: "string", Required: true},
		{Name: "status", Type: "string", Required: true, Enum: []string{"active", "inactive"}},
		{Name: "count", Type: "int"},
		{Name: "label", Type: "string", Pattern: "^[a-z]+$"},
	},
}

func TestValidate_Valid(t *testing.T) {
	err := validate.Validate(testSchema, map[string]any{
		"name": "foo", "status": "active",
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidate_MissingRequired(t *testing.T) {
	err := validate.Validate(testSchema, map[string]any{
		"status": "active",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention 'name': %s", err.Error())
	}
}

func TestValidate_InvalidEnum(t *testing.T) {
	err := validate.Validate(testSchema, map[string]any{
		"name": "foo", "status": "broken",
	})
	if err == nil {
		t.Fatal("expected error for invalid enum")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error should mention 'status': %s", err.Error())
	}
}

func TestValidate_PatternMismatch(t *testing.T) {
	err := validate.Validate(testSchema, map[string]any{
		"name": "foo", "status": "active", "label": "UPPER",
	})
	if err == nil {
		t.Fatal("expected error for pattern mismatch")
	}
	if !strings.Contains(err.Error(), "label") {
		t.Errorf("error should mention 'label': %s", err.Error())
	}
}

func TestValidate_PatternMatch(t *testing.T) {
	err := validate.Validate(testSchema, map[string]any{
		"name": "foo", "status": "active", "label": "lower",
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	err := validate.Validate(testSchema, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
	ve := err.(*validate.ValidationError)
	if len(ve.Errors) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(ve.Errors))
	}
}
