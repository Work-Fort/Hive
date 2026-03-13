// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"fmt"
	"net/http"

	auth "github.com/Work-Fort/Passport/go/service-auth"

	"github.com/Work-Fort/Hive/internal/domain"
)

// contextKey is an unexported type to prevent context key collisions.
type contextKey int

const (
	agentIDKey contextKey = iota
	agentKey
)

// AgentIDFromContext returns the agent ID from the context, or empty string.
func AgentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(agentIDKey).(string)
	return v
}

// AgentFromContext returns the resolved agent from the context, or nil.
func AgentFromContext(ctx context.Context) *domain.Agent {
	v, _ := ctx.Value(agentKey).(*domain.Agent)
	return v
}

// contextWithAgentID returns a new context with the agent ID set.
func contextWithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentIDKey, agentID)
}

// contextWithAgent returns a new context with the resolved agent set.
func contextWithAgent(ctx context.Context, agent *domain.Agent) context.Context {
	return context.WithValue(ctx, agentKey, agent)
}

// httpContextFunc returns a function for mcp-go's WithHTTPContextFunc that
// extracts the agent identity from the Passport context and stores it in the context.
func httpContextFunc() func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		id, ok := auth.IdentityFromContext(ctx)
		if ok {
			ctx = contextWithAgentID(ctx, id.ID)
		}
		return ctx
	}
}

// resolveAgent looks up the agent by ID from the store and returns a context
// with the agent set. Returns an error if the agent ID is missing or not found.
func resolveAgent(ctx context.Context, store domain.Store) (context.Context, error) {
	agentID := AgentIDFromContext(ctx)
	if agentID == "" {
		return ctx, fmt.Errorf("missing agent identity")
	}
	agent, err := store.GetAgent(ctx, agentID)
	if err != nil {
		return ctx, fmt.Errorf("resolve agent %q: %w", agentID, err)
	}
	return contextWithAgent(ctx, agent), nil
}
