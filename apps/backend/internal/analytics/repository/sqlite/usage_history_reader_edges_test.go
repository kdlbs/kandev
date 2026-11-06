package sqlite

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySourceOwnedReadPreservesAcceptedObservations(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	point := "2026-03-29T01:00:00Z"
	for _, id := range []string{"first", "second"} {
		item := contractObservation(t, page.Targets[0].SubjectBinding, id, 1, "model", 10)
		item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	}
	start := time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", Source: "plugin:collector", Start: &start, End: &end, Limit: 1}
	first, err := repo.ListSessionUsageMeasurements(context.Background(), filter)
	require.NoError(t, err)
	require.Len(t, first, 1)
	filter.Offset = 1
	second, err := repo.ListSessionUsageMeasurements(context.Background(), filter)
	require.NoError(t, err)
	require.Len(t, second, 1, "source-owned reads must not reconcile two accepted source items into one canonical value")
	require.NotEqual(t, first[0].ID, second[0].ID)
	filter.Source = "plugin:foreign"
	filter.Offset = 0
	hidden, err := repo.ListSessionUsageMeasurements(context.Background(), filter)
	require.NoError(t, err)
	require.Empty(t, hidden)
}

func TestUsageHistorySourceOwnedReadFencesRetainedScope(t *testing.T) {
	for _, engine := range []struct {
		name     string
		postgres bool
	}{{name: "sqlite"}, {name: "postgres", postgres: true}} {
		t.Run(engine.name, func(t *testing.T) {
			if engine.postgres {
				testutil.PostgresDSNFromEnv(t)
			}
			repo, _ := currentTargetDatabaseFixture(t, engine.postgres)
			page, err := repo.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			item := contractObservation(t, page.Targets[0].SubjectBinding, "retained", 1, "model", 10)
			require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
			_, err = repo.db.Exec(`UPDATE workspaces SET org_id='new-org' WHERE id='ws-1'`)
			require.NoError(t, err)
			ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "new-user", OrgID: "new-org", Role: authn.RoleMember})
			rows, err := repo.ListSessionUsageMeasurements(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", Source: "plugin:collector"})
			require.NoError(t, err)
			require.Empty(t, rows, "current ownership does not grant the former organization's retained observations")
			denied := usagehistory.WithWorkspaceAccess(ctx, []string{})
			_, err = repo.ListSessionUsageMeasurements(denied, models.SessionUsageFilter{WorkspaceID: "ws-1", Source: "plugin:collector"})
			require.ErrorIs(t, err, ErrUsageNotVisible)
		})
	}
}

func TestUsageHistoryReportExplicitUndatedRangeDoesNotInventDates(t *testing.T) {
	repo, driver, scope := projectionFixture(t)
	generation, err := driver.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	completeReportMigration(t, repo)
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", Start: &start, End: &end, GroupBy: "model", IncludeUndated: true}
	report, err := repo.ListSessionUsage(context.Background(), filter)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
	require.True(t, report.Rows[0].Undated)
	require.Empty(t, report.Rows[0].Period)
	require.True(t, report.Totals.Partial)
	require.False(t, report.HasRangeData)
	filter.GroupBy = "day"
	trend, err := repo.ListSessionUsage(context.Background(), filter)
	require.NoError(t, err)
	require.Empty(t, trend.Rows, "an undated lifetime value cannot appear on a chart axis")
}

func TestUsageHistoryReportOverflowHasExplicitOutcome(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, amount := range []int64{math.MaxInt64, 1} {
		point := time.Date(2026, 3, 1+i, 1, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
		item := contractObservation(t, page.Targets[0].SubjectBinding, point, 1, "model", amount)
		item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	}
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	require.ErrorIs(t, err, analytics.ErrUsageAmountOverflow)
	require.Nil(t, report, "overflow cannot become a rounded total or a zero")
}

func TestUsageHistoryReportMixedCurrencyDoesNotOverflowAnInapplicableSum(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, currency := range []string{"USD", "EUR"} {
		point := time.Date(2026, 3, 1+i, 1, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
		item := contractObservation(t, page.Targets[0].SubjectBinding, point, 1, "model", 1)
		item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
		item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(math.MaxInt64), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}
		item.Quality.CostCoverageComplete = true
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	}
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	require.NoError(t, err)
	require.Len(t, report.Rows, 2)
	require.Nil(t, report.Totals.CostSubcents)
	require.EqualValues(t, 2, *report.Totals.TotalTokens)
}

func TestUsageHistorySavedReaderWaitsForRetentionMigration(t *testing.T) {
	repo, driver, scope := projectionFixture(t)
	generation, err := driver.StartUsageHistoryRebuild(t.Context(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1"}
	rows, err := repo.ListCanonicalSessionUsageMeasurements(t.Context(), filter)
	require.NoError(t, err)
	require.Empty(t, rows, "saved canonical values cannot cut over before all source migration watermarks complete")
	completeReportMigration(t, repo)
	rows, err = repo.ListCanonicalSessionUsageMeasurements(t.Context(), filter)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 100, *rows[0].TotalTokens)
}

func TestUsageHistoryCanonicalPageRejectsChangedPublication(t *testing.T) {
	repo, driver, scope := projectionFixture(t)
	generation, err := driver.StartUsageHistoryRebuild(t.Context(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1"}
	_, before, err := repo.ListCanonicalUsagePage(t.Context(), filter)
	require.NoError(t, err)
	filter.ProjectionGeneration = before.Generation + ".changed"
	_, _, err = repo.ListCanonicalUsagePage(t.Context(), filter)
	require.ErrorIs(t, err, analytics.ErrUsageProjectionChanged, "a continued saved read must reject another complete publication")
}

func TestUsageHistoryReportAllTimeGridIncludesLastPoint(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	point := "2026-03-29T00:30:00.123456789Z"
	item := contractObservation(t, page.Targets[0].SubjectBinding, "only-point", 1, "model", 7)
	item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Timezone: "Asia/Kathmandu"})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1, "all-time charts must include their last observation even when it is the only point")
	require.Equal(t, "2026-03-29", report.Rows[0].Period)
	require.EqualValues(t, 7, *report.Rows[0].TotalTokens)
	require.EqualValues(t, 7, *report.Totals.TotalTokens)
}
