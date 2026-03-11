// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Bind         string
	Port         int
	APIKey       string
	Health       *HealthService
	Store        domain.Store
	Provisioning *ProvisioningService
}

// NewServer creates and configures the HTTP server.
func NewServer(cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Huma API — registers /openapi and /docs automatically on the mux.
	config := huma.DefaultConfig("Hive API", "1.0.0")
	api := humago.New(mux, config)

	// REST API routes via Huma
	registerTeamRoutes(api, cfg.Store)
	registerRoleRoutes(api, cfg.Store)
	registerDocumentRoutes(api, cfg.Store)
	registerAgentRoutes(api, cfg.Store)
	registerTaskRoutes(api, cfg.Store)
	registerPermissionRoutes(api, cfg.Store)

	// Health — raw handler (conditional status codes 200/218/503)
	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// MCP server — raw handler (JSON-RPC 2.0, not REST)
	authz := NewAuthzService(cfg.Store)
	mcpHandler := NewMCPHandler(MCPDeps{
		Store:        cfg.Store,
		Provisioning: cfg.Provisioning,
		Authz:        authz,
	})
	mux.Handle("/mcp", mcpHandler)

	// Wrap with API key auth middleware.
	// Middleware already skips non-/v1/ paths, so /openapi and /docs are public.
	handler := APIKeyAuth(cfg.APIKey, mux)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      handler,
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
