// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Hive/internal/domain"
)

// ProvisioningService resolves agent provisioning data by composing
// role inheritance chains and agent memory into a hierarchical response.
type ProvisioningService struct {
	store        domain.Store
	health       *HealthService
	maxRoleDepth int
}

// NewProvisioningService creates a new ProvisioningService.
func NewProvisioningService(store domain.Store, health *HealthService, maxRoleDepth int) *ProvisioningService {
	if maxRoleDepth <= 0 {
		maxRoleDepth = 10 // fallback to default
	}
	return &ProvisioningService{
		store:        store,
		health:       health,
		maxRoleDepth: maxRoleDepth,
	}
}

// Resolve builds the complete provisioning response for an agent.
func (ps *ProvisioningService) Resolve(ctx context.Context, agentID string) (*domain.ProvisioningResponse, error) {
	agent, err := ps.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	var groups []domain.ProvisioningRoleGroup
	var memory []domain.Document
	var currentAssignment *domain.CurrentAssignment

	if agent.CurrentWorkflowID != "" {
		currentAssignment = &domain.CurrentAssignment{
			Role:           agent.AssignedRole,
			Project:        agent.CurrentProject,
			WorkflowID:     agent.CurrentWorkflowID,
			LeaseExpiresAt: agent.LeaseExpiresAt,
		}

		role, err := ps.store.LookupRoleByName(ctx, agent.AssignedRole)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				log.Warn("provisioning: claimed agent has unknown assigned_role",
					"agent_id", agent.ID, "assigned_role", agent.AssignedRole)
				// groups stays empty; fall through to memory synthesis.
			} else {
				return nil, fmt.Errorf("lookup current role %q: %w", agent.AssignedRole, err)
			}
		} else {
			chain, err := ps.store.GetRoleChain(ctx, role.ID, ps.maxRoleDepth)
			if err != nil {
				return nil, fmt.Errorf("get current role chain: %w", err)
			}
			entries := make([]domain.ProvisioningChainEntry, 0, len(chain))
			for _, r := range chain {
				docs, err := ps.store.ListRoleDocuments(ctx, r.ID)
				if err != nil {
					return nil, fmt.Errorf("list documents for role %s: %w", r.ID, err)
				}
				docSlice := make([]domain.Document, len(docs))
				for i, d := range docs {
					docSlice[i] = *d
				}
				entries = append(entries, domain.ProvisioningChainEntry{
					Role: r.Name, Documents: docSlice,
				})
			}
			groups = []domain.ProvisioningRoleGroup{{Priority: 0, Chain: entries}}
		}

		now := time.Now().UTC()
		memory = append(memory, domain.Document{
			ID:        "synthetic:current-project",
			Kind:      domain.DocumentKindMemory,
			Title:     "Current Project",
			Content:   fmt.Sprintf("You are currently acting as **%s** for project **%s**.\n", agent.AssignedRole, agent.CurrentProject),
			AgentID:   agent.ID,
			CreatedAt: now, UpdatedAt: now,
		})
	} else {
		agentRoles, err := ps.store.GetAgentRoles(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("get agent roles: %w", err)
		}

		groups = make([]domain.ProvisioningRoleGroup, 0, len(agentRoles))
		for _, ar := range agentRoles {
			chain, err := ps.store.GetRoleChain(ctx, ar.RoleID, ps.maxRoleDepth)
			if err != nil {
				return nil, fmt.Errorf("get role chain for %s: %w", ar.RoleID, err)
			}

			entries := make([]domain.ProvisioningChainEntry, 0, len(chain))
			for _, role := range chain {
				docs, err := ps.store.ListRoleDocuments(ctx, role.ID)
				if err != nil {
					return nil, fmt.Errorf("list documents for role %s: %w", role.ID, err)
				}

				docSlice := make([]domain.Document, len(docs))
				for i, d := range docs {
					docSlice[i] = *d
				}

				entries = append(entries, domain.ProvisioningChainEntry{
					Role:      role.Name,
					Documents: docSlice,
				})
			}

			groups = append(groups, domain.ProvisioningRoleGroup{
				Priority: ar.Priority,
				Chain:    entries,
			})
		}
	}

	memDocs, err := ps.store.ListAgentMemory(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent memory: %w", err)
	}
	for _, d := range memDocs {
		memory = append(memory, *d)
	}

	return &domain.ProvisioningResponse{
		Agent: domain.AgentIdentity{
			ID:      agent.ID,
			Name:    agent.Name,
			TeamID:  agent.TeamID,
			Model:   agent.Model,
			Runtime: agent.Runtime,
		},
		CurrentAssignment: currentAssignment,
		Roles:             groups,
		Memory:            memory,
	}, nil
}

