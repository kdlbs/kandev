package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySelectionParityConflictingDimensions(t *testing.T) {
	for _, scenario := range []string{"unknown_dimensions", "unknown_semantics", "distinct_currencies"} {
		t.Run(scenario, func(t *testing.T) {
			repo, api := historyContractFixture(t)
			a, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			b, err := api.ListUsageCollectionTargets(context.Background(), "collector-b", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			base := contractObservation(t, a.Targets[0].SubjectBinding, "coarse-000", 1, "model", 100)
			base.ModelProvider = stringPtr("provider")
			base.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(0), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
			base.Quality.CostCoverageComplete = true
			peer := contractObservation(t, b.Targets[0].SubjectBinding, "peer", 1, "model", 100)
			peer.ModelProvider = stringPtr("provider")
			peer.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(75), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
			peer.Quality.CostCoverageComplete = true
			switch scenario {
			case "unknown_dimensions":
				base.ModelID, base.ModelProvider = nil, nil
			case "unknown_semantics":
				inside := true
				peer.TokenSemantics = &pluginsdk.UsageTokenSemanticsV1{Version: "1", CacheReadIncludedInInput: &inside}
			case "distinct_currencies":
				peer.Cost.Currency = stringPtr("EUR")
			}
			sealContractObservation(t, &base)
			sealContractObservation(t, &peer)
			require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", base).Status)
			require.Equal(t, "accepted", reportContractObservation(t, api, "collector-b", peer).Status)
			ordinary := selectContractPartition(t, repo, base.ItemID)
			require.Len(t, ordinary.Facts, 1)
			fact := ordinary.Facts[0]
			require.EqualValues(t, 100, *fact.Measurement.TotalTokens)
			require.True(t, fact.Partial)
			require.NotNil(t, fact.Measurement.CostSubcents)
			require.Zero(t, *fact.Measurement.CostSubcents, "reported zero must remain measured zero")
			require.Equal(t, "USD", fact.Measurement.Currency)
			require.Equal(t, "null", fact.TokenSemantics)
			if scenario == "unknown_dimensions" {
				require.Empty(t, fact.Measurement.Model)
				require.Empty(t, fact.Measurement.Provider)
			}
			usageParityExpand(t, api, base, 221)
			require.True(t, selectContractPartition(t, repo, base.ItemID).Preparing)
			actual := usageParityRebuild(t, repo)
			require.ElementsMatch(t, usageParityEvidence(ordinary.Facts), usageParityEvidence(actual))
		})
	}
}

func TestUsageHistorySelectionParityLifetimeCorrectionsAndSupersession(t *testing.T) {
	small, smallAPI, ordinaryItem := usageParityLifetimeFixture(t, 2)
	large, largeAPI, durableItem := usageParityLifetimeFixture(t, 221)
	ordinary := selectContractPartition(t, small, ordinaryItem.ItemID)
	require.Len(t, ordinary.Facts, 2, "dated partial data and lifetime totals remain separate alternatives")
	first := usageParityRebuild(t, large)
	require.Len(t, first, 1)
	require.EqualValues(t, 100, *first[0].Measurement.TotalTokens, "a partial dated import cannot shrink lifetime usage")
	require.Equal(t, "lifetime", first[0].CoverageKind)
	usageParityDatedAlternative(t, large)

	for i, fixture := range []struct {
		repo *Repository
		api  retainedUsageAPI
		item pluginsdk.UsageObservationV1
	}{{small, smallAPI, ordinaryItem}, {large, largeAPI, durableItem}} {
		item := fixture.item
		item.Revision = 2
		item.ModelID, item.ModelProvider = stringPtr("corrected-model"), stringPtr("corrected-provider")
		item.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(40), TotalTokens: models.Int64(40)}
		item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(20), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, fixture.api, "collector-a", item).Status)
		if i == 1 {
			saved, state, err := fixture.repo.ListCanonicalUsagePage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			require.True(t, state.Pending)
			require.Len(t, saved, 1)
			require.EqualValues(t, 100, *saved[0].TotalTokens)
		}
	}
	corrected := usageParityLifetimeFacts(selectContractPartition(t, small, ordinaryItem.ItemID).Facts)
	require.Len(t, corrected, 1)
	require.EqualValues(t, 40, *corrected[0].Measurement.TotalTokens)
	require.Equal(t, "corrected-model", corrected[0].Measurement.Model)
	require.EqualValues(t, 20, *corrected[0].Measurement.CostSubcents)
	require.Equal(t, first[0].Key, corrected[0].Key, "mutable dimensions cannot change the canonical work key")
	actual := usageParityRebuild(t, large)
	require.ElementsMatch(t, usageParityEvidence(corrected), usageParityEvidence(actual))

	for _, fixture := range []struct {
		repo *Repository
		api  retainedUsageAPI
		item pluginsdk.UsageObservationV1
	}{{small, smallAPI, ordinaryItem}, {large, largeAPI, durableItem}} {
		replacement := contractObservation(t, fixture.item.SubjectBinding, "a-replacement", 1, "final-model", 30)
		replacement.ModelProvider = stringPtr("final-provider")
		replacement.SupersedesItemID = &fixture.item.ItemID
		replacement.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(10), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
		replacement.Quality.CostCoverageComplete = true
		sealContractObservation(t, &replacement)
		require.Equal(t, "accepted", reportContractObservation(t, fixture.api, "collector-a", replacement).Status)
		stale := fixture.item
		stale.Revision = 99
		sealContractObservation(t, &stale)
		require.Equal(t, "conflict", reportContractObservation(t, fixture.api, "collector-a", stale).Status, "superseded work cannot resurrect")
	}
	expected := usageParityLifetimeFacts(selectContractPartition(t, small, "a-replacement").Facts)
	require.Len(t, expected, 1)
	require.EqualValues(t, 30, *expected[0].Measurement.TotalTokens)
	require.Equal(t, "final-model", expected[0].Measurement.Model)
	require.EqualValues(t, 10, *expected[0].Measurement.CostSubcents)
	actual = usageParityRebuild(t, large)
	require.ElementsMatch(t, usageParityEvidence(expected), usageParityEvidence(actual))
	var remaining int
	require.NoError(t, large.db.Get(&remaining, `SELECT COUNT(*) FROM usage_observation_items WHERE retired = 0 AND item_id LIKE 'coarse-%'`))
	require.Equal(t, 220, remaining, "ordinary report absence does not retire old dimension alternatives")
	usageParityDatedAlternative(t, large)
}

