# Validation Design Spec

## Goal

Add structured validation to all Hive entity input boundaries — REST API, MCP tools, and file import — with a single source of truth for field rules. Validate markdown content with CommonMark parsing. Generate JSON Schema for IDE integration.

## Scope

Six entity types: teams, roles, permissions, agents, documents, tasks. Three input boundaries: REST (Huma), MCP tools, file import. One CLI command: `hive schema <entity>`.

## Architecture

### Approach: Domain-Level Validation Package

A shared `internal/validate` package defines entity schemas as a registry of field definitions (following the anvil `ConfigKeyDefinition` pattern). Each entity type has a schema struct with required fields, types, enum values, and patterns. A `Validate()` function takes a schema and `map[string]any` fields, returning structured errors. Each boundary formats errors for its protocol.

Markdown content is validated separately via goldmark (CommonMark parser). JSON Schema Draft 2020-12 generation reads from the same registry — single source of truth.

## Core Validation Model

### Field Definitions

```go
type FieldDef struct {
    Name     string   // field name (YAML key — see Field Naming below)
    Type     string   // "string", "int", "time"
    Required bool     // must be non-zero
    Enum     []string // allowed values (nil = any)
    Pattern  string   // regex pattern (empty = none)
}

type EntitySchema struct {
    Name   string     // "team", "role", "agent", "document", "task", "permission"
    Fields []FieldDef
}
```

### Field Naming Convention

`FieldDef.Name` uses **YAML export keys** (the canonical human-readable format). These match the field names in `internal/transfer/yaml.go` structs: `"team"`, `"name"`, `"status"`, etc. — not the REST JSON keys (`"team_id"`) or domain struct fields (`TeamID`).

This is a deliberate choice: the validate package is the source of truth for what values are valid, and YAML keys are the most user-facing representation. At callsites, callers map their field names to the YAML key when building the `map[string]any`:

```go
// MCP handler — maps extracted params to YAML keys
validate.ValidateTask(map[string]any{"title": title, "team": teamName, "status": status})

// Import — struct fields already match YAML keys
validate.ValidateTask(map[string]any{"title": tf.Title, "team": tf.Team, "status": tf.Status})
```

### Validation Result

```go
type FieldError struct {
    Field   string // "status", "kind", "name"
    Message string // "must be one of: pending, in_progress, completed"
}

type ValidationError struct {
    Entity string       // "task", "document"
    Errors []FieldError
}

func (e *ValidationError) Error() string // implements error
```

`ValidationError.Error()` produces: `validation failed for task: status must be one of: pending, in_progress, completed; title is required`

### Entity Schemas

One `EntitySchema` per entity type, defined as package-level vars:

| Entity | Required Fields | Enum Constraints |
|--------|----------------|-----------------|
| Team | `name` | — |
| Role | `name` | — |
| Permission | `name` | — |
| Agent | `name`, `team` | — |
| Document | `title`, `kind` | `kind`: `role`, `memory` |
| Task | `title`, `team`, `status` | `status`: `pending`, `in_progress`, `completed` |

