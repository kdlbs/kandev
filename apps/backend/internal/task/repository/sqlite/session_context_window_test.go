package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func contextWindowSample(used int64, disabled bool) map[string]interface{} {
	sample := map[string]interface{}{
		"size":      int64(200000),
		"used":      used,
		"remaining": 200000 - used,
		"source":    "acp",
	}
	if disabled {
		sample["compaction_inference_disabled"] = true
	}
	return sample
}

// TestUpdateSessionContextWindowCompactionEligibility covers the SQLite
// eligibility matrix: excluded samples never increment, an eligible sample
// after an excluded baseline starts a new baseline, and ordinary eligible
// drops, duplicates, resets, and legacy unmarked samples keep their behavior.
func TestUpdateSessionContextWindowCompactionEligibility(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-compaction-eligibility"
		sessionID = "session-compaction-eligibility"
	)

	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, taskID, sessionID, "turn-compaction-eligibility")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "unrelated", "kept"))

	update := func(sample map[string]interface{}) int64 {
		t.Helper()
		count, err := repo.UpdateSessionContextWindow(ctx, sessionID, sample)
		require.NoError(t, err)
		return count
	}
	requireStoredCount := func(want int64) {
		t.Helper()
		session, err := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, err)
		require.EqualValues(t, want, session.Metadata[models.SessionMetaKeyContextCompactionCount])
	}

	require.EqualValues(t, 0, update(contextWindowSample(120000, false)), "first eligible sample")
	require.EqualValues(t, 0, update(contextWindowSample(80000, true)), "excluded drop")
	requireStoredCount(0)
	require.EqualValues(t, 0, update(contextWindowSample(60000, false)), "eligible after excluded baseline")
	require.EqualValues(t, 1, update(contextWindowSample(50000, false)), "ordinary eligible drop")
	require.EqualValues(t, 1, update(contextWindowSample(50000, false)), "duplicate eligible sample")
	require.EqualValues(t, 1, update(contextWindowSample(10000, true)), "excluded sample after count")
	require.EqualValues(t, 1, update(contextWindowSample(5000, true)), "excluded drop after excluded sample")
	requireStoredCount(1)
	require.EqualValues(t, 1, update(contextWindowSample(4000, false)), "eligible baseline after excluded")
	require.EqualValues(t, 2, update(contextWindowSample(3000, false)), "eligible drop after new baseline")
	require.EqualValues(t, 2, update(contextWindowSample(3000, false)), "equal eligible usage")
	require.EqualValues(t, 2, update(contextWindowSample(9000, false)), "increasing eligible usage")
	requireStoredCount(2)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "kept", session.Metadata["unrelated"])
	window, ok := session.Metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
	require.True(t, ok)
	require.EqualValues(t, 9000, window["used"])
}

func TestUpdateSessionContextWindowQueryGuardsEligibility(t *testing.T) {
	sqliteQuery := updateSessionContextWindowQuery(dialect.SQLite3)
	require.Contains(t, sqliteQuery, "compaction_inference_disabled")
	require.Contains(t, sqliteQuery, "WHEN ? = 0")
	require.NotContains(t, sqliteQuery, "jsonb_set")

	postgresQuery := updateSessionContextWindowQuery(dialect.PGX)
	require.Contains(t, postgresQuery, "compaction_inference_disabled")
	require.Contains(t, postgresQuery, "WHEN NOT ?::boolean")
	require.Contains(t, postgresQuery, "jsonb_set")
	require.NotContains(t, postgresQuery, "json_extract")
}

func TestUpdateSessionContextWindowCompactionEligibilitySurvivesReopen(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-compaction-reopen"
		sessionID = "session-compaction-reopen"
	)
	dbPath := filepath.Join(t.TempDir(), "context-eligibility.db")

	openRepo := func(t *testing.T) *Repository {
		t.Helper()
		dbConn, err := db.OpenSQLite(dbPath)
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbConn.Close() })
		sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
		repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
		require.NoError(t, err)
		return repo
	}

	repo := openRepo(t)
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), "ws-compaction-reopen", "Workspace", now, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), "wf-compaction-reopen", "ws-compaction-reopen", "Workflow", now, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, workflow_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), taskID, "ws-compaction-reopen", "wf-compaction-reopen", "Task", now, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at) VALUES (?, ?, ?, ?)
	`), sessionID, taskID, now, now)
	require.NoError(t, err)
	count, err := repo.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(90000, true))
	require.NoError(t, err)
	require.EqualValues(t, 0, count)
	require.NoError(t, repo.db.Close())

	reopened := openRepo(t)
	t.Cleanup(func() { _ = reopened.db.Close() })
	count, err = reopened.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(50000, false))
	require.NoError(t, err)
	require.EqualValues(t, 0, count, "reopened excluded baseline must not count")
	count, err = reopened.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(40000, false))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestUpdateSessionContextWindowCompactionEligibilityAfterReset(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-compaction-reset"
		sessionID = "session-compaction-reset"
	)
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, taskID, sessionID, "turn-compaction-reset")

	count, err := repo.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(120000, false))
	require.NoError(t, err)
	require.EqualValues(t, 0, count)
	count, err = repo.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(80000, false))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyContextWindow, nil))

	count, err = repo.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(50000, false))
	require.NoError(t, err)
	require.EqualValues(t, 1, count, "reset must clear the baseline but preserve the lifetime count")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.EqualValues(t, 1, session.Metadata[models.SessionMetaKeyContextCompactionCount])
}

func TestUpdateSessionContextWindowMissingMarkerKeepsLegacyInference(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-compaction-legacy"
		sessionID = "session-compaction-legacy"
	)
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, taskID, sessionID, "turn-compaction-legacy")

	sample := func(used int64) map[string]interface{} {
		return map[string]interface{}{"size": int64(200000), "used": used, "remaining": 200000 - used}
	}
	count, err := repo.UpdateSessionContextWindow(ctx, sessionID, sample(120000))
	require.NoError(t, err)
	require.EqualValues(t, 0, count)
	count, err = repo.UpdateSessionContextWindow(ctx, sessionID, sample(80000))
	require.NoError(t, err)
	require.EqualValues(t, 1, count, "a missing marker must retain legacy eligibility")
}
