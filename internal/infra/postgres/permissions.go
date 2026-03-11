// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
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
			"INSERT INTO permissions (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING",
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
		WHERE ap.agent_id = $1
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

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_permissions WHERE agent_id = $1", agentID)
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
			"SELECT id FROM permissions WHERE name = $1", p.PermissionID,
		).Scan(&permID)
		if err != nil {
			return fmt.Errorf("resolve permission %q: %w", p.PermissionID, err)
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_permissions (agent_id, permission_id, scope_team_id) VALUES ($1, $2, $3)",
			agentID, permID, scopeTeamID)
		if err != nil {
			return fmt.Errorf("insert permission: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) HasPermission(ctx context.Context, agentID, permName, scopeTeamID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_permissions ap
		JOIN permissions p ON p.id = ap.permission_id
		WHERE ap.agent_id = $1 AND p.name = $2
		AND (ap.scope_team_id IS NULL OR ap.scope_team_id = $3)
	`, agentID, permName, scopeTeamID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return count > 0, nil
}

func (s *Store) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM permissions ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var perms []*domain.Permission
	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *Store) LookupPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	var p domain.Permission
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name FROM permissions WHERE name = $1", name,
	).Scan(&p.ID, &p.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: permission named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup permission by name: %w", err)
	}
	return &p, nil
}
