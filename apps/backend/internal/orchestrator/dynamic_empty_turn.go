package orchestrator

import (
	"context"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

const stopReasonEndTurn = "end_turn"

// countedEmptyTurn identifies the prompt turn whose empty completion was
// counted as an unclassified failure, so the same turn's later completion
// cannot clear the count it produced.
type countedEmptyTurn struct {
	executionID      string
	promptGeneration uint64
}

// emptyTurnCompletion is the part of a stream completion frame that decides
// whether the turn ended normally.
type emptyTurnCompletion struct {
	data       watcher.AgentEventData
	stopReason string
	isError    bool
	// completeOutput is set when the completion frame itself carries assistant
	// output (message text or plan content) that the ordinary completion path
	// persists.
	completeOutput bool
}

// routeDynamicEmptyTurn counts a normal end_turn completion with no assistant
// output, reasoning, or tool activity as an unclassified failure of the
// current dynamic candidate. It reports true only when a successor was
// launched, in which case the caller skips the ordinary completion follow-up.
// Every precondition failure leaves the completion untouched.
func (s *Service) routeDynamicEmptyTurn(
	ctx context.Context,
	completion emptyTurnCompletion,
	session *models.TaskSession,
	streamGuard *lockedCancelInFlightGuard,
) bool {
	if !s.emptyTurnCompletionAdmissible(completion, session, streamGuard) {
		return false
	}
	// A completion frame carrying assistant output is not an empty turn, so it
	// is never counted; the caller persists that output.
	if completion.completeOutput {
		return false
	}
	data := s.withPromptAttemptEvidence(completion.data)
	if !dynamicPreResultSafe(data) || data.AgentExecutionID == "" || data.PromptGeneration == 0 ||
		!s.profileExecutionResolver.UnclassifiedPolicyEnabled(ctx, session.AgentProfileID, session.ExecutionProfileID) {
		return false
	}
	evidence := s.unclassifiedEmptyTurnEvidence(ctx, data, session)
	if !evidence.TaskScope || !evidence.CurrentAttempt || !evidence.StepKnown || evidence.StepVeto {
		return false
	}
	s.markCountedEmptyTurn(data)
	failure := &routingerr.Error{
		Code: routingerr.CodeAgentRuntime, Class: routingerr.ClassUnclassified, Phase: routingerr.PhasePromptSend,
	}
	result := s.routeDynamicAgentFailureWithEvidence(ctx, data, failure, &evidence, true)
	if !result.handled {
		s.retireCountedEmptyTurn(data.SessionID)
		return false
	}
	return !result.manualRecovery
}

// emptyTurnCompletionAdmissible requires a normal, non-error end_turn handled
// under the session's stream guard with no cancellation in flight.
func (s *Service) emptyTurnCompletionAdmissible(
	completion emptyTurnCompletion,
	session *models.TaskSession,
	streamGuard *lockedCancelInFlightGuard,
) bool {
	if session == nil || s.profileExecutionResolver == nil || completion.data.SessionID == "" {
		return false
	}
	if streamGuard == nil || !streamGuard.locked || s.isCancelInFlight(completion.data.SessionID) {
		return false
	}
	return completion.stopReason == stopReasonEndTurn && !completion.isError
}

// unclassifiedEmptyTurnEvidence builds trusted evidence for an empty end_turn.
// It carries no provider identity or diagnostic text.
func (s *Service) unclassifiedEmptyTurnEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) dynamicruntime.UnclassifiedFailureEvidence {
	currentAttempt := s.currentDynamicPromptAttempt(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
	return s.unclassifiedFailureEvidence(
		ctx, data, session, currentAttempt, dynamicruntime.UnclassifiedOriginEmptyTurn,
		routingerr.PhasePromptSend, promptUnclassifiedAttemptID(data), "", "", false,
		data.EvidenceKnown, data.OutputObserved, data.EffectObserved,
	)
}

func (s *Service) markCountedEmptyTurn(data watcher.AgentEventData) {
	s.countedEmptyTurns.Store(data.SessionID, countedEmptyTurn{
		executionID: data.AgentExecutionID, promptGeneration: data.PromptGeneration,
	})
}

// consumeCountedEmptyTurn reports whether the completion belongs to the turn
// whose empty end was counted, and retires that record.
func (s *Service) consumeCountedEmptyTurn(sessionID, executionID string, promptGeneration uint64) bool {
	if sessionID == "" {
		return false
	}
	value, ok := s.countedEmptyTurns.Load(sessionID)
	if !ok {
		return false
	}
	counted, ok := value.(countedEmptyTurn)
	if !ok || counted.executionID != executionID || counted.promptGeneration != promptGeneration {
		return false
	}
	s.countedEmptyTurns.CompareAndDelete(sessionID, value)
	return true
}

func (s *Service) retireCountedEmptyTurn(sessionID string) {
	if sessionID != "" {
		s.countedEmptyTurns.Delete(sessionID)
	}
}
