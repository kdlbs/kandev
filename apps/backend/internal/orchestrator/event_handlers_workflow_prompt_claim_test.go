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

// postClaimReloadErrorRepo fails the guarded reload that follows a successful
// prompt claim. The turn service arms it only after that claim has persisted
// the RUNNING state and created the turn.
type postClaimReloadErrorRepo struct {
	sessionExecutorStore
	targetSessionID string
	err             error
	mu              sync.Mutex
	failNextReload  bool
}

func (r *postClaimReloadErrorRepo) armReloadFailure() {
	r.mu.Lock()
	r.failNextReload = true
	r.mu.Unlock()
}

func (r *postClaimReloadErrorRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	r.mu.Lock()
	fail := sessionID == r.targetSessionID && r.failNextReload
	if fail {
		r.failNextReload = false
	}
	r.mu.Unlock()
	if fail {
		return nil, r.err
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

type postClaimReloadErrorTurnService struct {
	*repoBackedTurnService
	repo *postClaimReloadErrorRepo
}

func (s *postClaimReloadErrorTurnService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.repoBackedTurnService.StartTurn(ctx, sessionID)
	if err == nil {
		s.repo.armReloadFailure()
	}
	return turn, err
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

func TestWorkflowAutoStartRollsBackPromptClaimWhenPostClaimReloadFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-reload-error", "session-auto-start-reload-error", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-reload-error")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-auto-start-reload-error")

	reloadErr := errors.New("post-claim session reload failed")
	failingRepo := &postClaimReloadErrorRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		err:                  reloadErr,
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	queue := messagequeue.NewServiceMemory(testLogger())
	if _, err := queue.QueueMessage(ctx, session.ID, session.TaskID, "handoff", "", messagequeue.QueuedByUser, true, nil); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.repo = failingRepo
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = queue
	svc.turnService = &postClaimReloadErrorTurnService{
		repoBackedTurnService: &repoBackedTurnService{repo: repo},
		repo:                  failingRepo,
	}

	err = svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false)
	if !errors.Is(err, reloadErr) {
		t.Fatalf("auto-start error = %v, want post-claim reload error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after reload failure = %s, want WAITING_FOR_INPUT", stored.State)
	}
	if active, err := repo.GetActiveTurnBySessionID(ctx, session.ID); err == nil || active != nil {
		t.Fatalf("active turn after reload failure = %#v, %v; want no active turn", active, err)
	}
	turns, err := repo.ListTurnsBySession(ctx, session.ID)
	requireNoError(t, err)
	if len(turns) != 1 || turns[0].CompletedAt == nil {
		t.Fatalf("turns after reload failure = %#v, want one completed rollback turn", turns)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("prompt dispatches = %d, want none after reload failure", len(agentMgr.capturedPrompts))
	}
	queued, ok := queue.TakeQueued(ctx, session.ID)
	if !ok || queued.Content != "handoff" {
		t.Fatalf("requeued handoff = %#v, ok=%t; want original handoff", queued, ok)
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
