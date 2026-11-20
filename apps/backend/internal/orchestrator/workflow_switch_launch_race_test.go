package orchestrator

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// The production callback path persists the launch row through
// persistFullTaskSessionIfCurrent, not through the executor helper. A parked
// workflow switch must be recognized there too.
func TestPersistFullTaskSessionIfCurrent_WorkflowParkIsSuperseded(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyWorkflowParking,
		models.WorkflowParking{
			Stamp:           "park-1",
			ParkedAt:        time.Now().UTC(),
			SourceSessionID: "session1",
			DestinationID:   "session2",
		},
	))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	stale := &models.TaskSession{ID: "session1", TaskID: "task1", State: models.TaskSessionStateCreated}

	err := svc.persistFullTaskSessionIfCurrent(ctx, stale, models.TaskSessionStateCreated)
	if !executor.IsSessionStateSwitched(err) {
		t.Fatalf("persistFullTaskSessionIfCurrent() error = %v, want SessionStateSwitchedError", err)
	}
}

func TestPersistFullTaskSessionIfCurrent_UnexpectedStateWithoutSwitchIsError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	stale := &models.TaskSession{ID: "session1", TaskID: "task1", State: models.TaskSessionStateCreated}

	err := svc.persistFullTaskSessionIfCurrent(ctx, stale, models.TaskSessionStateCreated)
	if err == nil || executor.IsSessionStateSwitched(err) {
		t.Fatalf("persistFullTaskSessionIfCurrent() error = %v, want a plain unexpected-state failure", err)
	}
}

func TestPersistFullTaskSessionIfCurrent_NewParkingStampSupersedesUnchangedWaitingState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyWorkflowParking,
		models.WorkflowParking{
			Stamp:           "park-new",
			ParkedAt:        time.Now().UTC(),
			SourceSessionID: "session1",
		},
	))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	stale := &models.TaskSession{
		ID: "session1", TaskID: "task1", State: models.TaskSessionStateStarting,
	}

	err := svc.persistFullTaskSessionIfCurrent(ctx, stale, models.TaskSessionStateWaitingForInput)
	require.True(t, executor.IsSessionStateSwitched(err), "new parking stamp must supersede the launch")
	current, getErr := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateWaitingForInput, current.State)
}

// A losing launch must not mark the task FAILED or the session FAILED: the
// workflow switch still owns the session and will launch the replacement.
func TestHandleSessionLaunchFailure_WorkflowSwitchSupersededKeepsTaskActive(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyWorkflowParking,
		models.WorkflowParking{
			Stamp:           "park-1",
			ParkedAt:        time.Now().UTC(),
			SourceSessionID: "session1",
		},
	))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	err := svc.handleSessionLaunchFailure(
		ctx, "task1", "session1",
		&executor.SessionStateSwitchedError{SessionID: "session1", State: models.TaskSessionStateWaitingForInput},
	)
	require.Error(t, err)

	session, getErr := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, getErr)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)

	taskRepo.mu.Lock()
	defer taskRepo.mu.Unlock()
	require.Zero(t, taskRepo.stateWrites["task1"], "a superseded launch must not write a task state")
}

func TestTransitionTaskSessionState_LateLaunchFailureYieldsToNewParkingStamp(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	launchSnapshot, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session1", models.SessionMetaKeyWorkflowParking, models.WorkflowParking{
		Stamp: "park-new", ParkedAt: time.Now().UTC(), SourceSessionID: "session1",
	}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	expected := models.TaskSessionStateWaitingForInput
	failureCtx := executor.WithWorkflowLaunchFailureOwnership(ctx, launchSnapshot)

	changed, finalState, err := svc.transitionTaskSessionState(
		failureCtx, "task1", "session1", &expected, models.TaskSessionStateFailed, "agent launch failed", nil,
	)

	require.True(t, executor.IsSessionStateSwitched(err), "the new source stamp must supersede launch failure bookkeeping")
	require.False(t, changed)
	require.Equal(t, models.TaskSessionStateWaitingForInput, finalState)
	current, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, current.State)
}

func TestTransitionTaskSessionState_StaleLaunchFailureCannotFailReplacementAttempt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyAgentStartAttemptID, "attempt-old",
	))
	launchSnapshot, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "session1", models.SessionMetaKeyAgentStartAttemptID, "attempt-new",
	))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	expected := models.TaskSessionStateStarting
	failureCtx := executor.WithWorkflowLaunchFailureOwnership(ctx, launchSnapshot)

	changed, finalState, err := svc.transitionTaskSessionState(
		failureCtx, "task1", "session1", &expected, models.TaskSessionStateFailed, "old launch failed", nil,
	)

	require.NoError(t, err)
	require.False(t, changed, "a replacement start attempt must own the unchanged session state")
	require.Equal(t, models.TaskSessionStateStarting, finalState)
	current, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, current.State)
	require.Equal(t, "attempt-new", models.StringFromAny(current.Metadata[models.SessionMetaKeyAgentStartAttemptID]))
}

// gatedSessionPersistRepo holds the first launch-state write of one session
// until released, modelling a launch whose workspace is already materialized
// while its session row is not yet persisted (the incident window).
type gatedSessionPersistRepo struct {
	*sqliterepo.Repository
	sessionID   atomic.Value
	calls       atomic.Int32
	once        sync.Once
	releaseOnce sync.Once
	reached     chan struct{}
	release     chan struct{}
}

func (r *gatedSessionPersistRepo) UpdateTaskSessionIfCurrentStateForLaunch(
	ctx context.Context, session *models.TaskSession, expected models.TaskSessionState,
	attemptID string,
) (bool, error) {
	if id, _ := r.sessionID.Load().(string); id != "" && session.ID == id {
		if r.calls.Add(1) == 1 {
			r.once.Do(func() {
				close(r.reached)
				<-r.release
			})
		}
	}
	return r.Repository.UpdateTaskSessionIfCurrentStateForLaunch(ctx, session, expected, attemptID)
}

