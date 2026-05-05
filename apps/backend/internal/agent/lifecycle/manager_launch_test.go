package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
)

// resumeTestAgent is a minimal agent with a BuildCommand that respects the
// Resume helper (--resume <session_id>) so we can verify buildAgentCommand
// correctly gates the flag based on CanRecover.
type resumeTestAgent struct {
	testAgent
	canRecover *bool
}

func (a *resumeTestAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	return agents.Cmd("test-agent", "--acp").
		Resume(agents.NewParam("--resume"), opts.SessionID, false).
		Build()
}

func (a *resumeTestAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{
		Cmd: agents.NewCommand("test-agent", "--acp"),
		SessionConfig: agents.SessionConfig{
			CanRecover: a.canRecover,
		},
	}
}

func TestBuildAgentCommand_ResumeFlag(t *testing.T) {
	mgr := newTestManager()
	canRecoverTrue := true
	canRecoverFalse := false

	t.Run("CanRecover=true with ACPSessionID includes --resume", func(t *testing.T) {
		ag := &resumeTestAgent{canRecover: &canRecoverTrue}
		req := &LaunchRequest{ACPSessionID: "sess-123"}
		cmds := mgr.buildAgentCommand(req, nil, ag)
		require.Contains(t, cmds.initial, "--resume")
		require.Contains(t, cmds.initial, "sess-123")
	})

	t.Run("CanRecover=false with ACPSessionID omits --resume", func(t *testing.T) {
		ag := &resumeTestAgent{canRecover: &canRecoverFalse}
		req := &LaunchRequest{ACPSessionID: "sess-123"}
		cmds := mgr.buildAgentCommand(req, nil, ag)
		require.False(t, strings.Contains(cmds.initial, "--resume"),
			"expected no --resume flag, got: %s", cmds.initial)
		require.False(t, strings.Contains(cmds.initial, "sess-123"),
			"expected no session ID in command, got: %s", cmds.initial)
	})

	t.Run("CanRecover=true with empty ACPSessionID omits --resume", func(t *testing.T) {
		ag := &resumeTestAgent{canRecover: &canRecoverTrue}
		req := &LaunchRequest{ACPSessionID: ""}
		cmds := mgr.buildAgentCommand(req, nil, ag)
		require.False(t, strings.Contains(cmds.initial, "--resume"),
			"expected no --resume flag when ACPSessionID is empty, got: %s", cmds.initial)
	})

	t.Run("CanRecover=nil (default true) with ACPSessionID includes --resume", func(t *testing.T) {
		ag := &resumeTestAgent{canRecover: nil}
		req := &LaunchRequest{ACPSessionID: "sess-456"}
		cmds := mgr.buildAgentCommand(req, nil, ag)
		require.Contains(t, cmds.initial, "--resume")
		require.Contains(t, cmds.initial, "sess-456")
	})
}

// cliFlagTestAgent is a minimal BuildCommand that produces a stable prefix
// so tests can assert CLI flag tokens are appended after the agent's own
// argv by CommandBuilder.BuildCommand (not by the agent itself).
type cliFlagTestAgent struct{ testAgent }

func (a *cliFlagTestAgent) BuildCommand(_ agents.CommandOptions) agents.Command {
	return agents.Cmd("copilot", "--acp").Build()
}

