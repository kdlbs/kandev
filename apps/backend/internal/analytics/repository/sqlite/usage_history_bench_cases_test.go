//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

type usageBenchOracle struct {
	Tokens, Cost, NativeTokens int64
	TokenRows, CostRows        int
	TokenMissing, CostMissing  bool
	Currencies, Providers      map[string]bool
}

type usageBenchCase struct {
	Name     string
	Filter   models.SessionUsageFilter
	Expected usageBenchOracle
}

func usageBenchCases(deletedTask string) []*usageBenchCase {
	var cases []*usageBenchCase
	for _, calendar := range []string{"UTC", "America/New_York", "Asia/Kathmandu", "Europe/Lisbon"} {
		for _, period := range []string{"recent", "five_years"} {
			location, err := time.LoadLocation(calendar)
			if err != nil {
				panic(err)
			}
			start := time.Date(2025, 12, 2, 0, 0, 0, 0, location)
			group := "day"
			if period == "five_years" {
				start, group = time.Date(2021, 1, 1, 0, 0, 0, 0, location), "month"
			}
			end := time.Date(2026, 1, 1, 0, 0, 0, 0, location)
			cases = append(cases, &usageBenchCase{Name: period + "/" + calendar, Filter: models.SessionUsageFilter{WorkspaceID: usageBenchWorkspace, Timezone: calendar, GroupBy: group, Start: &start, End: &end, Limit: 200}})
		}
	}
	for _, item := range []struct{ name, provider, model string }{{"lifetime", "lifetime-provider", ""}, {"native", "native-provider", ""}, {"mixed_actors", "", "mixed-model"}, {"foreign_coarse", "coarse-provider", ""}} {
		cases = append(cases, &usageBenchCase{Name: item.name, Filter: models.SessionUsageFilter{WorkspaceID: usageBenchWorkspace, Timezone: "America/New_York", GroupBy: "model", Provider: item.provider, Model: item.model, Limit: 200}})
	}
	deleted := cases[0].Filter
	deleted.GroupBy, deleted.TaskIDs = "task", []string{deletedTask}
	deleted.Start, deleted.End = nil, nil
	cases = append(cases, &usageBenchCase{Name: "deleted_task", Filter: deleted})
	location, _ := time.LoadLocation("America/New_York")
	start, end := time.Date(2025, 3, 1, 0, 0, 0, 0, location), time.Date(2025, 4, 1, 0, 0, 0, 0, location)
	cases = append(cases, &usageBenchCase{Name: "dst_new_york", Filter: models.SessionUsageFilter{WorkspaceID: usageBenchWorkspace, Timezone: "America/New_York", GroupBy: "day", Start: &start, End: &end, Limit: 200}})
	coarseStart, coarseEnd := time.Date(2025, 12, 15, 12, 0, 0, 0, time.UTC), time.Date(2025, 12, 16, 12, 0, 0, 0, time.UTC)
	cases = append(cases, &usageBenchCase{Name: "foreign_coarse_boundary", Filter: models.SessionUsageFilter{WorkspaceID: usageBenchWorkspace, Timezone: "America/New_York", GroupBy: "day", Provider: "coarse-provider", Start: &coarseStart, End: &coarseEnd, Limit: 200}})
	emptyStart, emptyEnd := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)
	cases = append(cases, &usageBenchCase{Name: "empty_range", Filter: models.SessionUsageFilter{WorkspaceID: usageBenchWorkspace, Timezone: "UTC", GroupBy: "day", Start: &emptyStart, End: &emptyEnd, Limit: 200}})
	for _, scenario := range cases {
		scenario.Expected.Currencies, scenario.Expected.Providers = map[string]bool{}, map[string]bool{}
	}
	return cases
}

// Scalar expectations are accumulated from the producer recipe before source
// normalization or selection. They never read canonical payloads or summaries.
// The flags below describe only explicit special-fixture alternatives/roles.
type usageBenchExpectedRecord struct {
	Measurement                                      models.SessionUsageMeasurement
	Native, TokenAbsent, CostAbsent, AllTimeExcluded bool
}

