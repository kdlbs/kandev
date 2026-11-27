package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestFreshStartGenerationStore(t *testing.T) {
	exerciseFreshStartGenerationStore(t, newRepoForSessionTests(t))
}

func TestPostgresFreshStartGenerationStore(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	exerciseFreshStartGenerationStore(t, repo)
}

func exerciseFreshStartGenerationStore(t *testing.T, repo *Repository) {
	t.Helper()
	for _, condition := range []string{"current", "rotated", "native changed", "foreign task", "foreign incarnation", "nonadjacent", "wrong predecessor", "cancelled"} {
		t.Run(condition, func(t *testing.T) {
			ctx := t.Context()
			taskID, sessionID := "fresh-store-task-"+condition, "fresh-store-session-"+condition
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Fresh generation"}))
			session := &models.TaskSession{ID: sessionID, TaskID: taskID}
			require.NoError(t, repo.CreateTaskSession(ctx, session))
			running := &models.ExecutorRunning{SessionID: sessionID, TaskID: taskID, AgentExecutionID: "execution", ResumeToken: "native", Status: models.ExecutorRunningStatusRunning}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
			generation := &models.HarnessSessionGeneration{SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
				Generation: 1, NativeSessionID: "native", CreationReason: "native_started", CreatedAt: time.Now().UTC()}
			committed, err := repo.CommitHarnessSessionGenerationForExecution(ctx, generation, 0, taskID, "execution")
			require.NoError(t, err)
			require.True(t, committed, "the current first generation must commit")
			generation.Generation, generation.PredecessorGeneration, generation.CreationReason = 2, 1, "native_replaced"
			expectedTask := taskID
			switch condition {
			case "rotated":
				running.AgentExecutionID = "successor"
				require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
			case "native changed":
				require.NoError(t, repo.UpdateResumeToken(ctx, sessionID, "execution", "new-native", ""))
			case "foreign task":
				expectedTask = "another-task"
			case "foreign incarnation":
				generation.IncarnationID = "another-incarnation"
			case "nonadjacent":
				generation.Generation = 3
			case "wrong predecessor":
				generation.PredecessorGeneration = 0
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			committed, err = repo.CommitHarnessSessionGenerationForExecution(ctx, generation, 1, expectedTask, "execution")
			if condition == "nonadjacent" || condition == "wrong predecessor" || condition == "cancelled" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, condition == "current", committed)
			current, err := repo.GetCurrentHarnessSessionGeneration(t.Context(), sessionID, session.QueueIncarnationID)
			require.NoError(t, err)
			if condition == "current" {
				require.EqualValues(t, 2, current.Generation)
				committed, err = repo.CommitHarnessSessionGenerationForExecution(t.Context(), generation, 1, taskID, "execution")
				require.NoError(t, err)
				require.False(t, committed, "the predecessor CAS must reject a duplicate successor")
			} else {
				require.EqualValues(t, 1, current.Generation)
			}
		})
	}
}

func TestPostgresFreshStartGenerationWaitsForExecutorRotation(t *testing.T) {
	repo, worker, holderDB := newTaskPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "fresh-lock-task", Title: "Fresh generation lock"}))
	session := &models.TaskSession{ID: "fresh-lock-session", TaskID: "fresh-lock-task"}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{SessionID: session.ID, TaskID: session.TaskID,
		AgentExecutionID: "old-execution", ResumeToken: "old-native", Status: models.ExecutorRunningStatusRunning}))
	generation := &models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		Generation: 1, NativeSessionID: "old-native", CreationReason: "native_started", CreatedAt: time.Now().UTC()}
	committed, err := repo.CommitHarnessSessionGenerationForExecution(ctx, generation, 0, session.TaskID, "old-execution")
	require.NoError(t, err)
	require.True(t, committed)
	workerPID := pgBackendPID(t, worker.db)
	held, err := holderDB.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = held.Rollback() }()
	require.NoError(t, lockSessionTurnWrites(ctx, held, repo.db.DriverName(), session.ID))
	_, err = held.ExecContext(ctx, repo.db.Rebind(`UPDATE executors_running SET agent_execution_id = ?, resume_token = ? WHERE session_id = ?`),
		"new-execution", "new-native", session.ID)
	require.NoError(t, err)
	generation.Generation, generation.PredecessorGeneration, generation.CreationReason = 2, 1, "native_replaced"
	type commitResult struct {
		committed bool
		err       error
	}
	result := make(chan commitResult, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ok, err := worker.CommitHarnessSessionGenerationForExecution(ctx, generation, 1, session.TaskID, "old-execution")
		result <- commitResult{ok, err}
	}()
	t.Cleanup(func() { cancel(); _ = held.Rollback(); <-finished })
	require.Eventually(t, func() bool {
		var blocked bool
		err := repo.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE pid = $1 AND locktype = 'advisory' AND NOT granted)`, workerPID).Scan(&blocked)
		return err == nil && blocked
	}, 5*time.Second, 10*time.Millisecond, "the real generation writer must wait on the executor writer's lock")
	require.NoError(t, held.Commit())
	observed := <-result
	require.NoError(t, observed.err)
	require.False(t, observed.committed, "ownership must be read after the competing rotation commits")
	generation.NativeSessionID = "new-native"
	committed, err = worker.CommitHarnessSessionGenerationForExecution(ctx, generation, 1, session.TaskID, "new-execution")
	require.NoError(t, err)
	require.True(t, committed, "the new execution must commit through the same production transaction")
	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	require.NoError(t, err)
	require.Equal(t, "new-native", current.NativeSessionID)
}
