package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type failResumeTokenUpdateRepo struct {
	sessionExecutorStore
	err error
}

func (r *failResumeTokenUpdateRepo) UpdateResumeToken(context.Context, string, string, string, string) error {
	return r.err
}

// TestProcessOnEnterResetAgentContext_ClearsLazyResumeTokenWithoutLiveExecution
// is the regression test for the lazy-resume reset: when no in-memory execution
// exists, resetAgentContext must erase the stale resume token so the next lazy
// launch does not reconnect to the pre-reset ACP conversation.
func TestProcessOnEnterResetAgentContext_ClearsLazyResumeTokenWithoutLiveExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-lazy-resume", "session-lazy-resume", "step-work")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "session-lazy-resume",
		SessionID:   "session-lazy-resume",
		TaskID:      "task-lazy-resume",
		ResumeToken: "old-acp-session",
		Status:      "stopped",
	}); err != nil {
		t.Fatalf("seed resumable execution: %v", err)
	}

	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	step := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "workflow-1", Name: "Review",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
		}},
	}
	session, err := repo.GetTaskSession(ctx, "session-lazy-resume")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	svc.processOnEnter(ctx, "task-lazy-resume", session, step, "review task", 0)

	if len(agentManager.restartProcessCalls) != 0 {
		t.Fatalf("expected no reset against a missing live execution, got %d calls", len(agentManager.restartProcessCalls))
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-lazy-resume")
	if err != nil {
		t.Fatalf("load resumable execution: %v", err)
	}
	if running.ResumeToken != "" {
		t.Fatalf("reset must clear lazy resume before the next agent turn, got %q", running.ResumeToken)
	}
}

func TestProcessOnEnterResetAgentContext_ReportsLazyResumeTokenClearFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-lazy-resume-error", "session-lazy-resume-error", "step-work")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "session-lazy-resume-error",
		SessionID:   "session-lazy-resume-error",
		TaskID:      "task-lazy-resume-error",
		ResumeToken: "old-acp-session",
		Status:      "stopped",
	}); err != nil {
		t.Fatalf("seed resumable execution: %v", err)
	}

	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.repo = &failResumeTokenUpdateRepo{
		sessionExecutorStore: repo,
		err:                  errors.New("resume token store unavailable"),
	}
	session, err := repo.GetTaskSession(ctx, "session-lazy-resume-error")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if svc.resetAgentContext(ctx, "task-lazy-resume-error", session, "review") {
		t.Fatal("expected reset to fail when lazy resume token cannot be cleared")
	}
	if len(agentManager.restartProcessCalls) != 0 {
		t.Fatalf("expected no provider reset without a live execution, got %d calls", len(agentManager.restartProcessCalls))
	}
}

func TestResetAgentContext_FailedProviderResetRetainsResumeToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-reset-token", "s-reset-token", "step1")
	seedExecutorRunning(t, repo, "s-reset-token", "t-reset-token", "exec-reset-token")
	if err := repo.UpdateResumeToken(ctx, "s-reset-token", "exec-reset-token", "old-acp-session", ""); err != nil {
		t.Fatalf("seed stale resume token: %v", err)
	}

	svc := createTestServiceWithAgent(
		repo,
		newMockStepGetter(),
		newMockTaskRepo(),
		&mockAgentManager{
			repoForExecutionLookup: repo,
			restartProcessErr:      errors.New("provider reset failed"),
		},
	)
	session, err := repo.GetTaskSession(ctx, "s-reset-token")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if svc.resetAgentContext(ctx, "t-reset-token", session, "review") {
		t.Fatal("expected provider reset to fail")
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "s-reset-token")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != "old-acp-session" {
		t.Fatalf("failed reset must retain recovery token, got %q", running.ResumeToken)
	}
}

// TestResetAgentContext_InterleavingB_ClearBeforeStore proves that a successful
// reset persists the fresh token before an async session-created event arrives.
func TestResetAgentContext_InterleavingB_ClearBeforeStore(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const execID = "exec-1"
	const staleToken = "old-acp-session"
	const freshToken = "fresh-acp-session"
	seedExecutorRunning(t, repo, "s1", "t1", execID)
	if err := repo.UpdateResumeToken(ctx, "s1", execID, staleToken, ""); err != nil {
		t.Fatalf("seed stale resume token: %v", err)
	}

	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		getACPSessionIDForSessionFunc: func(string) (string, bool) {
			return freshToken, true
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	// 1. resetAgentContext resets the provider, clears stale state, and
	//    persists the current ACP session synchronously.
	if !svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to succeed")
	}

	// 2. Verify the old token was replaced without waiting for an async event.
	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row after reset: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh resume_token after reset, got %q", running.ResumeToken)
	}

	// 3. An async session-created event with the same current ID remains
	//    idempotent.
	svc.storeResumeToken(ctx, "t1", "s1", execID, freshToken, "")

	running, err = repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row after storeResumeToken: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh token %q after storeResumeToken, got %q", freshToken, running.ResumeToken)
	}
}

