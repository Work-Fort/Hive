// SPDX-License-Identifier: GPL-3.0-or-later
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
)

func TestCreateAndGetTask(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})

	task := &domain.Task{
		ID: "tk_001", TeamID: "t_001",
		Title: "Fix bug", Description: "Fix the login bug",
		Status: domain.TaskStatusPending,
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Title != "Fix bug" || got.Status != domain.TaskStatusPending {
		t.Errorf("got %+v", got)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTask(ctx, &domain.Task{
		ID: "tk_001", TeamID: "t_001", Title: "Fix bug",
		Status: domain.TaskStatusPending,
	})

	updated := &domain.Task{
		ID: "tk_001", TeamID: "t_001", Title: "Fix bug",
		Status: domain.TaskStatusInProgress,
	}
	if err := store.UpdateTask(ctx, "tk_001", updated); err != nil {
		t.Fatalf("update task: %v", err)
	}

	got, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.TaskStatusInProgress {
		t.Errorf("got status %q, want %q", got.Status, domain.TaskStatusInProgress)
	}
}

func TestListTeamTasks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateTeam(ctx, &domain.Team{ID: "t_002", Name: "beta"})
	store.CreateTask(ctx, &domain.Task{ID: "tk_001", TeamID: "t_001", Title: "Task A", Status: domain.TaskStatusPending})
	store.CreateTask(ctx, &domain.Task{ID: "tk_002", TeamID: "t_002", Title: "Task B", Status: domain.TaskStatusPending})

	tasks, err := store.ListTeamTasks(ctx, "t_001")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Task A" {
		t.Errorf("expected only Task A, got %+v", tasks)
	}
}

func TestTask_FlowTaskRef_Roundtrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.CreateTeam(ctx, &domain.Team{ID: "t_001", Name: "alpha"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a_001", Name: "alice", TeamID: "t_001"})

	task := &domain.Task{
		ID: "tk_001", TeamID: "t_001", AgentID: "a_001",
		Title: "flow task", Status: domain.TaskStatusPending,
		FlowTaskRef: "flow-task-117",
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.FlowTaskRef != "flow-task-117" {
		t.Errorf("FlowTaskRef = %q, want %q", got.FlowTaskRef, "flow-task-117")
	}

	got.FlowTaskRef = ""
	if err := store.UpdateTask(ctx, "tk_001", got); err != nil {
		t.Fatalf("update task: %v", err)
	}

	got2, err := store.GetTask(ctx, "tk_001")
	if err != nil {
		t.Fatalf("get task after clear: %v", err)
	}
	if got2.FlowTaskRef != "" {
		t.Errorf("FlowTaskRef after clear = %q, want empty", got2.FlowTaskRef)
	}
}
