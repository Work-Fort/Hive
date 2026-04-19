// SPDX-License-Identifier: GPL-3.0-or-later
package export

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/client"
	"github.com/Work-Fort/Hive/internal/infra"
	"github.com/Work-Fort/Hive/internal/transfer"
)

// NewCmd returns the export cobra command.
func NewCmd() *cobra.Command {
	var host string
	var port int
	var passportAPIKey string
	var db string

	cmd := &cobra.Command{
		Use:   "export <dir>",
		Short: "Export all entities to a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("host") {
				host = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("passport-api-key") {
				passportAPIKey = viper.GetString("passport-api-key")
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
				ds = transfer.NewRESTDataSource(client.New(baseURL, passportAPIKey))
			}

			result, err := transfer.Export(ctx, ds, dir)
			if err != nil {
				return err
			}

			fmt.Printf("Exported %d teams, %d roles, %d permissions, %d agents, %d documents, %d tasks\n",
				result.Teams, result.Roles, result.Permissions, result.Agents, result.Documents, result.Tasks)

			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Daemon host")
	cmd.Flags().IntVar(&port, "port", 17000, "Daemon port")
	cmd.Flags().StringVar(&passportAPIKey, "passport-api-key", "", "Passport API key (env: HIVE_PASSPORT_API_KEY)")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN for direct DB access")

	return cmd
}
