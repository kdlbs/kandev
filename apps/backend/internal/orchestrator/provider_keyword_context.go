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
		return nil
	}
	previous, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return nil
	}
	previous.mu.Lock()
	defer previous.mu.Unlock()
	if previous.effect || (previous.output && previous.providerDiagnosticText == "") {
		return nil
	}
	return append([]keywordObservation(nil), previous.keywordHistory...)
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
	if !attempt.promptIdentityMatchesLocked(data.AgentExecutionID, data.PromptGeneration) {
		return c
	}
	text := normalizeDiagnosticText(data.ProviderError.Message)
	paths, models := map[string]bool{}, map[string]bool{}
	seen := false
	for _, observation := range attempt.keywordHistory {
		if observation.execution == data.AgentExecutionID && observation.generation == data.PromptGeneration {
			seen = true
		}
		if observation.text != text || observation.group != group {
			continue
		}
		if observation.path != c.PathID && observation.path != "" {
			paths[observation.path] = true
		}
		if observation.model != c.ModelID && observation.model != "" {
			models[observation.model] = true
		}
	}
	current := keywordObservation{group: group, candidate: data.ExecutionProfileID, text: text, path: c.PathID, model: c.ModelID, execution: data.AgentExecutionID, generation: data.PromptGeneration}
	if !seen {
		if len(attempt.keywordHistory) == 16 {
			attempt.keywordHistory = attempt.keywordHistory[1:]
		}
		attempt.keywordHistory = append(attempt.keywordHistory, current)
	}
	c.ConsecutiveFailures = consecutiveKeywordCount(attempt.keywordHistory, current)
	c.OtherPaths, c.OtherModels = len(paths), len(models)
	return c
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
