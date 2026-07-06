package acp

import (
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const defaultAsyncTurnCompleteIdle = 5 * time.Second

var asyncTurnCompleteIdle = defaultAsyncTurnCompleteIdle

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

	delay := asyncTurnCompleteIdle
	a.asyncTurnMu.Lock()
	finalizer := a.asyncTurnFinalizers[event.SessionID]
	if finalizer == nil {
		finalizer = &asyncTurnFinalizer{}
		a.asyncTurnFinalizers[event.SessionID] = finalizer
	}
	finalizer.seq++
	seq := finalizer.seq
	if finalizer.timer != nil {
		finalizer.timer.Stop()
	}
	finalizer.timer = time.AfterFunc(delay, func() {
		a.emitAsyncTurnComplete(event.SessionID, seq)
	})
	a.asyncTurnMu.Unlock()
}

func (a *Adapter) emitAsyncTurnComplete(sessionID string, seq uint64) {
	if !a.isCurrentAsyncTurnFinalizer(sessionID, seq) {
		return
	}
	if a.currentPromptTurn() != nil {
		a.consumeAsyncTurnFinalizer(sessionID, seq)
		return
	}

	a.syncNotifQueue()

	if a.currentPromptTurn() != nil {
		a.consumeAsyncTurnFinalizer(sessionID, seq)
		return
	}
	if !a.consumeAsyncTurnFinalizer(sessionID, seq) {
		return
	}

	a.logger.Info("emitting synthetic complete event for idle async ACP turn",
		zap.String("session_id", sessionID),
		zap.Duration("idle", asyncTurnCompleteIdle))
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

func (a *Adapter) isCurrentAsyncTurnFinalizer(sessionID string, seq uint64) bool {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizers[sessionID]
	return finalizer != nil && finalizer.seq == seq
}

func (a *Adapter) consumeAsyncTurnFinalizer(sessionID string, seq uint64) bool {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
	finalizer := a.asyncTurnFinalizers[sessionID]
	if finalizer == nil || finalizer.seq != seq {
		return false
	}
	delete(a.asyncTurnFinalizers, sessionID)
	return true
}

func (a *Adapter) cancelAsyncTurnComplete(sessionID string) {
	a.asyncTurnMu.Lock()
	defer a.asyncTurnMu.Unlock()
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
}
