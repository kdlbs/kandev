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
// session a concurrent cancel had just terminated. The fix conditions that
// write on the state confirmed before the launch attempt started, so a
// cancellation landing during the attempt wins.
func TestRecoverDynamicRouteActionDoesNotResurrectCancelledSession(t *testing.T) {
	ctx := context.Background()
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateRunning)

	// expectedState is what applyDynamicRouteAction would have captured from
	// its own load, before finishDynamicRouteAction's launch attempt began.
	expectedState := models.TaskSessionStateRunning

	// Simulate a concurrent cancellation committing while the successor
	// launch (which recoverDynamicRouteAction is about to react to) was still
	// in flight.
	cancelled, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	cancelled.State = models.TaskSessionStateCancelled
	require.NoError(t, repo.UpdateTaskSession(ctx, cancelled))

	result, err := recoverDynamicRouteAction(ctx, repo, "session-1", expectedState, errors.New("launch failed"))
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)

	persisted, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, persisted.State,
		"a concurrently cancelled session must not be resurrected by a superseded route-action recovery write")
}

// TestRecoverDynamicRouteActionWritesWhenStateUnchanged is the control case:
// with no concurrent state change, the recovery write must still apply.
func TestRecoverDynamicRouteActionWritesWhenStateUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "session-1", models.TaskSessionStateRunning)

	result, err := recoverDynamicRouteAction(ctx, repo, "session-1", models.TaskSessionStateRunning, errors.New("launch failed"))
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)

	persisted, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, persisted.State)
	require.Equal(t, "action_required", persisted.RouteState)
}
