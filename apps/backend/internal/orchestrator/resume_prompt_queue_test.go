package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type resumePromptQueueReadinessChecker interface {
	CheckQueueAdmissionReadiness(context.Context, messagequeue.QueueSessionIdentity)
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.3
// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.4
func TestResumePromptQueueServiceProvidesAdmissionReadinessCheck(t *testing.T) {
	_, ok := interface{}(&Service{}).(resumePromptQueueReadinessChecker)
	require.True(t, ok, "orchestrator Service must expose the queue admission readiness check")
}

func TestCheckQueueAdmissionReadinessDrainsAfterSessionBecomesReady(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateStarting)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)

	_, err = svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "resume me", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)

	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-1", models.TaskSessionStateWaitingForInput, ""))
	svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

func TestCheckQueueAdmissionReadinessRechecksAfterReadinessArrivesFirst(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	_, err = svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "resume me", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

func TestCheckQueueAdmissionReadinessHonorsAutoRunOff(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRunForSession(ctx, identity, false))

	_, err = svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "keep pending", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	svc.CheckQueueAdmissionReadiness(ctx, identity)

	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

func TestCheckQueueAdmissionReadinessRejectsReplacedSessionIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)

	_, err = svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "must stay with old session", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(
		ctx,
		`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`,
		"replacement-incarnation", "session-1",
	)
	require.NoError(t, err)

	svc.CheckQueueAdmissionReadiness(ctx, identity)

	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

func TestCheckQueueAdmissionReadinessDispatchesOneConcurrentHead(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "session-1", "task-1", "execution-1")
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	_, err = svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "dispatch once", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)

	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			<-start
			svc.CheckQueueAdmissionReadiness(ctx, identity)
		}()
	}
	close(start)
	wait.Wait()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the winning queued dispatch")
	}
	agentMgr.mu.Lock()
	captured := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()
	require.Equal(t, []string{"dispatch once"}, captured)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestCheckQueueAdmissionReadinessPreservesFIFOAndProviderAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-1", "task-1", "execution-1")
	prompts := make(chan string, 2)
	completed := make(chan struct{}, 2)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(ctx context.Context, _ string, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
			session, err := repo.GetTaskSession(ctx, "session-1")
			if err != nil {
				return nil, err
			}
			session.State = models.TaskSessionStateWaitingForInput
			if err := repo.UpdateTaskSession(ctx, session); err != nil {
				return nil, err
			}
			prompts <- prompt
			return &executor.PromptResult{}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	svc.messageQueue.SetAutoMergeEnabled(false)
	svc.onQueuedMessageExecutionComplete = func() { completed <- struct{}{} }
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	for _, prompt := range []string{"first queued", "second queued"} {
		_, err := svc.messageQueue.QueueMessageWithMetadataForSession(
			ctx, identity, prompt, "", messagequeue.QueuedByUser, false, nil, nil,
		)
		require.NoError(t, err)
	}

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	gotPrompts := make([]string, 0, 2)
	for range 2 {
		select {
		case prompt := <-prompts:
			gotPrompts = append(gotPrompts, prompt)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for queued prompt; got=%q", gotPrompts)
		}
	}
	for range 2 {
		select {
		case <-completed:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for queued delivery cleanup")
		}
	}

	require.Equal(t, []string{"first queued", "second queued"}, gotPrompts)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestCheckQueueAdmissionReadinessDoesNotBypassClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	seedPendingClarificationMessage(t, repo, "task-1", "session-1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	_, err = svc.messageQueue.QueueMessageWithMetadataForSession(
		ctx, identity, "during clarification", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
func TestCheckQueueAdmissionReadinessDoesNotBypassWIPWait(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	task.QueuedForStepID = "step-1"
	task.WIPAdmitted = false
	require.NoError(t, repo.UpdateTask(ctx, task))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	_, err = svc.messageQueue.QueueMessageWithMetadataForSession(
		ctx, identity, "during WIP wait", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)
}
