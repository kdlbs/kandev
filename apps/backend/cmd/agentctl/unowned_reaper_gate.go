package main

import "github.com/kandev/kandev/internal/agentctl/server/config"

// unownedReaper is satisfied by *api.ControlServer, narrowed here so the
// gate decision below is testable without a real control server.
type unownedReaper interface {
	StartUnownedReaper()
	StopUnownedReaper()
}

// startUnownedReaperIfEnabled starts the unowned-period reaper only when the
// agent-survival capability is engaged for this launch, per
// StartUnownedReaper's own caller contract: starting it unconditionally
// would self-terminate a healthy attached instance once the period elapses,
// since nothing renews ownership without that capability's backend-side
// claim loop. StopUnownedReaper is always safe to call even when Start was
// never called, so the returned stop func needs no second branch.
func startUnownedReaperIfEnabled(cfg *config.Config, reaper unownedReaper) (stop func()) {
	if cfg.AgentSurvivalEnabled {
		reaper.StartUnownedReaper()
	}
	return reaper.StopUnownedReaper
}
