package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeBackend is the runtime contract test stand-in for *lifecycle.Manager.
// It records every call so the test can assert the facade translates
// Runtime verbs into Backend verbs faithfully.
type fakeBackend struct {
	executions map[string]*lifecycle.AgentExecution

	launchReq    *lifecycle.LaunchRequest
	launchExec   *lifecycle.AgentExecution
	launchErr    error
	promptCalls  []promptCall
	promptResult *lifecycle.PromptResult
	promptErr    error
	stopCalls    []stopCall
	stopErr      error
	mcpCalls     []mcpCall
	mcpErr       error
}

type promptCall struct {
	executionID  string
	prompt       string
	dispatchOnly bool
}

type stopCall struct {
	executionID string
	reason      string
	force       bool
}

type mcpCall struct {
	executionID string
	mode        string
}

func (f *fakeBackend) Launch(_ context.Context, req *lifecycle.LaunchRequest) (*lifecycle.AgentExecution, error) {
	f.launchReq = req
	if f.launchErr != nil {
		return nil, f.launchErr
	}
	return f.launchExec, nil
}

func (f *fakeBackend) PromptAgent(_ context.Context, executionID, prompt string, _ []v1.MessageAttachment, dispatchOnly bool) (*lifecycle.PromptResult, error) {
	f.promptCalls = append(f.promptCalls, promptCall{executionID: executionID, prompt: prompt, dispatchOnly: dispatchOnly})
	if f.promptErr != nil {
		return nil, f.promptErr
	}
	return f.promptResult, nil
}

func (f *fakeBackend) StopAgentWithReason(_ context.Context, executionID, reason string, force bool) error {
	f.stopCalls = append(f.stopCalls, stopCall{executionID: executionID, reason: reason, force: force})
	return f.stopErr
}

func (f *fakeBackend) GetExecution(executionID string) (*lifecycle.AgentExecution, bool) {
	exec, ok := f.executions[executionID]
	return exec, ok
}

func (f *fakeBackend) SetMcpMode(_ context.Context, executionID, mode string) error {
	f.mcpCalls = append(f.mcpCalls, mcpCall{executionID: executionID, mode: mode})
	return f.mcpErr
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{executions: map[string]*lifecycle.AgentExecution{}}
}

func TestRuntime_Launch_TranslatesSpecToLaunchRequest(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	backend := newFakeBackend()
	backend.launchExec = &lifecycle.AgentExecution{
		ID:        "exec-1",
		SessionID: "sess-1",
		StartedAt: now,
	}

	rt := agentruntime.New(backend)
	ref, err := rt.Launch(context.Background(), agentruntime.LaunchSpec{
		AgentProfileID: "claude-acp-default",
		ExecutorID:     "local_pc",
		Workspace: agentruntime.WorkspaceRef{
			Path:         "/work/foo",
			RepositoryID: "repo-1",
			IsEphemeral:  true,
		},
		Prompt:          "do the thing",
		PriorACPSession: "acp-prior",
		McpMode:         "config",
		Metadata: map[string]any{
			"source": "engine",
		},
	})
	if err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}
	if ref.ID != "exec-1" || ref.SessionID != "sess-1" {
		t.Fatalf("unexpected ExecutionRef: %+v", ref)
	}
	if !ref.StartedAt.Equal(now) {
		t.Fatalf("StartedAt mismatch: got %v want %v", ref.StartedAt, now)
	}
	// AgentctlURL is empty when the execution has no agentctl client wired
	// (which is the case for the fakeBackend). The field must be present on
	// the ref — callers that don't read it are unaffected.
	if ref.AgentctlURL != "" {
		t.Errorf("expected empty AgentctlURL for execution with no client, got %q", ref.AgentctlURL)
	}
	if backend.launchReq == nil {
		t.Fatal("backend.Launch was not called")
	}
	got := backend.launchReq
	if got.AgentProfileID != "claude-acp-default" {
		t.Errorf("AgentProfileID = %q", got.AgentProfileID)
	}
	if got.ExecutorType != "local_pc" {
		t.Errorf("ExecutorType = %q", got.ExecutorType)
	}
	if got.WorkspacePath != "/work/foo" {
		t.Errorf("WorkspacePath = %q", got.WorkspacePath)
	}
	if got.RepositoryID != "repo-1" {
		t.Errorf("RepositoryID = %q", got.RepositoryID)
	}
	if !got.IsEphemeral {
		t.Errorf("IsEphemeral = false, want true")
	}
	if got.TaskDescription != "do the thing" {
		t.Errorf("TaskDescription = %q", got.TaskDescription)
	}
	if got.ACPSessionID != "acp-prior" {
		t.Errorf("ACPSessionID = %q", got.ACPSessionID)
	}
	if got.McpMode != "config" {
		t.Errorf("McpMode = %q", got.McpMode)
	}
	if got.Metadata["source"] != "engine" {
		t.Errorf("Metadata[source] = %v", got.Metadata["source"])
	}
}

