package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// fixedProfileForCleanCompletion captures the fixed profile only while this
// execution still owns the completing turn. The caller resets its count only
// after checked turn settlement succeeds, under the same session guard.
func (s *Service) fixedProfileForCleanCompletion(ctx context.Context, data watcher.AgentEventData) string {
	if s.profileExecutionResolver == nil || s.repo == nil || s.turnService == nil || !cleanFixedCompletion(data) {
		return ""
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || !fixedCompletionMatchesSession(data, session) {
		return ""
	}
	turn, err := s.turnService.GetActiveTurn(ctx, session.ID)
	if err != nil || turn == nil || turn.ID != data.TurnID {
		return ""
	}
	return fixedSessionProfileID(session)
}

func cleanFixedCompletion(data watcher.AgentEventData) bool {
	if data.ExitCode != nil && *data.ExitCode != 0 {
		return false
	}
	return data.TurnID != "" && data.AgentExecutionID != "" &&
		data.ProviderError == nil && data.ErrorMessage == "" && data.FailureCode == "" &&
		!data.UserInitiated &&
		(data.DeliveryOutcome == "" || data.DeliveryOutcome == string(models.DeliverySubmissionCompleted))
}

func fixedCompletionMatchesSession(data watcher.AgentEventData, session *models.TaskSession) bool {
	if session == nil || session.RouteGeneration != 0 || isTerminalSessionState(session.State) ||
		session.TaskID != data.TaskID || session.AgentExecutionID != data.AgentExecutionID {
		return false
	}
	profileID := fixedSessionProfileID(session)
	return profileID != "" &&
		(data.ExecutionProfileID == "" || data.ExecutionProfileID == profileID) &&
		(data.AgentProfileID == "" || data.AgentProfileID == session.AgentProfileID)
}

func (s *Service) fixedProfileForCleanStreamCompletion(
	ctx context.Context,
	payload *agentruntime.AgentStreamEventPayload,
	turnID string,
) string {
	if payload.Data == nil || payload.Data.Type != agentEventComplete ||
		extractCompleteIsError(payload) || extractStopReason(payload) == stopReasonCancelled ||
		s.streamEventIsStalePrompt(payload) {
		return ""
	}
	return s.fixedProfileForCleanCompletion(ctx, watcher.AgentEventData{
		TaskID: payload.TaskID, SessionID: payload.SessionID, TurnID: turnID,
		AgentExecutionID: payload.ExecutionID, AgentProfileID: payload.AgentProfileID,
		ExecutionProfileID: payload.ExecutionProfileID, ProviderError: payload.Data.ProviderError,
		ErrorMessage: payload.Data.Error,
	})
}
