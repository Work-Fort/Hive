package transfer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
	"github.com/Work-Fort/Hive/internal/transfer"
)

func seedTestData(t *testing.T, s domain.Store) {
	t.Helper()
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(s.CreateTeam(ctx, &domain.Team{ID: "t1", Name: "engineering"}))
	must(s.CreateRole(ctx, &domain.Role{ID: "r1", Name: "developer"}))
	must(s.CreateRole(ctx, &domain.Role{ID: "r2", Name: "reviewer", ParentID: "r1"}))
	must(s.SeedPermissions(ctx, []string{"read-docs", "write-docs"}))
	must(s.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "claude", TeamID: "t1"}))
	must(s.SetAgentRoles(ctx, "a1", []domain.AgentRole{
		{AgentID: "a1", RoleID: "r1", Priority: 1},
	}))
	must(s.SetAgentPermissions(ctx, "a1", []domain.AgentPermission{
		{AgentID: "a1", PermissionID: "read-docs"},
	}))
	must(s.CreateDocument(ctx, &domain.Document{
		ID: "d1", Kind: domain.DocumentKindRole, Title: "Standards", RoleID: "r1",
	}))
	must(s.CreateDocument(ctx, &domain.Document{
		ID: "d2", Kind: domain.DocumentKindMemory, Title: "Notes", Content: "some notes", AgentID: "a1",
	}))
	must(s.CreateTask(ctx, &domain.Task{
		ID: "tk1", TeamID: "t1", Title: "Fix bug", Status: domain.TaskStatusPending,
	}))
}

func TestExport(t *testing.T) {
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	seedTestData(t, s)

	dir := t.TempDir()
	ds := transfer.NewDBDataSource(s)
	result, err := transfer.Export(context.Background(), ds, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Teams != 1 {
		t.Errorf("teams = %d, want 1", result.Teams)
	}
	if result.Roles != 2 {
		t.Errorf("roles = %d, want 2", result.Roles)
	}
	if result.Permissions != 2 {
		t.Errorf("permissions = %d, want 2", result.Permissions)
	}
	if result.Agents != 1 {
		t.Errorf("agents = %d, want 1", result.Agents)
	}
	if result.Documents != 2 {
		t.Errorf("documents = %d, want 2", result.Documents)
	}
	if result.Tasks != 1 {
		t.Errorf("tasks = %d, want 1", result.Tasks)
	}

	// Verify files exist
	assertFileExists(t, filepath.Join(dir, "teams", "engineering.yaml"))
	assertFileExists(t, filepath.Join(dir, "roles", "developer.yaml"))
	assertFileExists(t, filepath.Join(dir, "roles", "reviewer.yaml"))
	assertFileExists(t, filepath.Join(dir, "permissions", "read-docs.yaml"))
	assertFileExists(t, filepath.Join(dir, "agents", "claude.yaml"))
	assertFileExists(t, filepath.Join(dir, "documents", "role-developer--standards.md"))
	assertFileExists(t, filepath.Join(dir, "documents", "agent-claude--notes.md"))
	assertFileExists(t, filepath.Join(dir, "tasks", "engineering--fix-bug.yaml"))
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", path)
	}
}
