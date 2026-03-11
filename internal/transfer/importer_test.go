package transfer_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Hive/internal/infra/sqlite"
	"github.com/Work-Fort/Hive/internal/transfer"
)

func TestImportRoundTrip(t *testing.T) {
	// Export from a seeded store
	srcStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer srcStore.Close()
	seedTestData(t, srcStore)

	dir := t.TempDir()
	srcDS := transfer.NewDBDataSource(srcStore)
	if _, err := transfer.Export(context.Background(), srcDS, dir); err != nil {
		t.Fatal(err)
	}

	// Import into a fresh store
	dstStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dstStore.Close()

	dstDS := transfer.NewDBDataSource(dstStore)
	result, err := transfer.Import(context.Background(), dstDS, dir, transfer.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if result.Teams != 1 {
		t.Errorf("teams = %d, want 1", result.Teams)
	}
	if result.Roles != 2 {
		t.Errorf("roles = %d, want 2", result.Roles)
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
}

func TestImportConflictFails(t *testing.T) {
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedTestData(t, s)

	dir := t.TempDir()
	ds := transfer.NewDBDataSource(s)
	if _, err := transfer.Export(context.Background(), ds, dir); err != nil {
		t.Fatal(err)
	}

	// Import into same store — should fail on conflicts
	_, err = transfer.Import(context.Background(), ds, dir, transfer.ImportOptions{})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

func TestImportUpsert(t *testing.T) {
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seedTestData(t, s)

	dir := t.TempDir()
	ds := transfer.NewDBDataSource(s)
	if _, err := transfer.Export(context.Background(), ds, dir); err != nil {
		t.Fatal(err)
	}

	// Import with upsert — should succeed
	result, err := transfer.Import(context.Background(), ds, dir, transfer.ImportOptions{Upsert: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated == 0 {
		t.Error("expected some updates with upsert")
	}
}

func TestImportDryRun(t *testing.T) {
	srcStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer srcStore.Close()
	seedTestData(t, srcStore)

	dir := t.TempDir()
	srcDS := transfer.NewDBDataSource(srcStore)
	if _, err := transfer.Export(context.Background(), srcDS, dir); err != nil {
		t.Fatal(err)
	}

	dstStore, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dstStore.Close()

	dstDS := transfer.NewDBDataSource(dstStore)
	result, err := transfer.Import(context.Background(), dstDS, dir, transfer.ImportOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	// Dry run should report creates but not actually create
	if result.Teams != 1 {
		t.Errorf("teams = %d, want 1", result.Teams)
	}

	// Verify nothing was actually created
	teams, _ := dstDS.ListTeams(context.Background())
	if len(teams) != 0 {
		t.Errorf("expected 0 teams in store after dry run, got %d", len(teams))
	}
}
