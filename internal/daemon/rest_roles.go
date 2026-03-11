// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (h *REST) ListRoles(w http.ResponseWriter, r *http.Request) {
	parentID := r.URL.Query().Get("parent_id")
	roles, err := h.store.ListRoles(r.Context(), parentID)
	if mapDomainError(w, err) {
		return
	}
	if roles == nil {
		roles = []*domain.Role{}
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *REST) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	role := &domain.Role{ID: NewID("rl"), Name: req.Name, ParentID: req.ParentID}
	if mapDomainError(w, h.store.CreateRole(r.Context(), role)) {
		return
	}
	created, err := h.store.GetRole(r.Context(), role.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *REST) GetRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	role, err := h.store.GetRole(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *REST) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if mapDomainError(w, h.store.UpdateRole(r.Context(), id, req.Name, req.ParentID)) {
		return
	}
	updated, err := h.store.GetRole(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *REST) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteRole(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
