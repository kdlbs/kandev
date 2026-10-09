package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestPostgresBootstrapFailureIfCurrentAttemptRejectsSameExecutionRetry proves
// the production PostgreSQL predicate rejects a stale row revision even when
// the retry keeps the same execution ID and STARTING state. It skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresBootstrapFailureIfCurrentAttemptRejectsSameExecutionRetry(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const (
		taskID    = "task-bootstrap-attempt-pg"
		sessionID = "session-bootstrap-attempt-pg"
	)
	seedPostgresTask(t, repo, taskID)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateStarting,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-bootstrap-attempt-pg", TaskID: taskID, SessionID: sessionID,
		AgentExecutionID: "execution-shared-pg",
	}))
	require.NoError(t, repo.UpdateSessionMetadata(ctx, sessionID, map[string]interface{}{
		models.SessionMetaKeyAgentStartAttemptID: "attempt-old-pg",
	}))

	// Model a retry that keeps STARTING and the same runtime identity while it
	// replaces the start-attempt identity before the old failure reaches its CAS.
	newAttempt, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	newAttempt.ErrorMessage = "new PostgreSQL start attempt"
	require.NoError(t, repo.UpdateTaskSession(ctx, newAttempt))
	require.NoError(t, repo.UpdateSessionMetadata(ctx, sessionID, map[string]interface{}{
		models.SessionMetaKeyAgentStartAttemptID: "attempt-new-pg",
	}))

	changed, _, err := repo.CommitBootstrapFailureIfCurrentAttempt(
		ctx,
		taskID,
		sessionID,
		"execution-shared-pg",
		models.TaskSessionStateStarting,
		"",
		"attempt-old-pg",
		models.LastAgentError{Message: "stale PostgreSQL attempt failed", StampValue: "stale-attempt-pg"},
	)
	require.NoError(t, err)
	require.False(t, changed, "PostgreSQL CAS must reject the old start revision")

	stored, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, stored.State)
	require.Equal(t, "new PostgreSQL start attempt", stored.ErrorMessage)
	_, found := models.LoadLastAgentError(stored.Metadata)
	require.False(t, found, "the stale attempt must not persist its failure projection")
}
