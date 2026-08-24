package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// terminalizeAfterPromotionRepo pauses the prompt claim after the session has
// been promoted, so the test can reproduce a terminal transition in that gap.
type terminalizeAfterPromotionRepo struct {
	sessionExecutorStore
	targetSessionID string
	claimReached    chan struct{}
	allowClaim      chan struct{}
	once            sync.Once
}

func (r *terminalizeAfterPromotionRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if sessionID == r.targetSessionID {
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