func usageParityLifetimeFixture(t *testing.T, count int) (*Repository, retainedUsageAPI, pluginsdk.UsageObservationV1) {
	t.Helper()
	repo, api := historyContractFixture(t)
	targets, err := api.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	base := contractObservation(t, targets.Targets[0].SubjectBinding, "coarse-000", 1, "old-model", 100)
	base.ModelProvider = stringPtr("old-provider")
	base.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(50), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	base.Quality.CostCoverageComplete = true
	sealContractObservation(t, &base)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", base).Status)
	usageParityExpand(t, api, base, count)
	day := contractObservation(t, base.SubjectBinding, "partial-day", 1, "old-model", 20)
	day.ModelProvider = stringPtr("old-provider")
	day.Quality.TokenCoverageComplete = false
	day.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"), SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC")}
	sealContractObservation(t, &day)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", day).Status)
	return repo, api, base
}

func usageParityLifetimeFacts(facts []models.UsageCanonicalFact) []models.UsageCanonicalFact {
	var lifetime []models.UsageCanonicalFact
	for _, fact := range facts {
		if fact.CoverageKind == "lifetime" {
			lifetime = append(lifetime, fact)
		}
	}
	return lifetime
}

func usageParityDatedAlternative(t *testing.T, repo *Repository) {
	t.Helper()
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	facts, metadata, err := repo.ListUsageCanonicalFacts(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Start: &start, End: &end})
	require.NoError(t, err)
	require.False(t, metadata.Pending)
	require.Len(t, facts, 1)
	require.Equal(t, "calendar", facts[0].CoverageKind)
	require.EqualValues(t, 20, *facts[0].Measurement.TotalTokens)
	require.True(t, facts[0].Partial)
}
