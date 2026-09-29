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
		ctx, task, session, "queued follow-up", true, "", request, nil, nil, nil, true,
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
		ctx, task, session, "initial brief", true, "", request, nil, nil, nil, true,
	)

	require.NoError(t, err)
	require.NotNil(t, execution)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("prepared workspace did not start an agent process")
	}
}

func TestExistingWorkspaceStart_BindsInitialDeliveryBeforeAdmission(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	task := &v1.Task{ID: "task-delivery", WorkspaceID: "workspace-delivery"}
	session := &models.TaskSession{
		ID: "session-delivery", TaskID: task.ID, State: models.TaskSessionStateCreated,
		AgentProfileID: "profile-delivery",
	}
	repo.sessions[session.ID] = cloneMockTaskSession(session)
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		ID: "runtime-delivery", TaskID: task.ID, SessionID: session.ID,
		Status: models.ExecutorRunningStatusPrepared, AgentExecutionID: "execution-delivery",
	}
	var order atomic.Int32
	var identityOrder, admissionOrder, startOrder atomic.Int32
	started := make(chan struct{}, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-delivery", nil
		},
		setInitialDeliverySubmissionIDFunc: func(_ context.Context, executionID, submissionID string) error {
			if executionID != "execution-delivery" || submissionID != "message-delivery" {
				t.Fatalf("initial delivery identity = %q/%q", executionID, submissionID)
			}
			identityOrder.Store(order.Add(1))
			return nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			startOrder.Store(order.Add(1))
			started <- struct{}{}
			return nil
		},
	}
	request := &LaunchAgentRequest{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, SessionID: session.ID,
		InitialDeliverySubmissionID: "message-delivery",
		BeforeAgentStart: func(context.Context, string) error {
			admissionOrder.Store(order.Add(1))
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	_, err := exec.startAgentOnExistingWorkspaceWithRequest(
		ctx, task, session, "initial prompt", true, "", request, nil, nil, nil, false,
	)
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("existing workspace did not start the agent")
	}
	if identityOrder.Load() == 0 || admissionOrder.Load() <= identityOrder.Load() || startOrder.Load() <= admissionOrder.Load() {
		t.Fatalf("initial identity/admission/start ordering = %d/%d/%d", identityOrder.Load(), admissionOrder.Load(), startOrder.Load())
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
