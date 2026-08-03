package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	stallCancelTurnButtonTestID = "stall-cancel-turn-button"
	metaKeyActionVisibility     = "action_visibility"
)

// handleAgentStalled persists an advisory recovery affordance without changing
// the prompt, session, or task lifecycle.
func (s *Service) handleAgentStalled(ctx context.Context, payload lifecycle.AgentStalledPayload) {
	if s.messageCreator == nil || s.repo == nil || payload.TaskID == "" || payload.SessionID == "" {
		return
	}
	lock, release := s.acquireCancelInFlightGuard(payload.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if s.isCancelInFlight(payload.SessionID) {
		s.logger.Debug("ignoring agent stall while cancellation is in progress",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}
	session, err := s.repo.GetTaskSession(ctx, payload.SessionID)
	if err != nil || session == nil || session.State != models.TaskSessionStateRunning {
		return
	}
	generationOwner, ok := s.agentManager.(interface {
		OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	})
	if !ok || payload.PromptGeneration == 0 || !generationOwner.OwnsPromptGeneration(
		payload.SessionID,
		payload.AgentExecutionID,
		payload.PromptGeneration,
	) {
		return
	}
	turnID, err := s.peekActiveTurnID(ctx, payload.SessionID)
	if err != nil {
		return
	}
	if s.turnService != nil && turnID == "" {
		return
	}
	metadata := map[string]interface{}{
		metaKeyActionVisibility: "running",
		metaKeySessionID:        payload.SessionID,
		metaKeyTaskID:           payload.TaskID,
		"actions": []map[string]interface{}{{
			actionMetaKeyType:   "ws_request",
			actionMetaKeyLabel:  "Cancel turn",
			actionMetaKeyTestID: stallCancelTurnButtonTestID,
			"params": map[string]interface{}{
				"method":  "agent.cancel",
				"payload": map[string]interface{}{"session_id": payload.SessionID},
			},
		}},
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx,
		payload.TaskID,
		stallNoticeContent(payload),
		payload.SessionID,
		string(v1.MessageTypeStatus),
		turnID,
		metadata,
		false,
	); err != nil {
		s.logger.Warn("failed to create agent stall notice",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.Error(err))
	}
}

func stallNoticeContent(payload lifecycle.AgentStalledPayload) string {
	tool := strings.TrimSpace(payload.ToolTitle)
	if tool == "" {
		tool = strings.TrimSpace(payload.ToolName)
	}
	if tool == "" {
		return "Still waiting for the agent."
	}
	return fmt.Sprintf("Still waiting on %s.", tool)
}
