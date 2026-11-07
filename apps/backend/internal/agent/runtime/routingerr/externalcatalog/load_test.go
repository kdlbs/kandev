package externalcatalog_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr/externalcatalog"
)

func testVocabulary() externalcatalog.Vocabulary {
	codes := []routingerr.Code{
		routingerr.CodeQuotaLimited, routingerr.CodeRateLimited, routingerr.CodeModelCapacity,
		routingerr.CodeModelUnavailable, routingerr.CodeProviderUnavailable,
	}
	vocab := externalcatalog.Vocabulary{
		Confidences: []string{string(routingerr.ConfHigh), string(routingerr.ConfMedium), string(routingerr.ConfLow)},
		Phases: []string{
			string(routingerr.PhaseProcessStart), string(routingerr.PhaseSessionInit),
			string(routingerr.PhasePromptSend), string(routingerr.PhaseStreaming),
		},
	}
	for _, code := range codes {
		vocab.Codes = append(vocab.Codes, string(code))
	}
	return vocab
}

func TestLoad_EmptyPathKeepsBuiltInsOnly(t *testing.T) {
	catalog, err := externalcatalog.Load("", testVocabulary())
	if err != nil || catalog != nil {
		t.Fatalf("Load(\"\") = %v, %v; want nil catalog and nil error", catalog, err)
	}
	if catalog.Len() != 0 {
		t.Fatalf("nil catalog Len() = %d", catalog.Len())
	}
	if _, ok := catalog.Match(externalcatalog.Observation{Text: "anything"}); ok {
		t.Fatal("nil catalog matched")
	}
}

func TestLoad_ExamplesAreEquivalent(t *testing.T) {
	jsonCatalog, err := externalcatalog.Load(filepath.Join("examples", "catalog.example.json"), testVocabulary())
	if err != nil {
		t.Fatalf("load JSON example: %v", err)
	}
	yamlCatalog, err := externalcatalog.Load(filepath.Join("examples", "catalog.example.yaml"), testVocabulary())
	if err != nil {
		t.Fatalf("load YAML example: %v", err)
	}
	if jsonCatalog.Len() != 4 || yamlCatalog.Len() != 4 {
		t.Fatalf("rule counts JSON=%d YAML=%d, want 4", jsonCatalog.Len(), yamlCatalog.Len())
	}
	observations := []externalcatalog.Observation{
		{ProviderID: "example-gateway", Text: "Error: monthly credit balance exhausted"},
		eligible(externalcatalog.Observation{Phase: "streaming", HTTPStatus: 503, Text: "at capacity, try again later",
			ModelID: "model-a", OtherPaths: 1}),
		eligible(externalcatalog.Observation{Text: "daily limit reached", FreeDaily: true}),
		{HTTPStatus: 404, Text: "model example-1 has been retired"},
		{ProviderID: "other", Text: "Error: monthly credit balance exhausted"},
	}
	for i, obs := range observations {
		fromJSON, okJSON := jsonCatalog.Match(obs)
		fromYAML, okYAML := yamlCatalog.Match(obs)
		if okJSON != okYAML || fromJSON != fromYAML {
			t.Fatalf("observation %d: JSON=%+v,%v YAML=%+v,%v", i, fromJSON, okJSON, fromYAML, okYAML)
		}
		if wantMatch := i < 4; okJSON != wantMatch {
			t.Fatalf("observation %d matched=%v, want %v", i, okJSON, wantMatch)
		}
	}
}

