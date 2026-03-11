# Validation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a shared validation package for all Hive entity input boundaries, with CommonMark markdown validation and JSON Schema generation.

**Architecture:** A registry of `EntitySchema` definitions in `internal/validate` drives field-level validation (`Validate()`), markdown content checking via goldmark (`Markdown()`), and JSON Schema Draft 2020-12 generation (`GenerateJSONSchema()`). Huma REST uses struct tags + `Resolver` interface; MCP and import call the validate package directly.

**Tech Stack:** Go 1.26, goldmark (CommonMark parser), Huma v2 (Resolver interface), Cobra (CLI)

**Spec:** `docs/superpowers/specs/2026-03-11-validation-design.md`

---

## Chunk 1: Core Validation Package

### Task 1: Schema types and Validate function

**Files:**
- Create: `internal/validate/schema.go`
- Create: `internal/validate/schema_test.go`

- [ ] **Step 1: Write failing tests for Validate**

Create `internal/validate/schema_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/validate/... -v -count=1 2>&1 | tail -5`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Implement schema types and Validate**

Create `internal/validate/schema.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldDef defines validation rules for a single field.
type FieldDef struct {
	Name     string   // field name (YAML key)
	Type     string   // "string", "int", "time"
	Required bool     // must be non-zero
	Enum     []string // allowed values (nil = any)
	Pattern  string   // regex pattern (empty = none)
}

// EntitySchema defines validation rules for an entity type.
type EntitySchema struct {
	Name   string     // "team", "role", "agent", "document", "task", "permission"
	Fields []FieldDef
}

// FieldError describes a single field validation failure.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError holds all field errors for an entity.
type ValidationError struct {
	Entity string
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		msgs[i] = fe.Field + " " + fe.Message
	}
	return fmt.Sprintf("validation failed for %s: %s", e.Entity, strings.Join(msgs, "; "))
}

// Validate checks the given fields against the schema. Returns nil if valid.
func Validate(schema EntitySchema, fields map[string]any) *ValidationError {
	var errs []FieldError

	for _, fd := range schema.Fields {
		val, present := fields[fd.Name]

		if fd.Required {
			if !present || isZero(val) {
				errs = append(errs, FieldError{Field: fd.Name, Message: "is required"})
				continue
			}
		}

		if !present || isZero(val) {
			continue
		}

		if fd.Type == "string" || fd.Type == "time" {
			str, ok := val.(string)
			if !ok {
				errs = append(errs, FieldError{Field: fd.Name, Message: "must be a string"})
				continue
			}
			if len(fd.Enum) > 0 {
				if !contains(fd.Enum, str) {
					errs = append(errs, FieldError{Field: fd.Name, Message: fmt.Sprintf("must be one of: %s", strings.Join(fd.Enum, ", "))})
				}
			}
			if fd.Pattern != "" {
				matched, err := regexp.MatchString(fd.Pattern, str)
				if err != nil || !matched {
					errs = append(errs, FieldError{Field: fd.Name, Message: fmt.Sprintf("must match pattern %s", fd.Pattern)})
				}
			}
		}

		if fd.Type == "int" {
			if _, ok := val.(int); !ok {
				errs = append(errs, FieldError{Field: fd.Name, Message: "must be an integer"})
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{Entity: schema.Name, Errors: errs}
}

func isZero(v any) bool {
	switch val := v.(type) {
	case string:
		return val == ""
	case int:
		return val == 0
	default:
		return v == nil
	}
}

func contains(vals []string, s string) bool {
	for _, v := range vals {
		if v == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/validate/... -v -count=1 2>&1 | tail -20`
Expected: PASS — all 6 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/validate/schema.go internal/validate/schema_test.go
git commit -m "feat: add validate package with schema types and Validate function"
```

### Task 2: Entity schema definitions and convenience wrappers

**Files:**
- Create: `internal/validate/entities.go`
- Create: `internal/validate/entities_test.go`

- [ ] **Step 1: Write failing tests for entity validators**

Create `internal/validate/entities_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/validate/... -v -count=1 -run TestValidateTeam 2>&1 | tail -5`
Expected: FAIL (ValidateTeam not found)

- [ ] **Step 3: Implement entity schemas and wrappers**

Create `internal/validate/entities.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package validate

