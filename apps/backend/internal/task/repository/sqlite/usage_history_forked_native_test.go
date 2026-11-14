package sqlite

import (
	"context"
	"testing"
	"time"

	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// A forked session cannot be adopted, so deletion is allowed only after its
// native event has a durable migration gap.
// @covers AC-COSTS-USAGE-HISTORY-007.12
func TestUsageHistoryForkedNativeUsageDeletesWithGap(t *testing.T) {
	repo, analyticsRepo := newForkedNativeUsageDeletionFixture(t)
	ctx := context.Background()
	err := repo.DeleteTask(ctx, "task-forked")
	require.ErrorContains(t, err, "no accepted retained copy")
	var ledgerRows int
	require.NoError(t, repo.db.Get(&ledgerRows, `SELECT COUNT(*) FROM task_usage_events WHERE task_id = 'task-forked'`))
	require.Equal(t, 1, ledgerRows)

	for pages := 0; pages < 20; pages++ {
		done, err := analyticsRepo.BackfillUsageSourceAdoption(ctx, 200)
		require.NoError(t, err)
		if done {
			break
		}
		if pages == 19 {
			t.Fatal("usage source adoption did not finish")
		}
	}
	require.NoError(t, repo.DeleteTask(ctx, "task-forked"))

	var gapRows, retainedRows int
	require.NoError(t, repo.db.Get(&ledgerRows, `SELECT COUNT(*) FROM task_usage_events WHERE task_id = 'task-forked'`))
	require.NoError(t, repo.db.Get(&gapRows, `SELECT COUNT(*) FROM usage_history_migration_gaps
		WHERE source_key = 'native' AND workspace_id = 'ws-1' AND task_id = 'task-forked'`))
	require.NoError(t, repo.db.Get(&retainedRows, `SELECT COUNT(*) FROM session_usage_buckets
		WHERE source = 'native' AND source_record_id = 'forked-native-event'`))
	require.Zero(t, ledgerRows)
	require.Equal(t, 1, gapRows)
	require.Zero(t, retainedRows)
	report, err := analyticsRepo.ListSessionUsage(ctx, analyticsmodels.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.True(t, report.HistoryGap)
}

func TestUsageHistoryUnprovenNativeBlocksDeletionBeforeAdoption(t *testing.T) {
	repo, _ := newForkedNativeUsageDeletionFixture(t)
	err := repo.DeleteTask(context.Background(), "task-forked")
	require.ErrorContains(t, err, "native usage event")
	require.ErrorContains(t, err, "no accepted retained copy")
}

func newForkedNativeUsageDeletionFixture(t *testing.T) (*Repository, *analyticssqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	repo := newUsageEventsTestRepo(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Forked usage"}))
	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	require.NoError(t, err)
	repo.SetUsageHistoryRetainer(analyticsRepo)
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-forked", WorkspaceID: "ws-1", Title: "Forked", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-forked", TaskID: "task-forked", StartedAt: at,
		Metadata: map[string]any{"acp": map[string]any{"session_id": "forked-transcript"}, "codex_fork_source_session_id": "foreign-session"},
	}))
	require.NoError(t, repo.CreateTaskUsageEvent(ctx, newTestUsageEvent("forked-native-event", "task-forked", "session-forked")))
	return repo, analyticsRepo
}
