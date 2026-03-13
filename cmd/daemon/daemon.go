// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Hive/internal/config"
	hiveDaemon "github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/infra"
)

// NewCmd returns the daemon cobra command.
func NewCmd() *cobra.Command {
	var bind string
	var port int
	var db string
	var passportURL string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Hive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("bind") {
				bind = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("db") {
				db = viper.GetString("db")
			}
			if !cmd.Flags().Changed("passport-url") {
				passportURL = viper.GetString("passport-url")
			}
			return run(bind, port, db, passportURL)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", 17000, "Listen port")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN (postgres://... or SQLite file path)")
	cmd.Flags().StringVar(&passportURL, "passport-url", "http://passport.nexus:3000",
		"Passport auth service URL")

	return cmd
}

func run(bind string, port int, db, passportURL string) error {
	health := hiveDaemon.NewHealthService()

	// Database
	dsn := db
	if dsn == "" {
		dsn = filepath.Join(config.GlobalPaths.StateDir, "hive.db")
	}

	store, err := infra.Open(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	// Seed permissions
	permNames := []string{
		"role:read", "role:write",
		"memory:read", "memory:write",
		"task:read", "task:write",
		"agent:manage", "team:manage",
	}
	if err := store.SeedPermissions(context.Background(), permNames); err != nil {
		return fmt.Errorf("seed permissions: %w", err)
	}

	// Provisioning
	maxRoleDepth := viper.GetInt("max-role-depth")
	provisioning := hiveDaemon.NewProvisioningService(store, health, maxRoleDepth)

	// Boot-time check: role depth audit
	health.RegisterBootCheck("role_depth_audit", func(ctx context.Context) hiveDaemon.CheckResult {
		provisioning.AuditRoleDepths(ctx)
		return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityOK, Message: "role depth audit complete"}
	})

	// Periodic check: database connectivity
	health.RegisterPeriodicCheck("database", func(ctx context.Context) hiveDaemon.CheckResult {
		if err := store.Ping(ctx); err != nil {
			return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityError, Message: err.Error()}
		}
		return hiveDaemon.CheckResult{Severity: hiveDaemon.SeverityOK}
	})

	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:         bind,
		Port:         port,
		PassportURL:  passportURL,
		Health:       health,
		Store:        store,
		Provisioning: provisioning,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := hiveDaemon.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Start periodic health checks
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go health.StartPeriodic(healthCtx, 30*time.Second)

	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http shutdown", "err", err)
	}

	return nil
}
