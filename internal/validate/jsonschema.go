// SPDX-License-Identifier: GPL-3.0-or-later
package validate

import (
	"encoding/json"
	"fmt"
	"strings"
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
