package orchestrator

import (
	"sync"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// dynamicAttemptEvidence is deliberately process-local. It is only an
// observation fence for the current downstream execution; the durable
// continuation package is stored with the route generation separately.
type dynamicAttemptEvidence struct {
	mu            sync.Mutex
	executionID   string
	evidenceKnown bool
	output        bool
	effect        bool
}

func (s *Service) beginDynamicAttempt(sessionID string) {
	if sessionID == "" {
		return
	}
	s.dynamicAttemptEvidence.Store(sessionID, &dynamicAttemptEvidence{evidenceKnown: true})
}

func (s *Service) bindDynamicAttemptExecution(sessionID, executionID string) {
	if sessionID == "" || executionID == "" {
		return
	}
	v, ok := s.dynamicAttemptEvidence.Load(sessionID)
	if !ok {
		return
	}
	evidence, ok := v.(*dynamicAttemptEvidence)
	if !ok {
		return
	}
	evidence.mu.Lock()
	evidence.executionID = executionID
	evidence.mu.Unlock()
}

func (s *Service) observeDynamicAttempt(sessionID, executionID string, output, effect bool) {
	if sessionID == "" || (!output && !effect) {
		return
	}
	v, ok := s.dynamicAttemptEvidence.Load(sessionID)
	if !ok {
		return
	}
	evidence, ok := v.(*dynamicAttemptEvidence)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if evidence.executionID != "" && executionID != "" && evidence.executionID != executionID {
		return
	}
	if evidence.executionID != "" && executionID == "" {
		// Once a concrete execution is bound, an observation without its
		// execution identity is ambiguous. Keep the route gate fail-closed.
		evidence.evidenceKnown = false
		return
	}
	if evidence.executionID == "" && executionID != "" {
		evidence.executionID = executionID
	}
	evidence.output = evidence.output || output
	evidence.effect = evidence.effect || effect
}

func (s *Service) withDynamicAttemptEvidence(data watcher.AgentEventData) watcher.AgentEventData {
	if data.SessionID == "" {
		return data
	}
	v, ok := s.dynamicAttemptEvidence.Load(data.SessionID)
	if !ok {
		return data
	}
	evidence, ok := v.(*dynamicAttemptEvidence)
	if !ok {
		return data
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	data.DynamicRouteAttempt = true
	if evidence.executionID != "" &&
		(data.AgentExecutionID == "" || evidence.executionID != data.AgentExecutionID) {
		// The event belongs to a predecessor or successor that is not the
		// attempt represented by this evidence record. Keep the route gate
		// fail-closed and let the durable execution fence reject it too.
		data.EvidenceKnown = false
		data.OutputObserved = false
		data.EffectObserved = false
		return data
	}
	data.EvidenceKnown = evidence.evidenceKnown
	data.OutputObserved = evidence.output
	data.EffectObserved = evidence.effect
	return data
}

func (s *Service) clearDynamicAttemptEvidence(sessionID, executionID string) {
	if sessionID == "" || executionID == "" {
		return
	}
	v, ok := s.dynamicAttemptEvidence.Load(sessionID)
	if !ok {
		return
	}
	evidence, ok := v.(*dynamicAttemptEvidence)
	if !ok {
		return
	}
	evidence.mu.Lock()
	currentExecutionID := evidence.executionID
	evidence.mu.Unlock()
	if currentExecutionID == executionID {
		s.dynamicAttemptEvidence.CompareAndDelete(sessionID, evidence)
	}
}

func dynamicPreResultSafe(data watcher.AgentEventData) bool {
	return data.DynamicRouteAttempt && data.EvidenceKnown && !data.OutputObserved && !data.EffectObserved
}