func TestRuntime_Launch_PassesThroughLaunchRequestMetadata(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	backend.launchExec = &lifecycle.AgentExecution{ID: "exec-2"}

	base := &lifecycle.LaunchRequest{
		TaskID:      "task-A",
		SessionID:   "sess-A",
		UseWorktree: true,
		Repositories: []lifecycle.RepoLaunchSpec{
			{RepositoryID: "r1", BaseBranch: "main"},
		},
	}
	rt := agentruntime.New(backend)
	if _, err := rt.Launch(context.Background(), agentruntime.LaunchSpec{
		AgentProfileID: "p1",
		Prompt:         "hello",
		Metadata: map[string]any{
			"launch_request": base,
		},
	}); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	got := backend.launchReq
	if got == nil {
		t.Fatal("backend.Launch not called")
	}
	if got.TaskID != "task-A" || got.SessionID != "sess-A" {
		t.Errorf("base fields lost: TaskID=%q SessionID=%q", got.TaskID, got.SessionID)
	}
	if !got.UseWorktree {
		t.Errorf("UseWorktree lost from base launch_request")
	}
	if len(got.Repositories) != 1 || got.Repositories[0].RepositoryID != "r1" {
		t.Errorf("Repositories lost from base launch_request: %+v", got.Repositories)
	}
	// Spec fields override base.
	if got.AgentProfileID != "p1" {
		t.Errorf("AgentProfileID overlay lost: %q", got.AgentProfileID)
	}
	if got.TaskDescription != "hello" {
		t.Errorf("TaskDescription overlay lost: %q", got.TaskDescription)
	}
	// launch_request key itself does not leak into LaunchRequest.Metadata.
	if _, found := got.Metadata["launch_request"]; found {
		t.Errorf("launch_request key leaked into LaunchRequest.Metadata")
	}
}

