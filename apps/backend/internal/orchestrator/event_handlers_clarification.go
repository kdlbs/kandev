package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// subscribeClarificationEvents subscribes to clarification-related events.
func (s *Service) subscribeClarificationEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationAnswered, s.handleClarificationAnswered); err != nil {
		s.logger.Error("failed to subscribe to clarification.answered events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationPrimaryAnswered, s.handleClarificationPrimaryAnswered); err != nil {
		s.logger.Error("failed to subscribe to clarification.primary_answered events", zap.Error(err))
	}
}

// clarificationAnsweredData is the event payload for ClarificationAnswered events.
type clarificationAnsweredData struct {
	SessionID    string `json:"session_id"`
	TaskID       string `json:"task_id"`
	PendingID    string `json:"pending_id"`
	Question     string `json:"question"`
	AnswerText   string `json:"answer_text"`
	Rejected     bool   `json:"rejected"`
	RejectReason string `json:"reject_reason"`
}

type clarificationWatchdogEntry struct {
	cancel func()
}

// handleClarificationAnswered handles user responses to agent clarification questions.
// It constructs a follow-up prompt with the answer and sends it to the agent.
func (s *Service) handleClarificationAnswered(ctx context.Context, event *bus.Event) error {
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		s.logger.Error("failed to marshal clarification event data", zap.Error(err))
		return nil
	}
	var data clarificationAnsweredData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		s.logger.Error("failed to parse clarification event data", zap.Error(err))
		return nil
	}

	if data.SessionID == "" || data.TaskID == "" {
		s.logger.Warn("clarification answered event missing session_id or task_id",
			zap.String("session_id", data.SessionID),
			zap.String("task_id", data.TaskID))
		return nil
	}

	prompt := buildClarificationPrompt(data)

	s.logger.Info("resuming agent with clarification answer",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.Bool("rejected", data.Rejected))

	if _, err := s.PromptTask(ctx, data.TaskID, data.SessionID, prompt, "", false, nil); err != nil {
		if !s.retryClarificationAfterCancel(ctx, data, prompt, err) {
			s.logger.Error("failed to resume agent with clarification answer",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.Error(err))
		}
	}
	return nil
}

func (s *Service) handleClarificationPrimaryAnswered(_ context.Context, event *bus.Event) error {
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		s.logger.Error("failed to marshal primary clarification event data", zap.Error(err))
		return nil
	}
	var data clarificationAnsweredData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		s.logger.Error("failed to parse primary clarification event data", zap.Error(err))
		return nil
	}
	if data.SessionID == "" || data.TaskID == "" || data.PendingID == "" {
		s.logger.Warn("primary clarification event missing identifiers",
			zap.String("session_id", data.SessionID),
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return nil
	}

	s.scheduleClarificationWatchdog(data)
	return nil
}

func (s *Service) clarificationWatchdogKey(sessionID, pendingID string) string {
	return sessionID + "::" + pendingID
}

func (s *Service) getClarificationWatchdogTimeout() time.Duration {
	if s.clarificationWatchdogTimeout > 0 {
		return s.clarificationWatchdogTimeout
	}
	// After primary path delivery, if the agent doesn't send events within 15s,
	// its MCP client has timed out and the response was dropped. Trigger fallback.
	return 15 * time.Second
}

func (s *Service) scheduleClarificationWatchdog(data clarificationAnsweredData) {
	key := s.clarificationWatchdogKey(data.SessionID, data.PendingID)
	timeout := s.getClarificationWatchdogTimeout()

	if old, ok := s.clarificationWatchdogs.LoadAndDelete(key); ok {
		if oldEntry, ok := old.(*clarificationWatchdogEntry); ok && oldEntry.cancel != nil {
			oldEntry.cancel()
		}
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	entry := &clarificationWatchdogEntry{cancel: cancel}
	s.clarificationWatchdogs.Store(key, entry)

	s.logger.Info("scheduled clarification resume watchdog",
		zap.String("session_id", data.SessionID),
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.Duration("timeout", timeout))

	go s.runClarificationWatchdog(watchCtx, key, entry, data, timeout)
}

func (s *Service) runClarificationWatchdog(
	watchCtx context.Context,
	key string,
	entry *clarificationWatchdogEntry,
	data clarificationAnsweredData,
	timeout time.Duration,
) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-watchCtx.Done():
		return
	case <-timer.C:
		current, ok := s.clarificationWatchdogs.LoadAndDelete(key)
		if !ok || current != entry {
			return
		}
		if entry.cancel != nil {
			entry.cancel()
		}
		s.resumeClarificationViaFallback(data)
	}
}

