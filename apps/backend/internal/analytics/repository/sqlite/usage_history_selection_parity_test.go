package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-004.2 AC-COSTS-USAGE-HISTORY-006.4
func TestUsageHistorySelectionParityNativeCoverageAfterRestart(t *testing.T) {
	for _, test := range []struct {
		name       string
		native     []int64
		external   int64
		wantRows   int
		partial    bool
		candidates int
	}{
		{"equivalent_native", []int64{40, 60}, 100, 2, false, 221},
		{"fuller_external", []int64{40, 60}, 150, 1, true, 221},
		{"partial_native_overlap", []int64{40}, 100, 1, true, 222},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, api := historyContractFixture(t)
			usageParityNative(t, repo, test.native)
			targets, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			item := contractObservation(t, targets.Targets[0].SubjectBinding, "coarse-000", 1, "model", test.external)
			item.ModelProvider = stringPtr("provider")
			item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"), SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC")}
			item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(500), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
			item.Quality.CostCoverageComplete = true
			sealContractObservation(t, &item)
			require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
			ordinary := selectContractPartition(t, repo, item.ItemID)
			require.False(t, ordinary.Preparing)
			require.Len(t, ordinary.Facts, test.wantRows)
			var total int64
			for _, fact := range ordinary.Facts {
				total += *fact.Measurement.TotalTokens
				require.Equal(t, test.partial, fact.Partial)
				if test.partial {
					require.Equal(t, "plugin:collector-a", fact.TokenSource.Source)
					require.EqualValues(t, 500, *fact.Measurement.CostSubcents)
				} else {
					require.Equal(t, "native", fact.TokenSource.Source)
					require.Nil(t, fact.Measurement.CostSubcents, "coarse cost cannot be allocated to native points")
				}
			}
			require.Equal(t, test.external, total)
			usageParityExpand(t, api, item, test.candidates)
			bounded := selectContractPartition(t, repo, item.ItemID)
			require.True(t, bounded.Preparing)
			require.LessOrEqual(t, bounded.VisitedRows, 201)
			actual := usageParityRebuild(t, repo)
			require.ElementsMatch(t, usageParityEvidence(ordinary.Facts), usageParityEvidence(actual))
		})
	}
}

