package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// newDynamicRoutingCancelRaceRepo opens an in-memory sqlite repo and seeds the
// minimal workspace/workflow/task/session rows a dynamic route action reads.
func newDynamicRoutingCancelRaceRepo(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "kandev.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := taskrepo.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("repo provide: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	return repo
}

func seedDynamicRoutingCancelRaceSession(t *testing.T, repo *sqliterepo.Repository, sessionID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "T", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: "task-1", State: state, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

// TestRecoverDynamicRouteActionDoesNotResurrectCancelledSession proves the
// concrete race F4 describes: a successor launch fails after a concurrent
// cancellation has already committed CANCELLED to the row. Before the fix,
// recoverDynamicRouteAction reloaded the row, unconditionally overwrote State
// with WAITING_FOR_INPUT, and whole-row-wrote it back — resurrecting a
// session a concurrent cancel had just terminated. The fix refuses to
// resurrect a reloaded CANCELLED state at all, so a cancellation landing
// during the launch attempt wins.
func TestRecoverDynamicRouteActionDoesNotResurrectCancelledSession(t *testing.T) {
	ctx := context.Background()
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateRunning)

	// Simulate a concurrent cancellation committing while the successor
	// launch (which recoverDynamicRouteAction is about to react to) was still
	// in flight.
	cancelled, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	cancelled.State = models.TaskSessionStateCancelled
	require.NoError(t, repo.UpdateTaskSession(ctx, cancelled))

	result, err := recoverDynamicRouteAction(ctx, repo, "session-1", errors.New("launch failed"))
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)

	persisted, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, persisted.State,
		"a concurrently cancelled session must not be resurrected by a superseded route-action recovery write")
}

// TestRecoverDynamicRouteActionDoesNotResurrectOtherTerminalStates extends the
// CANCELLED case above to COMPLETED and FAILED: any terminal state a
// concurrent handler already committed while the successor launch was in
// flight must win over recoverDynamicRouteAction's WAITING_FOR_INPUT write,
// not just CANCELLED.
func TestRecoverDynamicRouteActionDoesNotResurrectOtherTerminalStates(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			repo := newDynamicRoutingCancelRaceRepo(t)
			seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateRunning)

			settled, err := repo.GetTaskSession(ctx, "session-1")
			require.NoError(t, err)
			settled.State = state
			require.NoError(t, repo.UpdateTaskSession(ctx, settled))

			result, err := recoverDynamicRouteAction(ctx, repo, "session-1", errors.New("launch failed"))
			require.NoError(t, err)
			require.Equal(t, "session-1", result.SessionID)

			persisted, err := repo.GetTaskSession(ctx, "session-1")
			require.NoError(t, err)
			require.Equal(t, state, persisted.State,
				"a concurrently settled terminal session must not be resurrected by a superseded route-action recovery write")
		})
	}
}

// TestRecoverDynamicRouteActionWritesWhenStateUnchanged is the control case:
// with no concurrent state change, the recovery write must still apply.
func TestRecoverDynamicRouteActionWritesWhenStateUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateRunning)

	result, err := recoverDynamicRouteAction(ctx, repo, "session-1", errors.New("launch failed"))
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)

	persisted, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, persisted.State)
	require.Equal(t, "action_required", persisted.RouteState)
}

// TestFinishDynamicRouteActionRecoversAfterLauncherMutatesState proves F5:
// the real launch-failure path does not gate the recovery write on a
// pre-launch snapshot. orchestrator.relaunchDynamicTaskAfterFailure resets
// the session row to CREATED before attempting the successor launch, then
// fails; a CAS built from the state captured before that launch attempt
// would never match CREATED and would silently drop the recovery write,
// stranding the session with no error surfaced. finishDynamicRouteAction
// must still land WAITING_FOR_INPUT/action_required with the launch error.
func TestFinishDynamicRouteActionRecoversAfterLauncherMutatesState(t *testing.T) {
	ctx := context.Background()
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateRunning)

	launchErr := errors.New("successor launch failed")
	launcher := func(ctx context.Context, sessionID string) error {
		// Mirrors internal/orchestrator/dynamic_launch.go's
		// relaunchDynamicTaskAfterFailure, which resets state to CREATED
		// before attempting (and, here, failing) the successor launch.
		if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateCreated, ""); err != nil {
			return err
		}
		return launchErr
	}

	result, err := finishDynamicRouteAction(ctx, repo, "session-1", launcher)
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)
	require.Equal(t, "action_required", result.State)

	persisted, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, persisted.State)
	require.Equal(t, "action_required", persisted.RouteState)
	require.Equal(t, "route_action_launch_failed", persisted.RouteReason)
	require.Equal(t, launchErr.Error(), persisted.ErrorMessage)
}
