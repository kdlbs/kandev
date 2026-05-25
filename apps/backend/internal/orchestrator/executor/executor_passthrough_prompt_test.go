package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedPassthroughSession installs a task + session into the mock repo and wires
// the agent manager to claim a valid execution ID, so Executor.Prompt reaches
// the IsPassthroughSession branch instead of bailing on lookup errors.
func seedPassthroughSession(t *testing.T, repo *mockRepository, agentManager *mockAgentManager, taskID, sessionID, execID string) {
	t.Helper()
	repo.sessions[sessionID] = &models.TaskSession{
		ID:     sessionID,
		TaskID: taskID,
		State:  models.TaskSessionStateWaitingForInput,
	}
	agentManager.getExecutionIDForSessionFunc = func(_ context.Context, sid string) (string, error) {
		if sid != sessionID {
			t.Fatalf("unexpected session lookup: got %q want %q", sid, sessionID)
		}
		return execID, nil
	}
}

func TestExecutor_Prompt_PassthroughWritesStdin(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	result, err := exec.Prompt(context.Background(), "task-1", "sess-1", "hello agent", nil, false)
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result == nil || result.StopReason != "passthrough_dispatched" {
		t.Fatalf("expected passthrough_dispatched result, got %+v", result)
	}

	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", got)
	}
	call := agentManager.writePassthroughStdinCalls[0]
	if call.SessionID != "sess-1" {
		t.Errorf("WritePassthroughStdin session = %q, want sess-1", call.SessionID)
	}
	// Mock's ResolvePassthroughConfig returns SubmitSequence "\r" by default
	// when isPassthroughSessionFunc reports the session is passthrough.
	wantData := "hello agent\r"
	if call.Data != wantData {
		t.Errorf("WritePassthroughStdin data = %q, want %q", call.Data, wantData)
	}
	if !strings.HasSuffix(call.Data, "\r") {
		t.Errorf("expected submit sequence suffix \\r, got %q", call.Data)
	}

	if got := len(agentManager.markPassthroughRunningCalls); got != 1 {
		t.Fatalf("expected 1 MarkPassthroughRunning call, got %d", got)
	}
	if agentManager.markPassthroughRunningCalls[0] != "sess-1" {
		t.Errorf("MarkPassthroughRunning session = %q, want sess-1", agentManager.markPassthroughRunningCalls[0])
	}

	if agentManager.promptAgentCallCount != 0 {
		t.Errorf("PromptAgent must not be called for passthrough sessions; call count = %d", agentManager.promptAgentCallCount)
	}
}

