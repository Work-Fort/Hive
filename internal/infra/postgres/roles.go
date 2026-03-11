// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateRole(ctx context.Context, r *domain.Role) error {
	var parentID *string
	if r.ParentID != "" {
		parentID = &r.ParentID
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO roles (id, name, parent_id) VALUES ($1, $2, $3)",
		r.ID, r.Name, parentID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: role %q", domain.ErrAlreadyExists, r.Name)
		}
		return fmt.Errorf("insert role: %w", err)
	}
	return nil
}

func (s *Store) GetRole(ctx context.Context, id string) (*domain.Role, error) {
	var r domain.Role
	var parentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, parent_id, created_at, updated_at FROM roles WHERE id = $1", id,
	).Scan(&r.ID, &r.Name, &parentID, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get role: %w", err)
	}
	if parentID.Valid {
		r.ParentID = parentID.String
	}
	return &r, nil
}

func (s *Store) ListRoles(ctx context.Context, parentID string) ([]*domain.Role, error) {
	var rows *sql.Rows
	var err error

	if parentID != "" {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, parent_id, created_at, updated_at FROM roles WHERE parent_id = $1 ORDER BY name",
			parentID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			"SELECT id, name, parent_id, created_at, updated_at FROM roles ORDER BY name")
	}
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []*domain.Role
	for rows.Next() {
		var r domain.Role
		var pid sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &pid, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		if pid.Valid {
			r.ParentID = pid.String
		}
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}

func (s *Store) UpdateRole(ctx context.Context, id, name, parentID string) error {
	var pid *string
	if parentID != "" {
		pid = &parentID
	}

	res, err := s.db.ExecContext(ctx,
		"UPDATE roles SET name = $1, parent_id = $2, updated_at = NOW() WHERE id = $3",
		name, pid, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: role %q", domain.ErrAlreadyExists, name)
		}
		return fmt.Errorf("update role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM roles WHERE parent_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count child roles: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: role has %d child roles", domain.ErrHasDependencies, count)
	}

	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agent_roles WHERE role_id = $1", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("count agent assignments for role: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: role assigned to %d agents", domain.ErrHasDependencies, count)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM roles WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: role %q", domain.ErrNotFound, id)
	}
	return tx.Commit()
}

// GetRoleChain returns the inheritance chain from the given role to the root,
// up to maxDepth levels. Uses a recursive CTE. Ordered leaf-to-root.
func (s *Store) GetRoleChain(ctx context.Context, roleID string, maxDepth int) ([]*domain.Role, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE chain(id, name, parent_id, created_at, updated_at, depth) AS (
			SELECT id, name, parent_id, created_at, updated_at, 1
			FROM roles WHERE id = $1
			UNION ALL
			SELECT r.id, r.name, r.parent_id, r.created_at, r.updated_at, c.depth + 1
			FROM roles r
			JOIN chain c ON r.id = c.parent_id
			WHERE c.depth < $2
		)
		SELECT id, name, parent_id, created_at, updated_at FROM chain ORDER BY depth
	`, roleID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("get role chain: %w", err)
	}
	defer rows.Close()

	var chain []*domain.Role
	for rows.Next() {
		var r domain.Role
		var pid sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &pid, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role chain: %w", err)
		}
		if pid.Valid {
			r.ParentID = pid.String
		}
		chain = append(chain, &r)
	}
	return chain, rows.Err()
}
