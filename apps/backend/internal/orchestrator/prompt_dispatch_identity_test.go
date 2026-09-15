package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type modelSwitchGateAgentManager struct {
	*mockAgentManager
	switchEntered chan struct{}
	allowSwitch   chan struct{}
}

func (m *modelSwitchGateAgentManager) SetSessionModelBySessionID(context.Context, string, string) error {
	close(m.switchEntered)
	<-m.allowSwitch
	return nil
}

func TestPromptTaskExpectedIdentityRejectsReplacementAfterClaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-prompt-identity", "session-prompt-identity"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-prompt-identity")

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve session identity: %v", err)
	}

	_, err = svc.promptTask(
		ctx, taskID, sessionID, "queued prompt", "", false, nil, true,
		promptTaskOptions{
			expectedSessionIdentity: &identity,
			afterClaim: func() error {
				_, updateErr := repo.DB().ExecContext(
					ctx,
					`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`,
					"replacement-incarnation",
					sessionID,
				)
				return updateErr
			},
		},
	)
	if !errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
		t.Fatalf("promptTask error = %v, want session identity mismatch", err)
	}
	if got := len(agentMgr.capturedPrompts); got != 0 {
		t.Fatalf("replacement session prompts = %d, want 0", got)
	}
}

func TestPromptTaskReleasesDispatchGuardBeforeMissingExecutionRecovery(t *testing.T) {
	ctx := context.Background()
	const taskID, sessionID = "task-prompt-identity-recovery", "session-prompt-identity-recovery"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = "profile-prompt-identity-recovery"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session profile: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, Title: "Recovery task", State: v1.TaskStateInProgress}
	var executionLookups atomic.Int32
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			if executionLookups.Add(1) <= 2 {
				return "execution-prompt-identity-recovery", nil
			}
			return "", executor.ErrExecutionNotFound
		},
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, errors.New("fallback launch failed")
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
	svc.messageCreator = &mockMessageCreator{}
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve session identity: %v", err)
	}
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatalf("enable auto-run: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "queued prompt", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !ok || queued == nil {
		t.Fatalf("reserve queued prompt: queued=%+v ok=%v err=%v", queued, ok, err)
	}
	if reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(identity, queued.ID, queued); reservation == nil {
		t.Fatal("mark queued dispatch in flight returned nil")
	}

	attempt, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner || attempt == nil {
		t.Fatalf("begin resume attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	attempt.setExecutionID("execution-prompt-identity-recovery")
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })

	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.promptTask(
			ctx, taskID, sessionID, "queued prompt", "", false, nil, true,
			promptTaskOptions{
				claimEntryID:            queued.ID,
				expectedSessionIdentity: &identity,
				resumeAttempt:           attempt,
			},
		)
		promptDone <- promptErr
	}()

	select {
	case promptErr := <-promptDone:
		if promptErr == nil {
			t.Fatal("promptTask unexpectedly succeeded after fallback launch failure")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("promptTask deadlocked while missing-execution recovery reacquired the dispatch guard")
	}
	if hasCancelInFlightGuard(svc, sessionID) {
		t.Fatal("queued dispatch guard remained held after missing-execution recovery")
	}
}

func TestPromptTaskKeepsQueuedDispatchGuardThroughModelSwitch(t *testing.T) {
	ctx := context.Background()
	const taskID, sessionID = "task-prompt-model-switch", "session-prompt-model-switch"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-prompt-model-switch")
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session model: %v", err)
	}

	agentMgr := &modelSwitchGateAgentManager{
		mockAgentManager: &mockAgentManager{
			isAgentRunning:         true,
			repoForExecutionLookup: repo,
		},
		switchEntered: make(chan struct{}),
		allowSwitch:   make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
	t.Cleanup(func() {
		select {
		case <-agentMgr.allowSwitch:
		default:
			close(agentMgr.allowSwitch)
		}
		svc.clearAcceptedQueuedDispatch(sessionID)
		svc.stopSendNowWorkers()
	})
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve session identity: %v", err)
	}
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatalf("enable auto-run: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "queued prompt", "new-model", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !ok || queued == nil {
		t.Fatalf("reserve queued prompt: queued=%+v ok=%v err=%v", queued, ok, err)
	}
	reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(identity, queued.ID, queued)
	if reservation == nil {
		t.Fatal("mark queued dispatch in flight returned nil")
	}

	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.promptTask(
			ctx, taskID, sessionID, "queued prompt", "new-model", false, nil, true,
			promptTaskOptions{
				claimEntryID:            queued.ID,
				expectedSessionIdentity: &identity,
			},
		)
		promptDone <- promptErr
	}()
	select {
	case <-agentMgr.switchEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for model switch")
	}
	if !hasCancelInFlightGuard(svc, sessionID) {
		t.Fatal("queued dispatch guard was not held during model switch")
	}
	close(agentMgr.allowSwitch)
	select {
	case promptErr := <-promptDone:
		if promptErr != nil {
			t.Fatalf("promptTask after model switch: %v", promptErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("promptTask did not return after model switch")
	}
}
