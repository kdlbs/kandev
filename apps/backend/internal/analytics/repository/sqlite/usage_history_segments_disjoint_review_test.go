package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Disjoint intervals for one proven transcript remain additive across sources.
func TestUsageHistorySegmentsDisjointNativeAndCalendarReview(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			binding := segmentBinding(t, repo, "disjoint-calendar", "session-1")
			day := disjointReviewDay(t, binding, "calendar-day-one", "calendar-layout", "2026-01-01", 20)
			reportSegmentBatch(t, repo, "disjoint-calendar", day)
			disjointReviewNative(t, repo)
			disjointReviewProvenWork(t, repo, 2)
			selected := selectContractPartition(t, repo, day.ItemID)
			require.False(t, selected.Preparing)
			assert.LessOrEqual(t, selected.VisitedRows, 201)
			disjointReviewFacts(t, "bounded selector", selected.Facts)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			disjointReviewPublished(t, repo)
		})
	}
}

func TestUsageHistorySegmentsDisjointCollectorsReview(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			first, second := disjointReviewCollectors(t, repo)
			disjointReviewProvenWork(t, repo, 2)
			for _, item := range []pluginsdk.UsageObservationV1{first, second} {
				selected := selectContractPartition(t, repo, item.ItemID)
				require.False(t, selected.Preparing)
				assert.LessOrEqual(t, selected.VisitedRows, 201)
				disjointReviewFacts(t, "bounded selector "+item.SourceRecordID, selected.Facts)
			}
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			disjointReviewPublished(t, repo)
		})
	}
}

func TestUsageHistorySegmentsDisjointOversizedReview(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			first, second := disjointReviewCollectors(t, repo)
			ordinary := selectContractPartition(t, repo, first.ItemID)
			require.False(t, ordinary.Preparing)
			disjointReviewFacts(t, "ordinary selector before duplicate candidates", ordinary.Facts)
			for i, original := range []pluginsdk.UsageObservationV1{first, second} {
				var copies []pluginsdk.UsageObservationV1
				for n := range 110 {
					copy := original
					copy.ItemID = fmt.Sprintf("%s-repeated-%03d", original.ItemID, n)
					sealContractObservation(t, &copy)
					copies = append(copies, copy)
				}
				reportSegmentBatch(t, repo, []string{"disjoint-first", "disjoint-second"}[i], copies...)
			}
			disjointReviewProvenWork(t, repo, 222)
			bounded := selectContractPartition(t, repo, first.ItemID)
			require.True(t, bounded.Preparing, "the oversized case must exercise the durable resolution path")
			assert.LessOrEqual(t, bounded.VisitedRows, 201)
			assert.Empty(t, bounded.Facts, "an incomplete scan cannot publish partial accounting")
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			disjointReviewPublished(t, repo)
		})
	}
}

func disjointReviewCollectors(t *testing.T, repo *Repository) (pluginsdk.UsageObservationV1, pluginsdk.UsageObservationV1) {
	t.Helper()
	first := disjointReviewDay(t, segmentBinding(t, repo, "disjoint-first", "session-1"), "first-day", "first-layout", "2026-01-01", 20)
	second := disjointReviewDay(t, segmentBinding(t, repo, "disjoint-second", "session-1"), "second-day", "second-layout", "2026-01-02", 100)
	reportSegmentBatch(t, repo, "disjoint-first", first)
	reportSegmentBatch(t, repo, "disjoint-second", second)
	return first, second
}

func disjointReviewDay(t *testing.T, binding, record, decomposition, date string, amount int64) pluginsdk.UsageObservationV1 {
	t.Helper()
	item := multiModelAdapterObservation(t, binding, record, "Sonnet", "anthropic", amount, true)
	start, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	item.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: decomposition, SegmentID: record}
	item.Coverage = &pluginsdk.UsageCoverageV1{
		CoverageStart: stringPtr(start.Format(time.RFC3339)), CoverageEnd: stringPtr(start.AddDate(0, 0, 1).Format(time.RFC3339)),
		SourceDate: stringPtr(date), SourceTimezone: stringPtr("UTC"),
	}
	item.Cost.Basis = pluginsdk.UsageCostBasisReported
	sealContractObservation(t, &item)
	return item
}

