package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// focusCeilingFixture is an idle-suspended session whose only ceiling slot is
// held by an unrelated in-flight launch. Agent launches fail and are counted,
// so a resume that reaches the executor returns promptly instead of waiting
// for an agent that never becomes ready.
type focusCeilingFixture struct {
	svc         *Service
	launchCalls *atomic.Int32
	occupierKey string
}

func newFocusCeilingFixture(t *testing.T, taskID, sessionID string, state models.TaskSessionState) focusCeilingFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, state)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	session.AgentProfileID = claudeACPProviderID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID,
		AgentExecutionID: "execution-" + sessionID, Status: models.ExecutorRunningStatusStopped,
		IdleSuspensionState: models.ExecutorIdleSuspensionSuspended,
		ExecutorID:          "executor-local", Resumable: true, ResumeToken: "same-conversation-token",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert idle-suspended runtime: %v", err)
	}

	launchCalls := &atomic.Int32{}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls.Add(1)
			return nil, errors.New("test launch refused")
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &inactiveTurnService{}
	svc.SetLSPLeaseLifecycle(&activeLSPLeaseForTest{sessionID: sessionID})

	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	decision := svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "task-occupier", sessionID: "session-occupier",
		origin: launchOriginAutomatic, seam: "test",
	})
	if !decision.admitted {
		t.Fatalf("occupier admission = %+v, want admitted", decision)
	}
	return focusCeilingFixture{svc: svc, launchCalls: launchCalls, occupierKey: decision.reservationKey}
}

// A focus-driven idle-suspension resume is passive inspection, not an explicit
// execution request, so it must be admitted against the session ceiling. With
// the ceiling saturated, focusing a parked session must not launch an agent.
func TestFocusTaskSessionIdleSuspensionResumeRespectsCeiling(t *testing.T) {
	ctx := context.Background()
	f := newFocusCeilingFixture(t, "task-focus-ceiling", "session-focus-ceiling", models.TaskSessionStateWaitingForInput)

	execution, _, resumed, err := f.svc.focusTaskSession(ctx, "task-focus-ceiling", "session-focus-ceiling")
	if n := f.launchCalls.Load(); n != 0 {
		t.Fatalf("saturated focus launched %d agents, want 0", n)
	}
	if err != nil {
		t.Fatalf("focusTaskSession: %v", err)
	}
	if resumed || execution != nil {
		t.Fatalf("saturated focus resumed: execution=%v resumed=%v, want none", execution != nil, resumed)
	}
}

// A completed idle-suspended session deferred by focus keeps its focus resume
// permission in the deferral, so the replay reaches the executor once capacity
// frees instead of rejecting the completed session.
func TestFocusTaskSessionDeferredCompletedResumeReplaysAfterCapacityFrees(t *testing.T) {
	ctx := context.Background()
	f := newFocusCeilingFixture(t, "task-focus-completed", "session-focus-completed", models.TaskSessionStateCompleted)

	if _, _, resumed, err := f.svc.focusTaskSession(ctx, "task-focus-completed", "session-focus-completed"); err != nil || resumed {
		t.Fatalf("saturated focus: resumed=%v err=%v, want deferred", resumed, err)
	}
	record := deferredLaunchOf(t, f.svc, "task-focus-completed")
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchResume) {
		t.Fatalf("focus did not record a deferred resume: %+v", record)
	}

	f.svc.sessionCeiling.release(f.occupierKey)
	f.svc.retryOneDeferredCeilingLaunch(ctx, &models.Task{ID: "task-focus-completed"})
	if n := f.launchCalls.Load(); n != 1 {
		t.Fatalf("replayed completed focus resume launched %d agents, want 1", n)
	}
}
