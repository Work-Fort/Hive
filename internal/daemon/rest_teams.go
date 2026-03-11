// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (h *REST) ListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.store.ListTeams(r.Context())
	if mapDomainError(w, err) {
		return
	}
	if teams == nil {
		teams = []*domain.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

func (h *REST) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	team := &domain.Team{ID: NewID("tm"), Name: req.Name}
	if mapDomainError(w, h.store.CreateTeam(r.Context(), team)) {
		return
	}
	created, err := h.store.GetTeam(r.Context(), team.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *REST) GetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	team, err := h.store.GetTeam(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (h *REST) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if mapDomainError(w, h.store.UpdateTeam(r.Context(), id, req.Name)) {
		return
	}
	updated, err := h.store.GetTeam(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *REST) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteTeam(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
