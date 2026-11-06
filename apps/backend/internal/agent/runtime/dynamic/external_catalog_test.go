package dynamic

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr/externalcatalog"
)

func TestExternalCatalogDecisionDrivesKeywordCircuit(t *testing.T) {
	doc := "version: 1\nrules:\n  - id: test.credit\n    match: {pattern: 'credit balance exhausted'}\n" +
		"    result: {code: quota_limited, confidence: high, scope: account, backoff: 45m}\n"
	catalog, err := externalcatalog.Parse([]byte(doc), externalcatalog.FormatYAML, routingerr.ExternalCatalogVocabulary())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	routingerr.SetExternalCatalog(catalog)
	t.Cleanup(func() { routingerr.SetExternalCatalog(nil) })

	failure := routingerr.Classify(routingerr.Input{
		Phase: routingerr.PhasePromptSend, ProviderID: "example-gateway", Stderr: "Error: credit balance exhausted",
		KeywordContext: routingerr.KeywordContext{Enabled: true, Diagnostic: true, AccountID: "acct", ModelID: "m"},
	})
	candidate := Candidate{ID: "c", BindingKey: "binding", ModelID: "m", KeywordContextEnabled: true, KeywordAccountID: "acct"}
	if got := keywordCircuitKey(candidate, failure); got != keywordAccountCircuitKey("acct") {
		t.Fatalf("circuit key = %q, want account circuit", got)
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	if got := keywordCircuitUntil(candidate, failure, now); !got.Equal(now.Add(45 * time.Minute)) {
		t.Fatalf("circuit until = %s, want catalog backoff", got)
	}
	candidate.KeywordContextEnabled = false
	if got := keywordFailureForCandidate(candidate, failure); got.Keyword != nil {
		t.Fatal("disabled candidate kept the catalog keyword decision")
	}
}