func TestExecutor_Prompt_PassthroughWriteErrorReturnsError(t *testing.T) {
	repo := newMockRepository()
	writeErr := errors.New("session sess-1 is not in passthrough mode")
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc:  func(_ context.Context, _ string) bool { return true },
		writePassthroughStdinFunc: func(_ context.Context, _ string, _ string) error { return writeErr },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	result, err := exec.Prompt(context.Background(), "task-1", "sess-1", "hi", nil, false)
	if err == nil {
		t.Fatalf("expected error from Prompt, got nil (result=%+v)", result)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("expected wrapped writeErr in chain, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
	// MarkPassthroughRunning must not be called when the write fails — otherwise
	// the UI would flash "running" for a prompt the agent never received.
	if got := len(agentManager.markPassthroughRunningCalls); got != 0 {
		t.Errorf("MarkPassthroughRunning must not be called when stdin write fails; got %d call(s)", got)
	}
}

func TestExecutor_Prompt_PassthroughMarkRunningErrorIsNonFatal(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc:   func(_ context.Context, _ string) bool { return true },
		markPassthroughRunningFunc: func(_ string) error { return errors.New("status flip failed") },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	result, err := exec.Prompt(context.Background(), "task-1", "sess-1", "hi", nil, false)
	if err != nil {
		t.Fatalf("expected nil error when MarkPassthroughRunning fails (data is already in PTY), got %v", err)
	}
	if result == nil || result.StopReason != "passthrough_dispatched" {
		t.Fatalf("expected passthrough_dispatched result, got %+v", result)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Errorf("expected the stdin write to have happened exactly once, got %d", got)
	}
}

func TestExecutor_Prompt_PassthroughWithAttachmentsDropsAttachmentsAndSucceeds(t *testing.T) {
	// Attachments have no place in passthrough mode (no ACP channel for binary
	// payloads). When prompt text is present we still deliver the text; the
	// caller (logs) records that the attachments were dropped.
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{Type: "image", MimeType: "image/png", Name: "screenshot.png"}}
	result, err := exec.Prompt(context.Background(), "task-1", "sess-1", "look at this", atts, false)
	if err != nil {
		t.Fatalf("Prompt with attachments should still succeed (attachments dropped); got error: %v", err)
	}
	if result == nil || result.StopReason != "passthrough_dispatched" {
		t.Fatalf("expected passthrough_dispatched result, got %+v", result)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", got)
	}
	if !strings.HasPrefix(agentManager.writePassthroughStdinCalls[0].Data, "look at this") {
		t.Errorf("PTY write should carry the typed text, got %q", agentManager.writePassthroughStdinCalls[0].Data)
	}
}

func TestExecutor_Prompt_PassthroughEmptyPromptReturnsError(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	_, err := exec.Prompt(context.Background(), "task-1", "sess-1", "", nil, false)
	if err == nil {
		t.Fatal("expected error for empty passthrough prompt, got nil")
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 0 {
		t.Errorf("WritePassthroughStdin must not be called for empty prompts; got %d call(s)", got)
	}
}

// TestExecutor_Prompt_PassthroughPreservesUnicodeAndMultiline verifies the PTY
// write payload preserves UTF-8 and embedded newlines verbatim. The submit
// suffix is appended at the END so the agent's TUI sees one logical "submit"
// at the right moment, even when the prompt itself contains LFs (Shift+Enter
// inserts those in the composer).
func TestExecutor_Prompt_PassthroughPreservesUnicodeAndMultiline(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	prompt := "café ☕\nline2\n日本語"
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", prompt, nil, false); err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	cfg := agents.PassthroughConfig{SubmitSequence: "\r"}
	want := agents.BuildPassthroughPayload(prompt, cfg)
	if len(agentManager.writePassthroughStdinCalls) != 2 {
		t.Fatalf("expected 2 WritePassthroughStdin calls for multiline, got %d", len(agentManager.writePassthroughStdinCalls))
	}
	joined := agentManager.writePassthroughStdinCalls[0].Data + agentManager.writePassthroughStdinCalls[1].Data
	if joined != want {
		t.Errorf("PTY payload = %q, want %q", joined, want)
	}
	if agentManager.writePassthroughStdinCalls[1].Data != "\r" {
		t.Errorf("submit chunk = %q, want \\r", agentManager.writePassthroughStdinCalls[1].Data)
	}
}

// TestExecutor_Prompt_PassthroughHonoursDynamicSubmitSequence verifies the
// PTY submit suffix is taken from PassthroughConfig (per-agent) rather than
// hardcoded. A future TUI that submits on "\n" or "\x0d\x0a" plugs in via
// PassthroughConfig.SubmitSequence with no code change here.
func TestExecutor_Prompt_PassthroughHonoursDynamicSubmitSequence(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
		resolvePassthroughConfigFunc: func(_ context.Context, _ string) (agents.PassthroughConfig, error) {
			return agents.PassthroughConfig{Supported: true, SubmitSequence: "\n"}, nil
		},
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "hi", nil, false); err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", got)
	}
	want := "hi\n"
	if agentManager.writePassthroughStdinCalls[0].Data != want {
		t.Errorf("PTY payload = %q, want %q (dynamic SubmitSequence not applied)", agentManager.writePassthroughStdinCalls[0].Data, want)
	}
}

func TestExecutor_Prompt_ACPPathUnchanged(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		// IsPassthroughSession defaults to false; explicit false here for clarity.
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return false },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "hello", nil, false); err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if agentManager.promptAgentCallCount != 1 {
		t.Errorf("expected PromptAgent to be called once, got %d", agentManager.promptAgentCallCount)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 0 {
		t.Errorf("WritePassthroughStdin must not be called in ACP mode; got %d", got)
	}
	if got := len(agentManager.markPassthroughRunningCalls); got != 0 {
		t.Errorf("MarkPassthroughRunning must not be called in ACP mode; got %d", got)
	}
}
