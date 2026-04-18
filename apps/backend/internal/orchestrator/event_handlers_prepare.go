package orchestrator

import (
	"context"
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

// handlePrepareCompleted persists prepare_result in session metadata using
// json_set to atomically set one key without clobbering others.
func (s *Service) handlePrepareCompleted(_ context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*lifecycle.PrepareCompletedEventPayload)
	if !ok {
		return nil
	}

	pr := lifecycle.SerializePrepareResult(&lifecycle.EnvPrepareResult{
		Success:      payload.Success,
		Steps:        payload.Steps,
		ErrorMessage: payload.ErrorMessage,
		Duration:     time.Duration(payload.DurationMs) * time.Millisecond,
	})
	if err := s.repo.SetSessionMetadataKey(context.Background(), payload.SessionID, "prepare_result", pr); err != nil {
		s.logger.Warn("failed to persist prepare_result",
			zap.String("session_id", payload.SessionID), zap.Error(err))
	}

	s.logger.Info("environment preparation completed",
		zap.String("session_id", payload.SessionID),
		zap.Bool("success", payload.Success),
		zap.Int("steps", len(payload.Steps)))
	return nil
}