func TestBuildAgentCommand_CLIFlagsAppended(t *testing.T) {
	mgr := newTestManager()
	ag := &cliFlagTestAgent{}

	t.Run("enabled entries reach argv, disabled do not", func(t *testing.T) {
		profile := &AgentProfileInfo{
			ProfileID: "p1",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: "--allow-all-tools", Enabled: true},
				{Flag: "--allow-all-paths", Enabled: false}, // must be skipped
				{Flag: "--add-dir /shared", Enabled: true},  // must be split
			},
		}
		cmds := mgr.buildAgentCommand(&LaunchRequest{}, profile, ag)

		require.Contains(t, cmds.initial, "--allow-all-tools")
		require.NotContains(t, cmds.initial, "--allow-all-paths")
		// The tokeniser splits "--add-dir /shared" into two argv elements,
		// not one — confirm both are present.
		require.Contains(t, cmds.initial, "--add-dir")
		require.Contains(t, cmds.initial, "/shared")
	})

	t.Run("malformed flag does not abort launch — falls back to empty tokens", func(t *testing.T) {
		profile := &AgentProfileInfo{
			ProfileID: "p2",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: `--broken "unterminated`, Enabled: true},
			},
		}
		cmds := mgr.buildAgentCommand(&LaunchRequest{}, profile, ag)
		// The bad flag is dropped entirely; the launch still produces the
		// agent's base command so a user with a typo still gets their task
		// to run, just without the flag they intended.
		require.Equal(t, "copilot --acp", cmds.initial)
	})

	t.Run("nil profile produces bare command", func(t *testing.T) {
		cmds := mgr.buildAgentCommand(&LaunchRequest{}, nil, ag)
		require.Equal(t, "copilot --acp", cmds.initial)
	})
}

// trackingPreparer records whether Prepare was called.
type trackingPreparer struct {
	called bool
}

func (p *trackingPreparer) Name() string { return "tracking" }

func (p *trackingPreparer) Prepare(_ context.Context, _ *EnvPrepareRequest, _ PrepareProgressCallback) (*EnvPrepareResult, error) {
	p.called = true
	return &EnvPrepareResult{Success: true, WorkspacePath: "/tmp/ws"}, nil
}

func TestRunEnvironmentPreparer_CalledOnFreshLaunch(t *testing.T) {
	mgr := newTestManager()
	preparer := &trackingPreparer{}
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(executor.NameStandalone, preparer)

	req := &LaunchRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		ExecutorType:   string(executor.NameStandalone),
		RepositoryPath: "/tmp/repo",
	}

	result := mgr.runEnvironmentPreparer(context.Background(), req, "/tmp/repo")
	require.True(t, preparer.called, "preparer should be called on fresh launch")
	require.NotNil(t, result)
	require.True(t, result.Success)
}

func TestRunEnvironmentPreparer_SkippedWithoutRepoPath(t *testing.T) {
	mgr := newTestManager()
	preparer := &trackingPreparer{}
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(executor.NameStandalone, preparer)

	req := &LaunchRequest{
		TaskID:       "task-1",
		SessionID:    "session-1",
		ExecutorType: string(executor.NameStandalone),
		// No RepositoryPath — preparer should be skipped
	}

	result := mgr.runEnvironmentPreparer(context.Background(), req, "")
	require.False(t, preparer.called, "preparer should be skipped when no repository path")
	require.Nil(t, result)
}

func TestLaunchResolveWorkspacePath_EphemeralCreatesQuickChatDir(t *testing.T) {
	mgr := newTestManager()
	mgr.dataDir = t.TempDir()

	req := &LaunchRequest{
		SessionID:   "session-abc",
		IsEphemeral: true,
	}

	workspacePath, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	require.NotEmpty(t, workspacePath, "ephemeral task should get a quick-chat workspace")
	require.Contains(t, workspacePath, "quick-chat")
	require.Contains(t, workspacePath, "session-abc")
}

func TestLaunchResolveWorkspacePath_NonEphemeralSkipsQuickChatDir(t *testing.T) {
	mgr := newTestManager()
	mgr.dataDir = t.TempDir()

	req := &LaunchRequest{
		SessionID:   "session-xyz",
		IsEphemeral: false,
	}

	workspacePath, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	require.Empty(t, workspacePath, "non-ephemeral task without repo should NOT get a quick-chat workspace")
}

