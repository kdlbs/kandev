package orchestrator

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// promptAttemptEvidence is deliberately process-local. It fences recovery to
// the concrete execution and prompt generation that produced the failure; the
// durable dynamic continuation package is stored with the route generation
// separately.
type promptAttemptEvidence struct {
	mu               sync.Mutex
	executionID      string
	promptGeneration uint64
	evidenceKnown    bool
	output           bool
	effect           bool
	dynamic          bool
}

func (s *Service) beginPromptAttempt(
	sessionID, executionID string,
	promptGeneration uint64,
	dynamic bool,
) {
	if sessionID == "" {
		return
	}
	s.dynamicAttemptEvidence.Store(sessionID, &promptAttemptEvidence{
		executionID:      executionID,
		promptGeneration: promptGeneration,
		evidenceKnown:    true,
		dynamic:          dynamic,
	})
}

func (s *Service) beginDynamicAttempt(sessionID string) {
	s.beginPromptAttempt(sessionID, "", 1, true)
}

func (s *Service) beginInteractivePromptAttempt(
	ctx context.Context,
	sessionID, executionID string,
	dynamic bool,
) {
	s.beginPromptAttempt(sessionID, executionID, s.nextPromptGeneration(ctx, sessionID), dynamic)
}

func (s *Service) beginInitialPromptAttempt(sessionID string, dynamic bool) {
	s.beginPromptAttempt(sessionID, "", 1, dynamic)
}

func (s *Service) bindPromptAttemptToExecution(ctx context.Context, sessionID, executionID string) {
	s.bindPromptAttempt(sessionID, executionID, s.promptGenerationForSession(ctx, sessionID))
}

func (s *Service) nextPromptGeneration(ctx context.Context, sessionID string) uint64 {
	generation := s.promptGenerationForSession(ctx, sessionID)
	if generation == ^uint64(0) {
		return 0
	}
	return generation + 1
}

func (s *Service) promptGenerationForSession(ctx context.Context, sessionID string) uint64 {
	if s.agentManager == nil || sessionID == "" {
		return 0
	}
	reader, ok := s.agentManager.(interface {
		GetPromptGenerationForSession(context.Context, string) (uint64, error)
	})
	if !ok {
		return 0
	}
	generation, err := reader.GetPromptGenerationForSession(ctx, sessionID)
	if err != nil {
		return 0
	}
	return generation
}

func (s *Service) isDynamicPromptSession(session *models.TaskSession) bool {
	return s.profileExecutionResolver != nil && session != nil &&
		session.RouteGeneration > 0 && session.ExecutionProfileID != ""
}

