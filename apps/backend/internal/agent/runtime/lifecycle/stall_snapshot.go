package lifecycle

import (
	"context"
	"time"
)

type promptStallSnapshot struct {
	lastActivity   time.Time
	activityEpoch  uint64
	accepted       bool
	outputObserved bool
	diagnostic     stallDiagnostic
}

func (sm *SessionManager) capturePromptStall(
	ctx context.Context,
	execution *AgentExecution,
	generation uint64,
) (promptStallSnapshot, bool) {
	// Lock order: promptLifecycleMu, then executionStore.mu; never the reverse.
	if ctx == nil {
		ctx = context.Background()
	}
	if err := execution.promptLifecycleMu.LockContext(ctx); err != nil {
		return promptStallSnapshot{}, false
	}
	defer execution.promptLifecycleMu.Unlock()
	if sm.executionStore != nil {
		sm.executionStore.mu.RLock()
		defer sm.executionStore.mu.RUnlock()
		if sm.executionStore.executions[execution.ID] != execution {
			return promptStallSnapshot{}, false
		}
	}
	if generation == 0 || execution.promptGeneration != generation ||
		execution.promptCompletionGeneration == generation {
		return promptStallSnapshot{}, false
	}
	at, output, epoch := execution.promptActivitySnapshot()
	snapshot := promptStallSnapshot{
		lastActivity: at, activityEpoch: epoch,
		accepted:       execution.dispatchedPromptGeneration == generation,
		outputObserved: output,
	}
	snapshot.diagnostic = newStallDiagnostic(execution, generation, snapshot)
	return snapshot, true
}
