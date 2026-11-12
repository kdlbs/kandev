package orchestrator

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

type keywordObservation struct {
	group      string
	candidate  string
	text       string
	path       string
	model      string
	execution  string
	generation uint64
}

func (s *Service) previousKeywordHistory(sessionID string) []keywordObservation {
	if !s.config.ProviderKeywordContext {
		s.clearKeywordFailureHistory(sessionID)
		return nil
	}
	if previous, ok := s.promptAttemptForSession(sessionID); ok {
		previous.mu.Lock()
		invalidated := previous.effect || (previous.output && previous.providerDiagnosticText == "")
		previous.mu.Unlock()
		if invalidated {
			return nil
		}
	}
	s.keywordFailureHistoryMu.Lock()
	defer s.keywordFailureHistoryMu.Unlock()
	return append([]keywordObservation(nil), s.keywordFailureHistory[sessionID]...)
}

func (s *Service) clearKeywordFailureHistory(sessionID string) {
	if sessionID == "" {
		return
	}
	s.keywordFailureHistoryMu.Lock()
	delete(s.keywordFailureHistory, sessionID)
	s.keywordFailureHistoryMu.Unlock()
}

func (s *Service) refineKeywordFailure(ctx context.Context, data watcher.AgentEventData, failure *routingerr.Error) *routingerr.Error {
	if !s.config.ProviderKeywordContext || failure == nil || failure.Keyword == nil || !hasSemanticProviderDiagnostic(data) || !dynamicPreResultSafe(data) {
		return failure
	}
	c := failure.Keyword.Context
	if s.profileExecutionResolver != nil {
		metadata := s.profileExecutionResolver.ProviderKeywordContext(ctx, data.ExecutionProfileID, data.SessionID)
		c.AccountID, c.Tier = metadata.AccountID, metadata.Tier
		if c.ModelID == "" {
			c.ModelID = metadata.ModelID
		}
		if c.PathID == "" {
			c.PathID = metadata.PathID
		}
	}
	c = s.observeKeywordFailure(data, c, failure.Keyword.Group)
	c.FreeDaily = strings.HasSuffix(c.ModelID, ":free") && strings.Contains(strings.ToLower(data.ProviderError.Message), "daily")
	refined := classifyKanbanFailure(data, c)
	// Refinement annotates an already admitted classification. It cannot
	// promote an unknown failure past its original admission contract.
	if refined.Code != failure.Code {
		return failure
	}
	return refined
}

func (s *Service) observeKeywordFailure(data watcher.AgentEventData, c routingerr.KeywordContext, group string) routingerr.KeywordContext {
	attempt, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		return c
	}
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	currentAttempt, ok := s.promptAttemptForSession(data.SessionID)
	if !ok || currentAttempt != attempt {
		return c
	}
	if !attempt.promptIdentityMatchesLocked(data.AgentExecutionID, data.PromptGeneration) {
		return c
	}
	text := normalizeDiagnosticText(data.ProviderError.Message)
	current := keywordObservation{group: group, candidate: data.ExecutionProfileID, text: text, path: c.PathID, model: c.ModelID, execution: data.AgentExecutionID, generation: data.PromptGeneration}
	s.keywordFailureHistoryMu.Lock()
	defer s.keywordFailureHistoryMu.Unlock()
	if s.keywordFailureHistory == nil {
		s.keywordFailureHistory = make(map[string][]keywordObservation)
	}
	history := s.keywordFailureHistory[data.SessionID]
	seen, otherPaths, otherModels := summarizeKeywordHistory(history, current)
	if !seen {
		if len(history) == 16 {
			history = history[1:]
		}
		history = append(history, current)
	}
	s.keywordFailureHistory[data.SessionID] = history
	attempt.keywordHistory = append(attempt.keywordHistory[:0], history...)
	c.ConsecutiveFailures = consecutiveKeywordCount(history, current)
	c.OtherPaths, c.OtherModels = otherPaths, otherModels
	return c
}

func summarizeKeywordHistory(history []keywordObservation, current keywordObservation) (bool, int, int) {
	paths, models := map[string]struct{}{}, map[string]struct{}{}
	seen := false
	for _, observation := range history {
		if observation.execution == current.execution && observation.generation == current.generation {
			seen = true
		}
		if observation.text != current.text || observation.group != current.group {
			continue
		}
		if observation.path != current.path && observation.path != "" {
			paths[observation.path] = struct{}{}
		}
		if observation.model != current.model && observation.model != "" {
			models[observation.model] = struct{}{}
		}
	}
	return seen, len(paths), len(models)
}

func consecutiveKeywordCount(history []keywordObservation, current keywordObservation) int {
	if current.candidate == "" {
		return 1
	}
	count := 0
	for index := len(history) - 1; index >= 0; index-- {
		observation := history[index]
		if observation.group != current.group || observation.candidate != current.candidate || observation.text != current.text || observation.path != current.path || observation.model != current.model {
			break
		}
		count++
	}
	return count
}

func (s *Service) classifyKanbanFailure(data watcher.AgentEventData) *routingerr.Error {
	if !s.config.ProviderKeywordContext {
		return classifyKanbanFailure(data)
	}
	context := routingerr.KeywordContext{Enabled: true}
	if hasSemanticProviderDiagnostic(data) {
		context.Diagnostic = true
		context.HTTPStatus = data.ProviderError.HTTPStatus
		context.ModelID = data.ProviderError.ModelID
		context.PathID = data.ProviderError.ProviderID
	}
	context.EmptyOutput = data.EvidenceKnown && !data.OutputObserved && !data.EffectObserved
	return classifyKanbanFailure(data, context)
}

// Semantic matching consumes a validated terminal diagnostic. Raw identity
// completeness remains separate in the existing unclassified retry path.
func hasSemanticProviderDiagnostic(data watcher.AgentEventData) bool {
	p := data.ProviderError
	return p != nil && p.Valid() && completeProviderDiagnosticSource(p.Source)
}