// TeamSchema defines validation rules for teams.
var TeamSchema = EntitySchema{
	Name: "team",
	Fields: []FieldDef{
		{Name: "name", Type: "string", Required: true},
		{Name: "created_at", Type: "time"},
		{Name: "updated_at", Type: "time"},
	},
}

// RoleSchema defines validation rules for roles.
var RoleSchema = EntitySchema{
	Name: "role",
	Fields: []FieldDef{
		{Name: "name", Type: "string", Required: true},
		{Name: "parent", Type: "string"},
		{Name: "created_at", Type: "time"},
		{Name: "updated_at", Type: "time"},
	},
}

// PermissionSchema defines validation rules for permissions.
var PermissionSchema = EntitySchema{
	Name: "permission",
	Fields: []FieldDef{
		{Name: "name", Type: "string", Required: true},
	},
}

// AgentSchema defines validation rules for agents.
var AgentSchema = EntitySchema{
	Name: "agent",
	Fields: []FieldDef{
		{Name: "name", Type: "string", Required: true},
		{Name: "team", Type: "string", Required: true},
		{Name: "created_at", Type: "time"},
		{Name: "updated_at", Type: "time"},
	},
}

// DocumentSchema defines validation rules for documents.
var DocumentSchema = EntitySchema{
	Name: "document",
	Fields: []FieldDef{
		{Name: "title", Type: "string", Required: true},
		{Name: "kind", Type: "string", Required: true, Enum: []string{"role", "memory"}},
		{Name: "role", Type: "string"},
		{Name: "agent", Type: "string"},
		{Name: "created_at", Type: "time"},
		{Name: "updated_at", Type: "time"},
	},
}

// TaskSchema defines validation rules for tasks.
var TaskSchema = EntitySchema{
	Name: "task",
	Fields: []FieldDef{
		{Name: "title", Type: "string", Required: true},
		{Name: "team", Type: "string", Required: true},
		{Name: "status", Type: "string", Required: true, Enum: []string{"pending", "in_progress", "completed"}},
		{Name: "description", Type: "string"},
		{Name: "content", Type: "string"},
		{Name: "agent", Type: "string"},
		{Name: "created_at", Type: "time"},
		{Name: "updated_at", Type: "time"},
	},
}

// AllSchemas maps entity names to their schemas.
var AllSchemas = map[string]EntitySchema{
	"team":       TeamSchema,
	"role":       RoleSchema,
	"permission": PermissionSchema,
	"agent":      AgentSchema,
	"document":   DocumentSchema,
	"task":       TaskSchema,
}

// ValidateTeam validates team fields.
func ValidateTeam(fields map[string]any) *ValidationError { return Validate(TeamSchema, fields) }

// ValidateRole validates role fields.
func ValidateRole(fields map[string]any) *ValidationError { return Validate(RoleSchema, fields) }

// ValidatePermission validates permission fields.
func ValidatePermission(fields map[string]any) *ValidationError {
	return Validate(PermissionSchema, fields)
}

// ValidateAgent validates agent fields.
func ValidateAgent(fields map[string]any) *ValidationError { return Validate(AgentSchema, fields) }

// ValidateDocument validates document front-matter fields.
func ValidateDocument(fields map[string]any) *ValidationError {
	return Validate(DocumentSchema, fields)
}

// ValidateTask validates task fields.
func ValidateTask(fields map[string]any) *ValidationError { return Validate(TaskSchema, fields) }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/validate/... -v -count=1 2>&1 | tail -25`
Expected: PASS — all 15 tests pass (6 from schema_test + 9 from entities_test)

- [ ] **Step 5: Commit**

```bash
git add internal/validate/entities.go internal/validate/entities_test.go
git commit -m "feat: add entity schema definitions and convenience validators"
```

### Task 3: Markdown validation with goldmark

**Files:**
- Create: `internal/validate/markdown.go`
- Create: `internal/validate/markdown_test.go`

- [ ] **Step 1: Add goldmark dependency**

Run: `go get github.com/yuin/goldmark`

- [ ] **Step 2: Write failing tests for Markdown**

Create `internal/validate/markdown_test.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package validate_test

import (
	"testing"

	"github.com/Work-Fort/Hive/internal/validate"
)

