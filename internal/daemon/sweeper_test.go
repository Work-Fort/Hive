// SPDX-License-Identifier: GPL-3.0-or-later
package daemon_test

import (
	"context"
	"testing"
	"time"

	"github.com/Work-Fort/Hive/internal/daemon"
	"github.com/Work-Fort/Hive/internal/domain"
	"github.com/Work-Fort/Hive/internal/infra/sqlite"
)

func TestSweeperStart_StopsOnContextCancel(t *testing.T) {
	store, _ := sqlite.Open("")
	t.Cleanup(func() { store.Close() })

	sw := daemon.NewSweeperService(store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sw.Start(ctx, 10*time.Millisecond); close(done) }()

	// Let the ticker fire at least once.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sweeper did not stop within 1s of cancel")
	}
}

func TestSweeperReleasesOneExpiredAgent(t *testing.T) {
	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	store.CreateTeam(ctx, &domain.Team{ID: "t", Name: "t"})
	store.CreateAgent(ctx, &domain.Agent{ID: "a", Name: "a", TeamID: "t"})
	past := time.Now().UTC().Add(-time.Minute)
	store.UpdateAgent(ctx, &domain.Agent{
		ID: "a", Name: "a", TeamID: "t",
		CurrentRole: "r", CurrentProject: "p",
		CurrentWorkflowID: "wf", LeaseExpiresAt: past,
	})

	sw := daemon.NewSweeperService(store)
	count, err := sw.SweepOnce(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if count != 1 {
		t.Errorf("released count = %d, want 1", count)
	}

	got, _ := store.GetAgent(ctx, "a")
	if got.CurrentWorkflowID != "" {
		t.Errorf("agent still claimed: %+v", got)
	}
}
