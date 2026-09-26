package backendapp

import (
	"errors"
	"fmt"
)

// quiesceForRestore stops every runtime that can write to the shared database
// before the restore path checkpoints and closes its pool. cancelWorkers is
// deliberately limited to worker/runtime contexts: it must not cancel the
// process context that owns the HTTP listener and restore job lifetime. The
// caller therefore remains able to publish restart_required and wait for an
// explicit process shutdown after the restore completes. The function keeps
// going after an individual stop failure so callers receive the complete
// failure set and no remaining worker is left running by an early return.
func quiesceForRestore(
	cancelWorkers func(),
	stopScheduling func() error,
	stopOrchestrator func() error,
	stopAgents func() error,
	stopWorkers []func() error,
) error {
	if cancelWorkers != nil {
		cancelWorkers()
	}
	var errs []error
	if stopScheduling != nil {
		if err := stopScheduling(); err != nil {
			errs = append(errs, fmt.Errorf("stop scheduling: %w", err))
		}
	}
	if stopOrchestrator != nil {
		if err := stopOrchestrator(); err != nil {
			errs = append(errs, fmt.Errorf("stop orchestrator: %w", err))
		}
	}
	if stopAgents != nil {
		if err := stopAgents(); err != nil {
			errs = append(errs, fmt.Errorf("stop agents: %w", err))
		}
	}
	for _, stopWorker := range stopWorkers {
		if stopWorker == nil {
			continue
		}
		if err := stopWorker(); err != nil {
			errs = append(errs, fmt.Errorf("stop database-backed worker: %w", err))
		}
	}
	return errors.Join(errs...)
}