func TestRuntime_Launch_PropagatesError(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	backend.launchErr = errors.New("boom")
	rt := agentruntime.New(backend)
	if _, err := rt.Launch(context.Background(), agentruntime.LaunchSpec{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRuntime_Resume_DelegatesToPromptAgent(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	rt := agentruntime.New(backend)
	if err := rt.Resume(context.Background(), "exec-1", "next prompt"); err != nil {
		t.Fatalf("Resume returned: %v", err)
	}
	if len(backend.promptCalls) != 1 {
		t.Fatalf("expected 1 prompt call, got %d", len(backend.promptCalls))
	}
	got := backend.promptCalls[0]
	if got.executionID != "exec-1" || got.prompt != "next prompt" || got.dispatchOnly != false {
		t.Errorf("unexpected prompt call: %+v", got)
	}
}

func TestRuntime_Resume_RequiresExecutionID(t *testing.T) {
	t.Parallel()
	rt := agentruntime.New(newFakeBackend())
	if err := rt.Resume(context.Background(), "", "prompt"); err == nil {
		t.Fatal("expected error for empty executionID")
	}
}

func TestRuntime_Resume_PropagatesError(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	backend.promptErr = errors.New("nope")
	rt := agentruntime.New(backend)
	if err := rt.Resume(context.Background(), "exec-1", "p"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRuntime_Stop_DelegatesToBackend(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	rt := agentruntime.New(backend)
	if err := rt.Stop(context.Background(), "exec-1", "user_cancelled"); err != nil {
		t.Fatalf("Stop returned: %v", err)
	}
	if len(backend.stopCalls) != 1 {
		t.Fatalf("expected 1 stop call, got %d", len(backend.stopCalls))
	}
	got := backend.stopCalls[0]
	if got.executionID != "exec-1" || got.reason != "user_cancelled" || got.force {
		t.Errorf("unexpected stop call: %+v", got)
	}
}

func TestRuntime_Stop_RequiresExecutionID(t *testing.T) {
	t.Parallel()
	rt := agentruntime.New(newFakeBackend())
	if err := rt.Stop(context.Background(), "", "reason"); err == nil {
		t.Fatal("expected error for empty executionID")
	}
}

func TestRuntime_GetExecution_ReturnsSnapshot(t *testing.T) {
	t.Parallel()
	finishedAt := time.Now().UTC()
	exit := 0
	backend := newFakeBackend()
	backend.executions["exec-1"] = &lifecycle.AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		TaskID:         "task-1",
		AgentProfileID: "p1",
		WorkspacePath:  "/w",
		Status:         v1.AgentStatus("ready"),
		StartedAt:      finishedAt.Add(-time.Minute),
		FinishedAt:     &finishedAt,
		ExitCode:       &exit,
		ACPSessionID:   "acp-1",
		Metadata:       map[string]interface{}{"k": "v"},
	}
	rt := agentruntime.New(backend)
	got, err := rt.GetExecution(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("GetExecution returned: %v", err)
	}
	if got.ID != "exec-1" || got.SessionID != "sess-1" || got.TaskID != "task-1" {
		t.Errorf("snapshot identifiers wrong: %+v", got)
	}
	if got.AgentProfileID != "p1" || got.WorkspacePath != "/w" {
		t.Errorf("snapshot fields wrong: %+v", got)
	}
	if got.Status != v1.AgentStatus("ready") {
		t.Errorf("Status mismatch: %q", got.Status)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Errorf("FinishedAt mismatch")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode mismatch")
	}
	if got.ACPSessionID != "acp-1" {
		t.Errorf("ACPSessionID mismatch: %q", got.ACPSessionID)
	}
	if got.Metadata["k"] != "v" {
		t.Errorf("Metadata mismatch: %+v", got.Metadata)
	}
	// AgentctlURL is empty when the execution has no agentctl client wired.
	if got.AgentctlURL != "" {
		t.Errorf("expected empty AgentctlURL for execution with no client, got %q", got.AgentctlURL)
	}
}

func TestRuntime_GetExecution_NotFound(t *testing.T) {
	t.Parallel()
	rt := agentruntime.New(newFakeBackend())
	_, err := rt.GetExecution(context.Background(), "missing")
	if !errors.Is(err, agentruntime.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRuntime_GetExecution_RequiresExecutionID(t *testing.T) {
	t.Parallel()
	rt := agentruntime.New(newFakeBackend())
	if _, err := rt.GetExecution(context.Background(), ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestRuntime_SetMcpMode_DelegatesToBackend(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	rt := agentruntime.New(backend)
	if err := rt.SetMcpMode(context.Background(), "exec-1", "config"); err != nil {
		t.Fatalf("SetMcpMode returned: %v", err)
	}
	if len(backend.mcpCalls) != 1 {
		t.Fatalf("expected 1 mcp call, got %d", len(backend.mcpCalls))
	}
	got := backend.mcpCalls[0]
	if got.executionID != "exec-1" || got.mode != "config" {
		t.Errorf("unexpected mcp call: %+v", got)
	}
}

func TestRuntime_SetMcpMode_RequiresExecutionID(t *testing.T) {
	t.Parallel()
	rt := agentruntime.New(newFakeBackend())
	if err := rt.SetMcpMode(context.Background(), "", "config"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRuntime_SubscribeEvents_UnsupportedInPhase1(t *testing.T) {
	t.Parallel()
	// Phase 1 leaves SubscribeEvents best-effort: the lifecycle manager
	// publishes events through the global event bus rather than
	// per-execution channels, so the facade returns ErrUnsupported.
	// Phase 2/3 may add a per-execution channel.
	rt := agentruntime.New(newFakeBackend())
	ch, err := rt.SubscribeEvents(context.Background(), "exec-1")
	if !errors.Is(err, agentruntime.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
	if ch != nil {
		t.Fatalf("expected nil channel, got %v", ch)
	}
}
