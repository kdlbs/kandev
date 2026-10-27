package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// nothingStaged reports that no worker file, fence or workspace exists for the
// environment: placement admission refused the node before staging, or a pool
// job never reached a node. Only the job and the inventory need cleanup.
func (c *nomadCleanup) nothingStaged() bool {
	return c.state.RefusedBeforeStage || c.neverPlaced()
}

// neverPlaced reports a pool environment whose state never recorded a node.
func (c *nomadCleanup) neverPlaced() bool {
	return c.state.PlacementMode == nomad.ModePool && c.state.NodeID == ""
}

// settleNeverPlaced finishes a pool job that was never bound to a node. An
// unregistered job (404) is already absent. A registered one is stopped and
// absence is recorded only once every allocation it may have created is
// terminal; no worker files exist for it, so nothing else is removed.
func (c *nomadCleanup) settleNeverPlaced(ctx context.Context) error {
	_, err := c.client.GetJob(ctx, c.state.Namespace, c.state.JobID)
	switch {
	case nomad.IsNotFound(err):
		return c.recordAbsent(ctx)
	case err != nil:
		return c.retained(nomadCleanupReasonHistoryMissing)
	}
	if err := c.client.StopJob(ctx, c.state.Namespace, c.state.JobID); err != nil {
		return c.fail(ctx, nomadCleanupReasonStopFailed)
	}
	if reason := c.confirmUnplacedStopped(ctx); reason != "" {
		return c.fail(ctx, reason)
	}
	return c.recordAbsent(ctx)
}

func (c *nomadCleanup) recordAbsent(ctx context.Context) error {
	c.state.Phase = nomadPhaseAbsent
	c.state.Revision++
	if err := checkpointNomadState(ctx, c.req, &c.state); err != nil {
		return c.retained(nomadCleanupReasonAbsentCheckpoint)
	}
	return c.finish(ctx)
}

// confirmUnplacedStopped waits until the stopped job has no non-terminal
// allocation, then returns an empty reason.
func (c *nomadCleanup) confirmUnplacedStopped(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, c.exec.stopConfirmTimeoutOrDefault())
	defer cancel()
	ticker := time.NewTicker(nomadAllocationPollInterval)
	defer ticker.Stop()
	for {
		allocs, err := c.client.ListAllocations(ctx, c.state.Namespace, c.state.JobID)
		if err != nil && ctx.Err() == nil {
			return nomadCleanupReasonHistoryMissing
		}
		if err == nil && allocationsTerminal(allocs) {
			return ""
		}
		select {
		case <-ctx.Done():
			return nomadCleanupReasonStopUnconfirmed
		case <-ticker.C:
		}
	}
}

func allocationsTerminal(allocs []nomad.AllocationRecord) bool {
	for _, alloc := range allocs {
		switch alloc.ClientStatus {
		case "complete", "failed", "lost":
		default:
			return false
		}
	}
	return true
}