func (s *Service) bindPromptAttempt(sessionID, executionID string, promptGeneration uint64) {
	if sessionID == "" || (executionID == "" && promptGeneration == 0) {
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if executionID != "" {
		if evidence.executionID != "" && evidence.executionID != executionID {
			evidence.evidenceKnown = false
			return
		}
		evidence.executionID = executionID
	}
	if promptGeneration != 0 {
		if evidence.promptGeneration != 0 && evidence.promptGeneration != promptGeneration {
			evidence.evidenceKnown = false
			return
		}
		evidence.promptGeneration = promptGeneration
	}
}

func (s *Service) bindDynamicAttemptExecution(sessionID, executionID string) {
	s.bindPromptAttempt(sessionID, executionID, 0)
}

func (s *Service) observePromptAttempt(
	sessionID, executionID string,
	promptGeneration uint64,
	output, effect bool,
) {
	if sessionID == "" || (!output && !effect) {
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.promptIdentityMatchesLocked(executionID, promptGeneration) {
		return
	}
	evidence.output = evidence.output || output
	evidence.effect = evidence.effect || effect
}

func (s *Service) observeDynamicAttempt(sessionID, executionID string, output, effect bool) {
	s.observePromptAttempt(sessionID, executionID, 0, output, effect)
}

func (s *Service) withPromptAttemptEvidence(data watcher.AgentEventData) watcher.AgentEventData {
	if data.SessionID == "" {
		return data
	}
	// Lifecycle captures terminal failure evidence before publishing the
	// failure event. That snapshot is authoritative when stream and lifecycle
	// events travel through separate bus subscriptions; retain it while still
	// using the process-local record to fence the session, execution, and
	// generation identity.
	lifecycleEvidenceKnown := data.EvidenceKnown
	lifecycleOutputObserved := data.OutputObserved
	lifecycleEffectObserved := data.EffectObserved
	evidence, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		// Lifecycle evidence is only authoritative after the process-local
		// attempt record fences the session, execution, and generation. Without
		// that record, a terminal snapshot could authorize a replacement for an
		// unrelated or already-cleared attempt.
		data.EvidenceKnown = false
		data.OutputObserved = false
		data.EffectObserved = false
		return data
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.promptIdentityMatchesLocked(data.AgentExecutionID, data.PromptGeneration) {
		data.EvidenceKnown = false
		data.OutputObserved = false
		data.EffectObserved = false
		if evidence.dynamic {
			data.DynamicRouteAttempt = true
		}
		return data
	}
	if evidence.dynamic {
		data.DynamicRouteAttempt = true
	}
	if lifecycleEvidenceKnown {
		data.EvidenceKnown = true
		data.OutputObserved = lifecycleOutputObserved || evidence.output
		data.EffectObserved = lifecycleEffectObserved || evidence.effect
	} else {
		data.EvidenceKnown = evidence.evidenceKnown
		data.OutputObserved = evidence.output
		data.EffectObserved = evidence.effect
	}
	return data
}

func (s *Service) withDynamicAttemptEvidence(data watcher.AgentEventData) watcher.AgentEventData {
	data.DynamicRouteAttempt = true
	return s.withPromptAttemptEvidence(data)
}

func (s *Service) promptAttemptPreResultSafe(data watcher.AgentEventData) bool {
	if data.SessionID == "" || data.DynamicRouteAttempt ||
		data.AgentExecutionID == "" || data.PromptGeneration == 0 ||
		!data.EvidenceKnown || data.OutputObserved || data.EffectObserved {
		return false
	}
	evidence, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return !evidence.dynamic && evidence.evidenceKnown &&
		evidence.executionID == data.AgentExecutionID &&
		evidence.promptGeneration == data.PromptGeneration &&
		!evidence.output && !evidence.effect
}

func (s *Service) clearPromptAttemptEvidence(sessionID, executionID string, promptGeneration uint64) {
	if sessionID == "" {
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	matches := evidence.promptIdentityMatchesForClearLocked(executionID, promptGeneration)
	evidence.mu.Unlock()
	if matches {
		s.dynamicAttemptEvidence.CompareAndDelete(sessionID, evidence)
	}
}

func (s *Service) promptAttemptForSession(sessionID string) (*promptAttemptEvidence, bool) {
	v, ok := s.dynamicAttemptEvidence.Load(sessionID)
	if !ok {
		return nil, false
	}
	evidence, ok := v.(*promptAttemptEvidence)
	return evidence, ok
}

func (e *promptAttemptEvidence) promptIdentityMatchesLocked(executionID string, promptGeneration uint64) bool {
	if e.executionID != "" {
		if executionID == "" || e.executionID != executionID {
			e.evidenceKnown = false
			return false
		}
	} else if executionID != "" {
		e.executionID = executionID
	}
	if e.promptGeneration != 0 {
		if promptGeneration == 0 || e.promptGeneration != promptGeneration {
			e.evidenceKnown = false
			return false
		}
	} else if promptGeneration != 0 {
		e.promptGeneration = promptGeneration
	}
	return true
}

func (e *promptAttemptEvidence) promptIdentityMatchesForClearLocked(executionID string, promptGeneration uint64) bool {
	if e.executionID != "" && (executionID == "" || e.executionID != executionID) {
		return false
	}
	if e.promptGeneration != 0 && (promptGeneration == 0 || e.promptGeneration != promptGeneration) {
		return false
	}
	return true
}

func dynamicPreResultSafe(data watcher.AgentEventData) bool {
	return data.DynamicRouteAttempt && data.EvidenceKnown && !data.OutputObserved && !data.EffectObserved
}
