// SPDX-License-Identifier: GPL-3.0-or-later
package daemon

import (
	"context"
	"time"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Hive/internal/domain"
)

// SweeperService periodically clears expired agent leases.
type SweeperService struct {
	store domain.Store
}

// NewSweeperService constructs a SweeperService.
func NewSweeperService(store domain.Store) *SweeperService {
	return &SweeperService{store: store}
}

// SweepOnce runs a single sweep pass. Returns the number of agents
// whose claim was cleared. Logs one line per released agent.
func (s *SweeperService) SweepOnce(ctx context.Context) (int, error) {
	released, err := s.store.SweepExpiredLeases(ctx, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	for _, a := range released {
		log.Info("sweeper: released expired claim",
			"agent_id", a.ID,
			"agent_name", a.Name,
			"workflow_id", a.CurrentWorkflowID,
			"role", a.CurrentRole,
			"project", a.CurrentProject,
			"lease_expired_at", a.LeaseExpiresAt.Format(time.RFC3339),
		)
	}
	return len(released), nil
}

// Start runs SweepOnce on `interval` until ctx is cancelled. Errors from
// individual sweeps are logged but do not stop the loop — the sweeper
// must survive transient DB issues.
func (s *SweeperService) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	// Run once immediately so a freshly-started daemon cleans stragglers.
	if _, err := s.SweepOnce(ctx); err != nil {
		log.Warn("sweeper: initial sweep failed", "err", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Debug("sweeper: shutting down")
			return
		case <-t.C:
			if _, err := s.SweepOnce(ctx); err != nil {
				log.Warn("sweeper: sweep failed", "err", err)
			}
		}
	}
}
