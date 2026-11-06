package routingerr

import (
	"strings"
	"sync/atomic"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr/externalcatalog"
)

const externalCatalogRulePrefix = "external_catalog."

var externalCatalog atomic.Pointer[externalcatalog.Catalog]

// externalCatalogCodes are the provider recovery codes an operator catalog
// may produce. Codes that hold an account until its owner releases it, and
// runtime codes with their own remediation, are not available.
var externalCatalogCodes = []Code{
	CodeAuthRequired, CodeMissingCredentials, CodeSubscriptionRequired,
	CodeQuotaLimited, CodeRateLimited, CodeNetworkUnavailable,
	CodeProviderUnavailable, CodeProviderOverloaded, CodeModelCapacity,
	CodeModelUnavailable, CodeProviderNotConfigured, CodeProviderResourceExhausted,
}

// SetExternalCatalog installs the operator catalog loaded at startup. A nil
// catalog leaves the built-in rules only.
func SetExternalCatalog(catalog *externalcatalog.Catalog) {
	externalCatalog.Store(catalog)
}

// ExternalCatalogVocabulary lists the classifier values an operator catalog
// may reference.
func ExternalCatalogVocabulary() externalcatalog.Vocabulary {
	vocab := externalcatalog.Vocabulary{
		Confidences: []string{string(ConfHigh), string(ConfMedium), string(ConfLow)},
		Phases: []string{
			string(PhaseAuthCheck), string(PhaseProcessStart), string(PhaseSessionInit),
			string(PhasePromptSend), string(PhaseStreaming), string(PhaseToolExecution),
			string(PhaseShutdown),
		},
	}
	for _, code := range externalCatalogCodes {
		vocab.Codes = append(vocab.Codes, string(code))
	}
	return vocab
}

// classifyExternalCatalog runs after every built-in rule and before the
// opt-in keyword/context fallback. Scope, backoff and reset hints apply only
// when that fallback is enabled for an eligible provider diagnostic.
func classifyExternalCatalog(in Input, excerpt string) *Error {
	catalog := externalCatalog.Load()
	if catalog == nil {
		return nil
	}
	context := in.KeywordContext
	eligible := context.Enabled && context.Diagnostic
	status := in.HTTPStatus
	if status == 0 {
		status = context.HTTPStatus
	}
	match, ok := catalog.Match(externalcatalog.Observation{
		ProviderID:          in.ProviderID,
		Phase:               string(in.Phase),
		HTTPStatus:          status,
		Text:                excerpt,
		HasTerm:             externalCatalogTermMatcher(in, eligible),
		ContextEligible:     eligible,
		AccountID:           context.AccountID,
		Tier:                context.Tier,
		ModelID:             context.ModelID,
		PathID:              context.PathID,
		FreeDaily:           context.FreeDaily,
		EmptyOutput:         context.EmptyOutput,
		ConsecutiveFailures: context.ConsecutiveFailures,
		OtherPaths:          context.OtherPaths,
		OtherModels:         context.OtherModels,
	})
	if !ok {
		return nil
	}
	result := &Error{Code: Code(match.Code), Confidence: Confidence(match.Confidence), Phase: in.Phase,
		ClassifierRule: externalCatalogRulePrefix + match.RuleID, ExitCode: in.ExitCode,
		ResetHint: in.ResetHint, RawExcerpt: excerpt}
	if eligible {
		result.Keyword = externalKeywordDecision(match, in)
		applyKeywordReset(in, result)
	}
	return result
}

// externalCatalogTermMatcher reuses the keyword classifier's diagnostic
// envelope and term normalization, so quoted or negated text has no terms.
func externalCatalogTermMatcher(in Input, eligible bool) func(string) bool {
	if !eligible {
		return nil
	}
	diagnostic := strings.TrimSpace(in.Stderr + "\n" + in.Stdout)
	if len(diagnostic) > 1024 {
		return nil
	}
	text, envelope := keywordDiagnosticText(SanitizeFullUnbounded(diagnostic))
	if !envelope {
		return nil
	}
	words := strings.Fields(normalizeKeywords(text))
	return func(term string) bool {
		return keywordTermMatches(words, strings.Fields(normalizeKeywords(term)))
	}
}

// externalKeywordDecision applies the catalog's effective scope: Match has
// already narrowed a requested account or model scope to path_model when the
// observation lacks that identity.
func externalKeywordDecision(match externalcatalog.Decision, in Input) *KeywordDecision {
	decision := keywordDecision(externalCatalogGroup(Code(match.Code)), in)
	decision.Scope = match.Scope
	if match.Backoff > 0 {
		decision.Backoff = match.Backoff
	}
	if match.Reset == externalcatalog.ResetUntilModelChange {
		decision.UntilModelChange = true
	}
	return decision
}

func externalCatalogGroup(code Code) keywordGroup {
	switch code {
	case CodeQuotaLimited, CodeRateLimited:
		return groupLimit
	case CodeModelUnavailable:
		return groupGone
	case CodeAuthRequired, CodeMissingCredentials, CodeSubscriptionRequired, CodeProviderNotConfigured:
		return groupAccount
	default:
		return groupCapacity
	}
}
