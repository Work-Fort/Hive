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
