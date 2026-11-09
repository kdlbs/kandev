package dynamic

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func (e *Engine) acquireCandidateProbes(candidate Candidate, sessionID string, generation int64, now time.Time) bool {
	keys := append([]string{candidate.BindingKey}, keywordCircuitKeys(candidate)...)
	if candidate.KeywordContextEnabled && candidate.KeywordAccountID != "" {
		keys = append(keys, keywordAccountCircuitKey(candidate.KeywordAccountID))
	}
	var leases []ProbeLease
	for _, key := range keys {
		if key == "" || !e.circuits.IsOpen(key, now) {
			continue
		}
		lease, ok := e.circuits.AcquireProbe(key, probeLeaseDuration)
		if !ok {
			for _, acquired := range leases {
				e.circuits.ReleaseProbe(acquired, false, 0)
			}
			return false
		}
		leases = append(leases, lease)
	}
	if len(leases) > 0 {
		e.probes[probeKey(sessionID, generation, candidate.ID)] = leases
	}
	return true
}

func keywordCircuitKeys(candidate Candidate) []string {
	if !candidate.KeywordContextEnabled {
		return nil
	}
	if candidate.ModelID == "" {
		key := ResourceKey(ScopeProfile, candidate.ID)
		if key == candidate.BindingKey {
			return nil
		}
		return []string{key}
	}
	model, _ := json.Marshal([]string{candidate.ModelID})
	pathModel, _ := json.Marshal([]string{candidate.BindingKey, candidate.ModelID})
	return []string{"keyword_model:" + string(model), "keyword_path_model:" + string(pathModel)}
}

func keywordCircuitKey(candidate Candidate, failure *routingerr.Error) string {
	if failure.Keyword == nil || !candidate.KeywordContextEnabled {
		return candidate.BindingKey
	}
	if failure.Keyword.Scope == "account" && candidate.KeywordAccountID != "" {
		return keywordAccountCircuitKey(candidate.KeywordAccountID)
	}
	keys := keywordCircuitKeys(candidate)
	if len(keys) == 1 {
		return keys[0]
	}
	if len(keys) != 2 {
		return candidate.BindingKey
	}
	switch failure.Keyword.Scope {
	case "model":
		return keys[0]
	default:
		return keys[1]
	}
}

func keywordAccountCircuitKey(accountID string) string {
	return "keyword_account:" + accountID
}

func keywordFailureForCandidate(candidate Candidate, failure *routingerr.Error) *routingerr.Error {
	if candidate.KeywordContextEnabled || failure.Keyword == nil {
		return failure
	}
	legacyFailure := *failure
	legacyFailure.Keyword = nil
	return &legacyFailure
}

func keywordCircuitUntil(candidate Candidate, failure *routingerr.Error, now time.Time) time.Time {
	backoff := circuitBackoff
	if failure.Keyword != nil && failure.Keyword.Backoff > 0 {
		backoff = failure.Keyword.Backoff
	}
	until := now.Add(backoff)
	if failure.ResetHint != nil && (failure.ResetHint.After(until) || (failure.Keyword != nil && failure.ResetHint.After(now))) {
		until = *failure.ResetHint
	}
	if failure.Keyword != nil && failure.Keyword.UntilModelChange && candidate.ModelID != "" {
		return time.Time{}
	}
	return until
}

func keywordDefaultAction(candidate Candidate, failure *routingerr.Error, current Action) Action {
	if !candidate.KeywordContextEnabled || failure.Keyword == nil || candidate.Policies.Version != 0 || len(candidate.Rules) != 0 || !failure.FallbackAllowed {
		return current
	}
	switch failure.Keyword.Action {
	case "retry_same":
		return ActionRetrySame
	case "try_next":
		return ActionTryNext
	default:
		return ActionStop
	}
}