func usageBenchFactMatches(record usageBenchExpectedRecord, filter models.SessionUsageFilter, providers bool) bool {
	m := record.Measurement
	if m.WorkspaceID != filter.WorkspaceID || filter.Model != "" && filter.Model != m.Model || !providers && filter.Provider != "" && filter.Provider != m.Provider {
		return false
	}
	if len(filter.TaskIDs) > 0 && !slices.Contains(filter.TaskIDs, m.TaskID) {
		return false
	}
	allTime := filter.Start == nil && filter.End == nil
	if m.CoverageStart == nil {
		dated := filter.GroupBy == "day" || filter.GroupBy == "week" || filter.GroupBy == "month"
		return allTime || filter.IncludeUndated && !dated
	}
	if allTime && record.AllTimeExcluded {
		return false
	}
	if filter.End != nil && !m.CoverageStart.Before(*filter.End) {
		return false
	}
	if filter.Start != nil {
		point := m.CoverageEnd != nil && m.CoverageEnd.Sub(*m.CoverageStart) <= time.Microsecond
		if point {
			return !m.CoverageStart.Before(*filter.Start)
		}
		return m.CoverageEnd.After(*filter.Start)
	}
	return true
}

func (f *usageBenchFixture) recordOracle(record usageBenchExpectedRecord) {
	m := record.Measurement
	f.expectedFacts[m.ScopeID]++
	if !record.TokenAbsent {
		f.expectedContributions[m.ScopeID]++
	}
	if !record.CostAbsent {
		f.expectedContributions[m.ScopeID]++
	}
	for _, scenario := range f.cases {
		if usageBenchFactMatches(record, scenario.Filter, true) {
			provider := m.Provider
			if provider == "" {
				provider = unknownProviderFilter
			}
			scenario.Expected.Providers[provider] = true
		}
		if !usageBenchFactMatches(record, scenario.Filter, false) {
			continue
		}
		expected := &scenario.Expected
		if !record.TokenAbsent {
			expected.TokenRows++
			if m.TotalTokens == nil {
				expected.TokenMissing = true
			} else {
				expected.Tokens += *m.TotalTokens
				if record.Native && m.Provider == "native-provider" {
					expected.NativeTokens += *m.TotalTokens
				}
			}
		}
		if !record.CostAbsent {
			expected.CostRows++
			if m.CostSubcents == nil {
				expected.CostMissing = true
			} else {
				expected.Cost += *m.CostSubcents
			}
			if m.Currency != "" {
				expected.Currencies[m.Currency] = true
			}
		}
	}
}

func usageBenchExpectedAmounts(expected usageBenchOracle) (*int64, *int64) {
	var tokens, cost *int64
	if !expected.TokenMissing {
		tokens = models.Int64(expected.Tokens)
	}
	if !expected.CostMissing && len(expected.Currencies) <= 1 {
		cost = models.Int64(expected.Cost)
	}
	return tokens, cost
}

func (f *usageBenchFixture) readAndValidate(t testing.TB, scenario *usageBenchCase) *models.TokenUsageReport {
	t.Helper()
	return readUsageBenchReport(t, f.repo, scenario)
}

func readUsageBenchReport(t testing.TB, repo *Repository, scenario *usageBenchCase) *models.TokenUsageReport {
	t.Helper()
	result, err := fetchUsageBenchReport(context.Background(), repo, scenario.Filter)
	usageBenchMust(t, err)
	validateUsageBenchResult(t, result, scenario)
	return result.Pages[0]
}

