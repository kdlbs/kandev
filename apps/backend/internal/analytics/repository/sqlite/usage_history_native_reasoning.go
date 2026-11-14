package sqlite

import "github.com/kandev/kandev/internal/analytics/models"

type nativePublicReasoning struct {
	Tokens            *int64
	InsideOutput      bool
	RelationshipKnown bool
}

// nativeUsageReasoningForPublic maps explicit retained counters, not provider
// names. Both positive counters have an unsupported relationship: summing or
// describing their whole amount as an output subset would invent semantics.
func nativeUsageReasoningForPublic(thought, outputSubset *int64) nativePublicReasoning {
	switch {
	case outputSubset == nil:
		return nativePublicReasoning{Tokens: cloneInt64(thought), RelationshipKnown: true}
	case thought == nil || *thought == 0:
		return nativePublicReasoning{Tokens: cloneInt64(outputSubset), InsideOutput: true, RelationshipKnown: true}
	case *outputSubset == 0:
		return nativePublicReasoning{Tokens: cloneInt64(thought), RelationshipKnown: true}
	default:
		return nativePublicReasoning{}
	}
}

// normalizeNativeCanonicalAccounting returns the version the caller must retain
// alongside the canonical fact. Passing that version again is essential: a
// derived subset amount must never be reinterpreted as a second raw thought
// counter. Accepted source rows, native digests, and authoritative totals stay
// unchanged; repair mutations cause canonical facts to be rebuilt from raw rows.
func normalizeNativeCanonicalAccounting(fact *models.UsageCanonicalFact, version int) int {
	suppressNativeCanonicalTurns(fact)
	if version >= nativeUsageAccountingRepairVersion || !nativeCanonicalTokenSource(*fact) {
		return version
	}
	m := &fact.Measurement
	reasoning := nativeUsageReasoningForPublic(m.ReasoningTokens, m.ReasoningOutputTokens)
	m.ReasoningTokens, m.ReasoningInsideOutput = reasoning.Tokens, reasoning.InsideOutput
	if !reasoning.RelationshipKnown {
		m.TokenSemanticsVersion = 0
	}
	fact.TokenSemantics = (indexedObservation{Measurement: *m}).semantics()
	return nativeUsageAccountingRepairVersion
}

func nativeCanonicalTokenSource(fact models.UsageCanonicalFact) bool {
	if fact.TokenSource != nil {
		return fact.TokenSource.Source == usageSourceNative
	}
	return fact.Measurement.Source == usageSourceNative
}

// Until repair mutations publish, old SQL summaries lack the private output
// subset and cannot reconstruct its public amount. Mask that amount rather than
// presenting their old zero; canonical fact payloads can map the raw subset.
func nativeUsageReasoningMissingAggregateSQL(publicationPending bool) string {
	if publicationPending {
		return `SUM(CASE WHEN metric_kind='tokens' AND source='native' THEN token_count ELSE reasoning_missing END)`
	}
	return `SUM(reasoning_missing)`
}

func nativeUsageReasoningAggregateSQL(publicationPending bool) string {
	amount := `SUM(reasoning_tokens)`
	if publicationPending {
		amount = `SUM(CASE WHEN metric_kind='tokens' AND source='native' THEN NULL ELSE reasoning_tokens END)`
	}
	return `CASE WHEN ` + nativeUsageReasoningMissingAggregateSQL(publicationPending) +
		`=0 AND SUM(token_count)>0 THEN CAST(` + amount + ` AS BIGINT) END`
}

func suppressPendingNativeSummaryReasoning(summary *models.UsageDailySummary, publicationPending bool) {
	if publicationPending && summary.MetricKind == projectionMetricTokens && summary.Source == usageSourceNative {
		summary.ReasoningTokens = nil
		summary.ReasoningMissing = max(summary.ReasoningMissing, summary.TokenCount)
	}
}
