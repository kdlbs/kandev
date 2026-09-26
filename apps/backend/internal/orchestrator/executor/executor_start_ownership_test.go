package executor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestExistingWorkspaceStart_ActiveAgent(t *testing.T) {
	ctx := context.Background()
	var descriptionWrites atomic.Int32
	var environmentWrites atomic.Int32
	var processStarts atomic.Int32
	var stopCalls atomic.Int32
	started := make(chan struct{})
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-active", nil
		},
		isAgentRunningForSessionFunc: func(context.Context, string) bool { return true },
		setExecutionDescriptionFunc: func(context.Context, string, string) error {
			descriptionWrites.Add(1)
			return nil
		},
		setExecutionEnvFunc: func(context.Context, string, map[string]string) error {
			environmentWrites.Add(1)
			return nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			processStarts.Add(1)
			close(started)
			return nil
		},
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			stopCalls.Add(1)
			return nil
		},
	}
	repo := newMockRepository()
	exec := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{ID: "task-active", WorkspaceID: "workspace-active"}
	session := &models.TaskSession{
		ID: "session-active", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "profile-active",
	}
	repo.sessions[session.ID] = cloneMockTaskSession(session)
	request := &LaunchAgentRequest{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, SessionID: session.ID,
		TaskEnvironmentID: "environment-active", ExecutorType: "local_pc",
	}

	_, err := exec.startAgentOnExistingWorkspaceWithRequest(
		ctx, task, session, "queued follow-up", true, "", request, nil, true,
	)
	if err == nil {
		<-started
	}

	require.Zero(t, descriptionWrites.Load(), "active execution description must be preserved")
	require.Zero(t, environmentWrites.Load(), "active execution environment must be preserved")
	require.Zero(t, processStarts.Load(), "a second agent process must not start")
	require.Zero(t, stopCalls.Load(), "a losing start must not stop the active execution")
	require.ErrorIs(t, err, ErrExecutionAlreadyRunning)
}

func TestExistingWorkspaceStart_PreparedWorkspaceCanStart(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{}, 1)
	agentManager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-prepared", nil
		},
		isAgentRunningForSessionFunc: func(context.Context, string) bool { return false },
		startAgentProcessFunc: func(context.Context, string) error {
			started <- struct{}{}
			return nil
		},
	}
	repo := newMockRepository()
	exec := newTestExecutor(t, agentManager, repo)
	task := &v1.Task{ID: "task-prepared", WorkspaceID: "workspace-prepared"}
	session := &models.TaskSession{
		ID: "session-prepared", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "profile-prepared",
	}
	repo.sessions[session.ID] = cloneMockTaskSession(session)
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID: "runtime-prepared", TaskID: task.ID, SessionID: session.ID,
		Status: models.ExecutorRunningStatusPrepared, AgentExecutionID: "execution-prepared",
	}
	request := &LaunchAgentRequest{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, SessionID: session.ID,
		TaskEnvironmentID: "environment-prepared", ExecutorType: "local_pc",
	}

	execution, err := exec.startAgentOnExistingWorkspaceWithRequest(
		ctx, task, session, "initial brief", true, "", request, nil, true,
	)

	require.NoError(t, err)
	require.NotNil(t, execution)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("prepared workspace did not start an agent process")
	}
}

type runningStateRaceRepository struct{ *mockRepository }

func (r *runningStateRaceRepository) UpdateTaskSessionIfCurrentState(
	_ context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.sessions[session.ID]
	if current == nil || current.State != expected {
		return false, nil
	}
	current = cloneMockTaskSession(current)
	current.State = models.TaskSessionStateRunning
	r.sessions[session.ID] = current
	return false, nil
}

func TestSessionStartingRaceWithRunningStateReturnsBusy(t *testing.T) {
	repo := &runningStateRaceRepository{mockRepository: newMockRepository()}
	session := &models.TaskSession{ID: "session-race", TaskID: "task-race", State: models.TaskSessionStateWaitingForInput}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	starting := cloneMockTaskSession(session)
	starting.State = models.TaskSessionStateStarting

	err := exec.updateSessionStarting(context.Background(), session.TaskID, starting, models.TaskSessionStateWaitingForInput, true)

	require.ErrorIs(t, err, ErrExecutionAlreadyRunning)
	require.ErrorIs(t, err, errSessionAdvancedToRunning)
}