func TestMarkdown_ValidContent(t *testing.T) {
	cases := []string{
		"# Hello\n\nSome text.",
		"Plain text without any formatting.",
		"- item 1\n- item 2\n- item 3",
		"```go\nfunc main() {}\n```",
		"",
	}
	for _, content := range cases {
		if err := validate.Markdown(content); err != nil {
			t.Errorf("Markdown(%q) = %v, want nil", content, err)
		}
	}
}

func TestMarkdown_EmptyString(t *testing.T) {
	if err := validate.Markdown(""); err != nil {
		t.Fatalf("empty string should be valid: %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/validate/... -v -count=1 -run TestMarkdown 2>&1 | tail -5`
Expected: FAIL (Markdown not defined)

- [ ] **Step 4: Implement Markdown validation**

Create `internal/validate/markdown.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package validate

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
)

// Markdown validates that content can be parsed as CommonMark.
//
// CommonMark is very permissive — almost any string is valid. This function
// currently catches encoding issues and serves as the integration point for
// future AST-based linting rules (no raw HTML, heading structure, etc.).
func Markdown(content string) error {
	md := goldmark.New()
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return fmt.Errorf("invalid markdown: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/validate/... -v -count=1 2>&1 | tail -20`
Expected: PASS — all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/validate/markdown.go internal/validate/markdown_test.go go.mod go.sum
git commit -m "feat: add goldmark-based markdown validation"
```

### Task 4: JSON Schema generation

**Files:**
- Create: `internal/validate/jsonschema.go`
- Create: `internal/validate/jsonschema_test.go`

- [ ] **Step 1: Write failing tests for GenerateJSONSchema**

Create `internal/validate/jsonschema_test.go`:

```go
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

	// Check top-level fields
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v", schema["$schema"])
	}
	if schema["title"] != "Hive Task" {
		t.Errorf("title = %v", schema["title"])
	}
	if schema["type"] != "object" {
		t.Errorf("type = %v", schema["type"])
	}

	// Check properties exist
	props := schema["properties"].(map[string]any)
	for _, name := range []string{"title", "team", "status", "description", "agent", "created_at", "updated_at"} {
		if _, ok := props[name]; !ok {
			t.Errorf("missing property %q", name)
		}
	}

	// Check status enum
	statusProp := props["status"].(map[string]any)
	enumVals := statusProp["enum"].([]any)
	if len(enumVals) != 3 {
		t.Errorf("status enum length = %d, want 3", len(enumVals))
	}

	// Check required
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

	// Check time fields have date-time format
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/validate/... -v -count=1 -run TestGenerateJSONSchema 2>&1 | tail -5`
Expected: FAIL (GenerateJSONSchema not defined)

- [ ] **Step 3: Implement JSON Schema generation**

Create `internal/validate/jsonschema.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package validate

import (
	"encoding/json"
	"fmt"
)

// GenerateJSONSchema produces a JSON Schema Draft 2020-12 document from an EntitySchema.
func GenerateJSONSchema(schema EntitySchema) ([]byte, error) {
	properties := make(map[string]any)
	var required []string

	for _, fd := range schema.Fields {
		prop := make(map[string]any)

		switch fd.Type {
		case "string":
			prop["type"] = "string"
		case "int":
			prop["type"] = "integer"
		case "time":
			prop["type"] = "string"
			prop["format"] = "date-time"
		default:
			prop["type"] = "string"
		}

		if len(fd.Enum) > 0 {
			prop["enum"] = fd.Enum
		}
		if fd.Pattern != "" {
			prop["pattern"] = fd.Pattern
		}
		if fd.Required {
			required = append(required, fd.Name)
		}

		properties[fd.Name] = prop
	}

	title := fmt.Sprintf("Hive %s%s", strings.ToUpper(schema.Name[:1]), schema.Name[1:])

	doc := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                title,
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}

	return json.MarshalIndent(doc, "", "  ")
}
```


- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/validate/... -v -count=1 2>&1 | tail -25`
Expected: PASS — all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/validate/jsonschema.go internal/validate/jsonschema_test.go
git commit -m "feat: add JSON Schema Draft 2020-12 generation from entity schemas"
```

## Chunk 2: Integration with REST, MCP, and Import

### Task 5: Huma struct tags and Resolver integration

**Files:**
- Modify: `internal/daemon/rest_types.go` (add `enum` tags)
- Modify: `internal/daemon/rest_huma.go` (remove manual `validTaskStatus` calls; Huma enum tag handles it)

The Huma `Resolver` interface for markdown validation is added on the input types. When Huma parses the request, it calls `Resolve()` automatically before invoking the handler.

- [ ] **Step 1: Add `enum` tag to task status fields**

In `internal/daemon/rest_types.go`, add `enum` tag to `CreateTaskInput.Body.Status` and `UpdateTaskInput.Body.Status`:

```go
// CreateTaskInput — change Status line to:
Status string `json:"status,omitempty" doc:"Status: pending, in_progress, completed" enum:"pending,in_progress,completed"`

// UpdateTaskInput — change Status line to:
Status string `json:"status,omitempty" doc:"Status: pending, in_progress, completed" enum:"pending,in_progress,completed"`
```

- [ ] **Step 2: Remove manual validTaskStatus checks from rest_huma.go**

In `internal/daemon/rest_huma.go`:

- Remove the `validTaskStatus` function and its comment (lines 93-100)
- In the create-task handler (around line 582-584), remove the manual status validation block:
  ```go
  // REMOVE these lines:
  if input.Body.Status != "" && !validTaskStatus(input.Body.Status) {
      return nil, huma.NewError(http.StatusBadRequest, "invalid status; must be pending, in_progress, or completed")
  }
  ```
- In the update-task handler (around line 631-634), remove the manual status validation block:
  ```go
  // REMOVE these lines:
  if !validTaskStatus(input.Body.Status) {
      return nil, huma.NewError(http.StatusBadRequest, "invalid status; must be pending, in_progress, or completed")
  }
  ```

The Huma `enum` tag now handles this validation automatically before the handler runs.

- [ ] **Step 3: Add Resolver for markdown validation on document input types**

In `internal/daemon/rest_types.go`, add the `Resolve` methods. Add import for the validate package at the top of the file:

```go
import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Hive/internal/validate"
)
```

Then add these methods after the document input type definitions:

```go
// Resolve validates markdown content before the handler runs.
func (i *CreateRoleDocumentInput) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if i.Body.Content == "" {
		return nil
	}
	if err := validate.Markdown(i.Body.Content); err != nil {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("body.content"),
			Message:  err.Error(),
			Value:    i.Body.Content,
		}}
	}
	return nil
}

