package executor

import (
	"context"
	"errors"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// fakeCoordinatorLookup is a test double for CoordinatorLookup.
type fakeCoordinatorLookup struct {
	coordinatorID string
	ok            bool
	lookupErr     error
	profilesReady bool
	profilesErr   error
}

func (f fakeCoordinatorLookup) CoordinatorForConversationTask(context.Context, string) (string, bool, error) {
	return f.coordinatorID, f.ok, f.lookupErr
}

func (f fakeCoordinatorLookup) CoordinatorProfilesReady(context.Context, string) (bool, error) {
	return f.profilesReady, f.profilesErr
}

func coordinatorTaskAndSession() (*models.Task, *models.TaskSession) {
	task := &models.Task{ID: "conversation-task", Origin: models.TaskOriginCoordinator}
	session := &models.TaskSession{ID: "session", TaskID: "conversation-task"}
	return task, session
}

func TestResolveTaskSessionMCPMode_CoordinatorOriginTask(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesReady: true})

	mode, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.NoError(t, err)
	require.Equal(t, McpModeCoordinator, mode)
}

func TestResolveTaskSessionMCPMode_CoordinatorOriginWinsOverConfigMode(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	session.Metadata = map[string]interface{}{"config_mode": true}
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesReady: true})

	mode, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.NoError(t, err)
	require.Equal(t, McpModeCoordinator, mode)

	profile, err := exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.NoError(t, err)
	require.Equal(t, mcpprofile.SurfaceCoordinator, profile.Surface)
}

func TestResolveTaskSessionMCPProfile_CoordinatorOriginTask(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesReady: true})

	profile, err := exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.NoError(t, err)
	require.Equal(t, mcpprofile.SurfaceCoordinator, profile.Surface)
	require.False(t, profile.HasCapability(mcpprofile.CapabilityUserQuestion))
	require.False(t, profile.HasCapability(mcpprofile.CapabilityCanvas))
}

func TestResolveTaskSessionMCPMode_CoordinatorFailClosedNoLookupWired(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.Error(t, err)
	require.Empty(t, mode)

	profile, err := exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.Error(t, err)
	require.Zero(t, profile)
}

func TestResolveTaskSessionMCPMode_CoordinatorFailClosedNoMatch(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{ok: false})

	_, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.Error(t, err)

	_, err = exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.Error(t, err)
}

func TestResolveTaskSessionMCPMode_CoordinatorFailClosedLookupError(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{lookupErr: errors.New("boom")})

	_, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.Error(t, err)

	_, err = exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.Error(t, err)
}

func TestResolveTaskSessionMCPMode_CoordinatorFailClosedProfilesNotReady(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesReady: false})

	_, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.Error(t, err)

	_, err = exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.Error(t, err)
}

func TestResolveTaskSessionMCPMode_CoordinatorFailClosedProfilesReadErr(t *testing.T) {
	task, session := coordinatorTaskAndSession()
	repo := newMockRepository()
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesErr: errors.New("boom")})

	_, err := exec.resolveTaskSessionMCPMode(context.Background(), task.ID, session, true)
	require.Error(t, err)

	_, err = exec.resolveTaskSessionMCPProfile(context.Background(), task.ID, session, true)
	require.Error(t, err)
}

