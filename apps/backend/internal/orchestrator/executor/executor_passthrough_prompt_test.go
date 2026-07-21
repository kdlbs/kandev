package executor

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	// MarkPassthroughRunning is now called BEFORE the chunk loop so concurrent
	// PromptTask calls are blocked during the inter-chunk SubmitDelay window
	// (Greptile P1). The UI may briefly show "running" for a prompt that then
	// fails to deliver — preferable to a second prompt racing into the PTY
	// mid-submit. Expect exactly one MarkPassthroughRunning call even when the
	// subsequent write fails.
	if got := len(agentManager.markPassthroughRunningCalls); got != 1 {
		t.Errorf("MarkPassthroughRunning should be called once before the write; got %d call(s)", got)
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

func TestExecutor_Prompt_PassthroughWithAttachmentsSavesFilesAndReferencesPaths(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	workDir := t.TempDir()
	repo.sessions["sess-1"].WorkspacePath = workDir
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "../notes.txt",
		Data:     base64.StdEncoding.EncodeToString([]byte("passthrough attachment body")),
	}}
	result, err := exec.Prompt(context.Background(), "task-1", "sess-1", "look at this", atts, false)
	if err != nil {
		t.Fatalf("Prompt with attachments should succeed: %v", err)
	}
	if result == nil || result.StopReason != "passthrough_dispatched" {
		t.Fatalf("expected passthrough_dispatched result, got %+v", result)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", got)
	}
	data := agentManager.writePassthroughStdinCalls[0].Data
	relPath := filepath.Join(".kandev", "attachments", "sess-1", "notes.txt")
	if !strings.Contains(data, "look at this") {
		t.Errorf("PTY write should carry the typed text, got %q", data)
	}
	if !strings.Contains(data, relPath) {
		t.Errorf("PTY write should reference saved attachment path %q, got %q", relPath, data)
	}
	contents, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("expected saved attachment file: %v", err)
	}
	if string(contents) != "passthrough attachment body" {
		t.Fatalf("saved attachment contents = %q", string(contents))
	}
}

func TestExecutor_Prompt_PassthroughAttachmentOnlyMessageSendsPathPrompt(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	workDir := t.TempDir()
	repo.sessions["sess-1"].WorkspacePath = workDir
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "only.txt",
		Data:     base64.StdEncoding.EncodeToString([]byte("attachment-only")),
	}}
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "", atts, false); err != nil {
		t.Fatalf("attachment-only passthrough prompt should send path instructions: %v", err)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", got)
	}
	if !strings.Contains(agentManager.writePassthroughStdinCalls[0].Data, filepath.Join(".kandev", "attachments", "sess-1", "only.txt")) {
		t.Errorf("PTY write should reference only attachment, got %q", agentManager.writePassthroughStdinCalls[0].Data)
	}
}

func TestExecutor_Prompt_PassthroughInvalidAttachmentsDeliverTextOnlyPrompt(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	repo.sessions["sess-1"].WorkspacePath = t.TempDir()
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "broken.txt",
		Data:     "not-base64!!!",
	}}
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "keep this text", atts, false); err != nil {
		t.Fatalf("text prompt should still send when attachments are unusable: %v", err)
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 1 {
		t.Fatalf("expected 1 WritePassthroughStdin call, got %d", got)
	}
	want := "keep this text\r"
	if agentManager.writePassthroughStdinCalls[0].Data != want {
		t.Fatalf("PTY write = %q, want text-only payload %q", agentManager.writePassthroughStdinCalls[0].Data, want)
	}
}

func TestExecutor_Prompt_PassthroughAttachmentsUseSessionWorktreeFallback(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	workDir := t.TempDir()
	repo.sessions["sess-1"].Worktrees = []*models.TaskSessionWorktree{{
		SessionID:    "sess-1",
		RepositoryID: "repo-1",
		WorktreePath: workDir,
	}}
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "fallback.txt",
		Data:     base64.StdEncoding.EncodeToString([]byte("worktree-fallback")),
	}}
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "with fallback", atts, false); err != nil {
		t.Fatalf("Prompt with worktree fallback should succeed: %v", err)
	}
	relPath := filepath.Join(".kandev", "attachments", "sess-1", "fallback.txt")
	if !strings.Contains(agentManager.writePassthroughStdinCalls[0].Data, relPath) {
		t.Errorf("PTY write should reference saved attachment path %q, got %q", relPath, agentManager.writePassthroughStdinCalls[0].Data)
	}
	contents, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("expected saved attachment file: %v", err)
	}
	if string(contents) != "worktree-fallback" {
		t.Fatalf("saved attachment contents = %q", string(contents))
	}
}

func TestExecutor_Prompt_PassthroughAttachmentsUseMultiWorktreeRootFallback(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	workDir := filepath.Join(t.TempDir(), "repo-one")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create worktree dir: %v", err)
	}
	repo.sessions["sess-1"].Worktrees = []*models.TaskSessionWorktree{
		{SessionID: "sess-1", RepositoryID: "repo-1", WorktreePath: workDir},
		{SessionID: "sess-1", RepositoryID: "repo-2", WorktreePath: filepath.Join(t.TempDir(), "repo-two")},
	}
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "multi.txt",
		Data:     base64.StdEncoding.EncodeToString([]byte("multi-worktree-root")),
	}}
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "with multi", atts, false); err != nil {
		t.Fatalf("Prompt with multi-worktree fallback should succeed: %v", err)
	}
	relPath := filepath.Join(".kandev", "attachments", "sess-1", "multi.txt")
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(workDir), relPath))
	if err != nil {
		t.Fatalf("expected saved attachment under shared worktree root: %v", err)
	}
	if string(contents) != "multi-worktree-root" {
		t.Fatalf("saved attachment contents = %q", string(contents))
	}
}

