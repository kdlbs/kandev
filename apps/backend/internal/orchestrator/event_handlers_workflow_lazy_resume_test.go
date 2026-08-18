package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

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

// TestResetAgentContext_ClearsThenAllowsFreshStoreResumeToken proves that the
// live-execution reset path clears the stale resume token AND that a subsequent
// storeResumeToken (mimicking the async ACP session.created event from the new
// session) can write the fresh token without interference — there is no race
// between clearPersistedResetState and a concurrent storeResumeToken because
// the event that triggers storeResumeToken for the new session arrives
// asynchronously from the agent's streaming goroutine AFTER the reset call
// returns.
//
// Race analysis (see also the structural proof in the function body of
// resetAgentContext in event_handlers_workflow.go):
//   - resetAgentContext calls agentManager.ResetAgentContext synchronously.
//     ResetAgentContext (manager_interaction.go) creates the new ACP session
//     via agentctl.ResetSession() — a synchronous RPC — and does NOT publish
//     an OnACPSessionCreated event (that event is published only by the fresh-
//     session and workspace-rebind paths).
//   - After ResetAgentContext returns, clearPersistedResetState runs on the
//     same goroutine and clears the resume token.
//   - The ACP session.created streaming event from the agent's new session
//     arrives later (on a separate WebSocket reader goroutine) and triggers
//     storeResumeToken via handleSessionStatusEvent — so the clear
//     deterministically precedes the store of the fresh token.
//   - The only path where storeResumeToken could race is a tail-end event from
//     the OLD ACP session arriving after the reset clear, but that stale event
//     carries the old ACP session ID and would overwrite with a defunct value.
//     This is a pre-existing concern unrelated to our fix (the old code already
//     cleared acp_session_id after ResetAgentContext).  The event bus's
//     QueueSubscribe already serializes event delivery per queue, so a stale
//     event from the old session cannot interleave in the middle of the reset
//     call itself.
func TestResetAgentContext_ClearsThenAllowsFreshStoreResumeToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-live-reset", "session-live-reset", "step-work")

	// Seed a live executor row with a stale resume token and a known
	// agent_execution_id so storeResumeToken's CAS can validate.
	const execID = "exec-live-reset"
	const staleToken = "old-acp-session"
	seedExecutorRunning(t, repo, "session-live-reset", "task-live-reset", execID)

	// Write a stale resume token directly (seedExecutorRunning doesn't set one).
	if err := repo.UpdateResumeToken(ctx, "session-live-reset", execID, staleToken, ""); err != nil {
		t.Fatalf("seed stale resume token: %v", err)
	}
	// Verify precondition.
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-live-reset")
	if err != nil {
		t.Fatalf("precondition: load executor row: %v", err)
	}
	if running.ResumeToken != staleToken {
		t.Fatalf("precondition: expected resume_token %q, got %q", staleToken, running.ResumeToken)
	}

	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)

	session, err := repo.GetTaskSession(ctx, "session-live-reset")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	// Step 1: resetAgentContext with a live execution — this clears the stale
	// resume token via clearPersistedResetState -> clearResumeToken.
	if !svc.resetAgentContext(ctx, "task-live-reset", session, "test") {
		t.Fatal("expected live-execution reset to succeed")
	}
	// The mock's ResetAgentContext just records the call; verify it was exercised.
	if len(agentManager.restartProcessCalls) != 1 || agentManager.restartProcessCalls[0] != execID {
		t.Fatalf("expected ResetAgentContext(execID=%q), got %v", execID, agentManager.restartProcessCalls)
	}

	// Step 2: verify the stale token was cleared.
	running, err = repo.GetExecutorRunningBySessionID(ctx, "session-live-reset")
	if err != nil {
		t.Fatalf("load executor row after reset: %v", err)
	}
	if running.ResumeToken != "" {
		t.Fatalf("expected cleared resume_token after reset, got %q", running.ResumeToken)
	}

	// Step 3: simulate the async ACP session.created event arriving after the
	// reset. The new session gets a fresh ACP session ID. storeResumeToken uses
	// a CAS on agent_execution_id — since the execution didn't rotate (same
	// execID), the write succeeds.
	const freshToken = "new-acp-session-after-reset"
	svc.storeResumeToken(ctx, "task-live-reset", "session-live-reset", execID, freshToken, "")

	running, err = repo.GetExecutorRunningBySessionID(ctx, "session-live-reset")
	if err != nil {
		t.Fatalf("load executor row after storeResumeToken: %v", err)
	}
	if running.ResumeToken != freshToken {
		t.Fatalf("expected fresh resume_token %q after storeResumeToken, got %q", freshToken, running.ResumeToken)
	}

	// Repeat: a second clearPersistedResetState (e.g., from a subsequent step
	// re-entry) must again clear the fresh token.
	svc.clearPersistedResetState(ctx, "session-live-reset", session)
	running, err = repo.GetExecutorRunningBySessionID(ctx, "session-live-reset")
	if err != nil {
		t.Fatalf("load executor row after second clear: %v", err)
	}
	if running.ResumeToken != "" {
		t.Fatalf("expected cleared resume_token after second clear, got %q", running.ResumeToken)
	}
}
