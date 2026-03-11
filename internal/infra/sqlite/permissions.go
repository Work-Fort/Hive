// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) SeedPermissions(ctx context.Context, names []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, name := range names {
		_, err := tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO permissions (id, name) VALUES (?, ?)",
			"perm_"+name, name)
		if err != nil {
			return fmt.Errorf("seed permission %q: %w", name, err)
		}
	}
	return tx.Commit()
}

func (s *Store) GetAgentPermissions(ctx context.Context, agentID string) ([]domain.AgentPermission, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ap.agent_id, p.name, COALESCE(ap.scope_team_id, '')
		FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = ?
		ORDER BY p.name
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent permissions: %w", err)
	}
	defer rows.Close()

	var perms []domain.AgentPermission
	for rows.Next() {
		var p domain.AgentPermission
		if err := rows.Scan(&p.AgentID, &p.PermissionID, &p.ScopeTeamID); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (s *Store) SetAgentPermissions(ctx context.Context, agentID string, perms []domain.AgentPermission) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_permissions WHERE agent_id = ?", agentID)
	if err != nil {
		return fmt.Errorf("clear permissions: %w", err)
	}

	for _, p := range perms {
		var scopeTeamID *string
		if p.ScopeTeamID != "" {
			scopeTeamID = &p.ScopeTeamID
		}

		// Resolve permission name to ID
		var permID string
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM permissions WHERE name = ?", p.PermissionID,
		).Scan(&permID)
		if err != nil {
			return fmt.Errorf("resolve permission %q: %w", p.PermissionID, err)
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_permissions (agent_id, permission_id, scope_team_id) VALUES (?, ?, ?)",
			agentID, permID, scopeTeamID)
		if err != nil {
			return fmt.Errorf("insert permission: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) HasPermission(ctx context.Context, agentID, permName, scopeTeamID string) (bool, error) {
	// Check for global permission (scope_team_id IS NULL) or scoped permission
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = ? AND p.name = ?
		AND (ap.scope_team_id IS NULL OR ap.scope_team_id = ?)
	`, agentID, permName, scopeTeamID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return count > 0, nil
}
