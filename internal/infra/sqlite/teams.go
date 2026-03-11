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

func (s *Store) CreateTeam(ctx context.Context, t *domain.Team) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO teams (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		t.ID, t.Name, t.CreatedAt.UTC(), t.UpdatedAt.UTC())
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: team %q", domain.ErrAlreadyExists, t.Name)
		}
		return fmt.Errorf("insert team: %w", err)
	}
	return nil
}

func (s *Store) GetTeam(ctx context.Context, id string) (*domain.Team, error) {
	var t domain.Team
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, created_at, updated_at FROM teams WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return &t, nil
}

func (s *Store) ListTeams(ctx context.Context) ([]*domain.Team, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, created_at, updated_at FROM teams ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []*domain.Team
	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, &t)
	}
	return teams, rows.Err()
}

func (s *Store) UpdateTeam(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE teams SET name = ?, updated_at = datetime('now') WHERE id = ?",
		name, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: team %q", domain.ErrAlreadyExists, name)
		}
		return fmt.Errorf("update team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Check for dependent agents
	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agents WHERE team_id = ?", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count agents for team: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: team has %d agents", domain.ErrHasDependencies, count)
	}

	// Check for dependent tasks
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE team_id = ?", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count tasks for team: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: team has %d tasks", domain.ErrHasDependencies, count)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM teams WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete team: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: team %q", domain.ErrNotFound, id)
	}
	return tx.Commit()
}

func (s *Store) LookupTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	var t domain.Team
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, created_at, updated_at FROM teams WHERE name = ?", name,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: team named %q", domain.ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup team by name: %w", err)
	}
	return &t, nil
}
