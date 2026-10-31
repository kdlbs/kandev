package sqlite

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-004.2 AC-COSTS-USAGE-HISTORY-004.6
func TestUsageHistorySelectionBoundedKeysetAndIndexedPartition(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	var items []pluginsdk.UsageObservationV1
	for i := range 205 {
		item := contractObservation(t, page.Targets[0].SubjectBinding, fmt.Sprintf("event-%03d", i), 1, "model", 1)
		at := time.Date(2020, 1, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339Nano)
		item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &at}
		sealContractObservation(t, &item)
		items = append(items, item)
	}
	for start := 0; start < len(items); start += 200 {
		end := start + 200
		if end > len(items) {
			end = len(items)
		}
		result, err := api.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: items[start:end]})
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
	}
	var partition models.UsagePartition
	require.NoError(t, repo.db.QueryRowx(`SELECT scope_id, subject_id, selection_partition FROM session_usage_buckets LIMIT 1`).Scan(&partition.ScopeID, &partition.SubjectID, &partition.WorkKey))
	seen := map[string]bool{}
	cursor := ""
	for {
		tx, err := repo.db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		selected, err := repo.SelectUsagePartitionTx(ctx, tx, models.UsageSelectionQuery{Partition: partition, Cursor: cursor, Limit: 17})
		require.NoError(t, err)
		require.NoError(t, tx.Rollback())
		require.False(t, selected.Preparing)
		require.LessOrEqual(t, selected.VisitedRows, 201)
		require.LessOrEqual(t, len(selected.Facts), 17)
		for _, fact := range selected.Facts {
			require.False(t, seen[fact.Key])
			seen[fact.Key] = true
		}
		if !selected.HasMore {
			break
		}
		require.NotEmpty(t, selected.NextCursor)
		require.NotEqual(t, cursor, selected.NextCursor)
		cursor = selected.NextCursor
	}
	require.Len(t, seen, 205)
	rows, err := repo.db.Queryx(`EXPLAIN QUERY PLAN SELECT id FROM session_usage_buckets WHERE scope_id = ? AND selection_partition = ? AND retired = 0 AND selection_coverage > ? ORDER BY selection_coverage, id LIMIT 201`, partition.ScopeID, partition.WorkKey, "")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plans = append(plans, detail)
	}
	require.NoError(t, rows.Err())
	require.Contains(t, strings.Join(plans, "\n"), "idx_session_usage_buckets_selection")
}

type historySelectionBackfiller interface {
	BackfillUsageSelection(context.Context, int) (bool, error)
}

func TestUsageHistoryCompleteSetReplacesBoundedPriorMembership(t *testing.T) {
	_, api := historyContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	setID := "bounded-set"
	revision := int64(1)
	var prior []pluginsdk.UsageObservationV1
	for i := range 200 {
		item := contractObservation(t, page.Targets[0].SubjectBinding, fmt.Sprintf("old-%03d", i), 1, "model", 1)
		item.SourceSetID = &setID
		item.SourceSetRevision = &revision
		sealContractObservation(t, &item)
		prior = append(prior, item)
	}
	_, err = api.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: prior})
	require.NoError(t, err)
	revision = 2
	next := contractObservation(t, page.Targets[0].SubjectBinding, "new-member", 1, "model", 2)
	next.SourceSetID = &setID
	next.SourceSetRevision = &revision
	sealContractObservation(t, &next)
	set := pluginsdk.UsageCompleteSetRevisionV1{SubjectBinding: next.SubjectBinding, SetID: setID, Revision: revision}
	set.PayloadDigest, err = set.ComputePayloadDigestV1([]string{next.ItemID})
	require.NoError(t, err)
	result, err := api.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{next}, CompleteSetRevision: &set})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Status)
}

func TestUsageHistorySelectionCollectorRevisionCannotChooseSourcePriority(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	_, err = api.ListUsageCollectionTargets(ctx, "collector-b", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, collector := range []string{"collector-a", "collector-b"} {
		item := contractObservation(t, page.Targets[0].SubjectBinding, "cost-row", int64(1+i*998), "model", 10)
		item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(int64(100 + i*100)), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
		item.Quality.CostCoverageComplete = true
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, api, collector, item).Status)
	}
	selected := selectContractPartition(t, repo, "cost-row")
	require.Len(t, selected.Facts, 1)
	require.EqualValues(t, 100, *selected.Facts[0].Measurement.CostSubcents)
	require.Equal(t, "plugin:collector-a", selected.Facts[0].CostSource.Source)
}

