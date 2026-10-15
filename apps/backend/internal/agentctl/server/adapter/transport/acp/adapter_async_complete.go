package acp

import (
	"sync"
	"time"

	"github.com/google/uuid"
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
	if event.Type == streams.EventTypeToolUpdate {
		switch event.ToolStatus {
		case toolStatusComplete, toolStatusError, toolStatusCancelled:
			return false
		}
	}
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

// stampUpdateTurnIdentity preserves enqueue-time prompt ownership before
// journaling. Unowned native chunks receive a replay-stable message scope.
func (a *Adapter) stampUpdateTurnIdentity(event *AgentEvent, promptGeneration uint64) {
	if event == nil {
		return
	}
	if event.PromptGeneration == 0 && (isAsyncTurnContentEvent(*event) || isAsyncToolTerminal(*event)) {
		event.PromptGeneration = promptGeneration
	}
	if event.SessionID == "" || event.ProtocolMessageID != "" || event.PromptGeneration != 0 {
		return
	}
	if event.Type != streams.EventTypeMessageChunk && event.Type != streams.EventTypeReasoning {
		return
	}
	if a.currentPromptTurn() != nil {
		return
	}
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizerLocked(event.SessionID)
	if finalizer.messageID == "" {
		finalizer.messageID = "native-async-" + uuid.NewString()
	}
	event.ProtocolMessageID = finalizer.messageID
}

func (a *Adapter) asyncTurnFinalizerLocked(sessionID string) *asyncTurnFinalizer {
	finalizer := a.asyncTurnFinalizers[sessionID]
	if finalizer == nil {
		finalizer = &asyncTurnFinalizer{activeTools: make(map[string]struct{})}
		a.asyncTurnFinalizers[sessionID] = finalizer
	}
	return finalizer
}

func (a *Adapter) maybeScheduleAsyncTurnComplete(event AgentEvent) {
	if event.PromptGeneration != 0 || event.SessionID == "" ||
		(!isAsyncTurnContentEvent(event) && !isAsyncToolTerminal(event)) {
		return
	}
	if a.currentPromptTurn() != nil {
		return
	}

	delay := currentAsyncTurnCompleteIdle()
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizers[event.SessionID]
	if isAsyncToolTerminal(event) {
		if finalizer == nil {
			return
		}
		if _, tracked := finalizer.activeTools[event.ToolCallID]; !tracked {
			return
		}
	}
	if finalizer == nil {
		finalizer = a.asyncTurnFinalizerLocked(event.SessionID)
	}
	finalizer.trackTool(event)
	finalizer.seq++
	seq := finalizer.seq
	finalizer.promptEpoch = a.asyncTurnEpochs[event.SessionID]
	promptEpoch := finalizer.promptEpoch
	if finalizer.timer != nil {
		finalizer.timer.Stop()
	}
	// Silence is not completion while a foreground tool is known to be active.
	// Keep its finalizer until terminal evidence or a real prompt boundary.
	if len(finalizer.activeTools) != 0 {
		return
	}
	// time.AfterFunc goroutines are not tracked by workerWg. A timer that fires
	// concurrently with Close may call emitAsyncTurnComplete after
	// cancelAllAsyncTurnCompletes returns; the path is safe because sendUpdate
	// checks a.closed and syncNotifQueue respects lifetimeCtx.
	finalizer.timer = time.AfterFunc(delay, func() {
		a.emitAsyncTurnComplete(event.SessionID, seq, promptEpoch)
	})
}

func isAsyncToolTerminal(event AgentEvent) bool {
	if event.Type != streams.EventTypeToolCall && event.Type != streams.EventTypeToolUpdate {
		return false
	}
	switch event.ToolStatus {
	case toolStatusComplete, toolStatusError, toolStatusCancelled:
		return true
	default:
		return false
	}
}

func (f *asyncTurnFinalizer) trackTool(event AgentEvent) {
	if (event.Type != streams.EventTypeToolCall && event.Type != streams.EventTypeToolUpdate) || event.ToolCallID == "" {
		return
	}
	if isAsyncToolTerminal(event) || event.ParentToolCallID != "" || event.NormalizedPayload.IsActiveBackgroundWork() {
		delete(f.activeTools, event.ToolCallID)
		return
	}
	f.activeTools[event.ToolCallID] = struct{}{}
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
	if finalizer == nil || len(finalizer.activeTools) != 0 ||
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
	a.asyncTurnEpochs[sessionID]++
	a.cancelAsyncTurnCompleteLocked(sessionID)
	a.turnStartedAt[sessionID] = time.Now()
	a.asyncTurnMu.Unlock()

	// D3: this is the single turn boundary for the whole feature, fired on
	// both a human prompt and a synthetic ScheduleWakeup self-resume — the
	// caller never distinguishes them here. The backend clears its
	// background-launch attestation on this event.
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeTurnStarted,
		SessionID: sessionID,
	})
}

// RecordedTurnStart returns the time agentctl last dispatched session/prompt
// for sessionID, and whether one has been recorded at all. The
// background-workload liveness probe compares descendant process start times
// against this value; both are read on this host in this process's clock, so
// no timestamp needs to cross the process boundary.
func (a *Adapter) RecordedTurnStart(sessionID string) (time.Time, bool) {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	t, ok := a.turnStartedAt[sessionID]
	return t, ok
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
	for sessionID := range a.turnStartedAt {
		delete(a.turnStartedAt, sessionID)
	}
}
