// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO agents (id, name, team_id) VALUES (?, ?, ?)",
		a.ID, a.Name, a.TeamID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent %q", domain.ErrAlreadyExists, a.Name)
		}
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	var a domain.Agent
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE id = ?", id,
	).Scan(&a.ID, &a.Name, &a.TeamID, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return &a, nil
}

func (s *Store) ListAgents(ctx context.Context, teamID string) ([]*domain.Agent, error) {
	var rows *sql.Rows
	var err error

	if teamID != "" {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, team_id, created_at, updated_at FROM agents WHERE team_id = ? ORDER BY name",
			teamID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, team_id, created_at, updated_at FROM agents ORDER BY name")
	}
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*domain.Agent
	for rows.Next() {
		var a domain.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.TeamID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

func (s *Store) UpdateAgent(ctx context.Context, id, name, teamID string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE agents SET name = ?, team_id = ?, updated_at = datetime('now') WHERE id = ?",
		name, teamID, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent %q", domain.ErrAlreadyExists, name)
		}
		return fmt.Errorf("update agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Check for assigned tasks
	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE agent_id = ?", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count tasks for agent: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: agent has %d tasks", domain.ErrHasDependencies, count)
	}

	// agent_roles and agent_permissions cascade on delete
	res, err := tx.ExecContext(ctx, "DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	return tx.Commit()
}

func (s *Store) SetAgentRoles(ctx context.Context, agentID string, roles []domain.AgentRole) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "DELETE FROM agent_roles WHERE agent_id = ?", agentID)
	if err != nil {
		return fmt.Errorf("clear agent roles: %w", err)
	}

	for _, r := range roles {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO agent_roles (agent_id, role_id, priority) VALUES (?, ?, ?)",
			agentID, r.RoleID, r.Priority)
		if err != nil {
			return fmt.Errorf("insert agent role: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetAgentRoles(ctx context.Context, agentID string) ([]domain.AgentRole, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT agent_id, role_id, priority FROM agent_roles WHERE agent_id = ? ORDER BY priority",
		agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent roles: %w", err)
	}
	defer rows.Close()

	var roles []domain.AgentRole
	for rows.Next() {
		var r domain.AgentRole
		if err := rows.Scan(&r.AgentID, &r.RoleID, &r.Priority); err != nil {
			return nil, fmt.Errorf("scan agent role: %w", err)
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}
