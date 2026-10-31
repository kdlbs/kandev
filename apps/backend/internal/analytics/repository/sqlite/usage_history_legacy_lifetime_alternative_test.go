package sqlite

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/stretchr/testify/require"
)

// A lifetime snapshot and one native event are alternative coverage of the same
// proven transcript. An event does not establish complete lifetime coverage.
func TestUsageHistoryLegacyLifetimePartialNativeAlternative(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, copies := range []int{1, 222} {
			t.Run(fmt.Sprintf("postgres=%t/copies=%d", postgres, copies), func(t *testing.T) {
				repo := legacyLifetimePartialNativeFixture(t, postgres, copies)
				preparing := legacyLifetimeSelectionPreparing(t, repo)
				require.Equal(t, copies > 200, preparing)
				finishFixtureRetention(t, repo)
				completeFixtureProjection(t, repo)
				facts := logLegacyLifetimePublication(t, repo)

				filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"}
				report, err := repo.ListSessionUsage(t.Context(), filter)
				require.NoError(t, err)
				require.Equal(t, "ready", report.ProjectionStatus)
				require.False(t, report.ProjectionPending)
				require.NotNil(t, report.Totals.TotalTokens)
				require.EqualValues(t, 100, *report.Totals.TotalTokens, "one partial native event must not add to its fuller lifetime alternative")
				require.Equal(t, models.Int64(500), report.Totals.CostSubcents)
				require.Len(t, report.Rows, 1)
				require.Equal(t, "plugin:legacy-lifetime", report.Rows[0].Source)
				require.Equal(t, []string{"anthropic"}, report.Providers, "provider options must exclude dated providers suppressed by an authoritative lifetime alternative")
				require.Equal(t, models.UsageCostBasisEstimated, report.Rows[0].CostBasis)

				saved, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
				require.NoError(t, err)
				require.Equal(t, report.ProjectionGeneration, state.Generation)
				require.Len(t, saved, 1)
				require.Equal(t, report.Totals.TotalTokens, saved[0].TotalTokens)
				require.Equal(t, report.Totals.CostSubcents, saved[0].CostSubcents)
				require.False(t, saved[0].TokenMetricAbsent)
				require.False(t, saved[0].CostMetricAbsent)
				require.Len(t, facts, 2, "dated evidence is retained for actual dated queries")
				for _, fact := range facts {
					require.NotEmpty(t, fact.BundleKey, "normal and durable unsegmented selection must preserve source decomposition identity")
				}
				assertLegacyLifetimeDatedNative(t, repo)
			})
		}
	}
}