// Resolve validates markdown content before the handler runs.
func (i *CreateAgentMemoryInput) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if i.Body.Content == "" {
		return nil
	}
	if err := validate.Markdown(i.Body.Content); err != nil {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("body.content"),
			Message:  err.Error(),
			Value:    i.Body.Content,
		}}
	}
	return nil
}

// Resolve validates markdown content before the handler runs.
func (i *UpdateDocumentInput) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if i.Body.Content == "" {
		return nil
	}
	if err := validate.Markdown(i.Body.Content); err != nil {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("body.content"),
			Message:  err.Error(),
			Value:    i.Body.Content,
		}}
	}
	return nil
}
```

- [ ] **Step 4: Verify build and existing tests pass**

Run: `go build ./... && go test ./internal/daemon/... -count=1 2>&1 | tail -10`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/rest_types.go internal/daemon/rest_huma.go
git commit -m "feat: add Huma enum tags and Resolver markdown validation for REST"
```

### Task 6: MCP tool validation

**Files:**
- Modify: `internal/daemon/mcp_tools.go` (add validation calls)
- Modify: `internal/daemon/mcp_tools_test.go` (add validation test cases)

- [ ] **Step 1: Add validation tests for MCP tools**

In `internal/daemon/mcp_tools_test.go`, add test cases for invalid status in `update_task` and markdown validation in `create_memory`/`update_memory`. Read the existing test file first to follow its patterns. The tests should call the MCP tool handler directly and check for error responses.

Note: `makeCreateTask` hardcodes `Status: domain.TaskStatusPending` and does not accept a status parameter, so no create-task validation test is needed.

Add test for invalid task status on update:

```go
func TestUpdateTask_InvalidStatus_MCP(t *testing.T) {
	// Follow existing test pattern:
	// 1. Set up store with team + agent + task
	// 2. Call makeUpdateTask handler with status="bogus"
	// 3. Verify tool result is an error containing "status"
}
```

Add test for valid task update still works:

```go
func TestUpdateTask_ValidStatus_MCP(t *testing.T) {
	// 1. Set up store with team + agent + task
	// 2. Call makeUpdateTask handler with status="in_progress"
	// 3. Verify success (no error)
}
```

