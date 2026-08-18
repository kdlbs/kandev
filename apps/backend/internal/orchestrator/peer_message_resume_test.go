package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// TestQueueAndInterruptForPeerMessage_StoppedSessionResumesAgent pins the
// "a message received from another task auto-starts the agent" contract
// behind the composer hint: a recovered-idle session (WAITING_FOR_INPUT with
// a resume token and no running agent — the post-restart shape the
// prevent-auto-start-on-open preference leaves it in) must be RESUMED by the
// queue dispatch when a peer (sub-task) message arrives, and the message must
// reach the agent. The preference only gates open-time starts in the
// frontend; message-driven starts go through ensureSessionRunning here.
func TestQueueAndInterruptForPeerMessage_StoppedSessionResumesAgent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: false, promptDone: make(chan struct{})}
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	msgCreator := &mockMessageCreator{}
	svc.messageCreator = msgCreator
	// Mirrors real agentctl bootstrap timing: LaunchAgent fires
	// handleAgentBootReady ~50ms later so the resume path completes in unit
	// tests without a real subprocess. Wrap the simulator to capture the
	// LaunchAgent request so we can assert the RESUME semantics (the seeded
	// ACP token must be passed through), not merely that some launch fired.
	wireBootReadySimulator(svc, agentMgr, "exec-resumed")
	var launchMu sync.Mutex
	var launchReqs []*executor.LaunchAgentRequest
	baseLaunch := agentMgr.launchAgentFunc
	agentMgr.launchAgentFunc = func(
		launchCtx context.Context,
		req *executor.LaunchAgentRequest,
	) (*executor.LaunchAgentResponse, error) {
		launchMu.Lock()
		launchReqs = append(launchReqs, req)
		launchMu.Unlock()
		return baseLaunch(launchCtx, req)
	}

	// The post-restart recovered-idle shape: session stopped with history,
	// resumable via its executors_running resume token, no agent process.
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = "profile-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "er1", SessionID: "session1", TaskID: "task1",
		ResumeToken: "acp-session-xyz",
		Resumable:   true,
		CreatedAt:   now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	queued, dispatched, err := svc.QueueAndInterruptForPeerMessage(
		ctx, "task1", "session1", "subtask status update",
		map[string]interface{}{"sender_task_id": "subtask-1"},
	)
	if err != nil {
		t.Fatalf("queue and interrupt for peer message: %v", err)
	}
	if queued == nil {
		t.Fatal("expected a queued entry")
	}
	if !dispatched {
		t.Fatal("expected QueueAndInterruptForPeerMessage to report the message as dispatched")
	}

	// The dispatch is async: join the executeQueuedMessage goroutine via the
	// mock's PromptAgent signal — PromptAgent only runs AFTER
	// ensureSessionRunning resumed the agent, so this proves both halves.
	select {
	case <-agentMgr.promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the peer message to be dispatched after resuming the agent")
	}
	agentMgr.mu.Lock()
	prompts := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()
	if len(prompts) != 1 || prompts[0] != "subtask status update" {
		t.Fatalf("expected the peer message to be dispatched to the agent, got prompts=%v", prompts)
	}

	// The agent must have been RESUMED by the dispatch: the launch the
	// boot-ready simulator wires into LaunchAgent must have registered the
	// new execution on the session's executors_running row, and it must have
	// carried the seeded ACP resume token (a fresh-start regression would
	// lose the prior ACP conversation but still satisfy the execution-ID
	// check).
	er, err := repo.GetExecutorRunningBySessionID(ctx, "session1")
	if err != nil || er == nil {
		t.Fatalf("ExecutorRunning lookup after dispatch failed: err=%v nil=%v", err, er == nil)
	}
	if er.AgentExecutionID != "exec-resumed" {
		t.Fatalf("expected the peer-message dispatch to resume the agent (execution exec-resumed), got %q", er.AgentExecutionID)
	}
	launchMu.Lock()
	defer launchMu.Unlock()
	if len(launchReqs) != 1 {
		t.Fatalf("expected exactly 1 resume launch, got %d", len(launchReqs))
	}
	if got := launchReqs[0].ACPSessionID; got != "acp-session-xyz" {
		t.Fatalf("expected the resume launch to carry the seeded ACP token, got %q", got)
	}
}