// TestLaunch_PromotesWorkspaceOnlyExecution verifies that when Launch finds an
// existing workspace-only execution in the store (created by a peer
// EnsureWorkspaceExecutionForSession / GetOrEnsureExecution call), it promotes
// it in place by populating AgentCommand instead of returning an
// "already has an agent running" error. This regression test covers the
// singleflight-collision bug that surfaced as "Task failed to start" toasts on
// backend restart, where a workspace-only execution was returned to the resume
// path and StartAgentProcess() then failed with "no agent command configured".
func TestLaunch_PromotesWorkspaceOnlyExecution(t *testing.T) {
	mgr := newTestManager()

	// Inject a workspace-only execution: AgentCommand is intentionally empty,
	// matching what createExecution produces when called from
	// ensureWorkspaceExecutionLocked.
	existing := &AgentExecution{
		ID:             "exec-workspace-only",
		SessionID:      "session-1",
		TaskID:         "task-1",
		AgentProfileID: "profile-1",
	}
	require.NoError(t, mgr.executionStore.Add(existing))

	req := &LaunchRequest{
		TaskID:              "task-1",
		SessionID:           "session-1",
		AgentProfileID:      "profile-1",
		ACPSessionID:        "acp-session-abc",
		PreviousExecutionID: "exec-prev",
	}

	got, err := mgr.Launch(context.Background(), req)
	require.NoError(t, err)
	require.Same(t, existing, got, "Launch must reuse the workspace-only execution, not create a new one")
	require.NotEmpty(t, got.AgentCommand, "AgentCommand must be populated by promotion")
	require.Equal(t, "acp-session-abc", got.ACPSessionID, "ACPSessionID must be carried over from the request")
	require.True(t, got.isResumedSession, "isResumedSession must be set when PreviousExecutionID is non-empty")
}

// TestLaunch_RejectsWhenAgentAlreadyRunning verifies the original "already has
// an agent running" guard still fires when the existing execution is a real
// agent-equipped one (AgentCommand populated), preventing duplicate launches.
func TestLaunch_RejectsWhenAgentAlreadyRunning(t *testing.T) {
	mgr := newTestManager()

	existing := &AgentExecution{
		ID:             "exec-running",
		SessionID:      "session-2",
		TaskID:         "task-2",
		AgentProfileID: "profile-2",
		AgentCommand:   "auggie --acp",
	}
	require.NoError(t, mgr.executionStore.Add(existing))

	req := &LaunchRequest{
		TaskID:         "task-2",
		SessionID:      "session-2",
		AgentProfileID: "profile-2",
	}

	_, err := mgr.Launch(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already has an agent running")
}

// TestLaunch_RaceRollback exercises the race window between the step-3 duplicate
// session pre-check and the step-8 executionStore.Add in Launch. A barrier inside
// CreateInstance keeps the goroutine in the race window; the test injects a
// conflicting execution into the store and then releases the barrier. Launch must
// roll back the runtime instance it created (StopInstance called once) and return
// a "race resolved during register" error instead of leaking an orphaned subprocess.
func TestLaunch_RaceRollback(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	execRegistry := NewExecutorRegistry(log)

	entered := make(chan struct{}, 1)
	barrier := make(chan struct{})
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		client:       (*agentctl.Client)(nil),
		entered:      entered,
		barrier:      barrier,
	}
	execRegistry.Register(backend)

	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.dataDir = t.TempDir()
	t.Cleanup(func() { close(mgr.stopCh) })

	req := &LaunchRequest{
		TaskID:         "task-1",
		SessionID:      "session-race",
		AgentProfileID: "profile-1",
		IsEphemeral:    true, // gives Launch a workspace path without a real repo
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := mgr.Launch(context.Background(), req)
		errCh <- err
	}()

	// Wait for CreateInstance to begin (the goroutine is now past the step-3
	// pre-check and inside the race window).
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for CreateInstance to start")
	}

	// Inject a conflicting execution for the same session.
	_ = mgr.executionStore.Add(&AgentExecution{
		ID:        "exec-injected",
		SessionID: "session-race",
		TaskID:    "task-1",
	})

	// Release CreateInstance — Launch will now proceed to Add and discover
	// the conflict, triggering rollbackRacedExecution.
	close(barrier)

	err := <-errCh
	if err == nil {
		t.Fatal("expected error from race rollback, got nil")
	}
	if !strings.Contains(err.Error(), "race resolved during register") {
		t.Errorf("unexpected error message: %v", err)
	}
	if got := backend.stopCount.Load(); got != 1 {
		t.Errorf("StopInstance called %d times, want 1 (runtime instance must be stopped on rollback)", got)
	}
}
