package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func (s *Service) handleBackgroundWorkUpdatedEvent(ctx context.Context, payload *runtime.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || payload.Data.BackgroundWork == nil {
		return
	}
	obs := *payload.Data.BackgroundWork
	obs.SessionID = sessionID
	if s.backgroundWorkObserver != nil {
		_ = s.backgroundWorkObserver.RecordBackgroundWorkloadObservation(ctx, obs, payload.TaskID, sessionID)
	}
	if s.eventBus != nil {
		subject := events.BuildBackgroundWorkUpdatedSubject(sessionID)
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkUpdated, "orchestrator", &obs))
	}
}

func (s *Service) handleBackgroundWorkOutputEvent(ctx context.Context, payload *runtime.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || payload.Data.BackgroundWorkOutput == nil {
		return
	}
	chunk := *payload.Data.BackgroundWorkOutput
	chunk.SessionID = sessionID
	if s.backgroundWorkObserver != nil {
		_ = s.backgroundWorkObserver.AppendBackgroundWorkloadOutput(ctx, chunk, sessionID)
	}
	if s.eventBus != nil {
		subject := events.BuildBackgroundWorkOutputSubject(sessionID)
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkOutput, "orchestrator", &chunk))
	}
}

func buildObservationFromNormalized(payload *runtime.AgentStreamEventPayload, originTurnID string) *streams.WorkloadRunObservation {
	if payload.Data.Normalized == nil {
		return nil
	}
	bw := payload.Data.Normalized.BackgroundWork()
	if bw == nil {
		return nil
	}
	state := streams.RunStateRunning
	if bw.Ended || (!bw.Detached && isTerminalToolStatus(payload.Data.ToolStatus)) {
		state = streams.RunStateCompleted
		if payload.Data.ToolStatus == agentEventFailed || payload.Data.ToolStatus == agentEventError {
			state = streams.RunStateFailed
		}
	}
	title := payload.Data.ToolTitle
	if title == "" {
		title = payload.Data.ToolName
	}
	workID := bw.WorkID
	if workID == "" {
		workID = payload.Data.ToolCallID
	}
	return &streams.WorkloadRunObservation{
		SessionID:    payload.SessionID,
		WorkID:       workID,
		Kind:         streams.NormalizeWorkloadKind(string(bw.Kind)),
		Title:        title,
		State:        state,
		ParentWorkID: payload.Data.ParentToolCallID,
		SourceCallID: payload.Data.ToolCallID,
		OriginTurnID: originTurnID,
	}
}

func (s *Service) recordBackgroundWorkObservationFromToolPayload(ctx context.Context, payload *runtime.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		return
	}
	var obs streams.WorkloadRunObservation
	switch {
	case payload.Data.BackgroundWork != nil:
		obs = *payload.Data.BackgroundWork
		obs.SessionID = payload.SessionID
		if obs.OriginTurnID == "" {
			obs.OriginTurnID = s.nonCreatingActiveTurnID(ctx, payload.SessionID)
		}
	case payload.Data.Normalized != nil:
		fromNorm := buildObservationFromNormalized(payload, s.nonCreatingActiveTurnID(ctx, payload.SessionID))
		if fromNorm == nil {
			return
		}
		obs = *fromNorm
	default:
		return
	}

	if s.backgroundWorkObserver != nil {
		_ = s.backgroundWorkObserver.RecordBackgroundWorkloadObservation(ctx, obs, payload.TaskID, payload.SessionID)
	}
	if s.eventBus != nil {
		subject := events.BuildBackgroundWorkUpdatedSubject(payload.SessionID)
		_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.BackgroundWorkUpdated, "orchestrator", &obs))
	}
}
