package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishMCPAttachmentPublishesWithoutBlocking(t *testing.T) {
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	evidence := streams.MCPAttachmentEvidence{
		AttemptID:  "attempt-1",
		ServerName: "kandev",
		Kind:       streams.MCPAttachmentEvidenceDelivered,
	}

	m.PublishMCPAttachment(evidence)

	select {
	case event := <-m.updatesCh:
		if event.Type != streams.EventTypeMCPAttachment {
			t.Fatalf("event type = %q, want MCP attachment", event.Type)
		}
		if event.MCPAttachment == nil || event.MCPAttachment.AttemptID != "attempt-1" {
			t.Fatalf("attachment event = %+v, want attempt evidence", event.MCPAttachment)
		}
	default:
		t.Fatal("expected MCP attachment update")
	}

	// A saturated diagnostics channel must not stall agent lifecycle work.
	m.updatesCh <- adapter.AgentEvent{Type: streams.EventTypeMessageChunk}
	m.PublishMCPAttachment(evidence)
	if got := len(m.updatesCh); got != 1 {
		t.Fatalf("queued events = %d, want 1", got)
	}
}

func TestSendErrorEventWithProviderErrorCarriesSafeDetails(t *testing.T) {
	occurred := time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC)
	m := &Manager{
		updatesCh: make(chan adapter.AgentEvent, 1),
		logger:    newTestLogger(t),
	}
	m.SendErrorEventWithProviderError("provider failed", 7, &streams.ProviderError{
		Source:     streams.ProviderErrorSourceOpenCodeStderr,
		ModelID:    "kimi-k3",
		Message:    "5-hour usage limit reached",
		OccurredAt: occurred,
	})

	event := <-m.updatesCh
	if event.PromptGeneration != 7 || event.Error != "provider failed" {
		t.Fatalf("event = %+v", event)
	}
	if event.ProviderError == nil || event.ProviderError.ModelID != "kimi-k3" {
		t.Fatalf("provider error = %+v", event.ProviderError)
	}
}

func TestManagerReadStderrDeliversCleanedLinesToOptionalConsumer(t *testing.T) {
	consumer := &stderrLineCapture{lines: make(chan string, 2)}
	m := &Manager{
		stderr:         io.NopCloser(strings.NewReader("\x1b[31mquota\x1b[0m\nplain\n")),
		stderrConsumer: consumer,
		logger:         newTestLogger(t),
	}
	m.wg.Add(1)
	m.readStderr(make(chan struct{}))

	got := []string{<-consumer.lines, <-consumer.lines}
	want := []string{"quota", "plain"}
	if !slices.Equal(got, want) {
		t.Fatalf("consumer lines = %#v, want %#v", got, want)
	}
	if got := m.GetRecentStderr(); !slices.Equal(got, want) {
		t.Fatalf("recent stderr = %#v, want %#v", got, want)
	}
}

type stderrLineSanitizerFunc func(string) (string, bool)

func (f stderrLineSanitizerFunc) SanitizeStderrLine(line string) (string, bool) {
	return f(line)
}

func TestManagerProcessExitHelper(t *testing.T) {
	if os.Getenv("KANDEV_MANAGER_PROCESS_EXIT_HELPER") != "1" {
		return
	}
	fmt.Fprintln(os.Stderr, "workspace=https://opencode.ai/workspace/wrk_private")
	os.Exit(7)
}

func TestManagerProcessExitUsesSanitizedStderr(t *testing.T) {
	log, observed := newObservedTestLogger(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestManagerProcessExitHelper")
	cmd.Env = append(os.Environ(), "KANDEV_MANAGER_PROCESS_EXIT_HELPER=1")
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	cmd.Stderr = stderrWriter
	m := &Manager{
		cmd:       cmd,
		stderr:    stderr,
		logger:    log,
		doneCh:    make(chan struct{}),
		updatesCh: make(chan adapter.AgentEvent, 1),
		stderrSanitizer: stderrLineSanitizerFunc(func(string) (string, bool) {
			return "OpenCode provider failure", true
		}),
		groupAliveFn: func(int) bool { return false },
	}
	m.status.Store(StatusRunning)
	t.Cleanup(func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = stderrWriter.Close()
		_ = stderr.Close()
		m.wg.Wait()
	})
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("close parent stderr pipe: %v", err)
	}
	stderrDone := make(chan struct{})
	m.wg.Add(2)
	go m.readStderr(stderrDone)
	m.waitForExit(stderrDone)
	m.wg.Wait()

	event := <-m.updatesCh
	if strings.Contains(event.Error, "wrk_private") || strings.Contains(event.Error, "https://") {
		t.Fatalf("exit error leaked raw stderr: %q", event.Error)
	}
	data := event.Data
	if data == nil {
		t.Fatal("event data is nil")
	}
	recent, ok := data["recent_stderr"].([]string)
	if !ok || !slices.Equal(recent, []string{"OpenCode provider failure"}) {
		t.Fatalf("recent stderr = %#v", data["recent_stderr"])
	}
	for _, entry := range observed.All() {
		if strings.Contains(fmt.Sprint(entry.ContextMap()), "wrk_private") {
			t.Fatalf("logger captured raw stderr: %v", entry.ContextMap())
		}
	}
}

