package sqlite

import (
	"context"
	"testing"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPreUpgradeNativeUsageAppearsAfterMigration(t *testing.T) {
	ctx := context.Background()
	repo := newUsageEventsTestRepo(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Usage history"}))

	createUsageEventsTestTask(t, repo, "task-old")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-old", TaskID: "task-old", State: models.TaskSessionStateWaitingForInput,
		Metadata: map[string]any{"acp": map[string]any{"session_id": "old-transcript"}},
	}))
	require.NoError(t, repo.CreateTaskUsageEvent(ctx, newTestUsageEvent("old-event", "task-old", "session-old")))

	createUsageEventsTestTask(t, repo, "task-forked")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-forked", TaskID: "task-forked", State: models.TaskSessionStateWaitingForInput,
		Metadata: map[string]any{
			"acp":                          map[string]any{"session_id": "forked-transcript"},
			"codex_fork_source_session_id": "foreign-session",
		},
	}))
	require.NoError(t, repo.CreateTaskUsageEvent(ctx, newTestUsageEvent("forked-event", "task-forked", "session-forked")))

	analyticsRepo, err := analyticssqlite.NewWithDB(repo.db, repo.db)
	require.NoError(t, err)
	repo.SetUsageHistoryRetainer(analyticsRepo)
	for pages := 0; pages < 20; pages++ {
		done, err := analyticsRepo.BackfillUsageHistory(ctx, 200)
		require.NoError(t, err)
		if done {
			break
		}
		if pages == 19 {
			t.Fatal("usage history migration did not finish")
		}
	}

	var nativeBuckets, nativeGaps int
	require.NoError(t, repo.db.Get(&nativeBuckets, `SELECT COUNT(*) FROM session_usage_buckets
		WHERE source = 'native' AND source_record_id = 'old-event'`))
	require.NoError(t, repo.db.Get(&nativeGaps, `SELECT COUNT(*) FROM usage_history_migration_gaps
		WHERE source_key = 'native' AND task_id = 'task-old'`))
	require.Equal(t, 1, nativeBuckets)
	require.Zero(t, nativeGaps)

	var forkedGaps int
	require.NoError(t, repo.db.Get(&forkedGaps, `SELECT COUNT(*) FROM usage_history_migration_gaps
		WHERE source_key = 'native' AND task_id = 'task-forked'`))
	require.GreaterOrEqual(t, forkedGaps, 1)
	require.NoError(t, repo.DeleteTask(ctx, "task-forked"))
	require.NoError(t, repo.db.Get(&forkedGaps, `SELECT COUNT(*) FROM usage_history_migration_gaps
		WHERE source_key = 'native' AND task_id = 'task-forked'`))
	require.GreaterOrEqual(t, forkedGaps, 1)

	require.NoError(t, repo.DeleteTask(ctx, "task-old"))
	require.NoError(t, repo.db.Get(&nativeBuckets, `SELECT COUNT(*) FROM session_usage_buckets
		WHERE source = 'native' AND source_record_id = 'old-event'`))
	require.Equal(t, 1, nativeBuckets)
}
