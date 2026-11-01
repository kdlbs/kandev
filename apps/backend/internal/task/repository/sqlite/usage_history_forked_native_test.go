package sqlite

import (
	"context"
	"testing"
	"time"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// A forked session cannot be adopted as an eligible retained source, so its
// native events are recorded as an explicit migration gap. Destruction stays
// fenced until a product decision defines how that gap may be discarded.
// @covers AC-COSTS-USAGE-HISTORY-004.2
func TestUsageHistoryForkedNativeUsageBlocksDeletion(t *testing.T) {
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
	err = repo.DeleteTask(ctx, "task-forked")
	require.ErrorContains(t, err, "native usage event")
	require.ErrorContains(t, err, "no accepted retained copy")
}
