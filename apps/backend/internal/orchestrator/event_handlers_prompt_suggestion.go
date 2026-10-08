package orchestrator

import (
	"context"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// promptSuggestionMetadataKey stores the latest native next-prompt suggestion
// on the session so a reload restores it while its turn is still the latest.
const promptSuggestionMetadataKey = "prompt_suggestion"

// promptSuggestionSourceMetadataKey records whether the session's agent was
// asked for native suggestions, so the composer skips the utility fallback.
const promptSuggestionSourceMetadataKey = "prompt_suggestion_source"

const (
	promptSuggestionSourceNative = "native"
	promptSuggestionSourceNone   = "none"
)

// maxPromptSuggestionRunes bounds a stored suggestion. Native suggestions are a
// short sentence; anything this long is malformed and is dropped.
const maxPromptSuggestionRunes = 1000

// PromptSuggestionEventPayload is published on session.prompt_suggestion.
type PromptSuggestionEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
	Text      string `json:"text"`
}

type sessionLatestTurnReader interface {
	GetLatestTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error)
}

type sessionMetadataStateGuard interface {
	SetSessionMetadataKeyIfState(
		ctx context.Context,
		sessionID, key string,
		value interface{},
		expectedState models.TaskSessionState,
	) (bool, error)
}

// handlePromptSuggestionEvent binds a native suggestion to the session's latest
// turn and publishes it. The write only lands while the session is waiting for
// input, so a suggestion that loses the race with a newer prompt is dropped.
func (s *Service) handlePromptSuggestionEvent(ctx context.Context, payload *runtimeapi.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil || payload.SessionID == "" {
		return
	}
	if !s.resumeAttemptAllowsExecution(payload.SessionID, payload.ExecutionID, payload.AttemptID) {
		return
	}
	text := strings.TrimSpace(payload.Data.Text)
	if text == "" || utf8.RuneCountInString(text) >= maxPromptSuggestionRunes {
		return
	}
	turnID := s.latestSessionTurnID(ctx, payload.SessionID)
	guard, ok := s.repo.(sessionMetadataStateGuard)
	if turnID == "" || !ok {
		return
	}
	stored, err := guard.SetSessionMetadataKeyIfState(ctx, payload.SessionID, promptSuggestionMetadataKey,
		map[string]interface{}{"turn_id": turnID, "text": text}, models.TaskSessionStateWaitingForInput)
	if err != nil || !stored {
		s.logger.Debug("prompt suggestion not stored",
			zap.String("session_id", payload.SessionID), zap.Bool("stored", stored), zap.Error(err))
		return
	}
	if s.eventBus == nil {
		return
	}
	event := PromptSuggestionEventPayload{TaskID: payload.TaskID, SessionID: payload.SessionID, TurnID: turnID, Text: text}
	subject := events.BuildSessionPromptSuggestionSubject(payload.SessionID)
	if err := s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionPromptSuggestionUpdated, "orchestrator", event)); err != nil {
		s.logger.Warn("failed to publish prompt suggestion", zap.String("session_id", payload.SessionID), zap.Error(err))
	}
}

func (s *Service) latestSessionTurnID(ctx context.Context, sessionID string) string {
	reader, ok := s.repo.(sessionLatestTurnReader)
	if !ok {
		return ""
	}
	turn, err := reader.GetLatestTurnBySessionID(ctx, sessionID)
	if err != nil || turn == nil {
		return ""
	}
	return turn.ID
}

// GetSessionID lets the WebSocket session broadcaster route the event.
func (p PromptSuggestionEventPayload) GetSessionID() string { return p.SessionID }

// recordPromptSuggestionSource persists the negotiated native-suggestion
// capability reported when the agent starts. It writes only when the session
// becomes native or stops being native, so ordinary sessions add no write.
func (s *Service) recordPromptSuggestionSource(ctx context.Context, sessionID string, native bool) {
	if s.repo == nil {
		return
	}
	value := promptSuggestionSourceNone
	if native {
		value = promptSuggestionSourceNative
	} else {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil || session.Metadata[promptSuggestionSourceMetadataKey] != promptSuggestionSourceNative {
			return
		}
	}
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, promptSuggestionSourceMetadataKey, value); err != nil {
		s.logger.Warn("failed to record prompt suggestion source",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}
