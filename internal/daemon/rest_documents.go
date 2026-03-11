// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (h *REST) ListRoleDocuments(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")
	if _, err := h.store.GetRole(r.Context(), roleID); mapDomainError(w, err) {
		return
	}
	docs, err := h.store.ListRoleDocuments(r.Context(), roleID)
	if mapDomainError(w, err) {
		return
	}
	if docs == nil {
		docs = []*domain.Document{}
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *REST) CreateRoleDocument(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")
	if _, err := h.store.GetRole(r.Context(), roleID); mapDomainError(w, err) {
		return
	}
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	doc := &domain.Document{
		ID: NewID("doc"), Kind: domain.DocumentKindRole,
		Title: req.Title, Content: req.Content, RoleID: roleID,
	}
	if mapDomainError(w, h.store.CreateDocument(r.Context(), doc)) {
		return
	}
	created, err := h.store.GetDocument(r.Context(), doc.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *REST) GetDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := h.store.GetDocument(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *REST) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if mapDomainError(w, h.store.UpdateDocument(r.Context(), id, req.Title, req.Content)) {
		return
	}
	updated, err := h.store.GetDocument(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *REST) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteDocument(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *REST) ListAgentMemory(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}
	docs, err := h.store.ListAgentMemory(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if docs == nil {
		docs = []*domain.Document{}
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *REST) CreateAgentMemory(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	doc := &domain.Document{
		ID: NewID("doc"), Kind: domain.DocumentKindMemory,
		Title: req.Title, Content: req.Content, AgentID: agentID,
	}
	if mapDomainError(w, h.store.CreateDocument(r.Context(), doc)) {
		return
	}
	created, err := h.store.GetDocument(r.Context(), doc.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
