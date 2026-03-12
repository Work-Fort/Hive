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
func Validate(schema EntitySchema, fields map[string]any) error {
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
