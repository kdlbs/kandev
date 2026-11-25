package lifecycle

import (
	"context"
	"time"
)

// dispatchedPromptStallLoop runs the inactivity watchdog for dispatch-only
// prompts, which have no prompt waiter. A waiting prompt keeps its own
// watchdog in waitForPromptDone. The loop ends when the manager stops.
func (m *Manager) dispatchedPromptStallLoop(ctx context.Context) {
	defer m.wg.Done()
	if ctx == nil {
		ctx = context.Background()
	}
	interval, threshold := promptStallTiming()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	reported := make(map[string]uint64)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkDispatchedPromptStalls(ctx, threshold, reported)
		}
	}
}

// checkDispatchedPromptStalls publishes at most one stall notice per
// dispatch-only prompt generation. reported maps an execution ID to its last
// reported generation and drops executions that no longer await a dispatch.
func (m *Manager) checkDispatchedPromptStalls(ctx context.Context, threshold time.Duration, reported map[string]uint64) {
	if ctx == nil {
		ctx = context.Background()
	}
	pending := make(map[string]struct{})
	for _, execution := range m.executionStore.List() {
		if ctx.Err() != nil {
			return
		}
		if execution == nil || !execution.dispatchedPromptPending.Load() {
			continue
		}
		pending[execution.ID] = struct{}{}
		if err := execution.promptLifecycleMu.LockContext(ctx); err != nil {
			return
		}
		generation := execution.promptGeneration
		execution.promptLifecycleMu.Unlock()
		if generation == 0 || reported[execution.ID] == generation {
			continue
		}
		if _, stalled := m.sessionManager.reportPromptStall(ctx, execution, generation, threshold); stalled {
			reported[execution.ID] = generation
		}
	}
	for executionID := range reported {
		if _, ok := pending[executionID]; !ok {
			delete(reported, executionID)
		}
	}
}
