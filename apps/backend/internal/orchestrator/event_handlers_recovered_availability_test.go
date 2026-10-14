package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type agentManagerWithoutPromptGeneration struct {
	executor.AgentManagerClient
}

func TestRecoveredAvailabilityBootReadyDoesNotDowngradeRunningPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.ErrorMessage = "preserve the active prompt state"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("seed session error: %v", err)
	}

	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		isAgentRunning:           true,
		isAgentReadyFn:           func(context.Context, string) bool { return true },
		currentPromptExecutionID: "exec-1",
	}
	agentMgr.currentPromptGeneration.Store(7)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		Status: string(v1.AgentStatusReady), PromptGeneration: 7, RecoveredAvailabilityOnly: true,
	})

	current, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if current.State != models.TaskSessionStateRunning {
		t.Fatalf("late availability changed session state to %q, want RUNNING", current.State)
	}
	if current.ErrorMessage != "preserve the active prompt state" {
		t.Fatalf("late availability changed session error to %q", current.ErrorMessage)
	}
}

func TestRecoveredAvailabilityBootReadyRequiresCurrentOwnerAndGeneration(t *testing.T) {
	cases := []struct {
		name                    string
		execution               string
		generation              uint64
		managerGeneration       uint64
		ownerLookupOverride     bool
		ownerLookupID           string
		ownerLookupErr          error
		generationLookupErr     error
		generationReaderMissing bool
		ready                   bool
	}{
		{name: "replaced execution", execution: "exec-old", generation: 4, managerGeneration: 4, ready: true},
		{name: "newer prompt generation", execution: "exec-1", generation: 3, managerGeneration: 4, ready: true},
		{name: "missing current owner", execution: "exec-1", generation: 4, managerGeneration: 4,
			ownerLookupOverride: true, ready: true},
		{name: "owner read failed", execution: "exec-1", generation: 4, managerGeneration: 4,
			ownerLookupErr: errors.New("owner unavailable"), ready: true},
		{name: "generation read failed", execution: "exec-1", generation: 4, managerGeneration: 4,
			generationLookupErr: errors.New("generation unavailable"), ready: true},
		{name: "generation reader missing", execution: "exec-1", generation: 4, managerGeneration: 4,
			generationReaderMissing: true, ready: true},
		{name: "runtime not ready", execution: "exec-1", generation: 4, managerGeneration: 4, ready: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
			agentMgr := &mockAgentManager{
				repoForExecutionLookup:   repo,
				isAgentRunning:           true,
				isAgentReadyFn:           func(context.Context, string) bool { return tc.ready },
				currentPromptExecutionID: "exec-1",
			}
			agentMgr.currentPromptGeneration.Store(tc.managerGeneration)
			if tc.ownerLookupOverride || tc.ownerLookupErr != nil {
				agentMgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
					return tc.ownerLookupID, tc.ownerLookupErr
				}
			}
			if tc.generationLookupErr != nil {
				agentMgr.getPromptGenerationForSessionFunc = func(context.Context, string) (uint64, error) {
					return 0, tc.generationLookupErr
				}
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			if tc.generationReaderMissing {
				svc.agentManager = agentManagerWithoutPromptGeneration{AgentManagerClient: agentMgr}
			}
			if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued prompt", "", "user", false, nil); err != nil {
				t.Fatalf("queue prompt: %v", err)
			}

			svc.handleAgentBootReady(ctx, watcher.AgentEventData{
				TaskID: "t1", SessionID: "s1", AgentExecutionID: tc.execution,
				Status: string(v1.AgentStatusReady), PromptGeneration: tc.generation, RecoveredAvailabilityOnly: true,
			})

			if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
				t.Fatalf("queue count after stale availability = %d, want 1", got)
			}
			if len(agentMgr.capturedPrompts) != 0 {
				t.Fatalf("stale availability dispatched prompts: %v", agentMgr.capturedPrompts)
			}
		})
	}
}

func TestRecoveredAvailabilityBootReadyPreservesPausedQueue(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		isAgentRunning:           true,
		isAgentReadyFn:           func(context.Context, string) bool { return true },
		currentPromptExecutionID: "exec-1",
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	if err := svc.messageQueue.SetAutoRun(ctx, "s1", false); err != nil {
		t.Fatalf("pause automatic queue dispatch: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "paused prompt", "", "user", false, nil); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}

	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		Status: string(v1.AgentStatusReady), RecoveredAvailabilityOnly: true,
	})

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count != 1 || status.AutoRun {
		t.Fatalf("paused queue status = %+v, want one queued message with AutoRun disabled", status)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("availability event dispatched paused prompts: %v", agentMgr.capturedPrompts)
	}
}

func TestRecoveredAvailabilityBootReadyRejectsTaggedWrongAttempt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		isAgentRunning:           true,
		isAgentReadyFn:           func(context.Context, string) bool { return true },
		currentPromptExecutionID: "exec-1",
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	registry := svc.resumeAttemptStore()
	attempt, _ := registry.begin(ctx, "t1", "s1")
	attempt.setExecutionID("exec-1")
	attempt.finish(registry)
	wrongAttemptID := resumeAttemptIdentityPrefix + "1"
	if wrongAttemptID == attempt.identity() {
		wrongAttemptID = resumeAttemptIdentityPrefix + "2"
	}
	if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued prompt", "", "user", false, nil); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}

	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		AttemptID: wrongAttemptID, Status: string(v1.AgentStatusReady),
		RecoveredAvailabilityOnly: true,
	})

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("tagged stale availability queue count = %d, want 1", got)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("tagged stale availability dispatched prompts: %v", agentMgr.capturedPrompts)
	}
}
