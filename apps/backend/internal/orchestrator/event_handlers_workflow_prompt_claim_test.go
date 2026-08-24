package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// terminalizeAfterPromotionRepo pauses the prompt claim after the session has
// been promoted, so the test can reproduce a terminal transition in that gap.
type terminalizeAfterPromotionRepo struct {
	sessionExecutorStore
	targetSessionID string
	blockOnGetCall  int
	returnStale     bool
	claimReached    chan struct{}
	allowClaim      chan struct{}
	mu              sync.Mutex
	getCalls        int
	once            sync.Once
}

// dispatchBoundaryAgentManager blocks after agentctl has accepted the prompt,
// giving the test a deterministic way to inspect the guard after admission but
// before the prompt call returns.
type dispatchBoundaryAgentManager struct {
	*mockAgentManager
	dispatched chan struct{}
	release    chan struct{}
}

func (m *dispatchBoundaryAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	result, err := m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
	if err != nil {
		return result, err
	}
	onDispatched()
	close(m.dispatched)
	<-m.release
	return result, nil
}

func (r *terminalizeAfterPromotionRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	r.mu.Lock()
	r.getCalls++
	shouldBlock := sessionID == r.targetSessionID && r.getCalls == r.blockOnGetCall
	r.mu.Unlock()
	if shouldBlock {
		if r.returnStale {
			session, err := r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
			r.once.Do(func() { close(r.claimReached) })
			<-r.allowClaim
			return session, err
		}
		r.once.Do(func() { close(r.claimReached) })
		<-r.allowClaim
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

func TestWorkflowAutoStartPromptClaimRejectsSessionTerminalizedAfterPromotion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-terminalized-reuse", "session-terminalized-reuse", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-terminalized-reuse")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))

	svc := &Service{repo: repo, logger: testLogger()}
	promoted, err := svc.setNonterminalSessionPrimary(ctx, session.ID)
	requireNoError(t, err)
	if !promoted {
		t.Fatal("expected nonterminal session to be promoted")
	}

	barrierRepo := &terminalizeAfterPromotionRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		blockOnGetCall:       1,
		claimReached:         make(chan struct{}),
		allowClaim:           make(chan struct{}),
	}
	svc.repo = barrierRepo
	errCh := make(chan error, 1)
	go func() {
		_, _, _, _, _, claimErr := svc.claimSessionRunningForPrompt(
			ctx, session.TaskID, session.ID, "", false, nil, nil, "", true,
		)
		errCh <- claimErr
	}()

	<-barrierRepo.claimReached
	requireNoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCompleted, "finished concurrently"))
	close(barrierRepo.allowClaim)

	if err := <-errCh; !errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
		t.Fatalf("claim error = %v, want terminalized workflow auto-start error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateCompleted {
		t.Fatalf("terminalized session state = %s, want COMPLETED", stored.State)
	}
}

func TestWorkflowAutoStartRejectsSessionTerminalizedBeforeResume(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-terminalized", "session-auto-start-terminalized", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-terminalized")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))

	barrierRepo := &terminalizeAfterPromotionRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		blockOnGetCall:       1,
		returnStale:          true,
		claimReached:         make(chan struct{}),
		allowClaim:           make(chan struct{}),
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	queue := messagequeue.NewServiceMemory(testLogger())
	if _, err := queue.QueueMessage(ctx, session.ID, session.TaskID, "handoff", "", messagequeue.QueuedByUser, true, nil); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	svc := &Service{
		repo:         barrierRepo,
		logger:       testLogger(),
		agentManager: agentMgr,
		executor:     executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}),
		messageQueue: queue,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false)
	}()

	<-barrierRepo.claimReached
	requireNoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCompleted, "finished concurrently"))
	close(barrierRepo.allowClaim)

	if err := <-errCh; !errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
		t.Fatalf("auto-start error = %v, want terminalized workflow auto-start error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateCompleted {
		t.Fatalf("terminalized session state = %s, want COMPLETED", stored.State)
	}
	if status := queue.GetStatus(ctx, session.ID); status.Count != 1 {
		t.Fatalf("queued handoff count = %d, want 1 after terminal auto-start rejection", status.Count)
	}
}

func TestWorkflowAutoStartReleasesTerminalGuardAfterPromptAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-dispatch", "session-auto-start-dispatch", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-dispatch")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-auto-start-dispatch")

	agentMgr := &dispatchBoundaryAgentManager{
		mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true},
		dispatched:       make(chan struct{}),
		release:          make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false)
	}()

	<-agentMgr.dispatched
	guard, release := svc.acquireCancelInFlightGuard(session.ID)
	acquired := make(chan struct{})
	go func() {
		guard.Lock()
		close(acquired)
		guard.Unlock()
		release()
	}()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("workflow auto-start held the terminal guard after agentctl accepted the prompt")
	}
	close(agentMgr.release)
	if err := <-errCh; err != nil {
		t.Fatalf("auto-start prompt: %v", err)
	}
}