The implementer should read `internal/daemon/mcp_tools_test.go` to follow the exact test setup pattern (store seeding, context with agent identity, calling handler functions).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/... -v -count=1 -run TestUpdateTask_InvalidStatus_MCP 2>&1 | tail -5`
Expected: FAIL

- [ ] **Step 3: Add validation to MCP tool handlers**

In `internal/daemon/mcp_tools.go`, add import:

```go
"github.com/Work-Fort/Hive/internal/validate"
```

In `makeCreateMemory` (around line 150, after extracting `title` and `content`), add:

```go
if err := validate.Markdown(content); err != nil {
    return mcp.NewToolResultError(fmt.Sprintf("invalid content: %v", err)), nil
}
```

In `makeUpdateMemory` (around line 189, after computing `content`), add:

```go
if err := validate.Markdown(content); err != nil {
    return mcp.NewToolResultError(fmt.Sprintf("invalid content: %v", err)), nil
}
```

In `makeUpdateTask` (around line 366), the existing manual status switch at lines 366-373 already validates status. Update its error message to match the validate package format for consistency:

```go
if statusStr := request.GetString("status", ""); statusStr != "" {
    switch domain.TaskStatus(statusStr) {
    case domain.TaskStatusPending, domain.TaskStatusInProgress, domain.TaskStatusCompleted:
        updated.Status = domain.TaskStatus(statusStr)
    default:
        return mcp.NewToolResultError(fmt.Sprintf("invalid status: must be one of: pending, in_progress, completed")), nil
    }
}
```

This keeps the existing switch approach (which is the simplest correct pattern for partial updates where only one field needs validation). The validate package's enum values and the domain constants must stay in sync — both are derived from the same domain model.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/... -v -count=1 2>&1 | tail -20`
Expected: PASS — all existing + new tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/mcp_tools.go internal/daemon/mcp_tools_test.go
git commit -m "feat: add validation to MCP tool handlers"
```

### Task 7: Import validation

**Files:**
- Modify: `internal/transfer/importer.go` (add validation before entity creation)
- Modify: `internal/transfer/importer_test.go` (add validation failure test)

- [ ] **Step 1: Add import validation test**

In `internal/transfer/importer_test.go`, add a test that imports a file with an invalid task status:

```go
func TestImportValidationFailure(t *testing.T) {
	// 1. Export from a seeded store (reuse seedTestData)
	// 2. Manually edit the exported tasks YAML file to have status: "bogus"
	// 3. Import into a fresh store
	// 4. Verify error contains "status" and "must be one of"
}
```

The implementer should read `internal/transfer/importer_test.go` to follow the existing pattern (seedTestData, Export to temp dir, modify file, Import).

- [ ] **Step 2: Run tests to verify it fails**

Run: `go test ./internal/transfer/... -v -count=1 -run TestImportValidation 2>&1 | tail -5`
Expected: FAIL (validation not yet added)

- [ ] **Step 3: Add validation calls to importer**

In `internal/transfer/importer.go`, add import:

```go
"github.com/Work-Fort/Hive/internal/validate"
```

Add validation after each `parseDir` / `parseDocuments` call, before the import loops. Add these validation passes after Phase 1 (parsing) and before Phase 2 (import teams):

```go
// Validate all parsed entities
for _, tf := range teams {
    if err := validate.ValidateTeam(map[string]any{"name": tf.Name}); err != nil {
        return nil, fmt.Errorf("validate team %q: %w", tf.Name, err)
    }
}
for _, rf := range roles {
    if err := validate.ValidateRole(map[string]any{"name": rf.Name}); err != nil {
        return nil, fmt.Errorf("validate role %q: %w", rf.Name, err)
    }
}
for _, pf := range permissions {
    if err := validate.ValidatePermission(map[string]any{"name": pf.Name}); err != nil {
        return nil, fmt.Errorf("validate permission %q: %w", pf.Name, err)
    }
}
for _, af := range agents {
    if err := validate.ValidateAgent(map[string]any{"name": af.Name, "team": af.Team}); err != nil {
        return nil, fmt.Errorf("validate agent %q: %w", af.Name, err)
    }
}
for _, doc := range documents {
    if err := validate.ValidateDocument(map[string]any{"title": doc.FM.Title, "kind": doc.FM.Kind}); err != nil {
        return nil, fmt.Errorf("validate document %q: %w", doc.FM.Title, err)
    }
    if err := validate.Markdown(doc.Body); err != nil {
        return nil, fmt.Errorf("validate document %q content: %w", doc.FM.Title, err)
    }
}
for _, tf := range tasks {
    if err := validate.ValidateTask(map[string]any{
        "title": tf.Title, "team": tf.Team, "status": tf.Status,
    }); err != nil {
        return nil, fmt.Errorf("validate task %q: %w", tf.Title, err)
    }
}
```

This validation block runs unconditionally — both for live import and `--dry-run`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transfer/... -v -count=1 2>&1 | tail -25`
Expected: PASS — all existing + new tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/transfer/importer.go internal/transfer/importer_test.go
git commit -m "feat: add entity and markdown validation to import path"
```

## Chunk 3: CLI Command and E2E Tests

### Task 8: hive schema CLI command

**Files:**
- Create: `cmd/schema/schema.go`
- Modify: `cmd/root.go` (register subcommand)

- [ ] **Step 1: Create schema command**

Create `cmd/schema/schema.go`:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
package schema

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Work-Fort/Hive/internal/validate"
)

// NewCmd returns the schema cobra command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema <entity>",
		Short: "Print JSON Schema for an entity type",
		Long: fmt.Sprintf(
			"Print JSON Schema Draft 2020-12 for the specified entity type.\n\nAvailable entities: %s",
			strings.Join(entityNames(), ", "),
		),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			s, ok := validate.AllSchemas[name]
			if !ok {
				return fmt.Errorf("unknown entity %q; available: %s", name, strings.Join(entityNames(), ", "))
			}
			data, err := validate.GenerateJSONSchema(s)
			if err != nil {
				return fmt.Errorf("generate schema: %w", err)
			}
			_, err = os.Stdout.Write(data)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout) // trailing newline
			return nil
		},
	}
	return cmd
}

func entityNames() []string {
	names := make([]string, 0, len(validate.AllSchemas))
	for k := range validate.AllSchemas {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
```

