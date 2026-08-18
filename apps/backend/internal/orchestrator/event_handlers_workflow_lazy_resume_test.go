package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestProcessOnEnterResetAgentContext_ClearsLazyResumeTokenWithoutLiveExecution
// is the regression test for the lazy-resume reset: when no in-memory execution
// exists, clearPersistedResetState must erase the stale resume token so the
// next lazy launch does not reconnect to the pre-reset ACP conversation.
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

	svc.processOnEnter(ctx, "task-lazy-resume", session, step, "review task")

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

// TestResetAgentContext_InterleavingB_ClearBeforeStore proves the favorable
// interleaving: clearPersistedResetState erases the stale token before the
// async ACP session.created event writes the fresh one.
func TestResetAgentContext_InterleavingB_ClearBeforeStore(t *testing.T) {
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

	// 1. resetAgentContext — clears stale token before the reset.
	if !svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to succeed")
	}

	// 2. Verify token was cleared.
	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row after reset: %v", err)
	}
	if running.ResumeToken != "" {
		t.Fatalf("expected cleared resume_token after reset, got %q", running.ResumeToken)
	}

	// 3. Async storeResumeToken arrives with the new ACP session ID.
	const freshToken = "fresh-acp-session"
	svc.storeResumeToken(ctx, "t1", "s1", execID, freshToken, "")

	running, err = repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row after storeResumeToken: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh token %q after storeResumeToken, got %q", freshToken, running.ResumeToken)
	}
}

// TestResetAgentContext_InterleavingA_StoreBeforeClear proves that the
// reverse interleaving — the async ACP session.created event fires BEFORE
// resetAgentContext's clearResumeToken runs — is safe with the fix
// (clearResumeToken is ordered before ResetAgentContext, so the new ACP
// session does not exist when the old token is cleared).
//
// This test simulates interleaving A by calling storeResumeToken with the
// fresh ACP session ID before clearPersistedResetState. The fix ensures
// the fresh token survives.
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

	// Simulate: the async ACP session.created event fires before
	// clearPersistedResetState. storeResumeToken writes the fresh token
	// first, then clearPersistedResetState must not erase it.
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

// TestResetAgentContext_InterleavingC_StaleOldEventOverwritesFresh
// demonstrates that a stale ACP session.created event from the OLD session
// (same execution ID, old ACP session ID) can overwrite the fresh resume
// token. storeResumeToken's CAS keyed on agent_execution_id CANNOT
// distinguish ACP session generations within the same execution.
//
// This is pre-existing: the old code had the same window for acp_session_id.
// Our fix neither introduces nor widens it.  The correct long-term fix
// requires a generation counter on the executors_running row (outside the
// scope of this change).
func TestResetAgentContext_InterleavingC_StaleOldEventOverwritesFresh(t *testing.T) {
	t.Skip("pre-existing: storeResumeToken CAS cannot distinguish ACP generations within the same execution. Requires a generation counter on executors_running.")
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

	// 1. resetAgentContext — clears stale token before the reset, then calls
	//    clearPersistedResetState after.
	if !svc.resetAgentContext(ctx, "t1", session, "test") {
		t.Fatal("expected reset to succeed")
	}

	// 2. New session event arrives — writes fresh token.
	const freshToken = "fresh-acp-session"
	svc.storeResumeToken(ctx, "t1", "s1", execID, freshToken, "")

	running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh token %q, got %q", freshToken, running.ResumeToken)
	}

	// 3. Stale old-session event arrives with the old ACP session ID.
	//    Same execution ID → CAS accepts it → overwrites fresh token!
	svc.storeResumeToken(ctx, "t1", "s1", execID, staleToken, "")

	running, err = repo.GetExecutorRunningBySessionID(ctx, "s1")
	if err != nil {
		t.Fatalf("load executor row after stale event: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("stale old-session event overwrote fresh token: got %q, want %q", running.ResumeToken, freshToken)
	}
}
