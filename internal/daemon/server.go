// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind   string
	Port   int
	Health *HealthService
	Store  domain.Store
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// MCP placeholder — will be wired to mcp-go StreamableHTTPServer in plan 005.
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "MCP server not yet implemented", http.StatusNotImplemented)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// ListenAndServe starts the server on the configured address.
func ListenAndServe(srv *http.Server) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	fmt.Printf("Hive daemon listening on %s\n", ln.Addr())
	return srv.Serve(ln)
}
