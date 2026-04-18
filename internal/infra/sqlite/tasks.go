// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}

	var agentID *string
	if t.AgentID != "" {
		agentID = &t.AgentID
	}
	var flowTaskRef *string
	if t.FlowTaskRef != "" {
		flowTaskRef = &t.FlowTaskRef
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO tasks (id, team_id, agent_id, title, description, status, flow_task_ref, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		t.ID, t.TeamID, agentID, t.Title, t.Description, t.Status, flowTaskRef, t.CreatedAt.UTC(), t.UpdatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (s *Store) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	var t domain.Task
	var agentID, flowTaskRef sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, flow_task_ref, created_at, updated_at FROM tasks WHERE id = ?", id,
	).Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &flowTaskRef, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if agentID.Valid {
		t.AgentID = agentID.String
	}
	if flowTaskRef.Valid {
		t.FlowTaskRef = flowTaskRef.String
	}
	return &t, nil
}

func (s *Store) ListTeamTasks(ctx context.Context, teamID string) ([]*domain.Task, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, flow_task_ref, created_at, updated_at FROM tasks WHERE team_id = ? ORDER BY created_at",
		teamID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		var t domain.Task
		var agentID, flowTaskRef sql.NullString
		if err := rows.Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &flowTaskRef, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if agentID.Valid {
			t.AgentID = agentID.String
		}
		if flowTaskRef.Valid {
			t.FlowTaskRef = flowTaskRef.String
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
	var flowTaskRef *string
	if t.FlowTaskRef != "" {
		flowTaskRef = &t.FlowTaskRef
	}

	res, err := s.db.ExecContext(ctx,
		"UPDATE tasks SET title = ?, description = ?, status = ?, agent_id = ?, flow_task_ref = ?, updated_at = datetime('now') WHERE id = ?",
		t.Title, t.Description, t.Status, agentID, flowTaskRef, id)
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
	res, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: task %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) LookupTaskByTeamAndTitle(ctx context.Context, teamID, title string) (*domain.Task, error) {
	var t domain.Task
	var agentID, flowTaskRef sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, team_id, agent_id, title, description, status, flow_task_ref, created_at, updated_at FROM tasks WHERE team_id = ? AND title = ?",
		teamID, title,
	).Scan(&t.ID, &t.TeamID, &agentID, &t.Title, &t.Description, &t.Status, &flowTaskRef, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: task titled %q in team %q", domain.ErrNotFound, title, teamID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup task by team and title: %w", err)
	}
	if agentID.Valid {
		t.AgentID = agentID.String
	}
	if flowTaskRef.Valid {
		t.FlowTaskRef = flowTaskRef.String
	}
	return &t, nil
}
