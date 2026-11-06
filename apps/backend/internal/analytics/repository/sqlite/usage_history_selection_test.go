package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type historyPartitionSelector interface {
	SelectUsagePartitionTx(context.Context, *sqlx.Tx, models.UsageSelectionQuery) (models.UsageSelectionPage, error)
}

func selectContractPartition(t *testing.T, repo *Repository, itemID string) models.UsageSelectionPage {
	t.Helper()
	selector, ok := any(repo).(historyPartitionSelector)
	require.True(t, ok, "bounded selector must be available to projection transactions")
	var partition models.UsagePartition
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`SELECT scope_id, subject_id, selection_partition FROM session_usage_measurements WHERE item_id = ?
		UNION ALL SELECT scope_id, subject_id, selection_partition FROM session_usage_buckets WHERE item_id = ? LIMIT 1`), itemID, itemID).Scan(&partition.ScopeID, &partition.SubjectID, &partition.WorkKey))
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	page, err := selector.SelectUsagePartitionTx(context.Background(), tx, models.UsageSelectionQuery{Partition: partition, Limit: 200})
	require.NoError(t, err)
	return page
}

// @covers AC-COSTS-USAGE-HISTORY-004.2 AC-COSTS-USAGE-HISTORY-003.2
func TestUsageHistorySelectionWorkEquivalencePrecedesDimensions(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	_, err = api.ListUsageCollectionTargets(context.Background(), "replacement-collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	a := contractObservation(t, page.Targets[0].SubjectBinding, "unknown-dimensions", 1, "", 100)
	a.ModelID = nil
	sealContractObservation(t, &a)
	b := contractObservation(t, a.SubjectBinding, "known-dimensions", 1, "exact-model", 100)
	b.ModelProvider = stringPtr("provider")
	sealContractObservation(t, &b)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", a).Status)
	require.Equal(t, "accepted", reportContractObservation(t, api, "replacement-collector", b).Status)
	selected := selectContractPartition(t, repo, a.ItemID)
	require.Len(t, selected.Facts, 1, "unknown aggregates and known details cannot add the same proven work twice")
	require.EqualValues(t, 100, *selected.Facts[0].Measurement.TotalTokens)
	require.True(t, selected.Facts[0].Partial)
	key := selected.Facts[0].Key
	a.ModelID, a.ModelProvider, a.Revision = stringPtr("corrected-model"), stringPtr("corrected-provider"), 2
	sealContractObservation(t, &a)
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	_, changes, err := repo.MutateUsageObservationsTx(context.Background(), tx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{a}})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.NotNil(t, changes[0].OldPartition)
	require.NotNil(t, changes[0].NewPartition)
	require.NotEqual(t, changes[0].OldPartition.PartitionID, changes[0].NewPartition.PartitionID)
	require.NoError(t, tx.Commit())
	require.Equal(t, key, selectContractPartition(t, repo, a.ItemID).Facts[0].Key)
}

// @covers AC-COSTS-USAGE-HISTORY-003.5 AC-COSTS-USAGE-HISTORY-004.2
func TestUsageHistorySelectionPartialDatedImportPreservesLifetime(t *testing.T) {
	repo, api := historyRetainedContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	lifetime := contractObservation(t, page.Targets[0].SubjectBinding, "lifetime", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", lifetime).Status)
	day := contractObservation(t, lifetime.SubjectBinding, "day", 1, "model", 20)
	day.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"), SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC")}
	day.Quality.TokenCoverageComplete = false
	sealContractObservation(t, &day)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", day).Status)
	selected := selectContractPartition(t, repo, lifetime.ItemID)
	require.Len(t, selected.Facts, 2, "selector keeps dated and lifetime alternatives separate")
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	all, err := repo.ListCanonicalSessionUsageMeasurements(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.EqualValues(t, 100, *all[0].TotalTokens)
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	dated, err := repo.ListCanonicalSessionUsageMeasurements(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", Start: &start, End: &end})
	require.NoError(t, err)
	require.Len(t, dated, 1)
	require.EqualValues(t, 20, *dated[0].TotalTokens)
}

func TestUsageHistorySelectionAmbiguousIntervalsHaveNoInventedRemainder(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for _, input := range []struct {
		id, start, end string
		amount         int64
	}{
		{"overlap-a", "2026-01-01T00:00:00Z", "2026-01-03T00:00:00Z", 100},
		{"overlap-b", "2026-01-02T00:00:00Z", "2026-01-04T00:00:00Z", 150},
		{"disjoint-c", "2026-01-04T00:00:00Z", "2026-01-05T00:00:00Z", 20},
	} {
		item := contractObservation(t, page.Targets[0].SubjectBinding, input.id, 1, "model", input.amount)
		item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: &input.start, CoverageEnd: &input.end}
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	}
	selected := selectContractPartition(t, repo, "overlap-a")
	require.Len(t, selected.Facts, 2)
	var total int64
	var partial int
	for _, fact := range selected.Facts {
		total += *fact.Measurement.TotalTokens
		if fact.Partial {
			partial++
		}
	}
	require.EqualValues(t, 120, total)
	require.Equal(t, 1, partial)
}