type stderrLineCapture struct {
	lines chan string
}

func (c *stderrLineCapture) ConsumeStderrLine(line string) {
	c.lines <- line
}

func TestPublishMCPAttachmentAttemptCarriesBackendOwnedIdentity(t *testing.T) {
	m := &Manager{updatesCh: make(chan adapter.AgentEvent, 1), logger: newTestLogger(t)}
	m.PublishMCPAttachmentAttempt(streams.MCPAttachmentAttempt{
		AttemptID:   "attempt-1",
		TaskID:      "task-1",
		SessionID:   "session-1",
		ExecutionID: "execution-1",
	})

	event := <-m.updatesCh
	if event.MCPAttachmentAttempt == nil {
		t.Fatal("expected MCP attachment attempt")
	}
	if got := event.MCPAttachmentAttempt; got.TaskID != "task-1" || got.ExecutionID != "execution-1" {
		t.Fatalf("attempt identity = %+v", got)
	}
}

func TestForwardUpdatesUsesItsGenerationStopChannel(t *testing.T) {
	stopCh := make(chan struct{})
	m := &Manager{
		adapter:   newStubAdapter(),
		updatesCh: make(chan adapter.AgentEvent, 1),
		stopCh:    stopCh,
		logger:    newTestLogger(t),
	}
	m.wg.Add(1)
	go m.forwardUpdates(m.adapter, stopCh)

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	m.stopCh = make(chan struct{})
	close(stopCh)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		close(m.stopCh)
		<-done
		t.Fatal("forwardUpdates did not stop with its original lifecycle channel")
	}
}

// stubAdapter is a minimal AgentAdapter for testing the startup sequence.
type stubAdapter struct {
	connectCalled       bool
	requiresProcessKill bool
	updatesCh           chan adapter.AgentEvent
}

func newStubAdapter() *stubAdapter {
	return &stubAdapter{updatesCh: make(chan adapter.AgentEvent, 10)}
}

type oneShotStubAdapter struct {
	*stubAdapter
}

func newOneShotStubAdapter() *oneShotStubAdapter {
	return &oneShotStubAdapter{stubAdapter: newStubAdapter()}
}

func (s *oneShotStubAdapter) IsOneShot() bool { return true }

