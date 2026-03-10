// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	hiveDaemon "github.com/Work-Fort/Hive/internal/daemon"
)

func NewCmd() *cobra.Command {
	var bind string
	var port int
	var db string
	var apiKey string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Hive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(bind, port, db, apiKey)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", 17000, "Listen port")
	cmd.Flags().StringVar(&db, "db", "", "Database DSN (postgres://... or SQLite file path)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for REST authentication")

	return cmd
}

func run(bind string, port int, db, apiKey string) error {
	health := hiveDaemon.NewHealthService()

	srv := hiveDaemon.NewServer(hiveDaemon.ServerConfig{
		Bind:   bind,
		Port:   port,
		Health: health,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := hiveDaemon.ListenAndServe(srv); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

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
