package lifecycle

import (
	"context"
	"path/filepath"
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

type progressPreparer struct{}

func (p *progressPreparer) Name() string { return "docker" }

func (p *progressPreparer) Prepare(_ context.Context, _ *EnvPrepareRequest, onProgress PrepareProgressCallback) (*EnvPrepareResult, error) {
	step := beginStep("Validate Docker")
	reportProgress(onProgress, step, 0, 1)
	completeStepSuccess(&step)
	reportProgress(onProgress, step, 0, 1)
	return &EnvPrepareResult{Success: true, Steps: []PrepareStep{step}, WorkspacePath: "/tmp/ws"}, nil
}

func TestLaunch_PublishesPrepareCompletedAfterRuntimeProgress(t *testing.T) {
	log := newTestLogger()
	execRegistry := NewExecutorRegistry(log)
	entered := make(chan struct{}, 1)
	barrier := make(chan struct{})
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		client:       newReadyAgentctlClient(t, log),
		entered:      entered,
		barrier:      barrier,
		progressStep: "Waiting for Docker container",
	}
	execRegistry.Register(backend)

	eventBus := &MockEventBusWithTracking{}
	mgr := NewManager(
		newTestRegistry(), eventBus, execRegistry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.preparerRegistry = NewPreparerRegistry(log)
	mgr.preparerRegistry.Register(executor.NameDocker, &progressPreparer{})
	t.Cleanup(func() { close(mgr.stopCh) })

	errCh := make(chan error, 1)
	go func() {
		_, err := mgr.Launch(context.Background(), &LaunchRequest{
			TaskID:         "task-1",
			SessionID:      "session-1",
			AgentProfileID: "profile-1",
			ExecutorType:   "local_docker",
			RepositoryPath: "/tmp/repo",
			BaseBranch:     "main",
		})
		errCh <- err
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runtime CreateInstance")
	}

	if completed := prepareCompletedPayloads(eventBus); len(completed) != 0 {
		t.Fatalf("PrepareCompleted published before runtime finished: %#v", completed)
	}

	close(barrier)
	if err := <-errCh; err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	completed := prepareCompletedPayloads(eventBus)
	require.NotEmpty(t, completed)
	final := completed[len(completed)-1]
	require.True(t, final.Success)
	requirePrepareStep(t, final.Steps, "Validate Docker")
	requirePrepareStep(t, final.Steps, "Waiting for Docker container")
}

func prepareCompletedPayloads(eventBus *MockEventBusWithTracking) []*PrepareCompletedEventPayload {
	eventBus.mu.Lock()
	defer eventBus.mu.Unlock()
	var out []*PrepareCompletedEventPayload
	for _, tracked := range eventBus.PublishedEvents {
		payload, ok := tracked.Event.Data.(*PrepareCompletedEventPayload)
		if ok {
			out = append(out, payload)
		}
	}
	return out
}

func requirePrepareStep(t *testing.T, steps []PrepareStep, name string) {
	t.Helper()
	for _, step := range steps {
		if step.Name == name {
			return
		}
	}
	t.Fatalf("expected prepare step %q in %#v", name, steps)
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

func TestLaunchResolveWorkspacePath_NonEphemeralRepoLessGetsScratchDir(t *testing.T) {
	mgr := newTestManager()
	mgr.dataDir = t.TempDir()

	req := &LaunchRequest{
		SessionID:   "session-xyz",
		TaskID:      "task-xyz",
		WorkspaceID: "ws-xyz",
		IsEphemeral: false,
	}

	workspacePath, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	require.NotEmpty(t, workspacePath, "non-ephemeral task without repo should still get a scratch workspace")
	// New layout: <homeDir>/tasks/<workspaceID>/<taskID> (sibling to worktree task dirs).
	require.Contains(t, workspacePath, filepath.Join("tasks", "ws-xyz", "task-xyz"))
	require.NotContains(t, workspacePath, "quick-chat")
}

func TestLaunchResolveWorkspacePath_NonEphemeralWithoutWorkspaceIDReturnsEmpty(t *testing.T) {
	mgr := newTestManager()
	mgr.dataDir = t.TempDir()

	// Non-ephemeral repo-less task missing workspace_id should not get a path
	// (scratch path requires workspace + task IDs to namespace correctly).
	req := &LaunchRequest{
		SessionID:   "session-no-ws",
		TaskID:      "task-1",
		IsEphemeral: false,
	}

	workspacePath, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	require.Empty(t, workspacePath)
}

func TestLaunchResolveWorkspacePath_PickedFolderUsedDirectly(t *testing.T) {
	mgr := newTestManager()
	mgr.dataDir = t.TempDir()
	picked := t.TempDir() // some existing folder the user picked

	req := &LaunchRequest{
		SessionID:     "session-pick",
		WorkspacePath: picked,
	}

	workspacePath, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	require.Equal(t, picked, workspacePath, "picked workspace_path should be used as-is, not replaced by scratch")
}

func TestLaunchResolveWorkspacePath_WorktreeWithoutRepoFallsBackToScratch(t *testing.T) {
	mgr := newTestManager()
	mgr.dataDir = t.TempDir()

	// UseWorktree=true but no RepositoryPath — should not return empty,
	// should fall through to the scratch workspace path.
	req := &LaunchRequest{
		SessionID:   "session-wt",
		TaskID:      "task-wt",
		WorkspaceID: "ws-wt",
		UseWorktree: true,
	}

	workspacePath, _, _, _ := mgr.launchResolveWorkspacePath(context.Background(), req)
	require.NotEmpty(t, workspacePath, "worktree-mode task without repo should fall through to scratch")
	require.Contains(t, workspacePath, filepath.Join("tasks", "ws-wt", "task-wt"))
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

func TestLaunch_PersistsDockerRuntimeSecrets(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	execRegistry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		client:       newReadyAgentctlClient(t, log),
		authToken:    "agentctl-token",
		nonce:        "bootstrap-nonce",
	}
	execRegistry.Register(backend)

	store := newInMemorySecretStore()
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry,
		&MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.SetSecretStore(store)
	mgr.dataDir = t.TempDir()
	t.Cleanup(func() { close(mgr.stopCh) })

	execution, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		ExecutorType:   "local_docker",
		IsEphemeral:    true,
	})
	if err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}

	if got := mgr.revealRuntimeSecret(context.Background(), execution.Metadata, MetadataKeyAuthTokenSecret); got != "agentctl-token" {
		t.Fatalf("revealed auth token = %q, want agentctl-token", got)
	}
	if got := mgr.revealRuntimeSecret(context.Background(), execution.Metadata, MetadataKeyBootstrapNonceSecret); got != "bootstrap-nonce" {
		t.Fatalf("revealed bootstrap nonce = %q, want bootstrap-nonce", got)
	}
}
