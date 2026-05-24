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

	// MarkPassthroughRunning must NOT be called when the write failed — otherwise
	// the UI would flash "running" for a prompt the agent never received.
	agentMgr.mu.Lock()
	markCount := len(agentMgr.markPassthroughCalls)
	agentMgr.mu.Unlock()
	if markCount != 0 {
		t.Errorf("MarkPassthroughRunning must not fire on PTY write failure; got %d call(s)", markCount)
	}
}