func legacyLifetimePartialNativeFixture(t *testing.T, postgres bool, copies int) *Repository {
	t.Helper()
	repo := segmentContractFixture(t, postgres)
	_ = segmentBinding(t, repo, "legacy-lifetime", "session-1")
	retainSegmentNative(t, repo, 10, func(native *usagehistory.NativeUsage) {
		native.CostPresent, native.CostBasis, native.CostSource, native.CostSubcents = true, models.UsageCostBasisReported, usageCostSourceReported, 50
		native.CacheReadTokens, native.CacheWriteTokens, native.ThoughtTokens = models.Int64(0), models.Int64(0), models.Int64(0)
	})
	var inputs []models.SessionUsageMeasurement
	for i := range copies {
		row := usageFixtureMeasurement(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
		row.Source, row.TranscriptID, row.Model, row.Provider = "plugin:legacy-lifetime", "segment-thread", "Sonnet", "anthropic"
		row.SourceRecordID, row.UsageIdentity, row.PayloadDigest = fmt.Sprintf("legacy-row-%03d", i), fmt.Sprintf("legacy-lifetime-%03d", i), fmt.Sprintf("legacy-payload-%03d", i)
		row.InputTokens, row.TotalTokens, row.CostSubcents = models.Int64(100), models.Int64(100), models.Int64(500)
		row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens, row.ReasoningTokens = models.Int64(0), models.Int64(0), models.Int64(0), models.Int64(0)
		row.CostCoverage = models.UsageCoverageComplete
		inputs = append(inputs, row)
	}
	for offset := 0; offset < len(inputs); offset += 200 {
		result, err := repo.UpsertSessionUsage(t.Context(), inputs[offset:min(offset+200, len(inputs))])
		require.NoError(t, err)
		for _, item := range result {
			require.Equal(t, models.UsageWriteApplied, item.Status)
		}
	}
	return repo
}

func logLegacyLifetimePublication(t *testing.T, repo *Repository) []models.UsageCanonicalFact {
	t.Helper()
	var indexed []struct {
		Source, Scope, Subject, Work, Bundle, Segment, Tokens, Cost, Status string
		Layout, Count                                                       int
	}
	require.NoError(t, repo.db.Select(&indexed, `SELECT source,scope_id AS scope,subject_id AS subject,selection_partition AS work,
 selection_bundle AS bundle,selection_segment AS segment,selection_token_bundle AS tokens,selection_cost_bundle AS cost,work_status AS status,
 selection_layout AS layout,COUNT(*) AS count FROM (SELECT source,scope_id,subject_id,selection_partition,selection_bundle,selection_segment,selection_token_bundle,selection_cost_bundle,work_status,selection_layout FROM session_usage_buckets UNION ALL SELECT source,scope_id,subject_id,selection_partition,selection_bundle,selection_segment,selection_token_bundle,selection_cost_bundle,work_status,selection_layout FROM session_usage_measurements) observations
 GROUP BY source,scope_id,subject_id,selection_partition,selection_bundle,selection_segment,selection_token_bundle,selection_cost_bundle,work_status,selection_layout ORDER BY source`))
	encoded, err := json.Marshal(indexed)
	require.NoError(t, err)
	t.Logf("indexed work/bundle/segment proof: %s", encoded)
	require.Len(t, indexed, 2)
	require.Equal(t, indexed[0].Scope, indexed[1].Scope)
	require.Equal(t, indexed[0].Subject, indexed[1].Subject)
	require.Equal(t, indexed[0].Work, indexed[1].Work)
	for _, row := range indexed {
		require.Equal(t, usageIdentityProven, row.Status)
	}
	var payloads []struct{ Payload, Bundle, Segment string }
	require.NoError(t, repo.db.Select(&payloads, `SELECT f.payload,f.bundle_key AS bundle,f.segment_key AS segment FROM usage_canonical_facts f
 JOIN usage_projection_scopes s ON s.scope_id=f.scope_id AND s.active_generation=f.generation JOIN usage_projection_applied a
 ON a.scope_id=f.scope_id AND a.generation=f.generation AND a.work_key=f.work_key AND a.build_id=f.build_id ORDER BY f.fact_key`))
	var facts []models.UsageCanonicalFact
	for _, payload := range payloads {
		var fact models.UsageCanonicalFact
		require.NoError(t, decodeUsageProjection(payload.Payload, &fact))
		require.Equal(t, payload.Bundle, fact.BundleKey)
		require.Equal(t, payload.Segment, fact.SegmentKey)
		t.Logf("published source=%s work=%s bundle=%q segment=%q tokens_absent=%t cost_absent=%t tokens=%v cost=%v", fact.Measurement.Source, fact.Partition.WorkKey, payload.Bundle, payload.Segment, fact.TokenMetricAbsent, fact.CostMetricAbsent, fact.Measurement.TotalTokens, fact.Measurement.CostSubcents)
		facts = append(facts, fact)
	}
	return facts
}

func assertLegacyLifetimeDatedNative(t *testing.T, repo *Repository) {
	t.Helper()
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Start: &start, End: &end}
	report, err := repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, models.Int64(10), report.Totals.TotalTokens)
	require.Equal(t, models.Int64(50), report.Totals.CostSubcents)
	require.Len(t, report.Rows, 1)
	require.Equal(t, usageSourceNative, report.Rows[0].Source)
	require.Nil(t, report.Rows[0].Turns)
	saved, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, report.ProjectionGeneration, state.Generation)
	require.Len(t, saved, 1)
	require.Equal(t, report.Totals.TotalTokens, saved[0].TotalTokens)
	require.Equal(t, report.Totals.CostSubcents, saved[0].CostSubcents)
}

// A complete native group can precede the oversized lifetime group. Continue
// through its real cursor rather than demanding Preparing on the first page.
func legacyLifetimeSelectionPreparing(t *testing.T, repo *Repository) bool {
	t.Helper()
	page := selectContractPartition(t, repo, "segment-native-event")
	require.LessOrEqual(t, page.VisitedRows, 201)
	if page.Preparing || !page.HasMore {
		return page.Preparing
	}
	require.NotEmpty(t, page.Facts)
	partition := page.Facts[0].Partition
	for page.HasMore && !page.Preparing {
		cursor := page.NextCursor
		require.NotEmpty(t, cursor)
		tx, err := repo.db.BeginTxx(t.Context(), nil)
		require.NoError(t, err)
		page, err = repo.SelectUsagePartitionTx(t.Context(), tx, models.UsageSelectionQuery{Partition: partition, Cursor: cursor, Limit: 200})
		require.NoError(t, tx.Rollback())
		require.NoError(t, err)
		require.LessOrEqual(t, page.VisitedRows, 201)
		if page.HasMore && !page.Preparing {
			require.NotEqual(t, cursor, page.NextCursor)
		}
	}
	return page.Preparing
}