func TestExecutor_Prompt_PassthroughAttachmentsUseTaskEnvironmentFallback(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	workDir := t.TempDir()
	repo.sessions["sess-1"].TaskEnvironmentID = "env-1"
	repo.taskEnvironments["env-1"] = &models.TaskEnvironment{
		ID:            "env-1",
		TaskID:        "task-1",
		WorkspacePath: workDir,
	}
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "env.txt",
		Data:     base64.StdEncoding.EncodeToString([]byte("env-fallback")),
	}}
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "with env", atts, false); err != nil {
		t.Fatalf("Prompt with task environment fallback should succeed: %v", err)
	}
	relPath := filepath.Join(".kandev", "attachments", "sess-1", "env.txt")
	if !strings.Contains(agentManager.writePassthroughStdinCalls[0].Data, relPath) {
		t.Errorf("PTY write should reference saved attachment path %q, got %q", relPath, agentManager.writePassthroughStdinCalls[0].Data)
	}
	contents, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("expected saved attachment file: %v", err)
	}
	if string(contents) != "env-fallback" {
		t.Fatalf("saved attachment contents = %q", string(contents))
	}
}

func TestExecutor_Prompt_PassthroughAttachmentsUseTaskEnvironmentByTaskIDFallback(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	workDir := t.TempDir()
	repo.taskEnvironments["env-1"] = &models.TaskEnvironment{
		ID:            "env-1",
		TaskID:        "task-1",
		WorkspacePath: workDir,
	}
	exec := newTestExecutor(t, agentManager, repo)

	atts := []v1.MessageAttachment{{
		Type:     "resource",
		MimeType: "text/plain",
		Name:     "env-by-task.txt",
		Data:     base64.StdEncoding.EncodeToString([]byte("env-by-task-fallback")),
	}}
	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "with env by task", atts, false); err != nil {
		t.Fatalf("Prompt with task environment by task fallback should succeed: %v", err)
	}
	relPath := filepath.Join(".kandev", "attachments", "sess-1", "env-by-task.txt")
	contents, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("expected saved attachment file: %v", err)
	}
	if string(contents) != "env-by-task-fallback" {
		t.Fatalf("saved attachment contents = %q", string(contents))
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

func TestExecutor_Prompt_PassthroughWhitespaceOnlyPromptReturnsError(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	_, err := exec.Prompt(context.Background(), "task-1", "sess-1", " \n\t ", nil, false)
	if err == nil {
		t.Fatal("expected error for whitespace-only passthrough prompt, got nil")
	}
	if got := len(agentManager.writePassthroughStdinCalls); got != 0 {
		t.Errorf("WritePassthroughStdin must not be called for whitespace-only prompts; got %d call(s)", got)
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
	if len(agentManager.writePassthroughStdinCalls) != 1 {
		t.Fatalf("expected 1 atomic WritePassthroughStdin call for multiline, got %d", len(agentManager.writePassthroughStdinCalls))
	}
	if agentManager.writePassthroughStdinCalls[0].Data != want {
		t.Errorf("PTY payload = %q, want %q", agentManager.writePassthroughStdinCalls[0].Data, want)
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

// TestExecutor_Prompt_PassthroughSubmitDelaySplitsWrites verifies the compose-box
// follow-up path (Executor.Prompt → promptPassthrough) honors SubmitDelay: the
// body and submit byte must arrive as two separate WritePassthroughStdin calls
// with the body first and the submit ("\r") second, so Claude's Ink TUI sees the
// trailing Enter as a discrete keystroke instead of absorbing it into a paste
// burst. The auto-inject path is covered by manager_passthrough_autoinject_test.go;
// this guards the executor path against regressing back to a single atomic write.
func TestExecutor_Prompt_PassthroughSubmitDelaySplitsWrites(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{
		isPassthroughSessionFunc: func(_ context.Context, _ string) bool { return true },
		resolvePassthroughConfigFunc: func(_ context.Context, _ string) (agents.PassthroughConfig, error) {
			return agents.PassthroughConfig{
				Supported:             true,
				SubmitSequence:        "\r",
				DisableBracketedPaste: true,
				SubmitDelay:           20 * time.Millisecond,
			}, nil
		},
	}
	seedPassthroughSession(t, repo, agentManager, "task-1", "sess-1", "exec-1")
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.Prompt(context.Background(), "task-1", "sess-1", "hello agent", nil, false); err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}

	if got := len(agentManager.writePassthroughStdinCalls); got != 2 {
		t.Fatalf("expected 2 WritePassthroughStdin calls (body, submit), got %d: %#v", got, agentManager.writePassthroughStdinCalls)
	}
	if agentManager.writePassthroughStdinCalls[0].Data != "hello agent" {
		t.Errorf("body write Data = %q, want %q", agentManager.writePassthroughStdinCalls[0].Data, "hello agent")
	}
	if agentManager.writePassthroughStdinCalls[1].Data != "\r" {
		t.Errorf("submit write Data = %q, want %q", agentManager.writePassthroughStdinCalls[1].Data, "\r")
	}
	// MarkPassthroughRunning must still fire exactly once after both writes — the
	// session should flip to RUNNING for the UI even though the two writes were
	// split. Regressing this would leave the spinner stuck on the composer.
	if got := len(agentManager.markPassthroughRunningCalls); got != 1 {
		t.Errorf("expected 1 MarkPassthroughRunning call, got %d", got)
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
