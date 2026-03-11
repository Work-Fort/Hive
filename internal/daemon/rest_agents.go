// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (h *REST) ListAgents(w http.ResponseWriter, r *http.Request) {
	teamID := r.URL.Query().Get("team_id")
	agents, err := h.store.ListAgents(r.Context(), teamID)
	if mapDomainError(w, err) {
		return
	}
	if agents == nil {
		agents = []*domain.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (h *REST) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		TeamID string `json:"team_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}
	if _, err := h.store.GetTeam(r.Context(), req.TeamID); mapDomainError(w, err) {
		return
	}
	agent := &domain.Agent{ID: NewID("ag"), Name: req.Name, TeamID: req.TeamID}
	if mapDomainError(w, h.store.CreateAgent(r.Context(), agent)) {
		return
	}
	created, err := h.store.GetAgent(r.Context(), agent.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *REST) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := h.store.GetAgent(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	roles, err := h.store.GetAgentRoles(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	if roles == nil {
		roles = []domain.AgentRole{}
	}
	resp := struct {
		*domain.Agent
		Roles []domain.AgentRole `json:"roles"`
	}{Agent: agent, Roles: roles}
	writeJSON(w, http.StatusOK, resp)
}

func (h *REST) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name   string `json:"name"`
		TeamID string `json:"team_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}
	if mapDomainError(w, h.store.UpdateAgent(r.Context(), id, req.Name, req.TeamID)) {
		return
	}
	updated, err := h.store.GetAgent(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *REST) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteAgent(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *REST) SetAgentRoles(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}
	var req struct {
		Roles []struct {
			RoleID   string `json:"role_id"`
			Priority int    `json:"priority"`
		} `json:"roles"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	roles := make([]domain.AgentRole, len(req.Roles))
	for i, ar := range req.Roles {
		if ar.RoleID == "" {
			writeError(w, http.StatusBadRequest, "role_id is required for each role")
			return
		}
		roles[i] = domain.AgentRole{AgentID: agentID, RoleID: ar.RoleID, Priority: ar.Priority}
	}
	if mapDomainError(w, h.store.SetAgentRoles(r.Context(), agentID, roles)) {
		return
	}
	updated, err := h.store.GetAgentRoles(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if updated == nil {
		updated = []domain.AgentRole{}
	}
	writeJSON(w, http.StatusOK, updated)
}
