package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// TestPromptTask_passthrough_writes_to_stdin verifies that when the session is
// in passthrough (PTY) mode, PromptTask routes the prompt to the PTY's stdin
// with the agent-declared submit sequence and never invokes the ACP prompt path.
func TestPromptTask_passthrough_writes_to_stdin(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isPassthrough: true,
	}
	// SubmitSequence override so we can assert the routed bytes exactly.
	agentMgr.passthroughConfigSet = true
	agentMgr.passthroughConfig = agents.PassthroughConfig{Supported: true, SubmitSequence: "\r"}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	result, err := svc.PromptTask(context.Background(), "task1", "session1", "list files", "", false, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil PromptResult")
	}
	if result.StopReason != "passthrough_dispatched" {
		t.Fatalf("expected stop reason 'passthrough_dispatched', got %q", result.StopReason)
	}

	if len(agentMgr.passthroughStdinCalls) != 1 {
		t.Fatalf("expected exactly 1 passthrough stdin write, got %d", len(agentMgr.passthroughStdinCalls))
	}
	call := agentMgr.passthroughStdinCalls[0]
	if call.SessionID != "session1" {
		t.Fatalf("expected session1, got %q", call.SessionID)
	}
	if call.Data != "list files\r" {
		t.Fatalf("expected prompt + \\r, got %q", call.Data)
	}

	// The ACP prompt path must never fire for passthrough sessions.
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("expected 0 ACP prompt calls, got %d", len(agentMgr.capturedPrompts))
	}
}

// TestPromptTask_passthrough_propagates_write_error verifies that PTY write
// failures surface to the caller (not silently swallowed).
func TestPromptTask_passthrough_propagates_write_error(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isPassthrough:       true,
		passthroughStdinErr: errors.New("pty closed"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected error when passthrough stdin write fails")
	}
}

// TestPromptTask_passthrough_resolve_config_error verifies that
// ResolvePassthroughConfig failures bubble up.
func TestPromptTask_passthrough_resolve_config_error(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isPassthrough:        true,
		passthroughConfigErr: errors.New("no execution"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected error when ResolvePassthroughConfig fails")
	}
	if len(agentMgr.passthroughStdinCalls) != 0 {
		t.Fatalf("expected no stdin writes when resolve fails, got %d", len(agentMgr.passthroughStdinCalls))
	}
}

// TestPromptTask_passthrough_applies_plan_mode_prefix verifies the passthrough
// branch sees the prompt AFTER sysprompt.InjectPlanMode is applied, so plan-mode
// follow-ups are framed the same way they would be for ACP sessions.
func TestPromptTask_passthrough_applies_plan_mode_prefix(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isPassthrough: true}
	agentMgr.passthroughConfigSet = true
	agentMgr.passthroughConfig = agents.PassthroughConfig{Supported: true, SubmitSequence: "\r"}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	if _, err := svc.PromptTask(context.Background(), "task1", "session1", "raw prompt", "", true, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agentMgr.passthroughStdinCalls) != 1 {
		t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
	}
	got := agentMgr.passthroughStdinCalls[0].Data
	if got == "raw prompt\r" {
		t.Fatalf("passthrough received raw prompt without plan-mode framing: %q", got)
	}
	// Sanity: still ends with SubmitSequence.
	if got[len(got)-1] != '\r' {
		t.Fatalf("expected stdin payload to end with \\r, got %q", got)
	}
}

// TestPromptTask_acp_path_unaffected covers the original ACP path: when the
// session is not in passthrough mode, prompts flow through the executor as
// before and no PTY stdin write happens.
func TestPromptTask_acp_path_unaffected(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		isPassthrough:  false,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	if _, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agentMgr.passthroughStdinCalls) != 0 {
		t.Fatalf("expected no passthrough stdin writes in ACP path, got %d", len(agentMgr.passthroughStdinCalls))
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected 1 ACP prompt call, got %d", len(agentMgr.capturedPrompts))
	}
}

// TestCancelAgent_passthrough_writes_ctrl_c verifies that cancelling a
// passthrough session writes Ctrl-C (0x03) to the PTY stdin instead of calling
// the ACP CancelAgent, and that DB reconciliation still runs.
func TestCancelAgent_passthrough_writes_ctrl_c(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		isPassthrough:  true,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := svc.CancelAgent(context.Background(), "session1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ctrl-C written to PTY, ACP cancel never invoked.
	if len(agentMgr.passthroughStdinCalls) != 1 {
		t.Fatalf("expected 1 passthrough stdin write, got %d", len(agentMgr.passthroughStdinCalls))
	}
	if got := agentMgr.passthroughStdinCalls[0].Data; got != "\x03" {
		t.Fatalf("expected Ctrl-C (\\x03), got %q", got)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 0 {
		t.Fatalf("expected 0 ACP cancel calls in passthrough mode, got %d", got)
	}

	// DB reconciliation still ran: session transitioned to WAITING_FOR_INPUT.
	updated, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected WAITING_FOR_INPUT after passthrough cancel, got %q", updated.State)
	}
}

// TestCancelAgent_passthrough_continues_on_write_error verifies that even when
// the PTY stdin write fails, CancelAgent still reconciles DB state so the UI
// doesn't stay stuck.
func TestCancelAgent_passthrough_continues_on_write_error(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isPassthrough:       true,
		passthroughStdinErr: errors.New("pty closed"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := svc.CancelAgent(context.Background(), "session1"); err != nil {
		t.Fatalf("expected nil error when PTY write fails (so UI unsticks), got %v", err)
	}

	// DB reconciliation must still have run despite the write failing.
	updated, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected WAITING_FOR_INPUT after passthrough cancel with write error, got %q", updated.State)
	}
}

// TestCancelAgent_acp_path_unaffected verifies that non-passthrough sessions
// still go through the original ACP CancelAgent flow and no PTY writes happen.
func TestCancelAgent_acp_path_unaffected(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		isPassthrough:  false,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := svc.CancelAgent(context.Background(), "session1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("expected 1 ACP cancel call, got %d", got)
	}
	if len(agentMgr.passthroughStdinCalls) != 0 {
		t.Fatalf("expected no PTY writes in ACP cancel path, got %d", len(agentMgr.passthroughStdinCalls))
	}
}
