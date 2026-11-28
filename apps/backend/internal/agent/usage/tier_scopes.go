package usage

import (
	"fmt"
	"strings"
	"time"
)

// Request scope kinds. A model scope covers one provider model; a group scope
// is one allowance shared by several models of the same account.
const (
	ScopeKindModel = "model"
	ScopeKindGroup = "group"
)

// Provenance of a default value.
const (
	ProvenancePublished   = "published"
	ProvenanceMeasured    = "measured"
	ProvenanceApproximate = "approximate"
	ProvenanceProvider    = "provider"
)

// RequestScope is a daily request allowance for one model or a shared model
// group inside a tier. Models are provider model IDs without the provider
// prefix. Nil RequestsPerDay inherits; an explicit zero disables the scope.
type RequestScope struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind,omitempty"`
	Models         []string   `json:"models,omitempty"`
	RequestsPerDay *float64   `json:"requests_per_day,omitempty"`
	DailyReset     string     `json:"daily_reset,omitempty"`
	Provenance     string     `json:"provenance,omitempty"`
	Learned        bool       `json:"learned,omitempty"`
	Provisional    bool       `json:"provisional,omitempty"`
	LearnedAt      *time.Time `json:"learned_at,omitempty"`
}

// ModelScopeID is the stable ID of a single-model scope.
func ModelScopeID(model string) string { return ScopeKindModel + ":" + model }

// ScopeModelID strips the provider prefix from a profile model ID.
func ScopeModelID(modelID string) string {
	if _, model, found := strings.Cut(strings.TrimSpace(modelID), "/"); found {
		return model
	}
	return strings.TrimSpace(modelID)
}

// Covers reports whether the scope applies to a provider model ID.
func (s RequestScope) Covers(model string) bool {
	return s.coversModelID(ScopeModelID(model))
}

// coversModelID reports whether the scope lists an already-normalized model ID.
func (s RequestScope) coversModelID(model string) bool {
	for _, m := range s.Models {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}

// ScopeUsageKey is the LimitUsage.Resets key of a scope.
func ScopeUsageKey(id string) string { return "scope:" + id }

// EvaluateScopeLimits checks every scope covering the model, after the tier
// checks, and reports the first exhausted one with its daily reset.
func EvaluateScopeLimits(scopes []RequestScope, model string, used LimitUsage, now time.Time) LimitAdmission {
	for _, scope := range scopes {
		if !scope.Covers(model) || !positive(scope.RequestsPerDay) {
			continue
		}
		margin := 1.0
		if scope.Learned {
			margin = 0.95
		}
		if !exceeds(used.Scopes[scope.ID], scope.RequestsPerDay, margin) {
			continue
		}
		_, reset, err := DailyLimitWindow(now, scope.DailyReset)
		if err != nil || !reset.After(now) {
			reset = now.Add(time.Minute)
		}
		return LimitAdmission{Reason: "limit_daily", RetryAt: reset}
	}
	return LimitAdmission{}
}

// CoveringScope returns the scope that learns for a model: its group first,
// then its own model scope.
func CoveringScope(scopes []RequestScope, model string) (RequestScope, bool) {
	var own RequestScope
	found := false
	for _, scope := range scopes {
		if !scope.Covers(model) {
			continue
		}
		if scope.Kind == ScopeKindGroup {
			return scope, true
		}
		own, found = scope, true
	}
	return own, found
}

// LearnScope replaces the learned value of one scope with the requests counted
// in it. Learned values move down as well as up because a provider can lower
// an allowance; owner values live in a separate column and are untouched.
func LearnScope(learned []RequestScope, scope RequestScope, counted float64, at time.Time) []RequestScope {
	if counted <= 0 {
		return learned
	}
	scope.RequestsPerDay = limitNumber(counted)
	scope.Learned = true
	scope.Provenance = ""
	stamp := at.UTC()
	scope.LearnedAt = &stamp
	if scope.DailyReset == "" {
		scope.DailyReset = defaultDailyReset
	}
	scope.Models = append([]string(nil), scope.Models...)
	result := make([]RequestScope, 0, len(learned)+1)
	for _, existing := range learned {
		if existing.ID != scope.ID {
			result = append(result, existing)
		}
	}
	return append(result, scope)
}

func mergeRequestScopes(base, next []RequestScope, learned bool) []RequestScope {
	if len(next) == 0 {
		return base
	}
	result := append([]RequestScope(nil), base...)
	for _, scope := range next {
		index := -1
		for i := range result {
			if result[i].ID == scope.ID {
				index = i
				break
			}
		}
		if index < 0 {
			scope.Learned = scope.Learned || learned
			result = append(result, scope)
			continue
		}
		result[index] = mergeRequestScope(result[index], scope, learned)
	}
	return result
}

func mergeRequestScope(base, next RequestScope, learned bool) RequestScope {
	if next.Kind != "" && len(next.Models) > 0 {
		base.Kind, base.Models = next.Kind, next.Models
	}
	if next.DailyReset != "" {
		base.DailyReset = next.DailyReset
	}
	if next.RequestsPerDay != nil {
		base.RequestsPerDay = next.RequestsPerDay
		base.Learned = next.Learned || learned
		base.Provisional = next.Provisional
		base.LearnedAt = next.LearnedAt
	}
	return base
}

func validateRequestScope(scope RequestScope) error {
	if strings.TrimSpace(scope.ID) == "" {
		return fmt.Errorf("scope requires an id")
	}
	if !validLimitNumber(scope.RequestsPerDay) {
		return fmt.Errorf("limits must be finite non-negative numbers")
	}
	if scope.Kind != "" && !oneOf(scope.Kind, ScopeKindModel, ScopeKindGroup) {
		return fmt.Errorf("invalid scope kind")
	}
	if scope.Kind == ScopeKindModel && len(scope.Models) != 1 {
		return fmt.Errorf("model scope requires one model")
	}
	if scope.Kind == ScopeKindGroup && len(scope.Models) < 2 {
		return fmt.Errorf("group scope requires at least two models")
	}
	if scope.DailyReset != "" {
		if _, _, err := DailyLimitWindow(time.Now(), scope.DailyReset); err != nil {
			return err
		}
	}
	return nil
}

// OwnerScopes clears learned markers from owner-entered scopes.
func OwnerScopes(scopes []RequestScope) []RequestScope {
	for i := range scopes {
		scopes[i].Learned, scopes[i].Provisional, scopes[i].LearnedAt, scopes[i].Provenance = false, false, nil, ""
	}
	return scopes
}