func usageParityNative(t *testing.T, repo *Repository, amounts []int64) {
	t.Helper()
	ctx := context.Background()
	point := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, actor := range []string{"parity-actor-a", "parity-actor-b"} {
		require.NoError(t, repo.AdmitBindingTx(ctx, tx, usagehistory.Binding{ID: actor, Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1", TaskIncarnationID: "2026-01-01T00:00:00Z", SessionID: "session-1", SessionIncarnationID: "session-1", ActorID: actor, ActorStatus: "attributed", ActorKind: "user", ActorSource: "authenticated-execution", CapturedAt: point}))
	}
	for i, amount := range amounts {
		occurred := point.Add(time.Duration(i) * time.Minute)
		require.NoError(t, repo.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{BindingID: "parity-actor-a", UsageEventID: fmt.Sprintf("parity-native-%02d", i), TaskID: "task-1", SessionID: "session-1", Model: "model", Provider: "provider", InputTokens: amount, TotalTokens: amount, UsageCompleteness: "complete", OccurredAt: occurred, CollectedAt: occurred}))
	}
	require.NoError(t, tx.Commit())
}

func usageParityExpand(t *testing.T, api retainedUsageAPI, base pluginsdk.UsageObservationV1, count int) {
	t.Helper()
	for start := 1; start < count; start += 200 {
		batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: 1}
		for i := start; i < min(start+200, count); i++ {
			item := base
			item.ItemID = fmt.Sprintf("coarse-%03d", i)
			item.SourceRecordID = item.ItemID
			sealContractObservation(t, &item)
			batch.Observations = append(batch.Observations, item)
		}
		result, err := api.ReportUsageObservations(context.Background(), "collector-a", batch)
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
	}
}

func usageParityRebuild(t *testing.T, repo *Repository) []models.UsageCanonicalFact {
	t.Helper()
	// These selector fixtures contain retained rows, without a raw task ledger.
	// Derive their native source certificate through real bounded maintenance.
	for {
		done, err := repo.repairNativeUsageAccountingBatch(t.Context(), 200)
		require.NoError(t, err)
		if done {
			break
		}
	}
	ctx := context.Background()
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	generation, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	checkpointed := false
	for range 200 {
		_, err := repo.AdvanceUsageHistoryProjection(ctx, scope, generation, 17)
		require.NoError(t, err)
		var payloads []string
		require.NoError(t, repo.db.Select(&payloads, `SELECT payload FROM usage_projection_resolutions`))
		for _, payload := range payloads {
			var checkpoint usageProjectionResolution
			require.NoError(t, decodeUsageProjection(payload, &checkpoint))
			if (checkpoint.AfterID != "" && checkpoint.Phase == projectionMetricTokens) || (checkpoint.Phase == projectionResolutionNative && checkpoint.Native.AfterID != "") {
				checkpointed = true
			}
		}
		if checkpointed {
			break
		}
	}
	require.True(t, checkpointed, "the restart must interrupt a persisted candidate page")
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	drainProjection(t, restarted, scope, generation)
	var facts []models.UsageCanonicalFact
	offset := 0
	for range 10 {
		page, metadata, err := restarted.ListUsageCanonicalFacts(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", SessionIDs: []string{"session-1"}, Limit: 200, Offset: offset})
		require.NoError(t, err)
		require.Equal(t, "ready", metadata.Status)
		require.False(t, metadata.Pending)
		facts = append(facts, page...)
		if !metadata.HasMore {
			return facts
		}
		require.Greater(t, metadata.NextOffset, offset)
		offset = metadata.NextOffset
	}
	t.Fatal("canonical fact pages did not finish")
	return nil
}

type usageParityFact struct {
	Key, Kind, Model, Provider, Semantics, Currency string
	Tokens, Cost                                    *int64
	TokenSource, CostSource                         *models.UsageMetricSource
	Coverage, CostCoverage, Reason                  string
	Partial                                         bool
}

func usageParityEvidence(facts []models.UsageCanonicalFact) []usageParityFact {
	out := make([]usageParityFact, 0, len(facts))
	for _, fact := range facts {
		m := fact.Measurement
		out = append(out, usageParityFact{fact.Key, fact.CoverageKind, m.Model, m.Provider, fact.TokenSemantics, m.Currency, m.TotalTokens, m.CostSubcents, fact.TokenSource, fact.CostSource, m.Coverage, m.CostCoverage, fact.Reason, fact.Partial})
	}
	return out
}

// @covers AC-COSTS-USAGE-HISTORY-006.1 AC-COSTS-USAGE-HISTORY-004.2
func TestUsageHistorySelectionParityEquivalentPointMetricActors(t *testing.T) {
	repo, api := historyContractFixture(t)
	usageParityNative(t, repo, []int64{100})
	targets, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, targets.Targets[0].SubjectBinding, "coarse-000", 1, "model", 150)
	item.ModelProvider = stringPtr("provider")
	item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr("2026-01-02T12:00:00Z")}
	item.Quality.TokenCoverageComplete, item.Quality.CostCoverageComplete = false, true
	item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(500), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	ordinary := selectContractPartition(t, repo, item.ItemID)
	require.Len(t, ordinary.Facts, 1, "one equivalent point contributes once across native and collector coverage keys")
	fact := ordinary.Facts[0]
	require.EqualValues(t, 100, *fact.Measurement.TotalTokens)
	require.EqualValues(t, 500, *fact.Measurement.CostSubcents)
	require.Equal(t, "parity-actor-a", fact.TokenSource.ActorID)
	require.Equal(t, "mixed", fact.CostSource.ActorStatus)
	require.Empty(t, fact.CostSource.ActorID)
	usageParityExpand(t, api, item, 221)
	require.True(t, selectContractPartition(t, repo, item.ItemID).Preparing)
	actual := usageParityRebuild(t, repo)
	require.ElementsMatch(t, usageParityEvidence(ordinary.Facts), usageParityEvidence(actual))
}

func TestUsageHistorySelectionParityNativeAggregatePageRestart(t *testing.T) {
	repo, api := historyContractFixture(t)
	amounts := make([]int64, 221)
	for i := range amounts {
		amounts[i] = 1
	}
	usageParityNative(t, repo, amounts)
	targets, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, targets.Targets[0].SubjectBinding, "coarse-000", 1, "model", 221)
	item.ModelProvider = stringPtr("provider")
	item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"), SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC")}
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	var partition models.UsagePartition
	require.NoError(t, repo.db.QueryRowx(`SELECT scope_id, subject_id, selection_partition FROM session_usage_buckets LIMIT 1`).Scan(&partition.ScopeID, &partition.SubjectID, &partition.WorkKey))
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	page, err := repo.SelectUsagePartitionTx(context.Background(), tx, models.UsageSelectionQuery{Partition: partition, Limit: 1})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.True(t, page.Preparing, "a truncated native aggregate cannot establish complete-equivalent interval coverage")
	facts := usageParityRebuild(t, repo)
	require.Len(t, facts, 221)
	keys := make(map[string]bool)
	for _, fact := range facts {
		require.False(t, keys[fact.Key])
		keys[fact.Key] = true
		require.Equal(t, "native", fact.TokenSource.Source)
		require.EqualValues(t, 1, *fact.Measurement.TotalTokens)
		require.False(t, fact.Partial, "the complete paged aggregate is equivalent to the external interval")
	}
}
