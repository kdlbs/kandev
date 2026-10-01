package orchestrator

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// subscribePrepareEvents subscribes to environment preparation events.
func (s *Service) subscribePrepareEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.ExecutorPrepareProgress, s.handlePrepareProgress); err != nil {
		s.logger.Error("failed to subscribe to executor.prepare.progress events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.ExecutorPrepareCompleted, s.handlePrepareCompleted); err != nil {
		s.logger.Error("failed to subscribe to executor.prepare.completed events", zap.Error(err))
	}
}

type preparationAttemptMarker struct {
	PreparationID        string `json:"preparation_id"`
	PreparationStartedAt string `json:"preparation_started_at"`
}

const prepareMetadataHandlerTimeout = 3 * time.Second

// handlePrepareProgress stores only the attempt identity. Progress details
// already flow to clients and may include command output that should not be
// copied into session metadata.
func (s *Service) handlePrepareProgress(_ context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*lifecycle.PrepareProgressEventPayload)
	if !ok || payload.SessionID == "" || payload.PreparationID == "" || payload.PreparationStartedAt == "" || s.repo == nil {
		return nil
	}
	startedAt, err := time.Parse(time.RFC3339Nano, payload.PreparationStartedAt)
	if err != nil {
		return nil
	}

	s.prepareResultMu.Lock()
	defer s.prepareResultMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), prepareMetadataHandlerTimeout)
	defer cancel()
	current, err := s.currentPreparationAttempt(ctx, payload.SessionID)
	if err != nil {
		s.logger.Warn("failed to read preparation attempt marker",
			zap.String("session_id", payload.SessionID), zap.Error(err))
		return nil
	}
	if preparationAttemptIsOlder(payload.PreparationStartedAt, payload.PreparationID, current.PreparationStartedAt, current.PreparationID) {
		return nil
	}
	marker := preparationAttemptMarker{
		PreparationID:        payload.PreparationID,
		PreparationStartedAt: startedAt.UTC().Format(time.RFC3339Nano),
	}
	if current != marker {
		if err := s.repo.SetSessionMetadataKey(ctx, payload.SessionID, "prepare_attempt", marker); err != nil {
			s.logger.Warn("failed to persist preparation attempt marker",
				zap.String("session_id", payload.SessionID), zap.Error(err))
		}
	}
	return nil
}

// handlePrepareCompleted persists prepare_result in session metadata using
// json_set to atomically set one key without clobbering others.
func (s *Service) handlePrepareCompleted(_ context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*lifecycle.PrepareCompletedEventPayload)
	if !ok || payload.SessionID == "" || s.repo == nil {
		return nil
	}

	s.prepareResultMu.Lock()
	defer s.prepareResultMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), prepareMetadataHandlerTimeout)
	defer cancel()
	current, err := s.currentPreparationAttempt(ctx, payload.SessionID)
	if err != nil {
		s.logger.Warn("failed to read preparation attempt marker",
			zap.String("session_id", payload.SessionID), zap.Error(err))
		return nil
	}
	if preparationAttemptIsOlder(payload.PreparationStartedAt, payload.PreparationID, current.PreparationStartedAt, current.PreparationID) {
		return nil
	}

	pr := lifecycle.SerializePrepareResult(&lifecycle.EnvPrepareResult{
		Success:              payload.Success,
		Steps:                payload.Steps,
		ErrorMessage:         payload.ErrorMessage,
		Duration:             time.Duration(payload.DurationMs) * time.Millisecond,
		PreparationID:        payload.PreparationID,
		PreparationStartedAt: parsePreparationStartedAt(payload.PreparationStartedAt),
	})
	marker := preparationAttemptMarker{
		PreparationID:        payload.PreparationID,
		PreparationStartedAt: payload.PreparationStartedAt,
	}
	if payload.PreparationID != "" && payload.PreparationStartedAt != "" && current != marker {
		if err := s.repo.SetSessionMetadataKey(ctx, payload.SessionID, "prepare_attempt", marker); err != nil {
			s.logger.Warn("failed to persist preparation attempt marker",
				zap.String("session_id", payload.SessionID), zap.Error(err))
			return nil
		}
	}
	if err := s.repo.SetSessionMetadataKey(ctx, payload.SessionID, "prepare_result", pr); err != nil {
		s.logger.Warn("failed to persist prepare_result",
			zap.String("session_id", payload.SessionID), zap.Error(err))
	}

	s.logger.Info("environment preparation completed",
		zap.String("session_id", payload.SessionID),
		zap.Bool("success", payload.Success),
		zap.Int("steps", len(payload.Steps)))
	return nil
}

func (s *Service) currentPreparationAttempt(ctx context.Context, sessionID string) (preparationAttemptMarker, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return preparationAttemptMarker{}, err
	}
	marker := preparationMarkerFromValue(session.Metadata["prepare_attempt"])
	if marker.PreparationStartedAt != "" {
		return marker, nil
	}
	return preparationMarkerFromValue(session.Metadata["prepare_result"]), nil
}

func preparationMarkerFromValue(value interface{}) preparationAttemptMarker {
	var marker preparationAttemptMarker
	encoded, err := json.Marshal(value)
	if err != nil || json.Unmarshal(encoded, &marker) != nil {
		return preparationAttemptMarker{}
	}
	return marker
}

func preparationAttemptIsOlder(incomingStartedAt, incomingID, currentStartedAt, currentID string) bool {
	if currentStartedAt == "" {
		return false
	}
	incoming, incomingErr := time.Parse(time.RFC3339Nano, incomingStartedAt)
	current, currentErr := time.Parse(time.RFC3339Nano, currentStartedAt)
	if currentErr != nil {
		return false
	}
	if incomingErr != nil || incomingID == "" {
		return true
	}
	if incoming.Equal(current) {
		return incomingID != currentID
	}
	return incoming.Before(current)
}

func parsePreparationStartedAt(value string) time.Time {
	startedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return startedAt
}
