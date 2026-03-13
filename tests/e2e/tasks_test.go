// SPDX-License-Identifier: GPL-3.0-or-later
package e2e_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Hive/client"
)

func TestTasks(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	// Setup: tasks require a team; agent is optional.
	team, err := c.CreateTeam(ctx(), "tasks-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000003", "task-agent", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Create (unassigned)
	task, err := c.CreateTask(ctx(), client.CreateTaskInput{
		TeamID:      team.ID,
		Title:       "Write tests",
		Description: "Cover all endpoints",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.Title != "Write tests" {
		t.Errorf("Title: got %q, want %q", task.Title, "Write tests")
	}
	if task.Status != "pending" {
		t.Errorf("Status: got %q, want %q", task.Status, "pending")
	}
	if task.AgentID != "" {
		t.Errorf("AgentID: expected empty (unassigned), got %q", task.AgentID)
	}

	// Get
	got, err := c.GetTask(ctx(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("GetTask ID: got %q, want %q", got.ID, task.ID)
	}

	// List by team
	tasks, err := c.ListTeamTasks(ctx(), team.ID)
	if err != nil {
		t.Fatalf("ListTeamTasks: %v", err)
	}
	if !containsTask(tasks, task.ID) {
		t.Errorf("ListTeamTasks: task %q not found", task.ID)
	}

	// Update — assign agent, change status
	updated, err := c.UpdateTask(ctx(), task.ID, client.UpdateTaskInput{
		Status:  "in_progress",
		AgentID: agent.ID,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Status != "in_progress" {
		t.Errorf("Status after update: got %q, want %q", updated.Status, "in_progress")
	}
	if updated.AgentID != agent.ID {
		t.Errorf("AgentID after update: got %q, want %q", updated.AgentID, agent.ID)
	}

	// Delete — must unassign first (task has no assignment restriction per design,
	// but agent deletion is blocked by assigned tasks, not task deletion)
	if err := c.DeleteTask(ctx(), task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	// Confirm gone
	_, err = c.GetTask(ctx(), task.ID)
	if !errors.Is(err, client.ErrNotFound) {
		t.Errorf("GetTask after delete: expected ErrNotFound, got %v", err)
	}
}

// TestAgentDeleteBlockedByTask verifies that an agent with an assigned task
// cannot be deleted.
func TestAgentDeleteBlockedByTask(t *testing.T) {
	h := newHarness(t)
	c := h.Client

	team, err := c.CreateTeam(ctx(), "block-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	agent, err := c.CreateAgent(ctx(), "00000000-0000-0000-0000-000000000004", "block-agent", team.ID)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	_, err = c.CreateTask(ctx(), client.CreateTaskInput{
		TeamID:  team.ID,
		AgentID: agent.ID,
		Title:   "blocking task",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	err = c.DeleteAgent(ctx(), agent.ID)
	if !errors.Is(err, client.ErrConflict) {
		t.Errorf("delete agent with task: expected ErrConflict, got %v", err)
	}
}

func containsTask(tasks []client.Task, id string) bool {
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}
