package sqlite

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

var errUsageSemanticsUnsupported = errors.New("unsupported token semantics version")

func validateContractObservation(input pluginsdk.UsageObservationV1) (models.SessionUsageMeasurement, error) {
	m := models.SessionUsageMeasurement{SourceRecordID: input.SourceRecordID, SourceVersion: input.SourceVersion, Revision: input.Revision, PayloadDigest: input.PayloadDigest,
		Model: stringValue(input.ModelID), Provider: stringValue(input.ModelProvider), AgentClient: stringValue(input.AgentClient), CostBasis: usageUnknown, Coverage: usageCoveragePartial, CostCoverage: "missing", AttributionStatus: "attributed"}
	if err := validateContractHeader(input); err != nil {
		return m, err
	}
	if err := validateUsageSegment(input.Segment); err != nil {
		return m, err
	}
	if input.Tokens != nil {
		t := input.Tokens
		m.InputTokens, m.OutputTokens, m.CacheReadTokens, m.CacheWriteTokens, m.ReasoningTokens, m.TotalTokens = t.InputTokens, t.OutputTokens, t.CacheReadTokens, t.CacheWriteTokens, t.ReasoningTokens, t.TotalTokens
		m.SourceTotalTokens = cloneInt64(t.TotalTokens)
		if err := validateContractTokens(t, input.TokenSemantics); err != nil {
			return m, err
		}
	}
	if err := normalizeContractCost(input.Cost, &m); err != nil {
		return m, err
	}
	if input.TokenSemantics != nil {
		m.TokenSemanticsVersion = 1
		m.ReasoningInsideOutput = boolValue(input.TokenSemantics.ReasoningIncludedInOutput)
		m.CacheReadInsideInput = boolValue(input.TokenSemantics.CacheReadIncludedInInput)
	}
	normalizeContractQuality(input.Quality, &m)
	if err := normalizeContractCoverage(input.Coverage, &m); err != nil {
		return m, err
	}
	labels, err := json.Marshal(input.RawSourceLabels)
	if err != nil || len(labels) > 8192 {
		return m, errors.New("raw source labels exceed the contract limit")
	}
	return m, nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func boolValue(v *bool) bool { return v != nil && *v }

func validateContractTokens(t *pluginsdk.UsageTokenAmountsV1, semantics *pluginsdk.UsageTokenSemanticsV1) error {
	if err := validateContractCategoryAmounts(t, semantics != nil); err != nil {
		return err
	}
	if semantics == nil {
		return nil
	}
	if err := validateContractSubsets(t, semantics); err != nil {
		return err
	}
	parts := []*int64{t.InputTokens, t.OutputTokens}
	if semantics.CacheReadIncludedInInput != nil && !*semantics.CacheReadIncludedInInput {
		parts = append(parts, t.CacheReadTokens)
	}
	if semantics.CacheWriteIncludedInInput != nil && !*semantics.CacheWriteIncludedInInput {
		parts = append(parts, t.CacheWriteTokens)
	}
	if semantics.ReasoningIncludedInOutput != nil && !*semantics.ReasoningIncludedInOutput {
		parts = append(parts, t.ReasoningTokens)
	}
	return validateContractTotal(parts, t.TotalTokens)
}

func validateContractSubsets(t *pluginsdk.UsageTokenAmountsV1, semantics *pluginsdk.UsageTokenSemanticsV1) error {
	for _, subset := range []struct {
		included       *bool
		amount, parent *int64
	}{
		{semantics.CacheReadIncludedInInput, t.CacheReadTokens, t.InputTokens},
		{semantics.CacheWriteIncludedInInput, t.CacheWriteTokens, t.InputTokens},
		{semantics.ReasoningIncludedInOutput, t.ReasoningTokens, t.OutputTokens},
	} {
		if boolValue(subset.included) && subset.amount != nil && subset.parent != nil && *subset.amount > *subset.parent {
			return errors.New("token subset exceeds its parent category")
		}
	}
	return nil
}

func validateContractTotal(parts []*int64, sourceTotal *int64) error {
	var total int64
	for _, value := range parts {
		if value != nil {
			if *value > math.MaxInt64-total {
				return errors.New("token total overflows int64")
			}
			total += *value
		}
	}
	if sourceTotal != nil && total > *sourceTotal {
		return errors.New("token semantics conflict with source total")
	}
	return nil
}

func normalizeContractCoverage(c *pluginsdk.UsageCoverageV1, m *models.SessionUsageMeasurement) error {
	if c == nil {
		return nil
	}
	var err error
	if c.OccurrenceAt != nil {
		err = normalizeContractPoint(c, m)
	} else {
		err = normalizeContractInterval(c, m)
	}
	if err != nil {
		return err
	}
	m.SourceDate, m.SourceTimezone = stringValue(c.SourceDate), stringValue(c.SourceTimezone)
	if (c.SourceDate == nil) != (c.SourceTimezone == nil) {
		return errors.New("source date requires its calendar timezone")
	}
	if c.SourceDate != nil {
		zone, err := time.LoadLocation(*c.SourceTimezone)
		if err != nil {
			return errors.New("invalid source calendar timezone")
		}
		day, err := time.ParseInLocation("2006-01-02", *c.SourceDate, zone)
		if err != nil {
			return errors.New("invalid source date")
		}
		if m.CoverageStart == nil || !day.Equal(*m.CoverageStart) || !day.AddDate(0, 0, 1).Equal(*m.CoverageEnd) {
			return errors.New("source-day interval must match its calendar")
		}
	}
	return nil
}

func normalizeContractCost(c *pluginsdk.UsageCostV1, m *models.SessionUsageMeasurement) error {
	if c == nil {
		return nil
	}
	m.Currency, m.CostBasis = stringValue(c.Currency), string(c.Basis)
	if c.Basis != pluginsdk.UsageCostBasisUnknown && c.Basis != pluginsdk.UsageCostBasisReported && c.Basis != pluginsdk.UsageCostBasisEstimated {
		return errors.New("unsupported cost basis")
	}
	if c.Amount != nil && (*c.Amount < 0 || len(m.Currency) != 3 || strings.ToUpper(m.Currency) != m.Currency || c.Basis == pluginsdk.UsageCostBasisUnknown) {
		return errors.New("cost requires a nonnegative amount, explicit currency, and known basis")
	}
	m.CostSubcents = c.Amount
	if c.Amount != nil {
		m.CostCoverage = usageCoveragePartial
	}
	return nil
}

func normalizeContractQuality(quality *pluginsdk.UsageObservationQualityV1, m *models.SessionUsageMeasurement) {
	if quality != nil {
		if quality.TokenCoverageComplete {
			m.Coverage = usageCoverageComplete
		}
		if quality.CostCoverageComplete && m.CostSubcents != nil {
			m.CostCoverage = usageCoverageComplete
		}
		m.Stale = quality.Stale
	}

}

func parseContractTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil, errors.New("coverage time must use RFC3339")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func normalizeContractPoint(c *pluginsdk.UsageCoverageV1, m *models.SessionUsageMeasurement) error {
	if c.CoverageStart != nil || c.CoverageEnd != nil {
		return errors.New("point and interval coverage are mutually exclusive")
	}
	point, err := parseContractTime(c.OccurrenceAt)
	if err != nil {
		return err
	}
	end := point.Add(time.Nanosecond)
	m.ObservedAt, m.CoverageStart, m.CoverageEnd = *point, point, &end
	m.CoverageKey = "point:" + point.Format(time.RFC3339Nano)

	return nil
}

func normalizeContractInterval(c *pluginsdk.UsageCoverageV1, m *models.SessionUsageMeasurement) error {
	start, err := parseContractTime(c.CoverageStart)
	if err != nil {
		return err
	}
	end, err := parseContractTime(c.CoverageEnd)
	if err != nil {
		return err
	}
	if (start == nil) != (end == nil) || (start != nil && !start.Before(*end)) {
		return errors.New("coverage must be a nonempty half-open interval")
	}
	m.CoverageStart, m.CoverageEnd = start, end
	if start != nil {
		m.ObservedAt = *start
		m.CoverageKey = "interval:" + start.Format(time.RFC3339Nano) + ":" + end.Format(time.RFC3339Nano)
	}
	return nil
}

func validateContractCategoryAmounts(t *pluginsdk.UsageTokenAmountsV1, knownSemantics bool) error {
	values := []*int64{t.InputTokens, t.OutputTokens, t.CacheReadTokens, t.CacheWriteTokens, t.ReasoningTokens, t.TotalTokens}
	for _, v := range values {
		if v != nil && *v < 0 {
			return errors.New("token amounts cannot be negative")
		}
		if knownSemantics && v != nil && t.TotalTokens != nil && *v > *t.TotalTokens {
			return errors.New("token category exceeds source total")
		}
	}
	return nil
}

func validateContractHeader(input pluginsdk.UsageObservationV1) error {
	if input.ItemID == "" || len(input.ItemID) > 512 || input.SourceRecordID == "" || input.Revision < 1 {
		return errors.New("item id, source record id, and positive revision are required")
	}
	digest, err := input.ComputePayloadDigestV1()
	if err != nil || digest != input.PayloadDigest {
		return errors.New("payload digest does not match observation content")
	}
	if input.TokenSemantics != nil && input.TokenSemantics.Version != "1" && input.TokenSemantics.Version != "v1" {
		return errUsageSemanticsUnsupported
	}
	return nil
}