// ValidateRoleParent checks that setting roleID's parent to newParentID
// would not create a cycle or exceed the max role depth.
func (ps *ProvisioningService) ValidateRoleParent(ctx context.Context, roleID, newParentID string) error {
	if newParentID == "" {
		return nil
	}

	if roleID != "" && roleID == newParentID {
		return fmt.Errorf("%w: role cannot be its own parent", domain.ErrCycleDetected)
	}

	ancestorChain, err := ps.store.GetRoleChain(ctx, newParentID, ps.maxRoleDepth)
	if err != nil {
		return fmt.Errorf("get ancestor chain: %w", err)
	}

	if roleID != "" {
		for _, ancestor := range ancestorChain {
			if ancestor.ID == roleID {
				return fmt.Errorf("%w: %s already appears in the ancestor chain of %s",
					domain.ErrCycleDetected, roleID, newParentID)
			}
		}
	}

	descendantDepth := 0
	if roleID != "" {
		descendantDepth, err = ps.maxDescendantDepth(ctx, roleID)
		if err != nil {
			return fmt.Errorf("get descendant depth: %w", err)
		}
	}

	totalDepth := descendantDepth + 1 + len(ancestorChain)
	if totalDepth > ps.maxRoleDepth {
		return fmt.Errorf("%w: depth %d exceeds max %d",
			domain.ErrDepthExceeded, totalDepth, ps.maxRoleDepth)
	}

	return nil
}

func (ps *ProvisioningService) maxDescendantDepth(ctx context.Context, roleID string) (int, error) {
	return ps.walkDescendants(ctx, roleID, 0)
}

func (ps *ProvisioningService) walkDescendants(ctx context.Context, roleID string, currentDepth int) (int, error) {
	if currentDepth >= ps.maxRoleDepth {
		return currentDepth, nil
	}

	children, err := ps.store.ListRoles(ctx, roleID)
	if err != nil {
		return 0, fmt.Errorf("list children of %s: %w", roleID, err)
	}

	if len(children) == 0 {
		return currentDepth, nil
	}

	maxDepth := currentDepth
	for _, child := range children {
		d, err := ps.walkDescendants(ctx, child.ID, currentDepth+1)
		if err != nil {
			return 0, err
		}
		if d > maxDepth {
			maxDepth = d
		}
	}
	return maxDepth, nil
}

// AuditRoleDepths scans all role chains at boot time and reports any that
// exceed maxRoleDepth to the HealthService as warnings.
func (ps *ProvisioningService) AuditRoleDepths(ctx context.Context) {
	roles, err := ps.store.ListRoles(ctx, "")
	if err != nil {
		ps.health.AddError(fmt.Sprintf("role depth audit failed: %v", err))
		return
	}

	checked := 0
	violations := 0
	for _, role := range roles {
		chain, err := ps.store.GetRoleChain(ctx, role.ID, ps.maxRoleDepth+1)
		if err != nil {
			ps.health.AddWarning(fmt.Sprintf("role depth audit: failed to get chain for role %q: %v", role.Name, err))
			continue
		}
		checked++
		if len(chain) > ps.maxRoleDepth {
			violations++
			ps.health.AddWarning(fmt.Sprintf(
				"role %q has chain depth %d (max %d)",
				role.Name, len(chain), ps.maxRoleDepth,
			))
		}
	}

	log.Debug("role depth audit complete", "roles_checked", checked, "violations", violations)
}
