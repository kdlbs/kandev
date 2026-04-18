package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)


// subscribePrepareEvents subscribes to environment preparation events.
func (s *Service) subscribePrepareEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.ExecutorPrepareCompleted, s.handlePrepareCompleted); err != nil {
		s.logger.Error("failed to subscribe to executor.prepare.completed events", zap.Error(err))
	}
}

// handlePrepareCompleted persists the prepare result in session metadata so it
// survives page refreshes. The frontend hydrates this back into the store on SSR.
func (s *Service) handlePrepareCompleted(ctx context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*lifecycle.PrepareCompletedEventPayload)
	if !ok {
		// Fallback: JSON round-trip for cases where the type doesn't match exactly.
		dataBytes, err := json.Marshal(event.Data)
		if err != nil {
			s.logger.Error("failed to marshal prepare completed event data",
				zap.String("actual_type", fmt.Sprintf("%T", event.Data)), zap.Error(err))
			return nil
		}
		var p lifecycle.PrepareCompletedEventPayload
		if err := json.Unmarshal(dataBytes, &p); err != nil {
			s.logger.Error("failed to unmarshal prepare completed payload",
				zap.String("actual_type", fmt.Sprintf("%T", event.Data)), zap.Error(err))
			return nil
		}
		payload = &p
		s.logger.Warn("prepare completed event used JSON fallback",
			zap.String("actual_type", fmt.Sprintf("%T", event.Data)))
	}

	session, err := s.repo.GetTaskSession(ctx, payload.SessionID)
	if err != nil {
		s.logger.Error("failed to get session for prepare persistence",
			zap.String("session_id", payload.SessionID), zap.Error(err))
		return nil
	}

	metadata := session.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	metadata["prepare_result"] = lifecycle.SerializePrepareResult(&lifecycle.EnvPrepareResult{
		Success:      payload.Success,
		Steps:        payload.Steps,
		ErrorMessage: payload.ErrorMessage,
		Duration:     time.Duration(payload.DurationMs) * time.Millisecond,
	})

	if err := s.repo.UpdateSessionMetadata(ctx, payload.SessionID, metadata); err != nil {
		s.logger.Error("failed to persist prepare result in session metadata",
			zap.String("session_id", payload.SessionID), zap.Error(err))
		return nil
	}

	s.logger.Info("persisted prepare result in session metadata",
		zap.String("session_id", payload.SessionID),
		zap.Bool("success", payload.Success),
		zap.Int("steps", len(payload.Steps)))
	return nil
}
