package sqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func currentTargetDatabaseFixture(t *testing.T, postgres bool) (*Repository, *tasksqlite.Repository) {
	t.Helper()
	var database *sqlx.DB
	if postgres {
		dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
		require.NotEmpty(t, dsn, "real PostgreSQL verification requires its isolated test DSN")
		database = testutil.OpenIsolatedPostgres(t, dsn)
		var version string
		require.NoError(t, database.Get(&version, `SHOW server_version`))
		t.Logf("PostgreSQL %s", version)
		_, err := database.Exec(`SET TIME ZONE 'Asia/Kathmandu'`)
		require.NoError(t, err)
	} else {
		raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "point.db"))
		require.NoError(t, err)
		database = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
	}
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	repo, err := NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "ws-1", Name: "Point", OrgID: "org-a"}))
	createCurrentTargetSession(t, tasks, "task-1", "session-1", "transcript-1")
	return repo, tasks
}

func createCurrentTargetSession(t *testing.T, tasks *tasksqlite.Repository, task, session, transcript string) *taskmodels.TaskSession {
	t.Helper()
	return createCurrentTargetSessionInWorkspace(t, tasks, "ws-1", task, session, transcript)
}

func createCurrentTargetSessionInWorkspace(t *testing.T, tasks *tasksqlite.Repository, workspace, task, session, transcript string) *taskmodels.TaskSession {
	t.Helper()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: task, WorkspaceID: workspace, Title: task, CreatedAt: at, UpdatedAt: at}))
	value := &taskmodels.TaskSession{ID: session, TaskID: task, StartedAt: at, Metadata: map[string]any{"acp": map[string]any{"session_id": transcript}}}
	require.NoError(t, tasks.CreateTaskSession(t.Context(), value))
	return value
}

func finishCurrentFixtureDeliveryRetirement(t *testing.T, tasks *tasksqlite.Repository) {
	t.Helper()
	for range 8 {
		result, err := tasks.RunAgentDeliveryRetirementBatch(t.Context(), taskmodels.AgentDeliveryRetirementBatchOptions{MaxCandidates: 200})
		require.NoError(t, err)
		if !result.Pending {
			return
		}
	}
	t.Fatal("fixture delivery retirement did not finish before session ID reuse")
}
