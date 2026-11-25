package acp

import (
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type cursorShellWatch struct {
	sessionID        string
	nativeGeneration uint64
	workID           string
	title            string
	promptGeneration uint64
	state            streams.RunState
}

func (a *Adapter) observeCursorShellWatch(
	sessionID string,
	update acp.SessionUpdate,
	promptGeneration uint64,
) *AgentEvent {
	if !a.cursorShellWatchEnabled() {
		return nil
	}
	frame, ok := a.dialect.cursorShellWatch(update)
	if !ok {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.cursorShellWatchSessionLocked(sessionID) {
		return nil
	}
	if frame.kind == cursorShellWatchRegistration {
		a.registerCursorShellWatchLocked(sessionID, frame, promptGeneration)
		return nil
	}
	watch, ok := a.advanceCursorShellWatchLocked(sessionID, frame)
	if !ok {
		return nil
	}
	return cursorShellWatchEvent(frame.toolCallID, watch)
}

func (a *Adapter) cursorShellWatchEnabled() bool {
	return a.agentID == cursorAgentID && a.backgroundWorkEnabled && a.dialect.cursorShellWatch != nil
}

func (a *Adapter) cursorShellWatchSessionLocked(sessionID string) bool {
	return !a.closed && sessionID != "" && a.sessionID == sessionID && a.nativeGeneration != 0
}

func (a *Adapter) registerCursorShellWatchLocked(
	sessionID string,
	frame cursorShellWatchFrame,
	promptGeneration uint64,
) {
	if _, exists := a.cursorShellWatches[frame.toolCallID]; exists {
		return
	}
	a.cursorShellWatches[frame.toolCallID] = cursorShellWatch{
		sessionID:        sessionID,
		nativeGeneration: a.nativeGeneration,
		workID:           uuid.NewString(),
		title:            frame.title,
		promptGeneration: promptGeneration,
	}
}

func (a *Adapter) advanceCursorShellWatchLocked(
	sessionID string,
	frame cursorShellWatchFrame,
) (cursorShellWatch, bool) {
	watch, exists := a.cursorShellWatches[frame.toolCallID]
	if !exists || watch.sessionID != sessionID || watch.nativeGeneration != a.nativeGeneration {
		return cursorShellWatch{}, false
	}
	if watch.state.IsTerminal() {
		return cursorShellWatch{}, false
	}
	switch frame.kind {
	case cursorShellWatchRunning:
		if watch.state != "" {
			return cursorShellWatch{}, false
		}
		watch.state = streams.RunStateRunning
	case cursorShellWatchCompleted:
		watch.state = streams.RunStateCompleted
	case cursorShellWatchFailed:
		watch.state = streams.RunStateFailed
	default:
		return cursorShellWatch{}, false
	}
	a.cursorShellWatches[frame.toolCallID] = watch
	return watch, true
}

func (a *Adapter) settleCursorShellWatchesForCancelledPrompt(sessionID string, promptGeneration uint64) {
	a.mu.Lock()
	if a.closed || sessionID == "" || a.sessionID != sessionID {
		a.mu.Unlock()
		return
	}
	var events []AgentEvent
	for callID, watch := range a.cursorShellWatches {
		if watch.sessionID != sessionID || watch.nativeGeneration != a.nativeGeneration ||
			watch.promptGeneration != promptGeneration || watch.state != streams.RunStateRunning {
			continue
		}
		watch.state = streams.RunStateInterrupted
		a.cursorShellWatches[callID] = watch
		events = append(events, *cursorShellWatchEvent(callID, watch))
	}
	a.mu.Unlock()
	for _, event := range events {
		a.sendUpdate(event)
	}
}

func cursorShellWatchEvent(callID string, watch cursorShellWatch) *AgentEvent {
	observation := &streams.WorkloadRunObservation{
		SessionID:    watch.sessionID,
		WorkID:       watch.workID,
		RunID:        watch.workID,
		Kind:         streams.WorkloadKindMonitor,
		Title:        watch.title,
		State:        watch.state,
		SourceCallID: callID,
		Capabilities: streams.InspectionOnlyWorkloadCapabilities(),
	}
	if watch.state.IsTerminal() {
		finishedAt := time.Now().UTC()
		observation.FinishedAt = &finishedAt
	}
	return &AgentEvent{
		Type:             streams.EventTypeBackgroundWorkUpdated,
		SessionID:        watch.sessionID,
		PromptGeneration: watch.promptGeneration,
		BackgroundWork:   observation,
	}
}
