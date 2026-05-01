package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
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
