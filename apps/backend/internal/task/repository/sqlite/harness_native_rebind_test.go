package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

type nativeRebindFixture struct {
	session *models.TaskSession
	request models.HarnessNativeRebind
}

func newNativeRebindFixture(t *testing.T, repo *Repository, suffix string) *nativeRebindFixture {
	t.Helper()
	ctx := context.Background()
	workspace := &models.Workspace{ID: "workspace-rebind-" + suffix, Name: "Rebind"}
	task := &models.Task{ID: "task-rebind-" + suffix, Title: "Rebind", WorkspaceID: workspace.ID}
	session := &models.TaskSession{ID: "session-rebind-" + suffix, TaskID: task.ID, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateWorkspace(ctx, workspace))
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	session, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: task.ID, AgentExecutionID: "execution-" + suffix, Status: "ready",
	}))
	require.NoError(t, repo.UpdateResumeToken(ctx, session.ID, "execution-"+suffix, "native-old-"+suffix, ""))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "native-old-" + suffix, CreationReason: "native_started",
	}))
	return &nativeRebindFixture{session: session, request: models.HarnessNativeRebind{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1, TaskID: task.ID, ExecutionID: "execution-" + suffix,
		PreviousNativeSessionID: "native-old-" + suffix, NativeSessionID: "native-new-" + suffix,
	}}
}

func requireNativeIdentity(t *testing.T, repo *Repository, fixture *nativeRebindFixture, native string, history int) {
	t.Helper()
	ctx := context.Background()
	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.request.IncarnationID)
	require.NoError(t, err)
	require.Equal(t, native, current.NativeSessionID)
	running, err := repo.GetExecutorRunningBySessionID(ctx, fixture.session.ID)
	require.NoError(t, err)
	require.Equal(t, native, running.ResumeToken)
	var count int
	require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(
		`SELECT COUNT(*) FROM harness_native_session_rebinds WHERE session_id = ?`), fixture.session.ID).Scan(&count))
	require.Equal(t, history, count)
}

func TestRebindHarnessGenerationNativeSession(t *testing.T) {
	testRebindHarnessGenerationNativeSession(t, newRepoForSessionTests)
}

func TestPostgresRebindHarnessGenerationNativeSession(t *testing.T) {
	testRebindHarnessGenerationNativeSession(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func testRebindHarnessGenerationNativeSession(t *testing.T, open func(*testing.T) *Repository) {
	ctx := context.Background()

	t.Run("rebinds token and generation together", func(t *testing.T) {
		repo := open(t)
		fixture := newNativeRebindFixture(t, repo, "accept")
		rebound, err := repo.RebindHarnessGenerationNativeSession(ctx, fixture.request)
		require.NoError(t, err)
		require.True(t, rebound)
		requireNativeIdentity(t, repo, fixture, fixture.request.NativeSessionID, 1)

		rebound, err = repo.RebindHarnessGenerationNativeSession(ctx, fixture.request)
		require.NoError(t, err)
		require.True(t, rebound, "a repeated rebind to the same native session is idempotent")
		requireNativeIdentity(t, repo, fixture, fixture.request.NativeSessionID, 1)
	})

	t.Run("accepts a token already stored by the fresh session event", func(t *testing.T) {
		repo := open(t)
		fixture := newNativeRebindFixture(t, repo, "raced")
		require.NoError(t, repo.UpdateResumeToken(ctx, fixture.session.ID, fixture.request.ExecutionID, fixture.request.NativeSessionID, ""))
		rebound, err := repo.RebindHarnessGenerationNativeSession(ctx, fixture.request)
		require.NoError(t, err)
		require.True(t, rebound)
		requireNativeIdentity(t, repo, fixture, fixture.request.NativeSessionID, 1)
	})

	refusals := map[string]func(*testing.T, *Repository, *nativeRebindFixture){
		"wrong previous native session": func(_ *testing.T, _ *Repository, fixture *nativeRebindFixture) {
			fixture.request.PreviousNativeSessionID = "native-foreign"
		},
		"foreign execution": func(_ *testing.T, _ *Repository, fixture *nativeRebindFixture) {
			fixture.request.ExecutionID = "execution-foreign"
		},
		"newer generation": func(t *testing.T, repo *Repository, fixture *nativeRebindFixture) {
			require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
				SessionID: fixture.session.ID, IncarnationID: fixture.request.IncarnationID, Generation: 2,
				PredecessorGeneration: 1, NativeSessionID: fixture.request.PreviousNativeSessionID, CreationReason: "context_continued",
			}))
		},
		"incomplete restore attempt": func(t *testing.T, repo *Repository, fixture *nativeRebindFixture) {
			_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`INSERT INTO session_restore_attempts
				(id, session_id, incarnation_id, expected_generation, action, outcome, created_at)
				VALUES (?, ?, ?, 1, 'silent_restart_restore', 'pending', ?)`),
				"attempt-"+fixture.session.ID, fixture.session.ID, fixture.request.IncarnationID, time.Now().UTC())
			require.NoError(t, err)
		},
	}
	for name, prepare := range refusals {
		t.Run("refuses "+name, func(t *testing.T) {
			repo := open(t)
			fixture := newNativeRebindFixture(t, repo, "refuse")
			previous := fixture.request.PreviousNativeSessionID
			prepare(t, repo, fixture)
			rebound, err := repo.RebindHarnessGenerationNativeSession(ctx, fixture.request)
			require.NoError(t, err)
			require.False(t, rebound)
			requireNativeIdentity(t, repo, fixture, previous, 0)
		})
	}
}
