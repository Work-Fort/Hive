// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Work-Fort/Hive/internal/domain"
)

const agentCols = "id, name, team_id, model, runtime, current_role, current_project, current_workflow_id, lease_expires_at, created_at, updated_at"

func scanAgent(row interface {
	Scan(dest ...any) error
}) (*domain.Agent, error) {
	var a domain.Agent
	var role, project, workflowID sql.NullString
	var leaseExpires sql.NullTime
	if err := row.Scan(
		&a.ID, &a.Name, &a.TeamID, &a.Model, &a.Runtime,
		&role, &project, &workflowID, &leaseExpires,
		&a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if role.Valid {
		a.CurrentRole = role.String
	}
	if project.Valid {
		a.CurrentProject = project.String
	}
	if workflowID.Valid {
		a.CurrentWorkflowID = workflowID.String
	}
	if leaseExpires.Valid {
		a.LeaseExpiresAt = leaseExpires.Time.UTC()
	}
	return &a, nil
}

func toNullable(a *domain.Agent) (any, any, any, any) {
	var role, project, workflowID any
	var leaseExpires any
	if a.CurrentRole != "" {
		role = a.CurrentRole
	}
	if a.CurrentProject != "" {
		project = a.CurrentProject
	}
	if a.CurrentWorkflowID != "" {
		workflowID = a.CurrentWorkflowID
	}
	if !a.LeaseExpiresAt.IsZero() {
		leaseExpires = a.LeaseExpiresAt.UTC()
	}
	return role, project, workflowID, leaseExpires
}

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	role, project, workflowID, leaseExpires := toNullable(a)
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO agents ("+agentCols+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		a.ID, a.Name, a.TeamID, a.Model, a.Runtime, role, project, workflowID, leaseExpires, a.CreatedAt.UTC(), a.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent %q", domain.ErrAlreadyExists, a.Name)
		}
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+agentCols+" FROM agents WHERE id = ?", id)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: agent %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return a, nil
}

func (s *Store) ListAgents(ctx context.Context, teamID string) ([]*domain.Agent, error) {
	var rows *sql.Rows
	var err error

	if teamID != "" {
		rows, err = s.db.QueryContext(ctx,
			"SELECT "+agentCols+" FROM agents WHERE team_id = ? ORDER BY name", teamID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT "+agentCols+" FROM agents ORDER BY name")
	}
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*domain.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) UpdateAgent(ctx context.Context, a *domain.Agent) error {
	role, project, workflowID, leaseExpires := toNullable(a)
	res, err := s.db.ExecContext(ctx,
		"UPDATE agents SET name = ?, team_id = ?, model = ?, runtime = ?, current_role = ?, current_project = ?, current_workflow_id = ?, lease_expires_at = ?, updated_at = datetime('now') WHERE id = ?",
		a.Name, a.TeamID, a.Model, a.Runtime, role, project, workflowID, leaseExpires, a.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: agent %q", domain.ErrAlreadyExists, a.Name)
		}
		return fmt.Errorf("update agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: agent %q", domain.ErrNotFound, a.ID)
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

func (s *Store) LookupAgentByName(ctx context.Context, name string) (*domain.Agent, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+agentCols+" FROM agents WHERE name = ?", name)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: agent named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup agent by name: %w", err)
	}
	return a, nil
}

func (s *Store) ClaimAgent(ctx context.Context, role, project, workflowID string, leaseExpiresAt time.Time) (*domain.Agent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for range 3 {
		var candidateID string
		row := tx.QueryRowContext(ctx, "SELECT id FROM agents WHERE current_workflow_id IS NULL ORDER BY id LIMIT 1")
		if err := row.Scan(&candidateID); errors.Is(err, sql.ErrNoRows) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("select free agent: %w", err)
		}

		res, err := tx.ExecContext(ctx,
			"UPDATE agents SET current_role = ?, current_project = ?, current_workflow_id = ?, lease_expires_at = ?, updated_at = datetime('now') WHERE id = ? AND current_workflow_id IS NULL",
			role, project, workflowID, leaseExpiresAt.UTC(), candidateID)
		if err != nil {
			return nil, fmt.Errorf("claim agent: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}

		row2 := tx.QueryRowContext(ctx, "SELECT "+agentCols+" FROM agents WHERE id = ?", candidateID)
		agent, err := scanAgent(row2)
		if err != nil {
			return nil, fmt.Errorf("read claimed agent: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit claim: %w", err)
		}
		return agent, nil
	}

	return nil, domain.ErrPoolExhausted
}

func (s *Store) ReleaseAgent(ctx context.Context, agentID, workflowID string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE agents SET current_role = NULL, current_project = NULL, current_workflow_id = NULL, lease_expires_at = NULL, updated_at = datetime('now') WHERE id = ? AND current_workflow_id = ?",
		agentID, workflowID)
	if err != nil {
		return fmt.Errorf("release agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.GetAgent(ctx, agentID); errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return domain.ErrWorkflowMismatch
	}
	return nil
}

func (s *Store) RenewAgentLease(ctx context.Context, agentID, workflowID string, leaseExpiresAt time.Time) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE agents SET lease_expires_at = ?, updated_at = datetime('now') WHERE id = ? AND current_workflow_id = ?",
		leaseExpiresAt.UTC(), agentID, workflowID)
	if err != nil {
		return fmt.Errorf("renew agent lease: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.GetAgent(ctx, agentID); errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return domain.ErrWorkflowMismatch
	}
	return nil
}

func (s *Store) ListAgentsByAssignment(ctx context.Context, f domain.AgentAssignmentFilter) ([]*domain.Agent, error) {
	var where []string
	var args []any
	if f.TeamID != "" {
		where = append(where, "team_id = ?")
		args = append(args, f.TeamID)
	}
	if f.Assigned != nil {
		if *f.Assigned {
			where = append(where, "current_workflow_id IS NOT NULL")
		} else {
			where = append(where, "current_workflow_id IS NULL")
		}
	}
	if f.WorkflowID != "" {
		where = append(where, "current_workflow_id = ?")
		args = append(args, f.WorkflowID)
	}
	if f.Role != "" {
		where = append(where, "current_role = ?")
		args = append(args, f.Role)
	}
	if f.Project != "" {
		where = append(where, "current_project = ?")
		args = append(args, f.Project)
	}

	q := "SELECT " + agentCols + " FROM agents"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY name"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents by assignment: %w", err)
	}
	defer rows.Close()

	var agents []*domain.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}
