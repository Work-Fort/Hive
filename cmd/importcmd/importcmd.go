// SPDX-License-Identifier: GPL-3.0-or-later
package importcmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/client"
	"github.com/Work-Fort/Hive/internal/infra"
	"github.com/Work-Fort/Hive/internal/transfer"
)

// NewCmd returns the import cobra command.
func NewCmd() *cobra.Command {
	var host string
	var port int
	var apiKey string
	var db string
	var upsert bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "import <dir>",
		Short: "Import entities from a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("host") {
				host = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("api-key") {
				apiKey = viper.GetString("api-key")
			}

			dir := args[0]
			ctx := cmd.Context()

			var ds transfer.DataSource
			if db != "" {
				store, err := infra.Open(db)
				if err != nil {
					return fmt.Errorf("open database: %w", err)
				}
				defer store.Close()
				ds = transfer.NewDBDataSource(store)
			} else {
				baseURL := fmt.Sprintf("http://%s:%d", host, port)
				ds = transfer.NewRESTDataSource(client.New(baseURL, apiKey))
			}

			opts := transfer.ImportOptions{
				Upsert: upsert,
				DryRun: dryRun,
			}

			result, err := transfer.Import(ctx, ds, dir, opts)
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Println("Dry run results:")
			} else {
				fmt.Println("Imported:")
			}
			fmt.Printf("  teams:       %d create\n", result.Teams)
			fmt.Printf("  roles:       %d create\n", result.Roles)
			fmt.Printf("  permissions: %d create\n", result.Permissions)
			fmt.Printf("  agents:      %d create\n", result.Agents)
			fmt.Printf("  documents:   %d create\n", result.Documents)
			fmt.Printf("  tasks:       %d create\n", result.Tasks)
			if result.Updated > 0 {
				fmt.Printf("  updated:     %d total\n", result.Updated)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Daemon host")
	cmd.Flags().IntVar(&port, "port", 17000, "Daemon port")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for REST authentication")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN for direct DB access")
	cmd.Flags().BoolVar(&upsert, "upsert", false, "Update existing entities instead of failing")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate only, don't create")

	return cmd
}
