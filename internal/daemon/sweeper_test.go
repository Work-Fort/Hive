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
