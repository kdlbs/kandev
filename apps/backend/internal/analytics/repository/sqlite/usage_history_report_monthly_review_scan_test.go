//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryMonthlyReviewEdges(t *testing.T) {
	engines := []string{"sqlite"}
	if os.Getenv("KANDEV_TEST_POSTGRES_DSN") != "" {
		engines = append(engines, "postgres")
	}
	for _, engine := range engines {
		t.Run(engine, func(t *testing.T) {
			fixture, filter := newUsageHistoryMonthlyReviewFixture(t, engine)
			scope, generation := fixture.subjects[0].scope, fixture.subjects[0].generation

			t.Run("daily currency normalization", func(t *testing.T) {
				assertUsageHistoryMonthlyReviewDimension(t, fixture, filter, scope, generation, "review-currency-model", 0)
				assertUsageHistoryMonthlyReviewDimension(t, fixture, filter, scope, generation, "unknown-cost-model", 0)
				requireUsageReportSourcesEqual(t, fixture, filter,
					reportSummarySourceFallback(scope, generation, filter), reportUTCMonthSummarySource(scope, generation, filter))
				report, err := fixture.repo.ListSessionUsage(t.Context(), filter)
				require.NoError(t, err)
				rows := map[string]models.TokenUsageRow{}
				unknownCostRows := map[string]models.TokenUsageRow{}
				for _, row := range report.Rows {
					switch row.Model {
					case "review-currency-model":
						rows[row.Currency] = row
					case "unknown-cost-model":
						unknownCostRows[row.Currency] = row
					}
				}
				require.Equal(t, models.Int64(10), rows["USD"].TotalTokens, "tokens from the known-USD day stay with USD")
				require.Equal(t, models.Int64(2), rows["USD"].CostSubcents)
				require.Equal(t, models.Int64(20), rows[""].TotalTokens, "tokens from tokens-only and unknown-currency days stay unassigned")
				require.Nil(t, rows[""].CostSubcents, "the tokens-only day keeps its absent cost visible")
				require.Nil(t, unknownCostRows[""].TotalTokens)
				require.Equal(t, models.Int64(2), unknownCostRows[""].CostSubcents, "unknown-currency cost remains in the blank currency row")
			})

			t.Run("provider options ignore selected provider while rows stay filtered", func(t *testing.T) {
				selected := filter
				selected.Provider = "selected-provider"
				selected.Model = "review-currency-model"
				requireUsageReportSourcesEqual(t, fixture, selected,
					reportSummarySourceFallback(scope, generation, selected), reportUTCMonthSummarySource(scope, generation, selected))
				report, err := fixture.repo.ListSessionUsage(t.Context(), selected)
				require.NoError(t, err)
				require.Equal(t, []string{"other-provider", "selected-provider"}, report.Providers)
				require.NotEmpty(t, report.Rows)
				for _, row := range report.Rows {
					require.Equal(t, "selected-provider", row.Provider)
				}
				require.Equal(t, models.Int64(30), report.Totals.TotalTokens)
			})

			t.Run("shared local day across UTC dates", func(t *testing.T) {
				sharedDay := monthlyReviewFilter(filter, "2026-01-01", "2026-02-01")
				sharedDay.Model = "review-shared-local-day-model"
				assertUsageHistoryMonthlyReviewDimension(t, fixture, sharedDay, scope, generation, sharedDay.Model, 0)
				requireUsageReportSourcesEqual(t, fixture, sharedDay,
					reportSummarySourceFallback(scope, generation, sharedDay), reportUTCMonthSummarySource(scope, generation, sharedDay))
				report, err := fixture.repo.ListSessionUsage(t.Context(), sharedDay)
				require.NoError(t, err)
				require.Equal(t, models.Int64(20), report.Totals.TotalTokens)
				require.Equal(t, models.Int64(20), monthlyReviewRow(t, report.Rows, sharedDay.Model, "USD").TotalTokens)
			})

			t.Run("local midnight within one UTC date", func(t *testing.T) {
				midnight := monthlyReviewFilter(filter, "2026-01-01", "2026-02-01")
				midnight.Model = "review-midnight-model"
				assertUsageHistoryMonthlyReviewDimension(t, fixture, midnight, scope, generation, midnight.Model, 0)
				requireUsageReportSourcesEqual(t, fixture, midnight,
					reportSummarySourceFallback(scope, generation, midnight), reportUTCMonthSummarySource(scope, generation, midnight))
				report, err := fixture.repo.ListSessionUsage(t.Context(), midnight)
				require.NoError(t, err)
				require.Equal(t, models.Int64(20), report.Totals.TotalTokens)
				require.Equal(t, models.Int64(10), monthlyReviewRow(t, report.Rows, midnight.Model, "USD").TotalTokens)
				require.Equal(t, models.Int64(10), monthlyReviewRow(t, report.Rows, midnight.Model, "").TotalTokens)
			})

			t.Run("balanced independent metrics preserve local-day currency", func(t *testing.T) {
				balancedTokenAt := time.Date(2026, time.January, 4, 1, 0, 0, 0, time.UTC)
				removeUsageReportCostMetricForPoint(t, fixture, "review-balanced-metrics-model", balancedTokenAt)
				balanced := monthlyReviewFilter(filter, "2026-01-01", "2026-02-01")
				balanced.Model, balanced.Provider = "review-balanced-metrics-model", "selected-provider"
				var evidence struct {
					Tokens       int64 `db:"tokens"`
					Costs        int64 `db:"costs"`
					UnknownCosts int64 `db:"unknown_costs"`
					RateMissing  int64 `db:"rate_missing"`
				}
				require.NoError(t, fixture.database.Get(&evidence, fixture.database.Rebind(`SELECT
					COALESCE(SUM(CASE WHEN metric_kind='tokens' THEN token_count ELSE 0 END),0) AS tokens,
					COALESCE(SUM(CASE WHEN metric_kind='cost' THEN cost_count ELSE 0 END),0) AS costs,
					COALESCE(SUM(CASE WHEN metric_kind='cost' AND NULLIF(currency,'') IS NULL THEN cost_count ELSE 0 END),0) AS unknown_costs,
					COALESCE(SUM(CASE WHEN metric_kind='cost' THEN rate_missing ELSE 0 END),0) AS rate_missing
					FROM usage_daily_summaries WHERE scope_id=? AND generation=? AND model=? AND provider=? AND calendar='UTC' AND usage_date='2026-01-04'`),
					scope, generation, balanced.Model, balanced.Provider))
				require.EqualValues(t, 2, evidence.Tokens)
				require.EqualValues(t, 2, evidence.Costs)
				require.Zero(t, evidence.UnknownCosts)
				require.EqualValues(t, 1, evidence.RateMissing)
				var orphanCostRows int
				require.NoError(t, fixture.database.Get(&orphanCostRows, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_projection_contributions
					WHERE scope_id=? AND generation=? AND model=? AND provider=? AND calendar='UTC' AND usage_date=?
					AND metric_kind='cost' AND currency='' AND cost_subcents IS NULL`), scope, generation, balanced.Model, balanced.Provider, balancedTokenAt.Format(time.DateOnly)))
				require.Zero(t, orphanCostRows, "the token-only point has no synthetic cost contribution")
				requireUsageReportSourcesEqual(t, fixture, balanced,
					reportSummarySourceFallback(scope, generation, balanced), reportUTCMonthSummarySource(scope, generation, balanced))
				assertUsageHistoryMonthlyReviewDimension(t, fixture, balanced, scope, generation, balanced.Model, 0)
				report, err := fixture.repo.ListSessionUsage(t.Context(), balanced)
				require.NoError(t, err)
				require.Equal(t, models.Int64(20), report.Totals.TotalTokens)
				require.Equal(t, models.Int64(7), report.Totals.CostSubcents)
				require.Equal(t, models.Int64(10), monthlyReviewRow(t, report.Rows, balanced.Model, "").TotalTokens,
					"the Jan 3 tokens-only point stays unassigned")
				usd := monthlyReviewRow(t, report.Rows, balanced.Model, "USD")
				require.Equal(t, models.Int64(10), usd.TotalTokens)
				require.Equal(t, models.Int64(7), usd.CostSubcents)
			})

			t.Run("UTC edge day mixed currency falls back as a month", func(t *testing.T) {
				edge := monthlyReviewFilter(filter, "2026-01-01", "2026-02-01")
				edge.Model, edge.Provider = "review-edge-currency-model", "selected-provider"
				assertUsageHistoryMonthlyReviewDimension(t, fixture, edge, scope, generation, edge.Model, 0)
				requireUsageReportSourcesEqual(t, fixture, edge,
					reportSummarySourceFallback(scope, generation, edge), reportUTCMonthSummarySource(scope, generation, edge))
				report, err := fixture.repo.ListSessionUsage(t.Context(), edge)
				require.NoError(t, err)
				require.Equal(t, models.Int64(20), report.Totals.TotalTokens)
				require.Nil(t, report.Totals.CostSubcents, "the mixed-currency total has no single cost currency")
				require.Equal(t, models.Int64(20), monthlyReviewRow(t, report.Rows, edge.Model, "").TotalTokens,
					"tokens on the mixed USD/EUR local day stay unassigned")
				require.Equal(t, models.Int64(2), monthlyReviewRow(t, report.Rows, edge.Model, "USD").CostSubcents)
				require.Equal(t, models.Int64(2), monthlyReviewRow(t, report.Rows, edge.Model, "EUR").CostSubcents)
			})

			t.Run("requested-calendar coarse currency shares local-day fallback", func(t *testing.T) {
				coarseCalendar := monthlyReviewFilter(filter, "2026-01-01", "2026-02-01")
				coarseCalendar.Model, coarseCalendar.Provider = "review-coarse-calendar-currency-model", "selected-provider"
				assertUsageHistoryMonthlyReviewDimension(t, fixture, coarseCalendar, scope, generation, coarseCalendar.Model, 0)
				requireUsageReportSourcesEqual(t, fixture, coarseCalendar,
					reportSummarySourceFallback(scope, generation, coarseCalendar), reportUTCMonthSummarySource(scope, generation, coarseCalendar))
				report, err := fixture.repo.ListSessionUsage(t.Context(), coarseCalendar)
				require.NoError(t, err)
				require.Equal(t, models.Int64(20), report.Totals.TotalTokens)
				require.Nil(t, report.Totals.CostSubcents, "the point and source-calendar costs have mixed currencies")
				require.Equal(t, models.Int64(20), monthlyReviewRow(t, report.Rows, coarseCalendar.Model, "").TotalTokens)
				require.Equal(t, models.Int64(2), monthlyReviewRow(t, report.Rows, coarseCalendar.Model, "USD").CostSubcents)
				require.Equal(t, models.Int64(2), monthlyReviewRow(t, report.Rows, coarseCalendar.Model, "EUR").CostSubcents)
			})

			t.Run("coarse and currency-unsafe month falls back once", func(t *testing.T) {
				coarse := filter
				coarse.Model = "review-coarse-unsafe-model"
				assertUsageHistoryMonthlyReviewDimension(t, fixture, coarse, scope, generation, coarse.Model, 0)
				requireUsageReportSourcesEqual(t, fixture, coarse,
					reportSummarySourceFallback(scope, generation, coarse), reportUTCMonthSummarySource(scope, generation, coarse))
				report, err := fixture.repo.ListSessionUsage(t.Context(), coarse)
				require.NoError(t, err)
				require.Equal(t, models.Int64(30), report.Totals.TotalTokens, "each point date and the coarse interval contributes once")
			})

			t.Run("task and session filters include deleted subjects", func(t *testing.T) {
				for _, subject := range []usageBenchSubject{fixture.subjects[0], fixture.subjects[2]} {
					for _, filterIDs := range []struct {
						name  string
						apply func(*models.SessionUsageFilter)
					}{{"task", func(f *models.SessionUsageFilter) { f.TaskIDs = []string{subject.task} }}, {"session", func(f *models.SessionUsageFilter) { f.SessionIDs = []string{subject.session} }}} {
						t.Run(filterIDs.name+"/"+subject.task, func(t *testing.T) {
							filtered := filter
							filtered.Model = "deleted-subject-model"
							if subject.task == fixture.subjects[0].task {
								filtered.Model = "review-currency-model"
							}
							filterIDs.apply(&filtered)
							report, err := fixture.repo.ListSessionUsage(t.Context(), filtered)
							require.NoError(t, err)
							require.NotEmpty(t, report.Rows)
							wantTokens := int64(10)
							if subject.task == fixture.subjects[0].task {
								wantTokens = 30
							}
							require.Equal(t, models.Int64(wantTokens), report.Totals.TotalTokens)
						})
					}
				}
			})
		})
	}
}

// removeUsageReportCostMetricForPoint models independent token and cost metric
// applicability in the published contribution source. Its summaries are
// rebuilt before report parity is checked.
func removeUsageReportCostMetricForPoint(t *testing.T, fixture *usageBenchFixture, model string, at time.Time) {
	t.Helper()
	var projection struct {
		Scope      string `db:"scope_id"`
		Generation string `db:"generation"`
	}
	require.NoError(t, fixture.database.Get(&projection, fixture.database.Rebind(`SELECT scope_id,generation FROM usage_projection_contributions WHERE model=? AND metric_kind='tokens' LIMIT 1`), model))
	require.NotEmpty(t, projection.Generation)
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"usage_daily_summaries", "usage_lifetime_summaries"} {
		_, err = tx.ExecContext(t.Context(), fixture.database.Rebind(`INSERT INTO usage_projection_summary_queue(scope_id,generation,summary_key)
			SELECT scope_id,generation,summary_key FROM `+table+` WHERE scope_id=? AND generation=?
			ON CONFLICT(scope_id,generation,summary_key) DO NOTHING`), projection.Scope, projection.Generation)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(t.Context(), fixture.database.Rebind(`INSERT INTO usage_projection_summary_queue(scope_id,generation,summary_key)
		SELECT DISTINCT scope_id,generation,summary_key FROM usage_projection_contributions WHERE scope_id=? AND generation=?
		ON CONFLICT(scope_id,generation,summary_key) DO NOTHING`), projection.Scope, projection.Generation)
	require.NoError(t, err)
	result, err := tx.ExecContext(t.Context(), fixture.database.Rebind(`DELETE FROM usage_projection_contributions
		WHERE scope_id=? AND generation=? AND model=? AND provider='selected-provider' AND calendar='UTC'
		AND usage_date=? AND metric_kind='cost' AND currency='' AND cost_subcents IS NULL`),
		projection.Scope, projection.Generation, model, at.UTC().Format(time.DateOnly))
	require.NoError(t, err)
	deleted, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted, "only the token point's absent cost contribution is removed")
	require.NoError(t, fixture.repo.rebuildUsageProjectionSummariesTx(t.Context(), tx, projection.Scope, projection.Generation))
	require.NoError(t, tx.Commit())
}

func newUsageHistoryMonthlyReviewFixture(t *testing.T, engine string) (*usageBenchFixture, models.SessionUsageFilter) {
	t.Helper()
	fixture := newUsageBenchFixture(t, engine, usageBenchSize{})
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	start := time.Date(2025, 2, 1, 0, 0, 0, 0, location)
	end := time.Date(2025, 3, 1, 0, 0, 0, 0, location)
	filter := models.SessionUsageFilter{WorkspaceID: usageBenchWorkspace, Timezone: location.String(), GroupBy: "month", Start: &start, End: &end, Limit: 100}

	addAt := func(subject usageBenchSubject, id, model, provider string, at time.Time, currency string, tokens, cost bool) {
		t.Helper()
		measurement := usageBenchMeasurement(subject, id, at, "plugin:bench")
		measurement.Model, measurement.Provider, measurement.Currency = model, provider, currency
		if !cost {
			measurement.CostSubcents, measurement.CostCoverage, measurement.CostBasis = nil, models.UsageCoverageMissing, models.UsageCostBasisUnknown
		}
		if !tokens {
			measurement.InputTokens, measurement.OutputTokens, measurement.TotalTokens, measurement.SourceTotalTokens = nil, nil, nil, nil
		}
		configure := func(input *pluginsdk.UsageObservationV1) {
			if !cost {
				input.Cost = nil
			}
			if !tokens {
				input.Tokens = nil
			}
		}
		fixture.reportSpecial(t, subject, measurement, configure)
	}
	add := func(subject usageBenchSubject, id, model, provider string, day int, currency string, tokens, cost bool) {
		t.Helper()
		addAt(subject, id, model, provider, time.Date(2025, time.February, day, 12, 0, 0, 0, time.UTC), currency, tokens, cost)
	}
	add(fixture.subjects[0], "review-usd-day", "review-currency-model", "selected-provider", 17, "USD", true, true)
	add(fixture.subjects[0], "review-token-only-day", "review-currency-model", "selected-provider", 18, "", true, false)
	unknownAt := time.Date(2025, time.February, 19, 12, 0, 0, 0, time.UTC)
	unknownCurrency := usageBenchMeasurement(fixture.subjects[0], "review-unknown-currency-day", unknownAt, "plugin:bench")
	unknownCurrency.Model, unknownCurrency.Provider = "review-currency-model", "selected-provider"
	unknownCurrency.Currency, unknownCurrency.CostBasis = "", models.UsageCostBasisReported
	unknownCurrency.CoverageKey = "point:" + unknownAt.Format(time.RFC3339Nano)
	legacy := indexedObservation{Measurement: unknownCurrency, ItemID: unknownCurrency.SourceRecordID, WorkKey: fixture.subjects[0].work, WorkStatus: usageIdentityProven, CoverageKey: unknownCurrency.CoverageKey, Table: usageBucketsTable}
	setUsageObservationSegment(&legacy, nil)
	fixture.insertBatch(t, []usageBenchSeedRecord{{observation: legacy}})
	add(fixture.subjects[0], "review-unknown-cost-known-usd", "unknown-cost-model", "selected-provider", 20, "USD", true, true)
	unknownCostAt := time.Date(2025, time.February, 21, 12, 0, 0, 0, time.UTC)
	unknownCost := usageBenchMeasurement(fixture.subjects[0], "review-unknown-cost-day", unknownCostAt, "plugin:bench")
	unknownCost.Model, unknownCost.Provider, unknownCost.Currency = "unknown-cost-model", "selected-provider", ""
	unknownCost.InputTokens, unknownCost.OutputTokens, unknownCost.TotalTokens, unknownCost.SourceTotalTokens = nil, nil, nil, nil
	unknownCost.CoverageKey = "point:" + unknownCostAt.Format(time.RFC3339Nano)
	legacy = indexedObservation{Measurement: unknownCost, ItemID: unknownCost.SourceRecordID, WorkKey: fixture.subjects[0].work, WorkStatus: usageIdentityProven, CoverageKey: unknownCost.CoverageKey, Table: usageBucketsTable}
	setUsageObservationSegment(&legacy, nil)
	fixture.insertBatch(t, []usageBenchSeedRecord{{observation: legacy}})
	add(fixture.subjects[1], "review-other-provider", "review-currency-model", "other-provider", 13, "EUR", true, true)
	add(fixture.subjects[2], "review-deleted-subject", "deleted-subject-model", "selected-provider", 14, "USD", true, true)
	addAt(fixture.subjects[0], "review-shared-local-usd", "review-shared-local-day-model", "selected-provider", time.Date(2026, time.January, 3, 23, 0, 0, 0, time.UTC), "USD", true, true)
	addAt(fixture.subjects[0], "review-shared-local-token-only", "review-shared-local-day-model", "selected-provider", time.Date(2026, time.January, 4, 2, 0, 0, 0, time.UTC), "", true, false)
	addAt(fixture.subjects[0], "review-midnight-usd", "review-midnight-model", "selected-provider", time.Date(2026, time.January, 4, 4, 30, 0, 0, time.UTC), "USD", true, true)
	addAt(fixture.subjects[0], "review-midnight-token-only", "review-midnight-model", "selected-provider", time.Date(2026, time.January, 4, 5, 30, 0, 0, time.UTC), "", true, false)
	balancedTokenAt := time.Date(2026, time.January, 4, 1, 0, 0, 0, time.UTC)
	balancedToken := usageBenchMeasurement(fixture.subjects[0], "review-balanced-token-only", balancedTokenAt, "plugin:bench")
	balancedToken.Model, balancedToken.Provider = "review-balanced-metrics-model", "selected-provider"
	reportMonthlyReviewObservation(t, fixture, fixture.subjects[0], balancedToken, func(input *pluginsdk.UsageObservationV1) {
		input.Cost = nil
	})
	costOnlyAt := time.Date(2026, time.January, 4, 8, 0, 0, 0, time.UTC)
	costOnly := usageBenchMeasurement(fixture.subjects[0], "review-balanced-cost-only", costOnlyAt, "plugin:bench")
	costOnly.Model, costOnly.Provider, costOnly.Currency = "review-balanced-metrics-model", "selected-provider", "USD"
	costOnly.InputTokens, costOnly.OutputTokens, costOnly.CacheReadTokens, costOnly.CacheWriteTokens = nil, nil, nil, nil
	costOnly.ReasoningTokens, costOnly.TotalTokens, costOnly.SourceTotalTokens = nil, nil, nil
	reportMonthlyReviewObservation(t, fixture, fixture.subjects[0], costOnly, func(input *pluginsdk.UsageObservationV1) {
		input.Tokens = nil
		input.Cost.Amount = models.Int64(5)
	})
	addAt(fixture.subjects[0], "review-balanced-paired", "review-balanced-metrics-model", "selected-provider", time.Date(2026, time.January, 4, 9, 0, 0, 0, time.UTC), "USD", true, true)
	addAt(fixture.subjects[0], "review-edge-usd", "review-edge-currency-model", "selected-provider", time.Date(2026, time.January, 31, 20, 0, 0, 0, time.UTC), "USD", true, true)
	addAt(fixture.subjects[1], "review-edge-eur", "review-edge-currency-model", "selected-provider", time.Date(2026, time.February, 1, 2, 0, 0, 0, time.UTC), "EUR", true, true)
	addAt(fixture.subjects[0], "review-coarse-calendar-usd-point", "review-coarse-calendar-currency-model", "selected-provider", time.Date(2026, time.January, 15, 16, 0, 0, 0, time.UTC), "USD", true, true)
	coarseCalendarLocation, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	coarseCalendarStart := time.Date(2026, time.January, 15, 0, 0, 0, 0, coarseCalendarLocation).UTC()
	coarseCalendarEnd := time.Date(2026, time.January, 16, 0, 0, 0, 0, coarseCalendarLocation).UTC()
	coarseCalendarMeasurement := usageBenchMeasurement(fixture.subjects[1], "review-coarse-calendar-eur", coarseCalendarStart, "plugin:bench")
	coarseCalendarMeasurement.Model, coarseCalendarMeasurement.Provider, coarseCalendarMeasurement.Currency = "review-coarse-calendar-currency-model", "selected-provider", "EUR"
	coarseCalendarMeasurement.CoverageStart, coarseCalendarMeasurement.CoverageEnd = &coarseCalendarStart, &coarseCalendarEnd
	coarseCalendarMeasurement.SourceDate, coarseCalendarMeasurement.SourceTimezone = "2026-01-15", coarseCalendarLocation.String()
	coarseCalendarMeasurement.CoverageKey = "calendar:" + coarseCalendarMeasurement.SourceDate
	fixture.reportSpecial(t, fixture.subjects[1], coarseCalendarMeasurement, nil)
	add(fixture.subjects[0], "review-coarse-usd", "review-coarse-unsafe-model", "selected-provider", 10, "USD", true, true)
	add(fixture.subjects[0], "review-coarse-token-only", "review-coarse-unsafe-model", "selected-provider", 11, "", true, false)
	coarseStart := time.Date(2025, time.February, 12, 0, 0, 0, 0, time.UTC)
	coarseEnd := coarseStart.AddDate(0, 0, 1)
	coarseMeasurement := usageBenchMeasurement(fixture.subjects[1], "review-coarse-interval", coarseStart, "plugin:bench")
	coarseMeasurement.Model, coarseMeasurement.Provider = "review-coarse-unsafe-model", "selected-provider"
	coarseMeasurement.CoverageStart, coarseMeasurement.CoverageEnd = &coarseStart, &coarseEnd
	coarseMeasurement.SourceDate, coarseMeasurement.SourceTimezone = coarseStart.Format(time.DateOnly), "UTC"
	coarseMeasurement.CoverageKey = "calendar:" + coarseStart.Format(time.DateOnly)
	fixture.reportSpecial(t, fixture.subjects[1], coarseMeasurement, nil)
	fixture.publish(t)
	require.NoError(t, fixture.tasks.DeleteTask(t.Context(), fixture.subjects[2].task))
	return fixture, filter
}

// reportMonthlyReviewObservation applies metric-role edits before normalizing
// the SDK payload, then admits it through the public capture path.
func reportMonthlyReviewObservation(t *testing.T, fixture *usageBenchFixture, subject usageBenchSubject, measurement models.SessionUsageMeasurement, configure func(*pluginsdk.UsageObservationV1)) {
	t.Helper()
	target := fixture.collectionTarget(t, subject, measurement.Source)
	input := usageBenchObservation(measurement, target.BindingID)
	configure(&input)
	usageBenchSeal(t, &input)
	normalized, err := validateContractObservation(input)
	require.NoError(t, err)
	normalized.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-observation-v1:"+measurement.Source+":"+input.ItemID)).String()
	normalized.Source, normalized.WorkspaceID = measurement.Source, target.WorkspaceID
	normalized.ScopeID, normalized.SubjectID, normalized.UsageBindingID = target.ScopeID, target.SubjectID, target.BindingID
	normalized.TaskID, normalized.SessionID, normalized.TranscriptID = target.TaskID, target.SessionID, target.ExternalSessionID
	normalized.UsageIdentity, normalized.CollectedAt = "v1:"+input.ItemID, measurement.CollectedAt
	work, status := contractWorkIdentity(target, measurement.Source, input.WorkIdentity)
	encoded, err := encodeObservationMetadata(input)
	require.NoError(t, err)
	row := indexedObservation{Measurement: normalized, Table: usageTableFor(normalized), ItemID: input.ItemID,
		WorkKey: work, WorkStatus: status, CoverageKey: contractCoverageKey(normalized), Metadata: string(encoded)}
	setUsageObservationSegment(&row, &input)
	fixture.admitSeedBatch(t, []usageBenchSeedRecord{{observation: row, input: &input}})
	fixture.retained[subject.scope+":external"]++
}

func assertUsageHistoryMonthlyReviewDimension(t *testing.T, fixture *usageBenchFixture, filter models.SessionUsageFilter, scope, generation, model string, wantSummarize int) {
	t.Helper()
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	metadata, err := fixture.repo.usageProjectionMetadataTx(t.Context(), tx, scope)
	require.NoError(t, err)
	require.NoError(t, fixture.repo.usageReportGrid(t.Context(), tx, filter, metadata))
	require.NoError(t, fixture.repo.prepareUsageReportUTCMonthDimensions(t.Context(), tx, scope, generation, filter))
	var summarize int
	require.NoError(t, tx.GetContext(t.Context(), &summarize, fixture.database.Rebind(`SELECT summarize FROM usage_report_utc_month_dimensions WHERE model=? AND provider='selected-provider'`), model))
	require.Equal(t, wantSummarize, summarize, "the month summary guard must match its currency and coverage evidence")
}

func monthlyReviewFilter(base models.SessionUsageFilter, startDate, endDate string) models.SessionUsageFilter {
	location, err := time.LoadLocation(base.Timezone)
	if err != nil {
		panic(err)
	}
	start, err := time.ParseInLocation(time.DateOnly, startDate, location)
	if err != nil {
		panic(err)
	}
	end, err := time.ParseInLocation(time.DateOnly, endDate, location)
	if err != nil {
		panic(err)
	}
	filter := base
	filter.Start, filter.End = &start, &end
	return filter
}

func monthlyReviewRow(t *testing.T, rows []models.TokenUsageRow, model, currency string) models.TokenUsageRow {
	t.Helper()
	for _, row := range rows {
		if row.Model == model && row.Currency == currency {
			return row
		}
	}
	t.Fatalf("missing monthly row for model=%s currency=%q", model, currency)
	return models.TokenUsageRow{}
}