func TestUsageHistorySelectionLegacyIndexCatchupIsDurable(t *testing.T) {
	repo, _ := historyContractFixture(t)
	ctx := context.Background()
	item := usageFixtureMeasurement(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	other := item
	other.SourceRecordID = "second-legacy-record"
	result, err := repo.UpsertSessionUsage(ctx, []models.SessionUsageMeasurement{item, other})
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE session_usage_measurements SET selection_partition = '', selection_coverage = ''`)
	require.NoError(t, err)
	indexer, ok := any(repo).(historySelectionBackfiller)
	require.True(t, ok, "startup must resume indexing retained legacy rows")
	done, err := indexer.BackfillUsageSelection(ctx, 1)
	require.NoError(t, err)
	require.False(t, done)
	done, err = indexer.BackfillUsageSelection(ctx, 1)
	require.NoError(t, err)
	require.True(t, done)
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM session_usage_measurements WHERE selection_partition <> ''`))
	require.Equal(t, 2, count)
	bounder, ok := any(repo).(interface {
		UsageSourceUpperBoundsTx(context.Context, *sqlx.Tx, string) (map[string]string, error)
	})
	require.True(t, ok, "rebuilds need source key bounds inside the admission transaction")
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	bounds, err := bounder.UsageSourceUpperBoundsTx(ctx, tx, result[0].Measurement.ScopeID)
	require.NoError(t, err)
	require.Equal(t, max(result[0].Measurement.ID, result[1].Measurement.ID), bounds[usageMeasurementsTable])
}

func TestUsageHistoryContractRejectsContradictorySubsetSemantics(t *testing.T) {
	_, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "subset", 1, "model", 3)
	included := true
	item.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(1), CacheReadTokens: models.Int64(2), TotalTokens: models.Int64(3)}
	item.TokenSemantics = &pluginsdk.UsageTokenSemanticsV1{Version: "1", CacheReadIncludedInInput: &included}
	sealContractObservation(t, &item)
	require.Equal(t, "rejected", reportContractObservation(t, api, "collector-a", item).Status)
}

func TestUsageHistorySelectionCurrencyAlternativesRemainIncomplete(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	_, err = api.ListUsageCollectionTargets(ctx, "collector-b", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, currency := range []string{"USD", "EUR"} {
		item := contractObservation(t, page.Targets[0].SubjectBinding, "currency-item", 1, "model", 10)
		item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(100), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}
		item.Quality.CostCoverageComplete = true
		sealContractObservation(t, &item)
		collector := "collector-a"
		if i == 1 {
			collector = "collector-b"
		}
		require.Equal(t, "accepted", reportContractObservation(t, api, collector, item).Status)
	}
	selected := selectContractPartition(t, repo, "currency-item")
	require.Len(t, selected.Facts, 1)
	require.True(t, selected.Facts[0].Partial)
	require.EqualValues(t, 100, *selected.Facts[0].Measurement.CostSubcents)
}

func TestUsageHistorySelectionOversizedEquivalenceGroupReturnsPreparing(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	items := make([]pluginsdk.UsageObservationV1, 0, 201)
	for i := range 201 {
		items = append(items, contractObservation(t, page.Targets[0].SubjectBinding, fmt.Sprintf("alternative-%03d", i), 1, "model", 1))
	}
	for start := 0; start < len(items); start += 200 {
		end := start + 200
		if end > len(items) {
			end = len(items)
		}
		result, err := api.ReportUsageObservations(context.Background(), "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: items[start:end]})
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
	}
	selected := selectContractPartition(t, repo, items[0].ItemID)
	require.True(t, selected.Preparing)
	require.Empty(t, selected.Facts)
	rows, state, err := repo.ListCanonicalUsagePage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Equal(t, "preparing", state.Status)
	require.True(t, state.Pending)
}
