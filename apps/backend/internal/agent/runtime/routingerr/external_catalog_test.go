package routingerr

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr/externalcatalog"
)

const externalCatalogTestDoc = `
version: 1
rules:
  - id: test.fixed_overlap
    match: {pattern: 'ECONNRESET'}
    result: {code: quota_limited, confidence: high}
  - id: test.credit_notice
    match: {pattern: '(?i)credit\s+balance\s+exhausted'}
    result: {code: quota_limited, confidence: high, scope: account, backoff: 45m}
  - id: test.capacity_terms
    match:
      terms: [shard, saturated]
      minTerms: 2
    result: {code: model_capacity, confidence: medium, scope: model}
  - id: test.retired_model
    match: {pattern: '(?i)has\s+been\s+retired', httpStatus: [404]}
    result: {code: model_unavailable, confidence: high, reset: until_model_change}
`

func installExternalCatalogForTest(t *testing.T, catalog *externalcatalog.Catalog) {
	t.Helper()
	SetExternalCatalog(catalog)
	t.Cleanup(func() { SetExternalCatalog(nil) })
}

func parseExternalCatalogForTest(t *testing.T) *externalcatalog.Catalog {
	t.Helper()
	catalog, err := externalcatalog.Parse([]byte(externalCatalogTestDoc), externalcatalog.FormatYAML, ExternalCatalogVocabulary())
	if err != nil {
		t.Fatalf("parse test catalog: %v", err)
	}
	return catalog
}

func TestExternalCatalog_UnsetKeepsBuiltInResult(t *testing.T) {
	SetExternalCatalog(nil)
	in := Input{Phase: PhaseStreaming, ProviderID: "example-gateway", Stderr: "Error: credit balance exhausted"}
	got := Classify(in)
	if got.ClassifierRule == externalCatalogRulePrefix+"test.credit_notice" || got.Code == CodeQuotaLimited {
		t.Fatalf("unset catalog classified %+v", got)
	}
}

func TestExternalCatalog_ExamplesClassifyThroughClassify(t *testing.T) {
	for _, name := range []string{"catalog.example.json", "catalog.example.yaml"} {
		t.Run(name, func(t *testing.T) {
			catalog, err := externalcatalog.Load(filepath.Join("externalcatalog", "examples", name), ExternalCatalogVocabulary())
			if err != nil {
				t.Fatalf("load %s: %v", name, err)
			}
			installExternalCatalogForTest(t, catalog)
			got := Classify(Input{Phase: PhaseStreaming, ProviderID: "example-gateway",
				Stderr: "Error: monthly credit balance exhausted"})
			if got.Code != CodeQuotaLimited || got.ClassifierRule != externalCatalogRulePrefix+"example.gateway.quota_notice" {
				t.Fatalf("Classify = %s/%s", got.Code, got.ClassifierRule)
			}
			if got.Class != ClassHard || !got.FallbackAllowed || got.Keyword != nil {
				t.Fatalf("invariants or disabled keyword decision wrong: %+v", got)
			}
		})
	}
}

func TestExternalCatalog_BuiltInRulesWin(t *testing.T) {
	installExternalCatalogForTest(t, parseExternalCatalogForTest(t))
	got := Classify(Input{Phase: PhaseStreaming, ProviderID: "example-gateway", Stderr: "Error: read ECONNRESET"})
	if got.Code != CodeNetworkUnavailable {
		t.Fatalf("Classify = %s/%s, want built-in network_unavailable", got.Code, got.ClassifierRule)
	}
}

