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

	mux.HandleFunc("GET /v1/health", HandleHealth(cfg.Health))

	// REST API routes
	rest := NewREST(cfg.Store)

	// Teams
	mux.HandleFunc("GET /v1/teams", rest.ListTeams)
	mux.HandleFunc("POST /v1/teams", rest.CreateTeam)
	mux.HandleFunc("GET /v1/teams/{id}", rest.GetTeam)
	mux.HandleFunc("PUT /v1/teams/{id}", rest.UpdateTeam)
	mux.HandleFunc("DELETE /v1/teams/{id}", rest.DeleteTeam)

	// Roles
	mux.HandleFunc("GET /v1/roles", rest.ListRoles)
	mux.HandleFunc("POST /v1/roles", rest.CreateRole)
	mux.HandleFunc("GET /v1/roles/{id}", rest.GetRole)
	mux.HandleFunc("PUT /v1/roles/{id}", rest.UpdateRole)
	mux.HandleFunc("DELETE /v1/roles/{id}", rest.DeleteRole)

	// Role documents
	mux.HandleFunc("GET /v1/roles/{id}/documents", rest.ListRoleDocuments)
	mux.HandleFunc("POST /v1/roles/{id}/documents", rest.CreateRoleDocument)

	// Standalone documents
	mux.HandleFunc("GET /v1/documents/{id}", rest.GetDocument)
	mux.HandleFunc("PUT /v1/documents/{id}", rest.UpdateDocument)
	mux.HandleFunc("DELETE /v1/documents/{id}", rest.DeleteDocument)

	// Agents
	mux.HandleFunc("GET /v1/agents", rest.ListAgents)
	mux.HandleFunc("POST /v1/agents", rest.CreateAgent)
	mux.HandleFunc("GET /v1/agents/{id}", rest.GetAgent)
	mux.HandleFunc("PUT /v1/agents/{id}", rest.UpdateAgent)
	mux.HandleFunc("DELETE /v1/agents/{id}", rest.DeleteAgent)

	// Agent roles
	mux.HandleFunc("PUT /v1/agents/{id}/roles", rest.SetAgentRoles)

	// Agent memory
	mux.HandleFunc("GET /v1/agents/{id}/memory", rest.ListAgentMemory)
	mux.HandleFunc("POST /v1/agents/{id}/memory", rest.CreateAgentMemory)

	// Tasks
	mux.HandleFunc("GET /v1/teams/{id}/tasks", rest.ListTeamTasks)
	mux.HandleFunc("POST /v1/tasks", rest.CreateTask)
	mux.HandleFunc("GET /v1/tasks/{id}", rest.GetTask)
	mux.HandleFunc("PUT /v1/tasks/{id}", rest.UpdateTask)
	mux.HandleFunc("DELETE /v1/tasks/{id}", rest.DeleteTask)

	// Permissions
	mux.HandleFunc("GET /v1/agents/{id}/permissions", rest.GetAgentPermissions)
	mux.HandleFunc("PUT /v1/agents/{id}/permissions", rest.SetAgentPermissions)

	// MCP server
	authz := NewAuthzService(cfg.Store)
	mcpHandler := NewMCPHandler(MCPDeps{
		Store:        cfg.Store,
		Provisioning: cfg.Provisioning,
		Authz:        authz,
	})
	mux.Handle("/mcp", mcpHandler)

	// Wrap with API key auth middleware
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