// TestResetAgentContext_InterleavingA_StoreBeforeClear proves that metadata
// cleanup does not clear a token that was already persisted by the fresh ACP
// session event.
func TestResetAgentContext_InterleavingA_StoreBeforeClear(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const execID = "exec-1"
	const staleToken = "old-acp-session"
	seedExecutorRunning(t, repo, "s1", "t1", execID)
	if err := repo.UpdateResumeToken(ctx, "s1", execID, staleToken, ""); err != nil {
		t.Fatalf("seed stale resume token: %v", err)
	}

	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	// Simulate the async ACP session.created event arriving before the remaining
	// reset metadata is cleared. The token is handled separately and survives.
	const freshToken = "fresh-acp-session"
	svc.storeResumeToken(ctx, "t1", "s1", execID, freshToken, "")
	svc.clearPersistedResetState(ctx, "s1", session)

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken == "" {
		t.Fatal("RACE: clearResumeToken erased the fresh token written by storeResumeToken")
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh token %q to survive, got %q", freshToken, running.ResumeToken)
	}
}

// TestResetAgentContext_InterleavingC_StaleOldEventOverwritesFresh proves that
// the current ACP session ID filters a delayed event from the old session even
// though both sessions share one lifecycle execution ID.
func TestResetAgentContext_InterleavingC_StaleOldEventDoesNotOverwriteFresh(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const execID = "exec-1"
	const staleToken = "old-acp-session"
	seedExecutorRunning(t, repo, "s1", "t1", execID)
	if err := repo.UpdateResumeToken(ctx, "s1", execID, staleToken, ""); err != nil {
		t.Fatalf("seed stale resume token: %v", err)
	}

	const freshToken = "fresh-acp-session"
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		getACPSessionIDForSessionFunc: func(string) (string, bool) {
			return freshToken, true
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	// 1. Reset the provider and persist the fresh ACP session.
	if !svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to succeed")
	}

	// 2. New session event arrives — writes the fresh token.
	svc.storeResumeToken(ctx, "t1", "s1", execID, freshToken, "")

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh token %q, got %q", freshToken, running.ResumeToken)
	}

	// 3. A stale old-session event arrives with the old ACP session ID. The
	//    execution ID is unchanged, but the ACP generation check rejects it.
	svc.storeResumeToken(ctx, "t1", "s1", execID, staleToken, "")

	running, err = repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row after stale event: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("stale old-session event overwrote fresh token: got %q, want %q", running.ResumeToken, freshToken)
	}
}