func TestExternalCatalog_DisabledContextIgnoresTermsAndHints(t *testing.T) {
	installExternalCatalogForTest(t, parseExternalCatalogForTest(t))
	got := Classify(Input{Phase: PhaseStreaming, ProviderID: "example-gateway", Stderr: "Error: shard saturated"})
	if got.ClassifierRule == externalCatalogRulePrefix+"test.capacity_terms" {
		t.Fatalf("terms rule matched with the context classifier disabled: %+v", got)
	}
	got = Classify(Input{Phase: PhaseStreaming, ProviderID: "example-gateway", Stderr: "Error: credit balance exhausted",
		KeywordContext: KeywordContext{AccountID: "acct", Tier: "subscription"}})
	if got.Code != CodeQuotaLimited || got.Keyword != nil {
		t.Fatalf("disabled context: Classify = %+v, want code without keyword decision", got)
	}
}

func TestExternalCatalog_EligibleContextAppliesScopeBackoffAndReset(t *testing.T) {
	installExternalCatalogForTest(t, parseExternalCatalogForTest(t))
	eligible := KeywordContext{Enabled: true, Diagnostic: true, AccountID: "acct", ModelID: "m"}
	got := Classify(Input{Phase: PhasePromptSend, ProviderID: "example-gateway", Stderr: "Error: credit balance exhausted",
		KeywordContext: eligible})
	if got.Keyword == nil || got.Keyword.Scope != externalcatalog.ScopeAccount || got.Keyword.Backoff != 45*time.Minute {
		t.Fatalf("credit notice keyword decision = %+v", got.Keyword)
	}
	got = Classify(Input{Phase: PhasePromptSend, ProviderID: "example-gateway", Stderr: "Error: shard saturated",
		KeywordContext: eligible})
	if got.Code != CodeModelCapacity || got.Keyword == nil || got.Keyword.Scope != externalcatalog.ScopeModel {
		t.Fatalf("terms rule = %s/%s keyword %+v", got.Code, got.ClassifierRule, got.Keyword)
	}
	got = Classify(Input{Phase: PhasePromptSend, ProviderID: "example-gateway", HTTPStatus: 404,
		Stderr: "Error: model x has been retired", KeywordContext: eligible})
	if got.Code != CodeModelUnavailable || got.Keyword == nil || !got.Keyword.UntilModelChange {
		t.Fatalf("retired model = %s keyword %+v", got.Code, got.Keyword)
	}
}

func TestExternalCatalog_EligibleScopeNeedsIdentity(t *testing.T) {
	installExternalCatalogForTest(t, parseExternalCatalogForTest(t))
	eligible := KeywordContext{Enabled: true, Diagnostic: true}
	got := Classify(Input{Phase: PhasePromptSend, ProviderID: "example-gateway", Stderr: "Error: credit balance exhausted",
		KeywordContext: eligible})
	if got.Keyword == nil || got.Keyword.Scope != externalcatalog.ScopePathModel {
		t.Fatalf("account scope without account id = %+v", got.Keyword)
	}
	got = Classify(Input{Phase: PhasePromptSend, ProviderID: "example-gateway", Stderr: "Error: shard saturated",
		KeywordContext: eligible})
	if got.Keyword == nil || got.Keyword.Scope != externalcatalog.ScopePathModel {
		t.Fatalf("model scope without model id = %+v", got.Keyword)
	}
}

func TestExternalCatalog_QuotedTermsDoNotMatch(t *testing.T) {
	installExternalCatalogForTest(t, parseExternalCatalogForTest(t))
	got := Classify(Input{Phase: PhasePromptSend, ProviderID: "example-gateway",
		Stderr:         `Error: the agent wrote "shard saturated" as an example`,
		KeywordContext: KeywordContext{Enabled: true, Diagnostic: true}})
	if got.ClassifierRule == externalCatalogRulePrefix+"test.capacity_terms" {
		t.Fatalf("quoted terms matched: %+v", got)
	}
}

func TestExternalCatalogVocabulary_ExcludesOwnerReleaseHolds(t *testing.T) {
	for _, code := range ExternalCatalogVocabulary().Codes {
		if HoldsUntilRelease(Code(code)) || ClassForCode(Code(code)) == ClassUnclassified {
			t.Fatalf("vocabulary includes %q", code)
		}
	}
}
