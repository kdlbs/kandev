package sqlite

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/analytics/models"
)

const (
	projectionMetricTokens = "tokens"
	projectionMetricCost   = "cost"
	projectionCalendarUTC  = "UTC"
)

var usageProjectionAmountColumns = []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "reasoning_tokens", "total_tokens", "source_total_tokens", "turns", "cost_subcents", "rate_tokens"}
var usageProjectionDimensionColumns = []string{"metric_kind", "actor_id", "actor_status", "actor_kind", "source", "model", "provider", "agent_client", "semantics", "currency", "calendar", "usage_date"}
var usageProjectionContributionColumns = []string{
	"scope_id", "generation", "build_id", "fact_key", "work_key", "summary_key", "coverage_kind", "coverage_start", "coverage_end",
	"metric_kind", "actor_id", "actor_status", "actor_kind", "source", "model", "provider", "agent_client", "semantics", "currency", "calendar", "usage_date",
	"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "reasoning_tokens", "total_tokens", "source_total_tokens", "turns",
	"cost_subcents", "rate_tokens", "partial", "estimated", "stale", "cost_basis",
}

type usageProjectionPeriod struct {
	Calendar string
	Date     string
}

func usageProjectionPeriods(fact models.UsageCanonicalFact) ([]usageProjectionPeriod, error) {
	m := fact.Measurement
	if fact.CoverageKind == usageLifetime {
		return []usageProjectionPeriod{{}}, nil
	}
	if fact.CoverageKind != usageCoveragePoint {
		return []usageProjectionPeriod{{Calendar: m.SourceTimezone, Date: m.SourceDate}}, nil
	}
	periods := []usageProjectionPeriod{{Calendar: projectionCalendarUTC, Date: m.CoverageStart.UTC().Format(time.DateOnly)}}
	if m.SourceTimezone != "" && m.SourceTimezone != projectionCalendarUTC {
		location, err := time.LoadLocation(m.SourceTimezone)
		if err != nil {
			return nil, err
		}
		periods = append(periods, usageProjectionPeriod{Calendar: m.SourceTimezone, Date: m.CoverageStart.In(location).Format(time.DateOnly)})
	}
	return periods, nil
}

func usageProjectionActor(source *models.UsageMetricSource) (string, string, string, string) {
	if source == nil {
		return "", models.UsageAttributionUnknown, usageUnknown, ""
	}
	return source.ActorID, source.ActorStatus, source.ActorKind, source.Source
}

func usageProjectionContribution(work usageProjectionWork, fact models.UsageCanonicalFact, metric string, period usageProjectionPeriod) map[string]any {
	m := fact.Measurement
	source := fact.TokenSource
	if metric == projectionMetricCost {
		source = fact.CostSource
	}
	actor, status, actorKind, provenance := usageProjectionActor(source)
	start, end := usageProjectionFactBounds(fact)
	row := map[string]any{
		"scope_id": work.Scope, "generation": work.Generation, "build_id": work.Build,
		"fact_key": fact.Key, "work_key": work.Work, "coverage_kind": fact.CoverageKind,
		"coverage_start": start, "coverage_end": end,
		"metric_kind": metric, "actor_id": actor, "actor_status": status, "actor_kind": actorKind,
		"source": provenance, "model": m.Model, "provider": m.Provider, "agent_client": m.AgentClient,
		"semantics": fact.TokenSemantics, "currency": "", "calendar": period.Calendar, "usage_date": period.Date,
		"partial": boolInt(fact.Partial), "estimated": boolInt(metric == projectionMetricCost && m.Estimated), "stale": boolInt(m.Stale), "cost_basis": usageUnknown,
	}
	for _, column := range usageProjectionAmountColumns {
		row[column] = nil
	}
	if metric == projectionMetricTokens {
		row["input_tokens"], row["output_tokens"] = m.InputTokens, m.OutputTokens
		row["cache_read_tokens"], row["cache_write_tokens"] = m.CacheReadTokens, m.CacheWriteTokens
		row["reasoning_tokens"], row["total_tokens"] = m.ReasoningTokens, m.TotalTokens
		row["source_total_tokens"], row["turns"] = m.SourceTotalTokens, m.Turns
		row["partial"] = boolInt(fact.Partial || m.Coverage != usageCoverageComplete)
	} else {
		row["cost_subcents"], row["currency"], row["cost_basis"] = m.CostSubcents, m.Currency, m.CostBasis
		row["partial"] = boolInt(fact.Partial || m.CostCoverage != usageCoverageComplete)
		if usageProjectionRateAvailable(fact) {
			row["rate_tokens"] = m.TotalTokens
		}
	}
	var key []any
	for _, column := range usageProjectionDimensionColumns {
		key = append(key, row[column])
	}
	encoded, _ := json.Marshal(key)
	row["summary_key"] = uuid.NewSHA1(uuid.NameSpaceURL, encoded).String()
	return row
}

func usageProjectionFactBounds(fact models.UsageCanonicalFact) (*time.Time, *time.Time) {
	m := fact.Measurement
	if fact.CoverageKind == usageCoveragePoint && m.CoverageStart != nil {
		// Point reads predicate occurrence time. This nonempty index envelope
		// also survives the database's microsecond timestamp resolution.
		end := m.CoverageStart.Add(time.Microsecond)
		return m.CoverageStart, &end
	}
	return m.CoverageStart, m.CoverageEnd
}

func usageProjectionRateAvailable(fact models.UsageCanonicalFact) bool {
	m := fact.Measurement
	return fact.TokenSource != nil && fact.CostSource != nil &&
		fact.TokenSource.ActorID == fact.CostSource.ActorID && fact.TokenSource.ActorStatus == fact.CostSource.ActorStatus &&
		fact.TokenSource.ActorKind == fact.CostSource.ActorKind &&
		!fact.Partial && m.Coverage == usageCoverageComplete && m.CostCoverage == usageCoverageComplete &&
		m.CostBasis != usageUnknown && m.CostBasis != usageMixedValue && m.Currency != "" && m.CostSubcents != nil && m.TotalTokens != nil
}