func disjointReviewNative(t *testing.T, repo *Repository) {
	t.Helper()
	var taskIncarnation, sessionIncarnation string
	require.NoError(t, repo.db.QueryRowx(`SELECT task_incarnation_id,session_incarnation_id FROM usage_subjects WHERE task_id='task-1' AND session_id='session-1'`).Scan(&taskIncarnation, &sessionIncarnation))
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
		ID: "disjoint-native-execution", Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1",
		TaskIncarnationID: taskIncarnation, SessionID: "session-1", SessionIncarnationID: sessionIncarnation,
		ActorID: "native-actor", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: at,
	}))
	require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, usagehistory.NativeUsage{
		BindingID: "disjoint-native-execution", UsageEventID: "disjoint-native-event", TaskID: "task-1", SessionID: "session-1",
		ProviderThreadID: "segment-thread", Model: "Sonnet", Provider: "anthropic", InputTokens: 100,
		OutputTokens: models.Int64(0), TotalTokens: 100, ReportedTotalTokens: models.Int64(100), UsageCompleteness: "complete",
		CostSubcents: 100, CostPresent: true, CostSource: usageCostSourceReported, CostBasis: models.UsageCostBasisReported,
		OccurredAt: at, CollectedAt: at,
	}))
	require.NoError(t, tx.Commit())
}

func disjointReviewProvenWork(t *testing.T, repo *Repository, count int) {
	t.Helper()
	var refs []selectionReference
	require.NoError(t, repo.db.Select(&refs, `SELECT id,'session_usage_measurements' AS source_table FROM session_usage_measurements
		UNION ALL SELECT id,'session_usage_buckets' AS source_table FROM session_usage_buckets`))
	require.Len(t, refs, count)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var work string
	for _, ref := range refs {
		row, err := repo.loadIndexedObservationTx(t.Context(), tx, ref.Table, ref.ID)
		require.NoError(t, err)
		require.Equal(t, usageIdentityProven, row.WorkStatus, "the Host must establish work identity, not infer it from model labels")
		require.NotEmpty(t, row.WorkKey)
		if work == "" {
			work = row.WorkKey
		}
		require.Equal(t, work, row.WorkKey, "both dated records must describe the same proven transcript")
		require.NotNil(t, row.Measurement.CoverageStart)
		require.NotNil(t, row.Measurement.CoverageEnd)
		require.NotEqual(t, "lifetime", canonicalCoverageKind(row.Measurement))
	}
}

func disjointReviewFacts(t *testing.T, boundary string, facts []models.UsageCanonicalFact) {
	t.Helper()
	var rows []models.SessionUsageMeasurement
	for _, fact := range facts {
		rows = append(rows, fact.Measurement)
	}
	disjointReviewAmounts(t, boundary, rows)
}

func disjointReviewAmounts(t *testing.T, boundary string, rows []models.SessionUsageMeasurement) {
	t.Helper()
	tokens, cost := int64(0), int64(0)
	dailyTokens, dailyCost := map[string]int64{}, map[string]int64{}
	for _, row := range rows {
		if !assert.NotNil(t, row.CoverageStart, boundary) {
			continue
		}
		date := row.CoverageStart.UTC().Format(time.DateOnly)
		if row.TotalTokens != nil {
			tokens += *row.TotalTokens
			dailyTokens[date] += *row.TotalTokens
		}
		if row.CostSubcents != nil {
			cost += *row.CostSubcents
			dailyCost[date] += *row.CostSubcents
		}
	}
	assert.EqualValues(t, 120, tokens, boundary+": token total")
	assert.EqualValues(t, 120, cost, boundary+": known cost total")
	want := map[string]int64{"2026-01-01": 20, "2026-01-02": 100}
	assert.Equal(t, want, dailyTokens, boundary+": disjoint token dates")
	assert.Equal(t, want, dailyCost, boundary+": disjoint cost dates")
}

func disjointReviewPublished(t *testing.T, repo *Repository) {
	t.Helper()
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Timezone: "UTC"}
	report, err := repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, "ready", report.ProjectionStatus)
	require.False(t, report.ProjectionPending)
	assert.Equal(t, models.Int64(120), report.Totals.TotalTokens, "published SQL token total")
	assert.Equal(t, models.Int64(120), report.Totals.CostSubcents, "published SQL known cost total")
	dailyTokens, dailyCost := map[string]int64{}, map[string]int64{}
	for _, row := range report.Rows {
		if row.TotalTokens != nil {
			dailyTokens[row.Period] += *row.TotalTokens
		}
		if row.CostSubcents != nil {
			dailyCost[row.Period] += *row.CostSubcents
		}
	}
	want := map[string]int64{"2026-01-01": 20, "2026-01-02": 100}
	assert.Equal(t, want, dailyTokens, "published SQL token days")
	assert.Equal(t, want, dailyCost, "published SQL cost days")
	rows, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, report.ProjectionGeneration, state.Generation)
	disjointReviewAmounts(t, "saved canonical page", rows)
}
