package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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
	svc := &Service{
		repo:         barrierRepo,
		logger:       testLogger(),
		agentManager: agentMgr,
		executor:     executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}),
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
}
