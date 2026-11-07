package externalcatalog_test

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr/externalcatalog"
)

const matchCatalog = `
version: 1
rules:
  - id: first
    match: {pattern: 'credit balance', providers: [gw]}
    result: {code: quota_limited, confidence: high}
  - id: second
    match: {pattern: 'credit'}
    result: {code: rate_limited, confidence: low}
  - id: account-limit
    match:
      terms: [usage, limit]
      minTerms: 2
      context: {tiers: [subscription]}
    result: {code: quota_limited, confidence: high, scope: account, backoff: 30m}
  - id: model-capacity
    match: {pattern: 'overloaded'}
    result: {code: model_capacity, confidence: medium, scope: model}
  - id: account-pinned
    match:
      pattern: 'suspended'
      context: {accounts: [acct-1]}
    result: {code: provider_unavailable, confidence: low}
`

func loadMatchCatalog(t *testing.T) *externalcatalog.Catalog {
	t.Helper()
	catalog, err := externalcatalog.Parse([]byte(matchCatalog), externalcatalog.FormatYAML, testVocabulary())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return catalog
}

func TestMatch_FirstRuleWins(t *testing.T) {
	catalog := loadMatchCatalog(t)
	got, ok := catalog.Match(externalcatalog.Observation{ProviderID: "gw", Text: "credit balance"})
	if !ok || got.RuleID != "first" {
		t.Fatalf("Match = %+v, %v; want first", got, ok)
	}
	got, ok = catalog.Match(externalcatalog.Observation{ProviderID: "other", Text: "credit balance"})
	if !ok || got.RuleID != "second" || got.Scope != externalcatalog.ScopePathModel || got.Reset != externalcatalog.ResetInherit {
		t.Fatalf("Match = %+v, %v; want second with defaults", got, ok)
	}
}

func TestMatch_ContextRulesRequireEligibleDiagnostic(t *testing.T) {
	catalog := loadMatchCatalog(t)
	obs := externalcatalog.Observation{Text: "usage limit hit", Tier: "subscription", AccountID: "acct-1"}
	if got, ok := catalog.Match(obs); ok {
		t.Fatalf("ineligible observation matched %+v", got)
	}
	got, ok := catalog.Match(eligible(obs))
	if !ok || got.RuleID != "account-limit" || got.Scope != externalcatalog.ScopeAccount || got.Backoff != 30*time.Minute {
		t.Fatalf("Match = %+v, %v; want account-limit account scope", got, ok)
	}
}

func TestMatch_ScopeNeverSuppliesMissingIdentity(t *testing.T) {
	catalog := loadMatchCatalog(t)
	got, ok := catalog.Match(eligible(externalcatalog.Observation{Text: "usage limit hit", Tier: "subscription"}))
	if !ok || got.Scope != externalcatalog.ScopePathModel {
		t.Fatalf("missing account: Match = %+v, %v; want path_model", got, ok)
	}
	got, ok = catalog.Match(externalcatalog.Observation{Text: "overloaded", ModelID: "m"})
	if !ok || got.Scope != externalcatalog.ScopePathModel {
		t.Fatalf("ineligible model widening: Match = %+v, %v; want path_model", got, ok)
	}
	got, ok = catalog.Match(eligible(externalcatalog.Observation{Text: "overloaded", ModelID: "m"}))
	if !ok || got.Scope != externalcatalog.ScopeModel {
		t.Fatalf("eligible model widening: Match = %+v, %v; want model", got, ok)
	}
	got, ok = catalog.Match(eligible(externalcatalog.Observation{Text: "overloaded"}))
	if !ok || got.Scope != externalcatalog.ScopePathModel {
		t.Fatalf("missing model: Match = %+v, %v; want path_model", got, ok)
	}
}

func TestMatch_ListPredicateRejectsMissingIdentity(t *testing.T) {
	catalog := loadMatchCatalog(t)
	if got, ok := catalog.Match(eligible(externalcatalog.Observation{Text: "suspended"})); ok {
		t.Fatalf("missing account matched %+v", got)
	}
	if got, ok := catalog.Match(eligible(externalcatalog.Observation{Text: "suspended", AccountID: "acct-2"})); ok {
		t.Fatalf("other account matched %+v", got)
	}
	got, ok := catalog.Match(eligible(externalcatalog.Observation{Text: "suspended", AccountID: "acct-1"}))
	if !ok || got.RuleID != "account-pinned" {
		t.Fatalf("Match = %+v, %v; want account-pinned", got, ok)
	}
}
