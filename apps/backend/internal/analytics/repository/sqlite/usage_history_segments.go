package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const (
	usageSegmentDeclared = 1
	usageSegmentLegacy   = 2
	usageMetricRoles     = 3
)

func usageSegmentHash(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func validateUsageSegment(segment *pluginsdk.UsageSegmentIdentityV1) error {
	if segment == nil {
		return nil
	}
	for _, id := range []string{segment.DecompositionID, segment.SegmentID} {
		if strings.TrimSpace(id) == "" || len(id) > 512 || !utf8.ValidString(id) || strings.ContainsRune(id, 0) {
			return errors.New("segment decomposition and member IDs must be nonempty UTF-8 strings of at most 512 bytes without NUL")
		}
	}
	return nil
}

func usageSourceBundle(source, decomposition string) string {
	prefix := "1"
	if source == usageSourceNative {
		prefix = "0"
	}
	return prefix + hex.EncodeToString([]byte(source)) + ":" + usageSegmentHash(decomposition)
}

func setUsageObservationSegment(row *indexedObservation, input *pluginsdk.UsageObservationV1) {
	m := row.Measurement
	setUsageSegmentLineage(row, input)
	coverage := usageSegmentTimeKey(m.CoverageStart) + ":" + row.CoverageKey
	row.GroupKey = coverage
	if row.Segment != "" {
		row.GroupKey += ":segment:" + row.Segment
	}
	row.ScanKey = coverage + ":" + row.Bundle + ":" + row.Segment + ":" + row.Table + ":" + m.ID
	row.TokenBundle, row.CostBundle = "", ""
	legacy := input == nil && m.Source != usageSourceNative
	if legacy || usageMeasurementHasTokens(m) || input != nil && input.Tokens != nil {
		row.TokenBundle = row.Bundle
	}
	if legacy || m.CostSubcents != nil || input != nil && input.Cost != nil {
		row.CostBundle = row.Bundle
	}
}

func setUsageSegmentLineage(row *indexedObservation, input *pluginsdk.UsageObservationV1) {
	m := row.Measurement
	decomposition := "unpartitioned"
	switch {
	case input != nil && input.Segment != nil:
		decomposition = "declared:" + input.Segment.DecompositionID
		row.Segment = usageSegmentHash(m.Source, decomposition, input.Segment.SegmentID)
		row.Layout = usageSegmentDeclared
	case row.Segment != "" && row.Layout == usageSegmentDeclared && row.Bundle != "":
		return
	case usageLegacySegmentCandidate(m, input):
		// The supported old mapper reported cumulative session/model series.
		// Provider is a mutable merged label, not proof of a disjoint member.
		decomposition = "legacy-session-model"
		if row.Segment == "" {
			row.Segment = usageSegmentHash(m.Source, decomposition, m.Model)
		}
		row.Layout = usageSegmentLegacy
	case row.Segment != "":
		// Explicit supersession can carry a supported legacy lineage into v1.
		decomposition = "legacy-session-model"
		row.Layout = usageSegmentLegacy
	case input != nil && (input.Tokens == nil || !usageMeasurementHasCanonicalTokens(m)):
		// Metric roles do not declare disjoint members or infer segment identity.
		row.Layout = usageMetricRoles
	default:
		row.Layout = 0
	}
	row.Bundle = usageSourceBundle(m.Source, decomposition)
}

func usageLegacySegmentCandidate(m models.SessionUsageMeasurement, input *pluginsdk.UsageObservationV1) bool {
	return input == nil && m.Source != usageSourceNative && m.SourceRecordID != "" && m.UsageIdentity != ""
}

func usageSegmentTimeKey(at *time.Time) string {
	if at == nil {
		return "0"
	}
	return "1" + at.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func usageSegmentSpanSeconds(row indexedObservation) int64 {
	m := row.Measurement
	if m.CoverageStart == nil || m.CoverageEnd == nil || !m.CoverageEnd.After(*m.CoverageStart) {
		return 0
	}
	return m.CoverageEnd.Unix() - m.CoverageStart.Unix() + 1
}

func usageMeasurementHasTokens(m models.SessionUsageMeasurement) bool {
	return m.InputTokens != nil || m.OutputTokens != nil || m.TotalTokens != nil ||
		m.CacheReadTokens != nil || m.CacheWriteTokens != nil || m.ReasoningTokens != nil || m.Turns != nil
}

func usageMeasurementHasCanonicalTokens(m models.SessionUsageMeasurement) bool {
	return m.InputTokens != nil || m.OutputTokens != nil || m.TotalTokens != nil ||
		m.CacheReadTokens != nil || m.CacheWriteTokens != nil || m.ReasoningTokens != nil
}

func clearUsageFactTokens(fact *models.UsageCanonicalFact) {
	m := &fact.Measurement
	m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheWriteTokens = nil, nil, nil, nil
	m.ReasoningTokens, m.ReasoningOutputTokens, m.TotalTokens, m.SourceTotalTokens, m.Turns = nil, nil, nil, nil, nil
	m.ReportedCacheWriteTokens = nil
	fact.TokenSource = nil
	fact.TokenMetricAbsent = true
}

func clearUsageFactCost(fact *models.UsageCanonicalFact) {
	fact.Measurement.CostSubcents, fact.Measurement.Currency = nil, ""
	fact.Measurement.CostBasis, fact.Measurement.CostCoverage = usageUnknown, "missing"
	fact.CostSource = nil
	fact.CostMetricAbsent = true
	fact.Measurement.Estimated = false
}
