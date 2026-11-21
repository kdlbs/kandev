package externalcatalog

import "time"

// Observation is the evidence one classification offers to the catalog. The
// caller fills it from its own input; zero values mean "not observed".
type Observation struct {
	ProviderID string
	Phase      string
	HTTPStatus int
	// Text is the sanitized diagnostic the classifier's own rules read.
	Text string
	// HasTerm reports whether a normalized term occurs in the diagnostic.
	// The caller supplies the classifier's term normalization so catalog
	// terms follow the same matching as built-in keyword evidence.
	HasTerm func(term string) bool
	// ContextEligible is true only when the opt-in context classifier is
	// enabled and the text is a provider diagnostic, not agent prose or a
	// quote. Rules that use terms or context predicates require it.
	ContextEligible     bool
	AccountID           string
	Tier                string
	ModelID             string
	PathID              string
	FreeDaily           bool
	EmptyOutput         bool
	ConsecutiveFailures int
	OtherPaths          int
	OtherModels         int
}

// Decision is the result of the first matching rule.
type Decision struct {
	RuleID     string
	Code       string
	Confidence string
	// Scope is the effective scope after identity checks.
	Scope string
	// Backoff is zero when the rule inherits the classifier default.
	Backoff time.Duration
	Reset   string
}

// Match returns the decision of the first rule that matches obs.
func (c *Catalog) Match(obs Observation) (Decision, bool) {
	if c == nil {
		return Decision{}, false
	}
	for i := range c.rules {
		rule := &c.rules[i]
		if rule.matches(obs) {
			return rule.decision(obs), true
		}
	}
	return Decision{}, false
}

func (r *compiledRule) matches(obs Observation) bool {
	if r.gated && !obs.ContextEligible {
		return false
	}
	if len(r.providers) > 0 && !contains(r.providers, obs.ProviderID) {
		return false
	}
	if len(r.phases) > 0 && !contains(r.phases, obs.Phase) {
		return false
	}
	if len(r.statuses) > 0 && !containsInt(r.statuses, obs.HTTPStatus) {
		return false
	}
	if r.pattern != nil && !r.pattern.MatchString(obs.Text) {
		return false
	}
	if !r.termsMatch(obs) {
		return false
	}
	return r.context == nil || contextMatches(*r.context, obs)
}

func (r *compiledRule) termsMatch(obs Observation) bool {
	if len(r.terms) == 0 {
		return true
	}
	if obs.HasTerm == nil {
		return false
	}
	count := 0
	for _, term := range r.terms {
		if obs.HasTerm(term) {
			count++
		}
	}
	return count >= r.minTerms
}

// contextMatches requires every present predicate. A list predicate never
// matches a missing identity.
func contextMatches(c contextDTO, obs Observation) bool {
	if !listMatches(c.Tiers, obs.Tier) || !listMatches(c.Accounts, obs.AccountID) ||
		!listMatches(c.Models, obs.ModelID) || !listMatches(c.Paths, obs.PathID) {
		return false
	}
	if c.FreeDaily != nil && *c.FreeDaily != obs.FreeDaily {
		return false
	}
	if c.EmptyOutput != nil && *c.EmptyOutput != obs.EmptyOutput {
		return false
	}
	return obs.ConsecutiveFailures >= c.MinConsecutiveFailures &&
		obs.OtherPaths >= c.MinOtherPaths && obs.OtherModels >= c.MinOtherModels
}

func listMatches(values []string, observed string) bool {
	return len(values) == 0 || (observed != "" && contains(values, observed))
}

func (r *compiledRule) decision(obs Observation) Decision {
	return Decision{
		RuleID:     r.id,
		Code:       r.code,
		Confidence: r.confidence,
		Scope:      effectiveScope(r.scope, obs),
		Backoff:    r.backoff,
		Reset:      r.reset,
	}
}

// effectiveScope widens a suspension only for an eligible provider diagnostic
// that carries the identity being suspended. Otherwise the failure stays
// scoped to the failing path and model.
func effectiveScope(requested string, obs Observation) string {
	if !obs.ContextEligible {
		return ScopePathModel
	}
	switch {
	case requested == ScopeAccount && obs.AccountID != "":
		return ScopeAccount
	case requested == ScopeModel && obs.ModelID != "":
		return ScopeModel
	default:
		return ScopePathModel
	}
}

func containsInt(values []int, value int) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
