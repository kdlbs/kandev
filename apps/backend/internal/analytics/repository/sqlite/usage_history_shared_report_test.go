package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySharedWorkReportingKeepsUnallocatedWorkspaceAmount(t *testing.T) {
	repo, api := historyRetainedContractFixture(t)
	tasks, err := tasksqlite.NewWithDB(repo.db, repo.db, nil)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	require.NoError(t, tasks.SetSessionMetadataKey(t.Context(), "session-1", "acp", map[string]string{"session_id": "shared-transcript"}))
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "task-2", WorkspaceID: "ws-1", Title: "Shared", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "session-2", TaskID: "task-2", StartedAt: at, Metadata: map[string]any{"acp": map[string]any{"session_id": "shared-transcript"}}}))
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	_, err = api.ListUsageCollectionTargets(t.Context(), "collector-b", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, target := range page.Targets {
		collector := "collector-a"
		if i == 1 {
			collector = "collector-b"
		}
		item := contractObservation(t, target.SubjectBinding, collector+"-lifetime", 1, "model", 100)
		require.Equal(t, "accepted", reportContractObservation(t, api, collector, item).Status)
		day := contractObservation(t, target.SubjectBinding, collector+"-day", 1, "model", 100)
		day.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"), SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC")}
		sealContractObservation(t, &day)
		require.Equal(t, "accepted", reportContractObservation(t, api, collector, day).Status)
	}
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	for _, group := range []string{"model", "task", "session"} {
		report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: group})
		require.NoError(t, err)
		require.NotNil(t, report.Totals.TotalTokens)
		require.EqualValues(t, 100, *report.Totals.TotalTokens, "workspace totals survive ambiguous subject attribution")
		require.Len(t, report.Rows, 1)
		require.Empty(t, report.Rows[0].TaskID)
		require.Empty(t, report.Rows[0].SessionID)
		require.False(t, report.Rows[0].TaskAvailable)
		require.False(t, report.Rows[0].SessionAvailable)
	}
	correction := contractObservation(t, page.Targets[0].SubjectBinding, "collector-a-lifetime", 2, "corrected-model", 70)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", correction).Status)
	completeFixtureProjection(t, repo)
	report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	require.NoError(t, err)
	require.EqualValues(t, 70, *report.Totals.TotalTokens, "a dated alternative cannot reappear under the previous model label")
}
