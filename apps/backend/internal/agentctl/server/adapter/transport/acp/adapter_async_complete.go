package acp

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const defaultAsyncTurnCompleteIdle = 5 * time.Second

// asyncTurnCompleteIdle is the debounce window. Tests shorten it via
// setAsyncTurnCompleteIdleForTest; do not add t.Parallel() to tests that call
// that helper, or they will race each other on this global.
var (
	asyncTurnCompleteIdleMu sync.RWMutex
	asyncTurnCompleteIdle   = defaultAsyncTurnCompleteIdle
)

func currentAsyncTurnCompleteIdle() time.Duration {
	asyncTurnCompleteIdleMu.RLock()
	defer asyncTurnCompleteIdleMu.RUnlock()
	return asyncTurnCompleteIdle
}

func isAsyncTurnContentEvent(event AgentEvent) bool {
	switch event.Type {
	case streams.EventTypeMessageChunk,
		streams.EventTypeReasoning,
		streams.EventTypeToolCall,
		streams.EventTypeToolUpdate,
		streams.EventTypePlan,
		streams.EventTypeAgentPlan,
		streams.EventTypePermissionRequest:
		return true
	default:
		return false
	}
}

func (a *Adapter) maybeScheduleAsyncTurnComplete(event AgentEvent) {
	if !isAsyncTurnContentEvent(event) || event.SessionID == "" {
		return
	}
	if a.currentPromptTurn() != nil {
		return
	}

	delay := currentAsyncTurnCompleteIdle()
	a.asyncTurnMu.Lock()
	finalizer := a.asyncTurnFinalizers[event.SessionID]
	if finalizer == nil {
		finalizer = &asyncTurnFinalizer{}
		a.asyncTurnFinalizers[event.SessionID] = finalizer
	}
	finalizer.seq++
	seq := finalizer.seq
	finalizer.promptEpoch = a.asyncTurnEpochs[event.SessionID]
	promptEpoch := finalizer.promptEpoch
	if finalizer.timer != nil {
		finalizer.timer.Stop()
	}
	// time.AfterFunc goroutines are not tracked by workerWg. A timer that fires
	// concurrently with Close may call emitAsyncTurnComplete after
	// cancelAllAsyncTurnCompletes returns; the path is safe because sendUpdate
	// checks a.closed and syncNotifQueue respects lifetimeCtx.
	finalizer.timer = time.AfterFunc(delay, func() {
		a.emitAsyncTurnComplete(event.SessionID, seq, promptEpoch)
	})
	a.asyncTurnMu.Unlock()
}

func (a *Adapter) emitAsyncTurnComplete(sessionID string, seq uint64, promptEpoch uint64) {
	// Fast-path stale timers before the more expensive notification-queue drain.
	// emitCurrentAsyncTurnComplete below is still the authoritative emit gate.
	if !a.isCurrentAsyncTurnFinalizer(sessionID, seq, promptEpoch) {
		return
	}
	if a.currentPromptTurn() != nil {
		a.consumeAsyncTurnFinalizer(sessionID, seq, promptEpoch)
		return
	}

	a.syncNotifQueue()

	if a.currentPromptTurn() != nil {
		a.consumeAsyncTurnFinalizer(sessionID, seq, promptEpoch)
		return
	}
	a.emitCurrentAsyncTurnComplete(sessionID, seq, promptEpoch)
}

func (a *Adapter) isCurrentAsyncTurnFinalizer(sessionID string, seq uint64, promptEpoch uint64) bool {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizers[sessionID]
	return finalizer != nil &&
		finalizer.seq == seq &&
		finalizer.promptEpoch == promptEpoch &&
		a.asyncTurnEpochs[sessionID] == promptEpoch
}

func (a *Adapter) consumeAsyncTurnFinalizer(sessionID string, seq uint64, promptEpoch uint64) bool {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizers[sessionID]
	if finalizer == nil ||
		finalizer.seq != seq ||
		finalizer.promptEpoch != promptEpoch ||
		a.asyncTurnEpochs[sessionID] != promptEpoch {
		return false
	}
	delete(a.asyncTurnFinalizers, sessionID)
	return true
}

func (a *Adapter) emitCurrentAsyncTurnComplete(sessionID string, seq uint64, promptEpoch uint64) {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizers[sessionID]
	if finalizer == nil ||
		finalizer.seq != seq ||
		finalizer.promptEpoch != promptEpoch ||
		a.asyncTurnEpochs[sessionID] != promptEpoch {
		return
	}
	delete(a.asyncTurnFinalizers, sessionID)

	a.logger.Info("emitting synthetic complete event for idle async ACP turn",
		zap.String("session_id", sessionID),
		zap.Duration("idle", currentAsyncTurnCompleteIdle()))
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeComplete,
		SessionID: sessionID,
		Data: map[string]any{
			"stop_reason":      "end_turn",
			"synthetic":        true,
			"synthetic_reason": "async_turn_idle",
		},
	})
}

func (a *Adapter) beginPromptTurn(sessionID string) {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	a.asyncTurnEpochs[sessionID]++
	a.cancelAsyncTurnCompleteLocked(sessionID)
}

func (a *Adapter) cancelAsyncTurnComplete(sessionID string) {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	a.cancelAsyncTurnCompleteLocked(sessionID)
}

func (a *Adapter) cancelAsyncTurnCompleteLocked(sessionID string) {
	finalizer := a.asyncTurnFinalizers[sessionID]
	if finalizer == nil {
		return
	}
	if finalizer.timer != nil {
		finalizer.timer.Stop()
	}
	delete(a.asyncTurnFinalizers, sessionID)
}

func (a *Adapter) cancelAllAsyncTurnCompletes() {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	for sessionID, finalizer := range a.asyncTurnFinalizers {
		if finalizer.timer != nil {
			finalizer.timer.Stop()
		}
		delete(a.asyncTurnFinalizers, sessionID)
	}
	for sessionID := range a.asyncTurnEpochs {
		delete(a.asyncTurnEpochs, sessionID)
	}
}
