package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
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
