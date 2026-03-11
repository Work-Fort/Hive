// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	var agentID *string
	if t.AgentID != "" {
		agentID = &t.AgentID
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO tasks (id, team_id, agent_id, title, description, status) VALUES ($1, $2, $3, $4, $5, $6)",
		t.ID, t.TeamID, agentID, t.Title, t.Description, t.Status)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (s *Store) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	var t domain.Task
	var agentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, created_at, updated_at FROM tasks WHERE id = $1", id,
	).Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if agentID.Valid {
		t.AgentID = agentID.String
	}
	return &t, nil
}

func (s *Store) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, created_at, updated_at FROM tasks WHERE team_id = $1 ORDER BY created_at",
		teamID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		var t domain.Task
		var agentID sql.NullString
		if err := rows.Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if agentID.Valid {
			t.AgentID = agentID.String
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func (s *Store) UpdateTask(ctx context.Context, id string, t *domain.Task) error {
	var agentID *string
	if t.AgentID != "" {
		agentID = &t.AgentID
	}

	res, err := s.db.ExecContext(ctx,
		"UPDATE tasks SET title = $1, description = $2, status = $3, agent_id = $4, updated_at = NOW() WHERE id = $5",
		t.Title, t.Description, t.Status, agentID, id)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteTask(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	return nil
}
