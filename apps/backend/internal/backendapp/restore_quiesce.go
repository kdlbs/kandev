package backendapp

import (
	"errors"
	"fmt"
)

// quiesceForRestore stops every runtime that can write to the shared database
// before the restore path checkpoints and closes its pool. It keeps going
// after an individual stop failure so callers receive the complete failure
// set and no remaining worker is left running by an early return.
func quiesceForRestore(
	cancelContext func(),
	stopScheduling func() error,
	stopOrchestrator func() error,
	stopAgents func() error,
	stopWorkers []func() error,
) error {
	if cancelContext != nil {
		cancelContext()
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
