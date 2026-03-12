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
		{Name: "content", Type: "string"},
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
func ValidateTeam(fields map[string]any) error { return Validate(TeamSchema, fields) }

// ValidateRole validates role fields.
func ValidateRole(fields map[string]any) error { return Validate(RoleSchema, fields) }

// ValidatePermission validates permission fields.
func ValidatePermission(fields map[string]any) error { return Validate(PermissionSchema, fields) }

// ValidateAgent validates agent fields.
func ValidateAgent(fields map[string]any) error { return Validate(AgentSchema, fields) }

// ValidateDocument validates document front-matter fields.
func ValidateDocument(fields map[string]any) error { return Validate(DocumentSchema, fields) }

// ValidateTask validates task fields.
func ValidateTask(fields map[string]any) error { return Validate(TaskSchema, fields) }
