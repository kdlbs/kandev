package sqlite

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryProviderOptionCompletionInvalidatesActiveReportSnapshot(t *testing.T) {
	for _, engine := range []struct {
		name     string
		postgres bool
	}{{name: "sqlite"}, {name: "postgres", postgres: true}} {
		t.Run(engine.name, func(t *testing.T) {
			if engine.postgres && os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
				t.Skip("actual PostgreSQL snapshot control requires its isolated test DSN")
			}
			fixture := newNativeTurnsFixture(t, engine.postgres, 0)
			first, err := fixture.tasks.GetTurn(t.Context(), "native-root-a")
			require.NoError(t, err)
			second, err := fixture.tasks.GetTurn(t.Context(), "native-root-b")
			require.NoError(t, err)
			turns := []*taskmodels.Turn{first, second}
			retainProviderSnapshotNativeEvent(t, fixture, turns, 0, "native-alpha", 3)
			retainProviderSnapshotNativeEvent(t, fixture, turns, 1, "native-zeta", 9)
			finishNativeTurnsBackfill(t, fixture.repo)
			completeFixtureProjection(t, fixture.repo)

			filter := models.SessionUsageFilter{
				WorkspaceID: nativeTurnsWorkspace, GroupBy: "model", SortBy: "reasoning", SortDirection: "desc", Limit: 1,
			}
			baseline, err := fixture.repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.Equal(t, "ready", baseline.ProjectionStatus)
			require.False(t, baseline.ProjectionPending)
			require.True(t, baseline.HasMore)
			require.Equal(t, "native-zeta", baseline.Rows[0].Model)
			require.EqualValues(t, 9, *baseline.Rows[0].ReasoningTokens)

			retainProviderSnapshotNativeEvent(t, fixture, turns, 2, "native-mid", 6)
			var generation string
			require.NoError(t, fixture.db.Get(&generation, fixture.db.Rebind(`SELECT active_generation FROM usage_projection_scopes WHERE scope_id=?`), fixture.scope))
			require.NotEmpty(t, generation)
			publishedWithPendingOptions := false
			for range 1000 {
				_, err = fixture.repo.AdvanceUsageHistoryProjection(t.Context(), fixture.scope, generation, 200)
				require.NoError(t, err)
				var sourceWorkPending, providerOptionsPending bool
				require.NoError(t, fixture.db.Get(&sourceWorkPending, fixture.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_work WHERE scope_id=? AND generation=?)`), fixture.scope, generation))
				require.NoError(t, fixture.db.Get(&providerOptionsPending, fixture.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_history WHERE scope_id=? AND generation=? AND (provider_options_ready=0 OR provider_options_version=0))`), fixture.scope, generation))
				if !sourceWorkPending && providerOptionsPending {
					publishedWithPendingOptions = true
					break
				}
			}
			require.True(t, publishedWithPendingOptions, "live work must publish before the auxiliary provider dimension finishes")

			pending, err := fixture.repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.Equal(t, "preparing", pending.ProjectionStatus)
			require.True(t, pending.ProjectionPending)
			require.True(t, pending.HasMore)
			require.Equal(t, "native-alpha", pending.Rows[0].Model, "masked reasoning leaves the pending page in fallback model order")
			require.Nil(t, pending.Rows[0].ReasoningTokens, "native reasoning safety masking remains active while publication is pending")
			pendingGeneration := pending.ProjectionGeneration
			require.NotEmpty(t, pendingGeneration)
			reasoningBeforeAuxiliaryCompletion := providerSnapshotNativeReasoning(t, fixture.repo)
			require.Equal(t, map[string]int64{"native-alpha": 3, "native-mid": 6, "native-zeta": 9}, reasoningBeforeAuxiliaryCompletion)

			for range 1000 {
				var providerOptionsPending bool
				require.NoError(t, fixture.db.Get(&providerOptionsPending, fixture.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_history WHERE scope_id=? AND generation=? AND (provider_options_ready=0 OR provider_options_version=0))`), fixture.scope, generation))
				if !providerOptionsPending {
					break
				}
				_, err = fixture.repo.AdvanceUsageHistoryProjection(t.Context(), fixture.scope, generation, 200)
				require.NoError(t, err)
			}
			var providerOptionsPending bool
			require.NoError(t, fixture.db.Get(&providerOptionsPending, fixture.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_history WHERE scope_id=? AND generation=? AND (provider_options_ready=0 OR provider_options_version=0))`), fixture.scope, generation))
			require.False(t, providerOptionsPending, "the test must finish only the auxiliary provider dimension")
			var sourceWorkPending bool
			require.NoError(t, fixture.db.Get(&sourceWorkPending, fixture.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_work WHERE scope_id=? AND generation=?)`), fixture.scope, generation))
			require.False(t, sourceWorkPending)

			continued := filter
			continued.Offset = 1
			continued.ProjectionGeneration = pendingGeneration
			_, err = fixture.repo.ListSessionUsage(t.Context(), continued)
			require.ErrorIs(t, err, analytics.ErrUsageProjectionChanged, "read-visible auxiliary readiness changes must invalidate the old page token")

			completed, err := fixture.repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.Equal(t, "ready", completed.ProjectionStatus)
			require.False(t, completed.ProjectionPending)
			require.NotEqual(t, pendingGeneration, completed.ProjectionGeneration)
			require.Equal(t, "native-zeta", completed.Rows[0].Model, "completed reasoning order must be reflected in a new snapshot")
			require.EqualValues(t, 9, *completed.Rows[0].ReasoningTokens)
			require.Equal(t, reasoningBeforeAuxiliaryCompletion, providerSnapshotNativeReasoning(t, fixture.repo), "auxiliary projection must preserve canonical saved reasoning")
		})
	}
}

func retainProviderSnapshotNativeEvent(t *testing.T, fixture nativeTurnsFixture, turns []*taskmodels.Turn, index int, model string, reasoning int64) {
	t.Helper()
	event := nativeTurnsEvent(index, turns)
	event.UsageEventID = "provider-snapshot-" + model
	event.ProviderResponseID = event.UsageEventID
	event.Model = model
	event.Provider = "snapshot-provider"
	event.TokensThought = models.Int64(0)
	event.ReasoningOutputTokens = models.Int64(reasoning)
	require.NoError(t, fixture.tasks.CreateTaskUsageEvent(t.Context(), event))
}

func providerSnapshotNativeReasoning(t *testing.T, repo *Repository) map[string]int64 {
	t.Helper()
	facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, Limit: 200})
	require.NoError(t, err)
	reasoning := make(map[string]int64)
	for _, fact := range facts {
		if !nativeCanonicalTokenSource(fact) || fact.Measurement.ReasoningTokens == nil {
			continue
		}
		reasoning[fact.Measurement.Model] = *fact.Measurement.ReasoningTokens
	}
	return reasoning
}
