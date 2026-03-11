// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (s *Store) CreateDocument(ctx context.Context, d *domain.Document) error {
	var roleID, agentID *string
	if d.RoleID != "" {
		roleID = &d.RoleID
	}
	if d.AgentID != "" {
		agentID = &d.AgentID
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO documents (id, kind, title, content, role_id, agent_id) VALUES ($1, $2, $3, $4, $5, $6)",
		d.ID, d.Kind, d.Title, d.Content, roleID, agentID)
	if err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

func (s *Store) GetDocument(ctx context.Context, id string) (*domain.Document, error) {
	var d domain.Document
	var roleID, agentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE id = $1", id,
	).Scan(&d.ID, &d.Kind, &d.Title, &d.Content, &roleID, &agentID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: document %q", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if roleID.Valid {
		d.RoleID = roleID.String
	}
	if agentID.Valid {
		d.AgentID = agentID.String
	}
	return &d, nil
}

func (s *Store) UpdateDocument(ctx context.Context, id, title, content string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE documents SET title = $1, content = $2, updated_at = NOW() WHERE id = $3",
		title, content, id)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) DeleteDocument(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM documents WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: document %q", domain.ErrNotFound, id)
	}
	return nil
}

func (s *Store) ListRoleDocuments(ctx context.Context, roleID string) ([]*domain.Document, error) {
	return s.listDocuments(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE role_id = $1 ORDER BY title",
		roleID)
}

func (s *Store) ListAgentMemory(ctx context.Context, agentID string) ([]*domain.Document, error) {
	return s.listDocuments(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE agent_id = $1 ORDER BY title",
		agentID)
}

func (s *Store) listDocuments(ctx context.Context, query string, arg string) ([]*domain.Document, error) {
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []*domain.Document
	for rows.Next() {
		var d domain.Document
		var roleID, agentID sql.NullString
		if err := rows.Scan(&d.ID, &d.Kind, &d.Title, &d.Content, &roleID, &agentID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		if roleID.Valid {
			d.RoleID = roleID.String
		}
		if agentID.Valid {
			d.AgentID = agentID.String
		}
		docs = append(docs, &d)
	}
	return docs, rows.Err()
}

func (s *Store) LookupDocumentByOwnerAndTitle(ctx context.Context, ownerID, title string) (*domain.Document, error) {
	var d domain.Document
	var roleID, agentID sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT id, kind, title, content, role_id, agent_id, created_at, updated_at FROM documents WHERE (role_id = $1 OR agent_id = $2) AND title = $3",
		ownerID, ownerID, title,
	).Scan(&d.ID, &d.Kind, &d.Title, &d.Content, &roleID, &agentID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: document titled %q for owner %q", domain.ErrNotFound, title, ownerID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup document by owner and title: %w", err)
	}
	if roleID.Valid {
		d.RoleID = roleID.String
	}
	if agentID.Valid {
		d.AgentID = agentID.String
	}
	return &d, nil
}
