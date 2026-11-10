package changes

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-TURN-CHANGES-003.1, AC-TASKS-TURN-CHANGES-007.1
func TestCoordinatorSettlesUnacceptedStart(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "start-failure.db"))
	require.NoError(t, err)
	store := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	repo, err := sqlite.NewWithDB(store, store, nil)
	require.NoError(t, err)
	admission := coordinatorAdmission()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: admission.TaskID, Title: "Start failure"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: admission.TaskEnvironmentID, TaskID: admission.TaskID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: admission.SessionID, TaskID: admission.TaskID, TaskEnvironmentID: admission.TaskEnvironmentID,
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: admission.TurnID, TaskID: admission.TaskID, TaskSessionID: admission.SessionID, StartedAt: time.Now().UTC(),
	}))
	failedStart := &failedStartPersistence{TurnChangesRepository: repo}
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{Enabled: true, SettingsUserID: "user-initiator", ResolutionKind: turnchanges.PolicyAuthenticatedUser}}
	client := &coordinatorCheckpointClient{}
	coordinator := NewCoordinator(failedStart, repo, policy, nil, nil)
	require.ErrorIs(t, coordinator.Admit(ctx, admission, client), context.DeadlineExceeded)
	set, err := repo.GetTurnChangeSet(ctx, admission.TaskID, admission.SessionID, turnChangeSetID(admission.TurnID))
	require.NoError(t, err)
	require.False(t, set.StartAccepted)
	require.Nil(t, set.TerminalAt)
	require.Equal(t, 1, client.startCalls)

	terminal := Terminal{Admission: admission, At: time.Now().UTC(), Outcome: "end_turn"}
	require.NoError(t, coordinator.CaptureTerminalEndpoints(ctx, terminal, client))
	require.NoError(t, coordinator.Finish(ctx, terminal, client), "replayed completion must reuse settlement")
	set, err = repo.GetTurnChangeSet(ctx, admission.TaskID, admission.SessionID, set.ID)
	require.NoError(t, err)
	require.NotNil(t, set.TerminalAt)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, set.Availability)
	require.Equal(t, models.TurnChangeReasonCheckoutUnavailable, set.Reason)
	require.True(t, set.CaptureEnabled)
	require.False(t, set.StartAccepted)
	require.False(t, set.Complete)
	require.False(t, set.SummaryComplete)
	require.False(t, set.ContentComplete)
	require.Equal(t, 1, client.startCalls, "a missing start cannot be recaptured after dispatch")
	require.Zero(t, client.endCalls)
	require.Zero(t, client.compareCalls)
	require.Zero(t, client.exportCalls)
}

type failedStartPersistence struct {
	repository.TurnChangesRepository
}

func (*failedStartPersistence) AcceptTurnChangeSetStart(context.Context, string, int64, []models.TurnRepositoryChangeSet) (bool, error) {
	return false, context.DeadlineExceeded
}
