package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestFocusTaskSessionDoesNotResumeWorkflowParkedSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-focus-parked", "session-focus-parked", models.TaskSessionStateWaitingForInput)
	parking := models.WorkflowParking{Stamp: "workflow-parking-1", ParkedAt: time.Now().UTC(), SourceSessionID: "session-focus-parked"}
	if err := repo.SetSessionMetadataKey(ctx, "session-focus-parked", models.SessionMetaKeyWorkflowParking, parking); err != nil {
		t.Fatalf("set workflow parking marker: %v", err)
	}

	svc := &Service{repo: repo}
	_, session, resumed, err := svc.focusTaskSession(ctx, "task-focus-parked", "session-focus-parked")
	if err != nil {
		t.Fatalf("focusTaskSession: %v", err)
	}
	if resumed || session == nil {
		t.Fatalf("workflow parked focus result = resumed:%v session:%v", resumed, session != nil)
	}
	updated, err := repo.GetTaskSession(ctx, "session-focus-parked")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if _, stillParked := models.LoadWorkflowParking(updated.Metadata); !stillParked {
		t.Fatal("explicit task focus cleared workflow ownership metadata")
	}
}

func TestFocusTaskSessionDoesNotWakeManuallyStoppedSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-focus-manual-stop", "session-focus-manual-stop", models.TaskSessionStateWaitingForInput)
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-focus-manual-stop", SessionID: "session-focus-manual-stop", TaskID: "task-focus-manual-stop",
		AgentExecutionID: "execution-focus-manual-stop", Status: models.ExecutorRunningStatusStopped,
		Resumable: true, ResumeToken: "manual-stop-token", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert manually stopped runtime: %v", err)
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &inactiveTurnService{}

	execution, _, resumed, err := svc.focusTaskSession(ctx, "task-focus-manual-stop", "session-focus-manual-stop")
	if err != nil {
		t.Fatalf("focusTaskSession: %v", err)
	}
	if resumed || execution != nil {
		t.Fatalf("manual stop focus returned execution:%v resumed:%v", execution != nil, resumed)
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-focus-manual-stop")
	if err != nil {
		t.Fatal(err)
	}
	if running.Status != models.ExecutorRunningStatusStopped || running.IdleSuspensionState != models.ExecutorIdleSuspensionNone {
		t.Fatalf("focus changed manual stop: status=%q provenance=%q", running.Status, running.IdleSuspensionState)
	}
}

func TestFocusTaskSessionResumesIdleSuspensionWithNewLSPLease(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-focus-idle-suspended", "session-focus-idle-suspended", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "session-focus-idle-suspended")
	if err != nil {
		t.Fatal(err)
	}
	session.AgentProfileID = "claude-acp"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-focus-idle-suspended", SessionID: "session-focus-idle-suspended", TaskID: "task-focus-idle-suspended",
		AgentExecutionID: "execution-focus-idle-suspended", Status: models.ExecutorRunningStatusStopped,
		IdleSuspensionState: models.ExecutorIdleSuspensionSuspended,
		ExecutorID:          "executor-local", Resumable: true, ResumeToken: "same-conversation-token", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert idle-suspended runtime: %v", err)
	}

	launchCalls := 0
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			if req.ACPSessionID != "same-conversation-token" {
				t.Errorf("resume token = %q, want preserved conversation token", req.ACPSessionID)
			}
			updated, getErr := repo.GetTaskSession(ctx, req.SessionID)
			if getErr != nil {
				return nil, getErr
			}
			updated.State = models.TaskSessionStateWaitingForInput
			updated.UpdatedAt = time.Now().UTC()
			if updateErr := repo.UpdateTaskSession(ctx, updated); updateErr != nil {
				return nil, updateErr
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-focus-resumed"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-focus-idle-suspended"] = &v1.Task{ID: "task-focus-idle-suspended", State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &inactiveTurnService{}
	svc.SetLSPLeaseLifecycle(&activeLSPLeaseForTest{sessionID: "session-focus-idle-suspended"})

	execution, _, resumed, err := svc.focusTaskSession(ctx, "task-focus-idle-suspended", "session-focus-idle-suspended")
	if err != nil {
		t.Fatalf("focusTaskSession: %v", err)
	}
	if !resumed || execution == nil {
		t.Fatalf("idle-suspended focus returned execution:%v resumed:%v", execution != nil, resumed)
	}
	if launchCalls != 1 {
		t.Fatalf("resume launch calls = %d, want 1", launchCalls)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("focus dispatched %d prompts, want none", len(agentMgr.capturedPrompts))
	}
	focusedAt := time.Now().UTC()
	key := idleParkingCandidateKey{
		workspaceID: "workspace-focus-idle-suspended", sessionID: "session-focus-idle-suspended",
		executionID: "execution-focus-resumed", idleSince: focusedAt.Add(-time.Hour).UnixNano(),
		policyUpdated: focusedAt.Add(-2 * time.Hour).UnixNano(),
	}
	if svc.idleParkingCandidateDue(key, focusedAt.Add(30*time.Second), time.Minute) {
		t.Fatal("focus recovery should start a fresh idle-suspension interval")
	}
	if !svc.idleParkingCandidateDue(key, focusedAt.Add(time.Minute), time.Minute) {
		t.Fatal("focused session should be eligible after the new idle interval")
	}
}