- [ ] **Step 2: Register in root command**

In `cmd/root.go`, add import:

```go
schemaCmd "github.com/Work-Fort/Hive/cmd/schema"
```

Add in `init()` after existing `AddCommand` calls:

```go
rootCmd.AddCommand(schemaCmd.NewCmd())
```

- [ ] **Step 3: Verify build and test**

Run: `go build ./... && go test ./... 2>&1 | tail -10`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/schema/schema.go cmd/root.go
git commit -m "feat: add hive schema CLI command for JSON Schema generation"
```

### Task 9: E2E validation tests

**Files:**
- Modify: `tests/e2e/` (add validation test cases to existing or new file)

- [ ] **Step 1: Write E2E test for invalid task status via REST**

Add to a new `tests/e2e/validation_test.go`:

```go
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
```

- [ ] **Step 3: Run E2E tests**

Run: `cd tests/e2e && go test -v -count=1 -run "TestCreateTask_InvalidStatus_REST|TestSchemaCommand" -timeout 120s 2>&1 | tail -30`
Expected: PASS

- [ ] **Step 4: Run full E2E suite**

Run: `cd tests/e2e && go test -v -count=1 -timeout 120s 2>&1 | tail -80`
Expected: PASS — all existing + new tests pass

- [ ] **Step 5: Commit**

```bash
git add tests/e2e/
git commit -m "test: add E2E tests for validation and schema command"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Schema types and Validate function | `internal/validate/schema.go` |
| 2 | Entity schemas and convenience wrappers | `internal/validate/entities.go` |
| 3 | Markdown validation with goldmark | `internal/validate/markdown.go` |
| 4 | JSON Schema generation | `internal/validate/jsonschema.go` |
| 5 | Huma struct tags and Resolver integration | `internal/daemon/rest_types.go`, `rest_huma.go` |
| 6 | MCP tool validation | `internal/daemon/mcp_tools.go` |
| 7 | Import validation | `internal/transfer/importer.go` |
| 8 | hive schema CLI command | `cmd/schema/schema.go`, `cmd/root.go` |
| 9 | E2E validation tests | `tests/e2e/` |

Total: 9 tasks across 3 chunks. Each task produces a working, testable commit.
