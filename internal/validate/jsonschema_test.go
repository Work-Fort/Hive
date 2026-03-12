// SPDX-License-Identifier: GPL-3.0-or-later
package validate_test

import (
	"encoding/json"
	"testing"

	"github.com/Work-Fort/Hive/internal/validate"
)

func TestGenerateJSONSchema_TaskSchema(t *testing.T) {
	data, err := validate.GenerateJSONSchema(validate.TaskSchema)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v", schema["$schema"])
	}
	if schema["title"] != "Hive Task" {
		t.Errorf("title = %v", schema["title"])
	}
	if schema["type"] != "object" {
		t.Errorf("type = %v", schema["type"])
	}

	props := schema["properties"].(map[string]any)
	for _, name := range []string{"title", "team", "status", "description", "agent", "created_at", "updated_at"} {
		if _, ok := props[name]; !ok {
			t.Errorf("missing property %q", name)
		}
	}

	statusProp := props["status"].(map[string]any)
	enumVals := statusProp["enum"].([]any)
	if len(enumVals) != 3 {
		t.Errorf("status enum length = %d, want 3", len(enumVals))
	}

	required := schema["required"].([]any)
	requiredNames := make(map[string]bool)
	for _, r := range required {
		requiredNames[r.(string)] = true
	}
	for _, name := range []string{"title", "team", "status"} {
		if !requiredNames[name] {
			t.Errorf("%q should be required", name)
		}
	}

	createdAt := props["created_at"].(map[string]any)
	if createdAt["format"] != "date-time" {
		t.Errorf("created_at format = %v, want date-time", createdAt["format"])
	}
}

func TestGenerateJSONSchema_AllEntities(t *testing.T) {
	for name, schema := range validate.AllSchemas {
		data, err := validate.GenerateJSONSchema(schema)
		if err != nil {
			t.Errorf("GenerateJSONSchema(%s): %v", name, err)
			continue
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			t.Errorf("GenerateJSONSchema(%s) produced invalid JSON: %v", name, err)
		}
	}
}