// TestResetAgentContext_FailureAfterLiveSessionMovedReconcilesState covers a
// deeper shape of "preserve recoverability on ResetAgentContext failure" than
// TestResetAgentContext_FailedProviderResetRetainsResumeToken: the lifecycle
// manager's ResetAgentContext can commit the execution's live ACP session to
// a NEW id (ResetSession itself succeeded) and only fail afterward — e.g. a
// later re-apply-session-model step erroring — so it still returns a
// non-nil error overall. Leaving the persisted resume_token at its pre-reset
// value in that case does not preserve a resumable session: the agent has
// already moved on and no longer recognizes the old ACP session, so the
// stale token can never be resumed. resetAgentContext must reconcile the
// persisted token to the live truth even when it reports failure.
func TestResetAgentContext_FailureAfterLiveSessionMovedReconcilesState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const execID = "exec-1"
	const staleToken = "old-acp-session"
	const movedToken = "new-acp-session-committed-before-failure"
	seedExecutorRunning(t, repo, "s1", "t1", execID)
	if err := repo.UpdateResumeToken(ctx, "s1", execID, staleToken, ""); err != nil {
		t.Fatalf("seed stale resume token: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", contextWindowMetadataKey, map[string]interface{}{
		"size": int64(200000), "used": int64(190000),
	}); err != nil {
		t.Fatalf("seed context window: %v", err)
	}

	var agentManager *mockAgentManager
	agentManager = &mockAgentManager{
		repoForExecutionLookup: repo,
		restartProcessErr:      errors.New("model reapplication failed after session reset"),
		getACPSessionIDForSessionFunc: func(string) (string, bool) {
			if len(agentManager.restartProcessCalls) == 0 {
				return staleToken, true
			}
			return movedToken, true
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	staleContextGeneration := svc.captureContextWindowGeneration("s1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to report failure")
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != movedToken {
		t.Fatalf("failed reset left a dead token persisted: got %q, want live session %q",
			running.ResumeToken, movedToken)
	}
	updated, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session after failed reset: %v", err)
	}
	if updated.Metadata[contextWindowMetadataKey] != nil {
		t.Fatalf("partial reset retained context from the old ACP session: %#v",
			updated.Metadata[contextWindowMetadataKey])
	}
	persisted, _, err := svc.persistContextWindowUpdate(ctx, "s1", staleContextGeneration, map[string]interface{}{
		"size": int64(200000), "used": int64(195000),
	})
	if err != nil {
		t.Fatalf("persist stale context update: %v", err)
	}
	if persisted {
		t.Fatal("context update captured before the partial reset was persisted")
	}
}

// TestResetAgentContext_FailureWithUnchangedLiveSessionPreservesToken pins the
// other half of the reconcile contract: when ResetAgentContext fails and the
// live ACP session did NOT move, the still-valid token and recovery cursor must
// survive. The observable guarantee from #2765 ("a failed reset keeps a usable
// recovery token") is unchanged. Without this test, reconciling unchanged state
// would blank a valid cursor and no test would notice.
func TestResetAgentContext_FailureWithUnchangedLiveSessionPreservesToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const execID = "exec-1"
	const liveToken = "acp-session-that-never-moved"
	const lastMessageUUID = "last-message-before-failed-reset"
	seedExecutorRunning(t, repo, "s1", "t1", execID)
	if err := repo.UpdateResumeToken(ctx, "s1", execID, liveToken, lastMessageUUID); err != nil {
		t.Fatalf("seed resume token: %v", err)
	}

	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		restartProcessErr:      errors.New("provider reset failed before touching the session"),
		getACPSessionIDForSessionFunc: func(string) (string, bool) {
			return liveToken, true
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to report failure")
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != liveToken {
		t.Fatalf("failed reset destroyed a still-valid token: got %q, want %q",
			running.ResumeToken, liveToken)
	}
	if running.LastMessageUUID != lastMessageUUID {
		t.Fatalf("failed reset destroyed the recovery cursor: got %q, want %q",
			running.LastMessageUUID, lastMessageUUID)
	}
}

// TestResetAgentContext_FailureFromRotatedExecutionDoesNotOverwrite proves that
// reconciliation stays scoped to the execution that attempted the reset. If
// the executors_running row has already rotated, the failed reset belongs to a
// defunct execution and must not change the live execution's state.
func TestResetAgentContext_FailureFromRotatedExecutionDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const rotatedInExecID = "exec-new"
	const staleExecID = "exec-old"
	const liveExecToken = "token-owned-by-new-execution"

	// The row has already rotated to the newer execution and carries its token.
	seedExecutorRunning(t, repo, "s1", "t1", rotatedInExecID)
	if err := repo.UpdateResumeToken(ctx, "s1", rotatedInExecID, liveExecToken, ""); err != nil {
		t.Fatalf("seed resume token: %v", err)
	}

	var agentManager *mockAgentManager
	agentManager = &mockAgentManager{
		// The reset is still operating on the older execution.
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return staleExecID, nil
		},
		restartProcessErr: errors.New("model reapplication failed after session reset"),
		getACPSessionIDForSessionFunc: func(string) (string, bool) {
			if len(agentManager.restartProcessCalls) == 0 {
				return "acp-session-before-defunct-reset", true
			}
			return "acp-session-of-defunct-execution", true
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to report failure")
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != liveExecToken {
		t.Fatalf("defunct execution overwrote the live execution's token: got %q, want %q",
			running.ResumeToken, liveExecToken)
	}
}

// TestResetAgentContext_FailureReconcileDroppedWhenSessionMovesMidFlight models
// the race the reconcile write is exposed to: the ACP session can move again
// between resetAgentContext reading the live id and storeResumeToken
// re-reading it for its stale-generation guard. Staged deterministically by
// returning a different id on each lookup — the first is what the reconcile
// tries to persist, the second is the newer generation the guard compares
// against. The reconcile must lose: a fresher generation already owns the
// session, and its own event will persist the correct token.
func TestResetAgentContext_FailureReconcileDroppedWhenSessionMovesMidFlight(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const execID = "exec-1"
	const seededToken = "acp-session-before-reset"
	seedExecutorRunning(t, repo, "s1", "t1", execID)
	if err := repo.UpdateResumeToken(ctx, "s1", execID, seededToken, ""); err != nil {
		t.Fatalf("seed resume token: %v", err)
	}

	var lookups int
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		restartProcessErr:      errors.New("model reapplication failed after session reset"),
		getACPSessionIDForSessionFunc: func(string) (string, bool) {
			lookups++
			switch lookups {
			case 1:
				return seededToken, true
			case 2:
				// What resetAgentContext observes and tries to reconcile to.
				return "acp-session-observed-by-reconcile", true
			default:
				// A newer generation took over before the guard re-read it.
				return "acp-session-from-newer-generation", true
			}
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to report failure")
	}
	if lookups < 3 {
		t.Fatalf("expected the reconcile write to be re-checked against the live id, got %d lookups", lookups)
	}

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != seededToken {
		t.Fatalf("stale reconcile beat a newer ACP session generation: got %q, want %q",
			running.ResumeToken, seededToken)
	}
}
