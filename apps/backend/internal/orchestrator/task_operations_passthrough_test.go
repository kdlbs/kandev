package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// getTaskSessionErrorRepo wraps a sessionExecutorStore and forces
// GetTaskSession to fail for every call, regardless of session ID. Used to
// exercise resolveIsPassthroughForLaunch's fail-safe fallback.
type getTaskSessionErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r getTaskSessionErrorRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.err
}

// TestResolveIsPassthroughForLaunch_FailsSafeOnLookupError proves the fix for
// the coderabbitai review finding on PR #1865: a transient GetTaskSession
// error during the passthrough check must default to isPassthrough=true (skip
// the Kandev MCP wrap and "@name" prompt-reference expansion), not false.
// Failing open on a lookup error for a session that IS actually passthrough
// would inject the hidden <kandev-system> block into a real terminal PTY —
// exactly the leak this check exists to prevent.
func TestResolveIsPassthroughForLaunch_FailsSafeOnLookupError(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.repo = getTaskSessionErrorRepo{sessionExecutorStore: svc.repo, err: errors.New("transient lookup failure")}

	got := svc.resolveIsPassthroughForLaunch(context.Background(), "session1")

	if !got {
		t.Fatalf("resolveIsPassthroughForLaunch() on GetTaskSession error = %v, want true (fail-safe)", got)
	}
}

// TestResolveIsPassthroughForLaunch_UsesSessionSnapshotOnSuccess proves the
// success path is unchanged: it returns the session's stored IsPassthrough
// value rather than always defaulting true.
func TestResolveIsPassthroughForLaunch_UsesSessionSnapshotOnSuccess(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load seeded session: %v", err)
	}
	session.IsPassthrough = false
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to persist passthrough flag: %v", err)
	}

	got := svc.resolveIsPassthroughForLaunch(context.Background(), "session1")

	if got {
		t.Fatalf("resolveIsPassthroughForLaunch() = %v, want false (session snapshot IsPassthrough)", got)
	}
}

// TestPromptTask_PassthroughRoutesToPTYStdin walks the full Service.PromptTask
// → Executor.Prompt → promptPassthrough path for a passthrough session and
// asserts the prompt reaches PTY stdin (not the ACP PromptAgent) with the
// submit-key suffix appended. This is the closest-to-prod regression guard for
// issue #989: it exercises session-state housekeeping, prompt routing, and the
// passthrough primitives in one go via the real orchestrator service + real
// executor.
func TestPromptTask_PassthroughRoutesToPTYStdin(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isPassthrough:  true,
		isAgentRunning: true,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")

	result, err := svc.PromptTask(context.Background(), "task1", "session1", "deploy please", "", false, nil, false)
	if err != nil {
		t.Fatalf("PromptTask returned error: %v", err)
	}
	if result == nil || result.StopReason != "passthrough_dispatched" {
		t.Fatalf("expected passthrough_dispatched result, got %+v", result)
	}

	agentMgr.mu.Lock()
	stdinCalls := append([]passthroughStdinCall(nil), agentMgr.passthroughStdinCalls...)
	markCalls := append([]string(nil), agentMgr.markPassthroughCalls...)
	prompts := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()

	if len(stdinCalls) != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", len(stdinCalls))
	}
	if stdinCalls[0].SessionID != "session1" {
		t.Errorf("WritePassthroughStdin session = %q, want session1", stdinCalls[0].SessionID)
	}
	if stdinCalls[0].Data != "deploy please\r" {
		t.Errorf("WritePassthroughStdin data = %q, want %q", stdinCalls[0].Data, "deploy please\r")
	}
	if len(markCalls) != 1 || markCalls[0] != "session1" {
		t.Errorf("expected MarkPassthroughRunning([session1]), got %v", markCalls)
	}
	if len(prompts) != 0 {
		t.Errorf("PromptAgent (ACP) must not be called for passthrough sessions, got prompts %v", prompts)
	}
}

// TestPromptTask_PassthroughEmptyPromptSurfacesError ensures the no-text +
// attachment-only edge case (which wsAddMessage permits) returns a clear error
// to the caller rather than silently dropping. Service.handlePromptError will
// surface this to the user via createPromptErrorMessage.
func TestPromptTask_PassthroughEmptyPromptSurfacesError(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isPassthrough:  true,
		isAgentRunning: true,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")

	attachments := []v1.MessageAttachment{{Type: "image", MimeType: "image/png", Name: "x.png"}}
	_, err := svc.PromptTask(context.Background(), "task1", "session1", "", "", false, attachments, false)
	if err == nil {
		t.Fatal("expected error for empty prompt + attachments in passthrough mode, got nil")
	}
	if !strings.Contains(err.Error(), "passthrough") {
		t.Errorf("expected error to mention passthrough, got %v", err)
	}

	// Session state must be reverted to WAITING_FOR_INPUT by handlePromptError —
	// no stuck "Agent is running" composer.
	updated, gerr := repo.GetTaskSession(context.Background(), "session1")
	if gerr != nil {
		t.Fatalf("failed to reload session: %v", gerr)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected session reverted to WAITING_FOR_INPUT, got %q", updated.State)
	}
}

// TestPromptTask_PassthroughWriteFailureRevertsSession ensures that when the
// PTY is gone (e.g., agent process exited but execution record lingers) the
// write failure surfaces as an error AND the session is reverted out of
// RUNNING — satisfying the issue's "clear error instead of silently dropping
// the comment" requirement.
func TestPromptTask_PassthroughWriteFailureRevertsSession(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isPassthrough:       true,
		isAgentRunning:      true,
		passthroughStdinErr: errors.New("interactive runner not available"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "anything", "", false, nil, false)
	if err == nil {
		t.Fatal("expected error from PromptTask when PTY write fails, got nil")
	}

	updated, gerr := repo.GetTaskSession(context.Background(), "session1")
	if gerr != nil {
		t.Fatalf("failed to reload session: %v", gerr)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected session reverted to WAITING_FOR_INPUT, got %q", updated.State)
	}

	// MarkPassthroughRunning now fires BEFORE the chunk loop so concurrent
	// PromptTask calls are blocked during the inter-chunk SubmitDelay window
	// (Greptile P1). When the subsequent write fails the session is still
	// reverted to WAITING_FOR_INPUT by handlePromptError above, so the brief
	// RUNNING flash is bounded and the UI re-enables the composer.
	agentMgr.mu.Lock()
	markCount := len(agentMgr.markPassthroughCalls)
	agentMgr.mu.Unlock()
	if markCount != 1 {
		t.Errorf("MarkPassthroughRunning should fire once before the write; got %d call(s)", markCount)
	}
}
