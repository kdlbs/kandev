package executor

import (
	"context"
	"errors"
	"sync"
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

type cancelledResumeStartupCase struct {
	name             string
	startErr         error
	currentAttemptID string
	wantCleanup      bool
}

type cancelledResumeStartupFixture struct {
	startEntered  chan struct{}
	releaseStart  chan struct{}
	releaseOnce   sync.Once
	cleanupCalled chan struct{}
	cleanupCalls  int
	processStops  int
	result        <-chan error
	cancel        context.CancelFunc
}

func TestCancelledResumeStartupDelegatesExactCleanup(t *testing.T) {
	cases := []cancelledResumeStartupCase{
		{name: "startup error after cancellation", startErr: errors.New("startup failed"), currentAttemptID: "attempt-current", wantCleanup: true},
		{name: "late startup success", currentAttemptID: "attempt-current", wantCleanup: true},
		{name: "stale attempt with reused execution", currentAttemptID: "attempt-successor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertCancelledResumeStartupCleanup(t, tc) })
	}
}

func assertCancelledResumeStartupCleanup(t *testing.T, tc cancelledResumeStartupCase) {
	f := newCancelledResumeStartupFixture(t, tc)
	t.Cleanup(func() { f.cancelAndJoin(t) })
	select {
	case <-f.startEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("StartAgentProcess did not reach its barrier")
	}
	f.cancel()
	f.releaseOnce.Do(func() { close(f.releaseStart) })
	select {
	case <-f.result:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled startup worker did not return")
	}
	assertCancelledResumeCleanupOwnership(t, f, tc.wantCleanup)
}

func newCancelledResumeStartupFixture(t *testing.T, tc cancelledResumeStartupCase) *cancelledResumeStartupFixture {
	const taskID, sessionID, executionID, attemptID = "task-cancelled-start", "session-cancelled-start", "execution-reused", "attempt-current"
	f := &cancelledResumeStartupFixture{
		startEntered: make(chan struct{}), releaseStart: make(chan struct{}), cleanupCalled: make(chan struct{}, 1),
	}
	manager := &mockAgentManager{
		startAgentProcessFunc: func(context.Context, string) error {
			close(f.startEntered)
			<-f.releaseStart
			return tc.startErr
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return executionID, nil },
		stopAgentWithReasonFunc:      func(context.Context, string, string, bool) error { f.processStops++; return nil },
	}
	repo := newMockRepository()
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateStarting, AgentExecutionID: executionID,
		Metadata: map[string]interface{}{models.SessionMetaKeyAgentStartAttemptID: tc.currentAttemptID},
	}
	exec := newTestExecutor(t, manager, repo)
	exec.SetOnCancelledResumeExecutionCleanup(func(
		_ context.Context, gotTaskID, gotSessionID, gotExecutionID, gotAttemptID string,
	) {
		if gotTaskID != taskID || gotSessionID != sessionID || gotExecutionID != executionID || gotAttemptID != attemptID {
			t.Errorf("cleanup identity = (%q, %q, %q, %q), want exact startup identity", gotTaskID, gotSessionID, gotExecutionID, gotAttemptID)
		}
		f.cleanupCalls++
		f.cleanupCalled <- struct{}{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	ctx = WithCancellableResumeContext(WithResumeAttemptID(ctx, attemptID))
	f.result = exec.runAgentProcessAsyncWithObservation(
		ctx, taskID, sessionID, executionID, "", func(context.Context) {}, false, true, attemptID,
	)
	return f
}

func (f *cancelledResumeStartupFixture) cancelAndJoin(t *testing.T) {
	t.Helper()
	f.cancel()
	f.releaseOnce.Do(func() { close(f.releaseStart) })
	select {
	case <-f.result:
	case <-time.After(5 * time.Second):
		t.Error("cancelled startup worker did not finish")
	}
}

func assertCancelledResumeCleanupOwnership(t *testing.T, f *cancelledResumeStartupFixture, wantCleanup bool) {
	t.Helper()
	if wantCleanup {
		select {
		case <-f.cleanupCalled:
		default:
			t.Fatal("current cancelled attempt did not delegate cleanup")
		}
		if f.cleanupCalls != 1 {
			t.Fatalf("cleanup callback calls = %d, want 1", f.cleanupCalls)
		}
	} else if f.cleanupCalls != 0 {
		t.Fatalf("stale attempt cleanup callback calls = %d, want 0", f.cleanupCalls)
	}
	if f.processStops != 0 {
		t.Fatalf("executor raw stop calls = %d, want callback-owned cleanup only", f.processStops)
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