func (r *gatedSessionPersistRepo) UpdateTaskSessionAgentStartAttemptIfCurrentStateForLaunch(
	ctx context.Context, session *models.TaskSession, expected models.TaskSessionState,
	attemptID string,
) (bool, error) {
	return r.Repository.UpdateTaskSessionAgentStartAttemptIfCurrentStateForLaunch(
		ctx, session, expected, attemptID,
	)
}

func (r *gatedSessionPersistRepo) releaseGate() {
	r.releaseOnce.Do(func() { close(r.release) })
}

// A launch that was still persisting its runtime when the real workflow
// profile switch parked its session must exit silently: the switch completes,
// the replacement session owns the step profile, the parked source stays
// answerable, and the task is never written FAILED.
func TestLaunchSession_RealWorkflowMoveDuringLaunchCompletes(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "existing-session", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-source"
	require.NoError(t, repo.UpdateTask(ctx, task))
	source, err := repo.GetTaskSession(ctx, "existing-session")
	require.NoError(t, err)
	source.AgentProfileID = "profile-old"
	source.ExecutorID = models.ExecutorIDLocal
	source.TaskEnvironmentID = "env-task1"
	require.NoError(t, repo.UpdateTaskSession(ctx, source))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-task1", TaskID: "task1", ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusReady,
	}))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test Task", Description: "move task", State: v1.TaskStateInProgress,
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-source"] = &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf1", Name: "Source", Position: 0,
		AgentProfileID:          "profile-old",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	stepGetter.steps["step-target"] = &wfmodels.WorkflowStep{
		ID: "step-target", WorkflowID: "wf1", Name: "Target", Position: 1,
		AgentProfileID: "profile-step",
		Events:         wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}

	gated := &gatedSessionPersistRepo{Repository: repo, reached: make(chan struct{}), release: make(chan struct{})}
	gated.sessionID.Store("existing-session")
	replacementStart := make(chan string, 1)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(launchCtx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			executionID := "exec-" + req.SessionID
			// The lifecycle registers the runtime as soon as it launches.
			now := time.Now().UTC()
			if err := repo.UpsertExecutorRunning(launchCtx, &models.ExecutorRunning{
				ID: "running-" + req.SessionID, SessionID: req.SessionID, TaskID: "task1",
				AgentExecutionID: executionID, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Errorf("register launching runtime: %v", err)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
		},
		startAgentProcessFunc: func(_ context.Context, executionID string) error {
			if executionID != "exec-existing-session" {
				replacementStart <- executionID
			}
			return nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	exec := withTestExecutorActor(executor.NewExecutor(agentMgr, gated, testLogger(), executor.ExecutorConfig{}))
	svc.executor = exec
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, testLogger(), scheduler.SchedulerConfig{})

	type launchResult struct {
		response *LaunchSessionResponse
		err      error
	}
	result := make(chan launchResult, 1)
	launchDone := make(chan struct{})
	release := gated.releaseGate
	launchCtx, cancelLaunch := context.WithCancel(ctx)
	t.Cleanup(func() {
		release()
		cancelLaunch()
		select {
		case <-launchDone:
		case <-time.After(5 * time.Second):
			t.Error("launch did not finish after releasing the test gate")
		}
	})
	go func() {
		defer close(launchDone)
		response, launchErr := svc.LaunchSession(launchCtx, &LaunchSessionRequest{
			TaskID: "task1", Intent: IntentStartCreated, SessionID: "existing-session",
			AgentProfileID: "profile-old", WorkflowStepID: "step-source",
		})
		result <- launchResult{response: response, err: launchErr}
	}()
	select {
	case <-gated.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("launch did not reach runtime persistence")
	}

	// A real task move changes the destination step and enters its profile while
	// the original LaunchSession call is still persisting its start state.
	task, err = repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-target"
	require.NoError(t, repo.UpdateTask(ctx, task))
	svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID: "task1", WorkflowID: "wf1", FromStepID: "step-source", ToStepID: "step-target",
		SessionID: "existing-session", TaskDescription: "move task",
	})
	var replacementExecutionID string
	select {
	case replacementExecutionID = <-replacementStart:
	case <-time.After(5 * time.Second):
		t.Fatal("the moved-to step did not start its replacement agent")
	}

	parked, err := repo.GetTaskSession(ctx, "existing-session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, parked.State)
	marker, ok := models.LoadWorkflowParking(parked.Metadata)
	require.True(t, ok, "the real switch records the parking marker")
	require.Equal(t, "existing-session", marker.SourceSessionID)

	// Release and join the stale launch before the repository's test cleanup.
	release()
	var oldLaunch launchResult
	select {
	case oldLaunch = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("superseded launch did not finish")
	}
	require.True(t, executor.IsSessionStateSwitched(oldLaunch.err),
		"the stale start must return the typed workflow-switch outcome")
	require.Nil(t, oldLaunch.response)

	parked, err = repo.GetTaskSession(ctx, "existing-session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, parked.State,
		"the parked source stays answerable instead of FAILED")
	stepSessionID := strings.TrimPrefix(replacementExecutionID, "exec-")
	stepSession, err := repo.GetTaskSession(ctx, stepSessionID)
	require.NoError(t, err)
	require.Equal(t, "profile-step", stepSession.AgentProfileID)
	require.True(t, stepSession.IsPrimary, "the step profile session owns the task")
	require.NotEqual(t, models.TaskSessionStateFailed, stepSession.State)
	updatedTask, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateInProgress, updatedTask.State)
}