func (s *stubAdapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (s *stubAdapter) PrepareCommandArgs() []string                   { return nil }
func (s *stubAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	s.connectCalled = true
	return nil
}
func (s *stubAdapter) Initialize(context.Context) error { return nil }
func (s *stubAdapter) GetAgentInfo() *adapter.AgentInfo { return nil }
func (s *stubAdapter) NewSession(_ context.Context, _ []types.McpServer) (string, error) {
	return "", nil
}
func (s *stubAdapter) LoadSession(context.Context, string, []types.McpServer) error { return nil }
func (s *stubAdapter) Prompt(context.Context, string, []v1.MessageAttachment, uint64) error {
	return nil
}
func (s *stubAdapter) Cancel(context.Context) error                   { return nil }
func (s *stubAdapter) Updates() <-chan adapter.AgentEvent             { return s.updatesCh }
func (s *stubAdapter) GetSessionID() string                           { return "" }
func (s *stubAdapter) GetOperationID() string                         { return "" }
func (s *stubAdapter) SetPermissionHandler(adapter.PermissionHandler) {}
func (s *stubAdapter) Close() error {
	close(s.updatesCh)
	return nil
}
func (s *stubAdapter) RequiresProcessKill() bool { return s.requiresProcessKill }

func TestStartProcessPipes_CreatesAllPipes(t *testing.T) {
	m := &Manager{
		cmd: exec.Command("cat"),
	}

	err := m.startProcessPipes()
	require.NoError(t, err)
	assert.NotNil(t, m.stdin, "stdin pipe should be created")
	assert.NotNil(t, m.stdout, "stdout pipe should be created")
	assert.NotNil(t, m.stderr, "stderr pipe should be created")

	t.Cleanup(func() {
		_ = m.stdin.Close()
		_ = m.stdout.Close()
		_ = m.closeStderrPipe()
	})
}

func TestStartProcessPipes_FailsAfterProcessStarted(t *testing.T) {
	// Regression test: documents the bug where cmd.Start() was called inside
	// buildFinalCommand() before pipes were created.
	// exec.Cmd pipes cannot be created after Start() is called.
	cmd := fixtureCmd("sleep 10")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	_, err := cmd.StdinPipe()
	assert.Error(t, err, "pipes cannot be created after process starts")
}

func TestBuildFinalCommand_DoesNotStartProcess(t *testing.T) {
	log := newTestLogger(t)
	stub := newStubAdapter()

	m := &Manager{
		cfg: &config.InstanceConfig{
			AgentArgs: []string{"cat"},
			WorkDir:   t.TempDir(),
		},
		logger:  log,
		adapter: stub,
	}

	err := m.buildFinalCommand()
	require.NoError(t, err)
	assert.NotNil(t, m.cmd, "cmd should be created")
	assert.Nil(t, m.cmd.Process, "process should not be started yet")
}

func TestManager_Start_PipesCreatedBeforeProcessStart(t *testing.T) {
	log := newTestLogger(t)
	stub := newStubAdapter()
	workDir := t.TempDir()

	m := &Manager{
		cfg: &config.InstanceConfig{
			AgentArgs: fixtureArgs(),
			WorkDir:   workDir,
			AgentEnv:  fixtureEnvSlice("cat"),
		},
		logger:             log,
		adapter:            stub,
		adapterCfg:         &adapter.Config{WorkDir: workDir},
		updatesCh:          make(chan adapter.AgentEvent, 100),
		pendingPermissions: make(map[string]*PendingPermission),
		workspaceTracker:   NewWorkspaceTracker(workDir, log),
	}
	m.status.Store(StatusStopped)
	m.exitCode.Store(-1)

	// Skip buildAdapterConfig since we already have a stub adapter.
	// Call the sub-steps in the same order as Start() to verify correctness.
	err := m.buildFinalCommand()
	require.NoError(t, err)
	assert.Nil(t, m.cmd.Process, "process must not be started after buildFinalCommand")

	err = m.startProcessPipes()
	require.NoError(t, err)
	assert.NotNil(t, m.stdin)
	assert.NotNil(t, m.stdout)
	assert.NotNil(t, m.stderr)

	// Now start the process — this is the fix: Start() happens after pipes.
	err = m.cmd.Start()
	require.NoError(t, err)
	waited := false
	t.Cleanup(func() {
		if waited || m.cmd == nil || m.cmd.Process == nil {
			return
		}
		_ = killProcessGroup(m.cmd.Process.Pid)
		_ = m.cmd.Process.Kill()
		_ = m.cmd.Wait()
	})
	assert.NotNil(t, m.cmd.Process, "process should be running")

	processLifecycle, err := installProcessLifecycle(m.cmd)
	require.NoError(t, err)
	defer releaseProcessLifecycle(processLifecycle)

	// Adapter connects after process starts.
	err = stub.Connect(m.stdin, m.stdout)
	require.NoError(t, err)
	assert.True(t, stub.connectCalled)

	// Clean up: close stdin so cat exits, then wait.
	_ = m.stdin.Close()
	_ = m.cmd.Wait()
	waited = true
}

func TestFormatAgentStartError_E2BIGIncludesEnvDiagnostics(t *testing.T) {
	err := formatAgentStartError(&os.PathError{Op: "fork/exec", Path: "npx", Err: syscall.E2BIG}, []string{
		"SMALL=1",
		"KANDEV_WAKE_PAYLOAD_JSON=" + strings.Repeat("x", 100),
		"PATH=/usr/bin",
	})

	msg := err.Error()
	if !strings.Contains(msg, "environment/arguments too large") {
		t.Fatalf("missing actionable E2BIG message: %s", msg)
	}
	if !strings.Contains(msg, "env_bytes=") {
		t.Fatalf("missing env byte diagnostics: %s", msg)
	}
	if !strings.Contains(msg, "KANDEV_WAKE_PAYLOAD_JSON") {
		t.Fatalf("missing largest env key diagnostics: %s", msg)
	}
	if !errors.Is(err, syscall.E2BIG) {
		t.Fatalf("formatted error must preserve E2BIG wrapping: %v", err)
	}
}

func TestBuildAdapterConfig_StripEnvRemovesDeclaredVars(t *testing.T) {
	log := newTestLogger(t)

	m := &Manager{
		cfg: &config.InstanceConfig{
			AgentArgs: []string{"cat"},
			WorkDir:   t.TempDir(),
			AgentEnv: []string{
				"ACP_BACKEND=windsurf",
				"PATH=/usr/bin",
				"HOME=/root",
			},
			StripEnv: []string{"ACP_BACKEND"},
			Protocol: agent.ProtocolACP,
		},
		logger: log,
	}

	require.NoError(t, m.buildAdapterConfig())
	t.Cleanup(func() { _ = m.adapter.Close() })

	for _, e := range m.cfg.AgentEnv {
		if strings.HasPrefix(e, "ACP_BACKEND=") {
			t.Errorf("ACP_BACKEND not stripped from AgentEnv: %q", e)
		}
	}
	if !slices.Contains(m.cfg.AgentEnv, "PATH=/usr/bin") {
		t.Errorf("PATH was stripped but should have been kept")
	}
	if !slices.Contains(m.cfg.AgentEnv, "HOME=/root") {
		t.Errorf("HOME was stripped but should have been kept")
	}
}

func TestBuildAdapterConfigForwardsNotificationQueueCapacity(t *testing.T) {
	m := &Manager{
		cfg: &config.InstanceConfig{
			AgentArgs:                 []string{"cat"},
			WorkDir:                   t.TempDir(),
			AgentEnv:                  []string{"PATH=/usr/bin"},
			Protocol:                  agent.ProtocolACP,
			NotificationQueueCapacity: 4096,
		},
		logger: newTestLogger(t),
	}

	if err := m.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig: %v", err)
	}
	t.Cleanup(func() { _ = m.adapter.Close() })

	if got := m.adapterCfg.NotificationQueueCapacity; got != 4096 {
		t.Fatalf("adapter notification queue capacity = %d, want 4096", got)
	}
}

