// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (h *REST) GetAgentPermissions(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}
	perms, err := h.store.GetAgentPermissions(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if perms == nil {
		perms = []domain.AgentPermission{}
	}
	writeJSON(w, http.StatusOK, perms)
}

func (h *REST) SetAgentPermissions(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if _, err := h.store.GetAgent(r.Context(), agentID); mapDomainError(w, err) {
		return
	}
	var req struct {
		Permissions []struct {
			Permission  string `json:"permission"`
			ScopeTeamID string `json:"scope_team_id"`
		} `json:"permissions"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	perms := make([]domain.AgentPermission, len(req.Permissions))
	for i, p := range req.Permissions {
		if p.Permission == "" {
			writeError(w, http.StatusBadRequest, "permission name is required for each entry")
			return
		}
		perms[i] = domain.AgentPermission{AgentID: agentID, PermissionID: p.Permission, ScopeTeamID: p.ScopeTeamID}
	}
	if mapDomainError(w, h.store.SetAgentPermissions(r.Context(), agentID, perms)) {
		return
	}
	updated, err := h.store.GetAgentPermissions(r.Context(), agentID)
	if mapDomainError(w, err) {
		return
	}
	if updated == nil {
		updated = []domain.AgentPermission{}
	}
	writeJSON(w, http.StatusOK, updated)
}
