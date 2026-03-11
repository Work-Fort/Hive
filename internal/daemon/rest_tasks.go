// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"net/http"

	"github.com/Work-Fort/Hive/internal/domain"
)

func (h *REST) ListTeamTasks(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	if _, err := h.store.GetTeam(r.Context(), teamID); mapDomainError(w, err) {
		return
	}
	tasks, err := h.store.ListTeamTasks(r.Context(), teamID)
	if mapDomainError(w, err) {
		return
	}
	if tasks == nil {
		tasks = []*domain.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *REST) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamID      string `json:"team_id"`
		AgentID     string `json:"agent_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}
	if _, err := h.store.GetTeam(r.Context(), req.TeamID); mapDomainError(w, err) {
		return
	}

	status := domain.TaskStatus(req.Status)
	if status == "" {
		status = domain.TaskStatusPending
	}

	task := &domain.Task{
		ID: NewID("tk"), TeamID: req.TeamID, AgentID: req.AgentID,
		Title: req.Title, Description: req.Description, Status: status,
	}
	if mapDomainError(w, h.store.CreateTask(r.Context(), task)) {
		return
	}
	created, err := h.store.GetTask(r.Context(), task.ID)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *REST) GetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := h.store.GetTask(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *REST) UpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetTask(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		AgentID     string `json:"agent_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Title != "" {
		existing.Title = req.Title
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Status != "" {
		existing.Status = domain.TaskStatus(req.Status)
	}
	existing.AgentID = req.AgentID

	if mapDomainError(w, h.store.UpdateTask(r.Context(), id, existing)) {
		return
	}
	updated, err := h.store.GetTask(r.Context(), id)
	if mapDomainError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *REST) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if mapDomainError(w, h.store.DeleteTask(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