// TestResolveTaskSessionMCPMode_NoRowCoordinatorMatchFailsStart covers the
// task-row-absent path: the task cannot be read (deleted concurrently, or
// never existed), but the lookup still finds a coordinator whose
// conversation_task_id equals this id. A match or lookup error must fail the
// start, in BOTH "no row" forms (ErrTaskNotFound and a fake returning a nil
// task with nil error), and for both resolvers, config-mode included
// (docs/specs/coordinator/system-design/copilot.md#principal-and-mode).
func TestResolveTaskSessionMCPMode_NoRowCoordinatorMatchFailsStart(t *testing.T) {
	session := &models.TaskSession{ID: "session", TaskID: "missing-task"}

	noRowForms := []struct {
		name        string
		getTaskFunc func(ctx context.Context, id string) (*models.Task, error)
	}{
		{
			name: "ErrTaskNotFound",
			getTaskFunc: func(context.Context, string) (*models.Task, error) {
				return nil, repoerrors.ErrTaskNotFound
			},
		},
		{
			name: "nil task nil error",
			getTaskFunc: func(context.Context, string) (*models.Task, error) {
				return nil, nil
			},
		},
	}

	for _, form := range noRowForms {
		t.Run(form.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.getTaskFunc = form.getTaskFunc
			repo.sessions[session.ID] = session
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.SetCoordinatorLookup(fakeCoordinatorLookup{coordinatorID: "coord-1", ok: true, profilesReady: true})

			_, err := exec.resolveTaskSessionMCPMode(context.Background(), session.TaskID, session, true)
			require.Error(t, err)

			_, err = exec.resolveTaskSessionMCPProfile(context.Background(), session.TaskID, session, true)
			require.Error(t, err)

			configSession := &models.TaskSession{ID: "session-config", TaskID: session.TaskID, Metadata: map[string]interface{}{"config_mode": true}}
			_, err = exec.resolveTaskSessionMCPMode(context.Background(), session.TaskID, configSession, true)
			require.Error(t, err)
			_, err = exec.resolveTaskSessionMCPProfile(context.Background(), session.TaskID, configSession, true)
			require.Error(t, err)
		})
	}
}

// TestResolveTaskSessionMCPMode_NoRowNoMatchKeepsTodaysResult covers the same
// two "no row" forms with no coordinator lookup match: the two resolvers must
// keep their pre-existing per-form results (BUILD DECISION F15) — mode
// resolver errors on ErrTaskNotFound and returns "" (no restricted mode) on a
// nil task; profile resolver errors on ErrTaskNotFound and returns the
// Legacy Kanban profile on a nil task. This exercises both with the
// coordinator lookup wired (feature on) to confirm a non-match still falls
// through.
func TestResolveTaskSessionMCPMode_NoRowNoMatchKeepsTodaysResult(t *testing.T) {
	session := &models.TaskSession{ID: "session", TaskID: "missing-task"}

	t.Run("ErrTaskNotFound", func(t *testing.T) {
		repo := newMockRepository()
		repo.getTaskFunc = func(context.Context, string) (*models.Task, error) {
			return nil, repoerrors.ErrTaskNotFound
		}
		repo.sessions[session.ID] = session
		exec := newTestExecutor(t, &mockAgentManager{}, repo)
		exec.SetCoordinatorLookup(fakeCoordinatorLookup{ok: false})

		_, err := exec.resolveTaskSessionMCPMode(context.Background(), session.TaskID, session, true)
		require.Error(t, err)

		_, err = exec.resolveTaskSessionMCPProfile(context.Background(), session.TaskID, session, true)
		require.Error(t, err)
	})

	t.Run("nil task nil error", func(t *testing.T) {
		repo := newMockRepository()
		repo.getTaskFunc = func(context.Context, string) (*models.Task, error) {
			return nil, nil
		}
		repo.sessions[session.ID] = session
		exec := newTestExecutor(t, &mockAgentManager{}, repo)
		exec.SetCoordinatorLookup(fakeCoordinatorLookup{ok: false})

		mode, err := exec.resolveTaskSessionMCPMode(context.Background(), session.TaskID, session, true)
		require.NoError(t, err)
		require.Empty(t, mode)

		profile, err := exec.resolveTaskSessionMCPProfile(context.Background(), session.TaskID, session, true)
		require.NoError(t, err)
		require.Equal(t, mcpprofile.SurfaceKanbanTask, profile.Surface)
	})
}

// TestResolveTaskSessionMCPMode_ReadErrorFailsEverySessionIncludingConfigMode
// asserts BUILD DECISION F14's reordering: the task is now loaded before the
// config-mode check, so a genuine (non-not-found) read error fails a
// config-mode session too — before this task, config-mode sessions returned
// before GetTask ever ran.
func TestResolveTaskSessionMCPMode_ReadErrorFailsEverySessionIncludingConfigMode(t *testing.T) {
	repo := newMockRepository()
	readErr := errors.New("read failed")
	repo.getTaskFunc = func(context.Context, string) (*models.Task, error) {
		return nil, readErr
	}
	session := &models.TaskSession{ID: "session", TaskID: "task", Metadata: map[string]interface{}{"config_mode": true}}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.resolveTaskSessionMCPMode(context.Background(), "task", session, true)
	require.Error(t, err)

	_, err = exec.resolveTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.Error(t, err)
}
