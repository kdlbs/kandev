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

// pinnedWithoutAllocation reports a pinned environment that recorded no
// allocation. Its worker fence is bound to the allocation, so none was written
// and there is nothing to release.
func (c *nomadCleanup) pinnedWithoutAllocation() bool {
	return c.state.PlacementMode != nomad.ModePool && c.state.AllocationID == ""
}

// pinnedNeverAllocated reports a pinned job whose allocation history is
// provably empty: no allocation ever existed, so no runtime can exist on the
// node. An unreadable history is not proof and keeps the row fail-closed, and
// so does a recorded allocation: a purged job lists no allocations once they
// are garbage collected, so an empty list cannot disprove a recorded one.
func (c *nomadCleanup) pinnedNeverAllocated(ctx context.Context) bool {
	if c.state.PlacementMode == nomad.ModePool || c.state.JobID == "" {
		return false
	}
	if c.state.AllocationID != "" || c.state.AllocationCreateIndex != 0 {
		return false
	}
	allocs, err := c.client.ListAllocations(ctx, c.state.Namespace, c.state.JobID)
	return err == nil && len(allocs) == 0
}

// settleNeverAllocated finishes a pinned job that never received an allocation
// without fencing the node. A registered job is stopped first, and absence is
// recorded only if the history is still empty once the stop is confirmed.
func (c *nomadCleanup) settleNeverAllocated(ctx context.Context) error {
	if !c.hasReceipt() {
		if err := c.recordEmptyDrainReceipt(ctx); err != nil {
			return err
		}
	}
	c.removeBootstrapEnvironment(ctx)
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
	allocs, err := c.client.ListAllocations(ctx, c.state.Namespace, c.state.JobID)
	switch {
	case err != nil:
		return c.fail(ctx, nomadCleanupReasonHistoryMissing)
	case len(allocs) != 0:
		return c.fail(ctx, nomadCleanupReasonUnexpectedAllocation)
	}
	return c.recordAbsent(ctx)
}