func (s *Service) resumeClarificationViaFallback(data clarificationAnsweredData) {
	prompt := buildClarificationPrompt(data)
	s.logger.Warn("clarification resume watchdog expired; triggering fallback resume",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("pending_id", data.PendingID))

	ctx := context.Background()
	if _, err := s.PromptTask(ctx, data.TaskID, data.SessionID, prompt, "", false, nil); err != nil {
		if !s.retryClarificationAfterCancel(ctx, data, prompt, err) {
			s.logger.Error("failed to resume agent via clarification watchdog fallback",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		}
	}
}

// retryClarificationAfterCancel handles the case where PromptTask fails because
// the agent is stuck in RUNNING state (MCP client timed out during clarification).
// It silently cancels the stuck turn and retries the prompt so the recovery is
// seamless for the user (no "Turn cancelled" separator in the chat).
// Returns true if recovery succeeded.
func (s *Service) retryClarificationAfterCancel(ctx context.Context, data clarificationAnsweredData, prompt string, promptErr error) bool {
	if !isAgentPromptInProgressError(promptErr) {
		return false
	}

	s.logger.Warn("agent stuck in RUNNING state during clarification recovery; cancelling turn",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))

	if err := s.cancelAgentSilent(ctx, data.TaskID, data.SessionID); err != nil {
		s.logger.Warn("cancel failed (agent likely dead), force-transitioning session state",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		// Force-revert session state so the retry prompt can proceed
		if revertErr := s.repo.UpdateTaskSessionState(ctx, data.SessionID, models.TaskSessionStateWaitingForInput, ""); revertErr != nil {
			s.logger.Error("failed to force-revert session state for clarification recovery",
				zap.String("session_id", data.SessionID),
				zap.Error(revertErr))
			return false
		}
		s.completeTurnForSession(ctx, data.SessionID)
	}

	if _, err := s.PromptTask(ctx, data.TaskID, data.SessionID, prompt, "", false, nil); err != nil {
		s.logger.Error("failed to resume agent after cancel in clarification recovery",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return false
	}

	s.logger.Info("successfully recovered stuck agent with clarification answer",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))
	return true
}

// cancelAgentSilent cancels the agent turn without creating a visible message
// in the chat. Used by clarification recovery so the cancel-and-retry is seamless.
func (s *Service) cancelAgentSilent(ctx context.Context, taskID, sessionID string) error {
	if err := s.agentManager.CancelAgent(ctx, sessionID); err != nil {
		return fmt.Errorf("cancel agent: %w", err)
	}

	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", true, nil)
	s.completeTurnForSession(ctx, sessionID)
	return nil
}

func (s *Service) cancelClarificationWatchdogsForSession(sessionID, reason string) {
	if sessionID == "" {
		return
	}

	prefix := sessionID + "::"
	cancelled := 0
	s.clarificationWatchdogs.Range(func(key, value interface{}) bool {
		keyStr, ok := key.(string)
		if !ok || !strings.HasPrefix(keyStr, prefix) {
			return true
		}
		s.clarificationWatchdogs.Delete(keyStr)
		if entry, ok := value.(*clarificationWatchdogEntry); ok && entry.cancel != nil {
			entry.cancel()
		}
		cancelled++
		return true
	})

	if cancelled > 0 {
		s.logger.Debug("cancelled clarification watchdogs after session activity",
			zap.String("session_id", sessionID),
			zap.String("reason", reason),
			zap.Int("count", cancelled))
	}
}

func (s *Service) cancelAllClarificationWatchdogs() {
	s.clarificationWatchdogs.Range(func(key, value interface{}) bool {
		keyStr, ok := key.(string)
		if ok {
			s.clarificationWatchdogs.Delete(keyStr)
		}
		if entry, ok := value.(*clarificationWatchdogEntry); ok && entry.cancel != nil {
			entry.cancel()
		}
		return true
	})
}

// buildClarificationPrompt constructs the resume prompt from a clarification answer.
func buildClarificationPrompt(data clarificationAnsweredData) string {
	if data.Rejected {
		reason := data.RejectReason
		if reason == "" {
			reason = "No reason provided"
		}
		return fmt.Sprintf("The user declined to answer your question: %q\nReason: %s\nPlease continue without this information.",
			data.Question, reason)
	}
	return fmt.Sprintf("You previously asked the user: %q\n%s\nPlease continue with this information.",
		data.Question, data.AnswerText)
}