func TestLoad_FileErrors(t *testing.T) {
	dir := t.TempDir()
	unsupported := filepath.Join(dir, "catalog.txt")
	if err := os.WriteFile(unsupported, []byte("version: 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := externalcatalog.Load(unsupported, testVocabulary())
	assertReason(t, catalog, err, externalcatalog.ReasonUnsupportedExtension)
	catalog, err = externalcatalog.Load(filepath.Join(dir, "missing.yaml"), testVocabulary())
	assertReason(t, catalog, err, externalcatalog.ReasonUnreadable)
}

func TestParse_RejectsInvalidDocuments(t *testing.T) {
	const validResult = `"result":{"code":"quota_limited","confidence":"high"}`
	cases := []struct {
		name   string
		format externalcatalog.Format
		doc    string
		reason string
	}{
		{"malformed json", externalcatalog.FormatJSON, `{"version":1,"rules":[`, externalcatalog.ReasonMalformed},
		{"trailing json", externalcatalog.FormatJSON, `{"version":1,"rules":[]} {}`, externalcatalog.ReasonMalformed},
		{"unknown json field", externalcatalog.FormatJSON, `{"version":1,"rules":[],"extra":true}`, externalcatalog.ReasonUnknownField},
		{"unknown yaml field", externalcatalog.FormatYAML, "version: 1\nrules: []\nextra: true\n", externalcatalog.ReasonUnknownField},
		{"second yaml document", externalcatalog.FormatYAML, "version: 1\nrules: []\n---\nversion: 1\n", externalcatalog.ReasonMalformed},
		{"missing version", externalcatalog.FormatJSON, `{"rules":[]}`, externalcatalog.ReasonMissingField},
		{"future version", externalcatalog.FormatJSON, `{"version":2,"rules":[]}`, externalcatalog.ReasonUnsupportedVersion},
		{"missing id", externalcatalog.FormatJSON, `{"version":1,"rules":[{"match":{"pattern":"x"},` + validResult + `}]}`, externalcatalog.ReasonMissingField},
		{"bad id", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a b","match":{"pattern":"x"},` + validResult + `}]}`, externalcatalog.ReasonInvalidValue},
		{"duplicate id", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},` + validResult + `},{"id":"a","match":{"pattern":"y"},` + validResult + `}]}`, externalcatalog.ReasonDuplicateID},
		{"no evidence", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"httpStatus":[429]},` + validResult + `}]}`, externalcatalog.ReasonMissingField},
		{"invalid pattern", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"("},` + validResult + `}]}`, externalcatalog.ReasonInvalidPattern},
		{"single bare term", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"terms":["quota"],"minTerms":1},` + validResult + `}]}`, externalcatalog.ReasonInvalidValue},
		{"terms without minimum", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"terms":["quota","limit"]},` + validResult + `}]}`, externalcatalog.ReasonMissingField},
		{"unknown phase", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x","phases":["mid_turn"]},` + validResult + `}]}`, externalcatalog.ReasonInvalidValue},
		{"bad status", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x","httpStatus":[42]},` + validResult + `}]}`, externalcatalog.ReasonInvalidValue},
		{"unknown code", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},"result":{"code":"account_blocked","confidence":"high"}}]}`, externalcatalog.ReasonInvalidValue},
		{"missing confidence", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},"result":{"code":"quota_limited"}}]}`, externalcatalog.ReasonMissingField},
		{"bad scope", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},"result":{"code":"quota_limited","confidence":"high","scope":"global"}}]}`, externalcatalog.ReasonInvalidValue},
		{"bad backoff", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},"result":{"code":"quota_limited","confidence":"high","backoff":"-5m"}}]}`, externalcatalog.ReasonInvalidDuration},
		{"free daily without evidence", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},"result":{"code":"quota_limited","confidence":"high","reset":"free_daily"}}]}`, externalcatalog.ReasonInvalidValue},
		{"model change without 404", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x"},"result":{"code":"model_unavailable","confidence":"high","reset":"until_model_change"}}]}`, externalcatalog.ReasonInvalidValue},
		{"empty account predicate", externalcatalog.FormatJSON, `{"version":1,"rules":[{"id":"a","match":{"pattern":"x","context":{"accounts":[""]}},` + validResult + `}]}`, externalcatalog.ReasonInvalidValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := externalcatalog.Parse([]byte(tc.doc), tc.format, testVocabulary())
			assertReason(t, catalog, err, tc.reason)
		})
	}
}

func TestParse_EmptyRulesIsValid(t *testing.T) {
	catalog, err := externalcatalog.Parse([]byte("version: 1\nrules: []\n"), externalcatalog.FormatYAML, testVocabulary())
	if err != nil || catalog == nil || catalog.Len() != 0 {
		t.Fatalf("Parse(empty rules) = %v, %v", catalog, err)
	}
}

func TestLoadError_DoesNotQuoteFileContent(t *testing.T) {
	const secret = "sk-test-secret-value"
	doc := "version: 1\nrules:\n  - id: a\n    match: {pattern: '" + secret + "('}\n    result: {code: quota_limited, confidence: high}\n"
	_, err := externalcatalog.Parse([]byte(doc), externalcatalog.FormatYAML, testVocabulary())
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %v; want a rejection without file content", err)
	}
	_, err = externalcatalog.Parse([]byte("version: [\""+secret), externalcatalog.FormatYAML, testVocabulary())
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("decode error = %v; want a rejection without file content", err)
	}
}

func assertReason(t *testing.T, catalog *externalcatalog.Catalog, err error, reason string) {
	t.Helper()
	if catalog != nil {
		t.Fatalf("rejected catalog was returned with %d rules", catalog.Len())
	}
	var loadErr *externalcatalog.LoadError
	if !errors.As(err, &loadErr) {
		t.Fatalf("error = %v, want *LoadError", err)
	}
	if loadErr.Reason != reason {
		t.Fatalf("reason = %q (%v), want %q", loadErr.Reason, loadErr, reason)
	}
}

func eligible(obs externalcatalog.Observation) externalcatalog.Observation {
	obs.ContextEligible = true
	text := strings.ToLower(obs.Text)
	obs.HasTerm = func(term string) bool { return strings.Contains(text, term) }
	return obs
}