func TestStartOneShotPreservesConfiguredTempEnvironment(t *testing.T) {
	log := newTestLogger(t)
	workDir := t.TempDir()
	serviceTemp := setServiceTempTestEnv(t)
	agentEnv := []string{
		"PATH=/usr/bin",
		"TMPDIR=/configured/tmpdir",
		"TMP=/configured/tmp",
		"TEMP=/configured/temp",
	}

	m := &Manager{
		cfg: &config.InstanceConfig{
			SessionID: "session-1",
			WorkDir:   workDir,
			AgentEnv:  agentEnv,
		},
		logger:  log,
		adapter: newOneShotStubAdapter(),
		adapterCfg: &adapter.Config{
			OneShotConfig: &adapter.OneShotConfig{
				Env:     agentEnv,
				WorkDir: workDir,
			},
		},
		updatesCh:          make(chan adapter.AgentEvent, 100),
		pendingPermissions: make(map[string]*PendingPermission),
		workspaceTracker:   NewWorkspaceTracker(workDir, log),
	}

	require.NoError(t, m.startOneShot())
	t.Cleanup(func() {
		close(m.stopCh)
		m.wg.Wait()
		m.workspaceTracker.Stop()
	})

	want := map[string]string{
		"TMPDIR": "/configured/tmpdir",
		"TMP":    "/configured/tmp",
		"TEMP":   "/configured/temp",
	}
	for key, value := range want {
		if got := lookupEnvValue(m.adapterCfg.OneShotConfig.Env, key); got != value {
			t.Fatalf("OneShotConfig.Env %s = %q, want configured service value %q", key, got, value)
		}
	}
	assertNoAgentTempRoot(t, serviceTemp)
}