All entities also include optional `created_at` and `updated_at` fields (`Type: "time"`). These are not required (REST/MCP create paths don't set them), but are included in the schema so JSON Schema generation covers them.

Additional optional fields per entity:

- **Role**: `parent` (string, optional — references parent role name)
- **Agent**: `roles` (not validated by field schema — cross-reference validation stays in the importer), `permissions` (same)
- **Document**: `role` (string, optional — set when `kind=role`), `agent` (string, optional — set when `kind=memory`), `content` (string, optional — validated separately by `Markdown()`)
- **Task**: `agent` (string, optional), `description` (string, optional)

Full example:

```go
var TaskSchema = EntitySchema{
    Name: "task",
    Fields: []FieldDef{
        {Name: "title", Type: "string", Required: true},
        {Name: "team", Type: "string", Required: true},
        {Name: "status", Type: "string", Required: true, Enum: []string{"pending", "in_progress", "completed"}},
        {Name: "description", Type: "string"},
        {Name: "agent", Type: "string"},
        {Name: "created_at", Type: "time"},
        {Name: "updated_at", Type: "time"},
    },
}
```

### Validate Function

```go
func Validate(schema EntitySchema, fields map[string]any) *ValidationError
```

Takes an entity schema and a `map[string]any` (generic — works for both parsed YAML and programmatic input). Checks:

1. Required fields are present and non-zero
2. String fields with `Enum` have an allowed value
3. String fields with `Pattern` match the regex
4. Type assertions match expected types

**Note on `map[string]any`**: The import path already has typed structs (`TaskFile`, `AgentFile`, etc.) which must be converted to `map[string]any` to call `Validate()`. This is a deliberate tradeoff — a small amount of map-building at callsites in exchange for a single validation code path. The struct's compile-time type safety handles field naming; the validate function handles value constraints. The alternative (typed validate functions per struct) would duplicate validation logic across the import, MCP, and any future input paths.

### Convenience Wrappers

```go
func ValidateTeam(fields map[string]any) *ValidationError
func ValidateRole(fields map[string]any) *ValidationError
func ValidatePermission(fields map[string]any) *ValidationError
func ValidateAgent(fields map[string]any) *ValidationError
func ValidateDocument(fields map[string]any) *ValidationError
func ValidateTask(fields map[string]any) *ValidationError
```

Each calls `Validate(XxxSchema, fields)` internally. Keeps callsites clean.

### When Validation Runs

- **Create**: Validation runs on all fields before the entity is created.
- **Upsert (import)**: Validation runs on the file's fields before either creating or updating. This ensures imported data meets constraints regardless of whether the entity already exists.
- **Update (REST/MCP)**: Huma struct tags validate provided fields. For MCP, validation runs on the fields being updated (not the full entity — partial updates only validate what's present).
- **Dry-run (import)**: Validation runs unconditionally — `--dry-run` skips store operations but not validation. This ensures dry-run catches the same errors a live import would.

## Markdown Validation

```go
func Markdown(content string) error
```

Uses goldmark (CommonMark-compliant Go parser) to parse the content into an AST.

**Current behavior**: CommonMark is extremely permissive — almost any string is valid CommonMark. The `Markdown()` function will rarely return errors today. Its primary purpose is to establish the validation hook at all input boundaries so that future linting rules can be added without touching REST handlers, MCP tools, or import logic. The function also catches genuine parse failures (e.g. encoding issues).

```go
func Markdown(content string) error {
    md := goldmark.New()
    var buf bytes.Buffer
    if err := md.Convert([]byte(content), &buf); err != nil {
        return fmt.Errorf("invalid markdown: %w", err)
    }
    return nil
}
```

**Future linting**: The goldmark AST can be walked to enforce style rules (no raw HTML, headings start at h1, etc.) in a later iteration. The function signature stays the same; AST checks are added internally when linting rules are defined. This is when `Markdown()` becomes a meaningful validator rather than a pass-through.

**Where it's called**:
- REST: via Huma `Resolver` on document input types
- MCP: in `create_memory` and `update_memory` tool handlers
- Import: in `importDocument` before creating the entity

## Huma Integration

Two integration points — no duplication of validation logic.

### 1. Struct Tags for Native Huma Validation

Add `enum` tags to existing input types. This provides both runtime validation and OpenAPI spec generation:

```go
// In rest_types.go
type CreateTaskInput struct {
    Body struct {
        // ...
        Status string `json:"status,omitempty" doc:"Status" enum:"pending,in_progress,completed"`
    }
}
```

### 2. Resolver Interface for Markdown Validation

Input types that accept markdown content implement `huma.Resolver` to call the shared `validate.Markdown()` function.

**Types that need `Resolve` method**: `CreateRoleDocumentInput`, `CreateAgentMemoryInput`, `UpdateDocumentInput`.

```go
func (i *CreateRoleDocumentInput) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
    if err := validate.Markdown(i.Body.Content); err != nil {
        return []error{&huma.ErrorDetail{
            Location: prefix.With("content"),
            Message:  err.Error(),
            Value:    i.Body.Content,
        }}
    }
    return nil
}
```

This runs automatically before the handler. If markdown is invalid, Huma returns a 422 with structured errors. No handler changes needed.

### Validation Matrix

| Check | Huma (REST) | MCP | Import |
|-------|-------------|-----|--------|
| Required fields | Struct tags (`minLength`) | `validate.Validate()` | `validate.Validate()` |
| Enum values | Struct tags (`enum`) | `validate.Validate()` | `validate.Validate()` |
| Markdown parse | `Resolver` calls `validate.Markdown()` | `validate.Markdown()` | `validate.Markdown()` |

The validate package is the single source of truth for rules. Huma struct tags mirror the enum/pattern values for OpenAPI generation, but validation for MCP and import goes through the shared `Validate()` function.

## MCP Integration

Validation calls added to tool handlers in `mcp_tools.go`. The `*ValidationError` is formatted as an MCP tool error string:

```go
// In create_memory handler, after extracting fields:
if err := validate.ValidateDocument(map[string]any{
    "title": title, "content": content, "kind": "memory",
}); err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}
```

## Import Integration

Validation runs in the importer after parsing each file, before any store operations (including upsert lookups). This applies to both live import and `--dry-run`:

```go
// In Import(), after parseDir for tasks:
for _, tf := range tasks {
    if err := validate.ValidateTask(map[string]any{
        "title": tf.Title, "team": tf.Team, "status": tf.Status,
    }); err != nil {
        return nil, fmt.Errorf("validate task %q: %w", tf.Title, err)
    }
}
```

For documents, markdown validation also runs:

```go
for _, doc := range documents {
    if err := validate.ValidateDocument(map[string]any{
        "title": doc.FM.Title, "kind": doc.FM.Kind,
    }); err != nil {
        return nil, fmt.Errorf("validate document %q: %w", doc.FM.Title, err)
    }
    if err := validate.Markdown(doc.Body); err != nil {
        return nil, fmt.Errorf("validate document %q content: %w", doc.FM.Title, err)
    }
}
```

## JSON Schema Generation

### Generator Function

```go
func GenerateJSONSchema(schema EntitySchema) ([]byte, error)
```

Takes an `EntitySchema`, produces a JSON Schema Draft 2020-12 document.

Field type mapping:

| FieldDef.Type | JSON Schema |
|---------------|-------------|
| `"string"` | `"type": "string"` |
| `"int"` | `"type": "integer"` |
| `"time"` | `"type": "string", "format": "date-time"` |

`Required: true` fields are added to the `"required"` array. `Enum` and `Pattern` values map directly to their JSON Schema equivalents.

### Output Example

`hive schema task` produces:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Hive Task",
  "type": "object",
  "properties": {
    "title": { "type": "string" },
    "team": { "type": "string" },
    "status": { "type": "string", "enum": ["pending", "in_progress", "completed"] },
    "agent": { "type": "string" },
    "description": { "type": "string" },
    "created_at": { "type": "string", "format": "date-time" },
    "updated_at": { "type": "string", "format": "date-time" }
  },
  "required": ["title", "team", "status"],
  "additionalProperties": false
}
```

### CLI Command

`cmd/schema/schema.go` — `hive schema <entity>` writes JSON Schema to stdout. Registered as a Cobra subcommand in `cmd/root.go`, following the same pattern as `cmd/export` and `cmd/importcmd`.

```
hive schema team
hive schema role
hive schema agent
hive schema document
hive schema task
hive schema permission
```

Entity name is validated against known schemas. Pipe to file for IDE integration.

**Document schema**: Covers front-matter fields only. The markdown body is validated by goldmark, not JSON Schema.

## Error Formatting

Each consumer formats `*ValidationError` for its protocol:

- **REST**: Never sees `ValidationError` directly — Huma tags + `Resolver` handle it with structured 422 responses
- **MCP**: Wraps in `mcp.NewToolResultError(err.Error())`
- **Import**: Wraps in `fmt.Errorf` with entity context

## Package Layout

```
internal/
  validate/
    schema.go       # FieldDef, EntitySchema, Validate(), ValidationError
    entities.go     # TeamSchema, RoleSchema, ..., convenience wrappers
    markdown.go     # Markdown() using goldmark
    jsonschema.go   # GenerateJSONSchema()
cmd/
  schema/
    schema.go       # hive schema <entity> CLI command (registered in cmd/root.go)
```

## Testing

**Unit tests** for `internal/validate`:

- `schema_test.go` — test `Validate()` with valid/invalid inputs per entity type: missing required fields, invalid enum values, pattern mismatches
- `markdown_test.go` — test `Markdown()` with valid CommonMark, empty string, edge cases
- `jsonschema_test.go` — test `GenerateJSONSchema()` produces valid Draft 2020-12 output with correct required fields and enum values

**Integration** (via existing test suites):

- REST: Send invalid payloads in E2E tests — verify 422 responses for bad status values and unparseable markdown
- MCP: Add cases to `mcp_tools_test.go` — submit invalid content, verify tool error returned
- Import: Add cases to `importer_test.go` — import files with invalid status/kind values, verify validation errors

## Dependencies

- `github.com/yuin/goldmark` — CommonMark parser (zero external deps, fuzz-tested)
- No new dependencies for JSON Schema generation (hand-built from registry, like anvil)