func validateUsageBenchReport(t testing.TB, report *models.TokenUsageReport, publication string, scenario *usageBenchCase) {
	t.Helper()
	if report.ProjectionStatus != "ready" || report.ProjectionPending || publication == "" || report.ProjectionGeneration != publication || !report.HasHistory || report.LastUpdated == nil {
		t.Fatalf("%s lacks a consistent complete publication", scenario.Name)
	}
	tokens, cost := usageBenchExpectedAmounts(scenario.Expected)
	if !reflect.DeepEqual(report.Totals.TotalTokens, tokens) || !reflect.DeepEqual(report.Totals.CostSubcents, cost) {
		detail, _ := json.Marshal(struct {
			Report   *models.TokenUsageReport
			Expected usageBenchOracle
		}{report, scenario.Expected})
		t.Fatalf("%s totals differ: %s", scenario.Name, detail)
	}
	var providers []string
	for provider := range scenario.Expected.Providers {
		providers = append(providers, provider)
	}
	slices.Sort(providers)
	// SQL option ordering follows the engine's collation. Compare every value
	// and its multiplicity without changing the report or captured SQL.
	actualProviders := slices.Clone(report.Providers)
	slices.Sort(actualProviders)
	if !slices.Equal(actualProviders, providers) {
		t.Fatalf("%s provider options differ: got=%v want=%v", scenario.Name, report.Providers, providers)
	}
}

func validateUsageBenchRows(t testing.TB, rows []models.TokenUsageRow, scenario *usageBenchCase) {
	t.Helper()
	var native int64
	for _, row := range rows {
		if row.Turns != nil {
			t.Fatal("query fixture has no proven turn aggregates; native responses are not turns")
		}
		if row.Provider == "native-provider" && row.TotalTokens != nil {
			native += *row.TotalTokens
		}
		if scenario.Name == "deleted_task" && (row.TaskAvailable || row.SessionAvailable) {
			t.Fatal("deleted benchmark subject unexpectedly has live availability")
		}
		validateUsageBenchSpecialRow(t, row, scenario.Name)
	}
	if native != scenario.Expected.NativeTokens {
		t.Fatalf("%s native token oracle=%d got=%d", scenario.Name, scenario.Expected.NativeTokens, native)
	}
	if scenario.Name == "mixed_actors" {
		validateUsageBenchMixedRows(t, rows, scenario.Expected)
	}
}

func validateUsageBenchSpecialRow(t testing.TB, row models.TokenUsageRow, scenario string) {
	t.Helper()
	if scenario == "foreign_coarse_boundary" && (row.Coverage != models.UsageCoveragePartial || row.Calendar != "Europe/Lisbon" || row.CalendarBasis != "source") {
		t.Fatal("benchmark allocated or relabeled coarse source-calendar coverage")
	}
}

// Independent metric roles can produce separate rows when their semantics or
// calendars differ. Require both source/actor proofs without allocating cost.
func validateUsageBenchMixedRows(t testing.TB, rows []models.TokenUsageRow, expected usageBenchOracle) {
	t.Helper()
	var tokens, cost int64
	var tokenFound, costFound bool
	for _, row := range rows {
		if row.EffectiveCostPerMillion != nil {
			t.Fatal("benchmark invented an allocated rate for mixed actors")
		}
		if row.TotalTokens != nil {
			if !usageBenchMixedTokenProof(row.TokenSource) {
				t.Fatalf("benchmark lost native token actor evidence: %+v", row.TokenSource)
			}
			tokens += *row.TotalTokens
			tokenFound = true
		}
		if row.CostSubcents != nil {
			if !usageBenchMixedCostProof(row.CostSource) {
				t.Fatalf("benchmark lost mixed cost actor evidence: %+v", row.CostSource)
			}
			if row.CostBasis != models.UsageCostBasisReported || row.Estimated {
				t.Fatal("benchmark changed selected reported cost quality")
			}
			cost += *row.CostSubcents
			costFound = true
		}
	}
	if !tokenFound || !costFound || tokens != expected.Tokens || cost != expected.Cost {
		t.Fatalf("benchmark split metric oracle differs: tokens=%d cost=%d expected=%d/%d", tokens, cost, expected.Tokens, expected.Cost)
	}
}

func usageBenchMixedTokenProof(source *models.UsageReportSource) bool {
	return source != nil && source.Source == usageSourceNative && source.ActorID == "user-1" &&
		source.ActorStatus == "attributed" && source.ActorKind == "user"
}

func usageBenchMixedCostProof(source *models.UsageReportSource) bool {
	return source != nil && source.Source == "plugin:mixed" && source.ActorID == "" &&
		source.ActorStatus == "mixed" && source.ActorKind == "unknown"
}
