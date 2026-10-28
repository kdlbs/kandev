package lifecycle

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

type recordingPassthroughRunner struct {
	startReq   process.InteractiveStartRequest
	startCalls int
}

func (r *recordingPassthroughRunner) Start(_ context.Context, req process.InteractiveStartRequest) (*process.InteractiveProcessInfo, error) {
	r.startReq = req
	r.startCalls++
	return &process.InteractiveProcessInfo{ID: "pty-initial", SessionID: req.SessionID, Command: req.Command}, nil
}

func (r *recordingPassthroughRunner) Stop(context.Context, string) error { return nil }
func (r *recordingPassthroughRunner) WriteStdin(string, string) error    { return nil }
func (r *recordingPassthroughRunner) ResizeBySession(string, uint16, uint16) error {
	return nil
}
func (r *recordingPassthroughRunner) GetBuffer(string) ([]process.ProcessOutputChunk, bool) {
	return nil, false
}
func (r *recordingPassthroughRunner) WaitForFirstIdle(context.Context, string) error { return nil }
func (r *recordingPassthroughRunner) IsProcessReadyOrPending(string) bool            { return false }
func (r *recordingPassthroughRunner) ConnectSessionWebSocket(string) bool            { return false }
func (r *recordingPassthroughRunner) HasActiveWebSocketBySession(string) bool        { return false }
func (r *recordingPassthroughRunner) WriteToDirectOutputBySession(string, []byte) error {
	return nil
}

func newCopilotPassthroughInitialManager(t *testing.T) (*Manager, *AgentExecution, *AgentProfileInfo, *recordingPassthroughRunner, string) {
	t.Helper()
	cacheRoot, configDir, pluginRunner := copilotPluginFixture(t)
	mgr, execution, _ := newPassthroughMCPTestManager(t, agents.CopilotACPAgentID)
	profile := &AgentProfileInfo{
		ProfileID: "profile-1", AgentName: agents.CopilotACPAgentID,
		CLIPassthrough:         true,
		CopilotSelectedPlugins: []string{"installed:copilot-plugins/spark"},
		EnvVars: []models.ProfileEnvVar{
			{Key: "COPILOT_HOME", Value: configDir},
			{Key: "PATH", Value: os.Getenv("PATH")},
		},
	}
	mgr.profileResolver = &mockPassthroughProfileResolver{
		agentName: agents.CopilotACPAgentID, cliPassthrough: true, profile: profile,
	}
	mgr.SetCopilotPluginRunner(pluginRunner)
	pty := &recordingPassthroughRunner{}
	mgr.passthroughRunnerOverride = pty
	execution.AgentID = agents.CopilotACPAgentID
	execution.AgentProfileID = profile.ProfileID
	execution.ExecutorType = "local"
	require.NoError(t, mgr.executionStore.Add(execution))
	return mgr, execution, profile, pty, cacheRoot
}

func TestCopilotPluginInitialPassthroughRejectsUnverifiedSelection(t *testing.T) {
	mgr, execution, profile, pty, _ := newCopilotPassthroughInitialManager(t)
	err := mgr.startPassthroughSession(context.Background(), execution, profile)
	var pluginErr *copilotplugins.Error
	require.ErrorAs(t, err, &pluginErr)
	require.Equal(t, copilotplugins.ReasonPolicyUnverified, pluginErr.Reason)
	require.Zero(t, pty.startCalls, "an unqualified selection must fail before creating a PTY process")
}

func TestCopilotPluginInitialPassthroughEmptySelectionStarts(t *testing.T) {
	mgr, execution, profile, pty, _ := newCopilotPassthroughInitialManager(t)
	profile.CopilotSelectedPlugins = nil
	require.NoError(t, mgr.startPassthroughSession(context.Background(), execution, profile))
	require.Equal(t, 1, pty.startCalls)
	require.NotContains(t, strings.Join(pty.startReq.Command, " "), "--plugin-dir")
}

func TestCopilotPluginInitialPassthroughRejectsUnsupportedExecutor(t *testing.T) {
	mgr, execution, profile, pty, _ := newCopilotPassthroughInitialManager(t)
	execution.ExecutorType = "local_docker"
	err := mgr.startPassthroughSession(context.Background(), execution, profile)
	var pluginErr *copilotplugins.Error
	require.ErrorAs(t, err, &pluginErr)
	require.Equal(t, copilotplugins.ReasonUnsupportedLaunch, pluginErr.Reason)
	require.Zero(t, pty.startCalls, "an unsupported executor must fail before creating a PTY process")
}

func TestCopilotPluginInitialPassthroughRejectsWrapper(t *testing.T) {
	mgr, execution, profile, pty, _ := newCopilotPassthroughInitialManager(t)
	execution.LaunchWrapper = copilotWrapperForTest()
	err := mgr.startPassthroughSession(context.Background(), execution, profile)
	var pluginErr *copilotplugins.Error
	require.ErrorAs(t, err, &pluginErr)
	require.Equal(t, copilotplugins.ReasonUnsupportedLaunch, pluginErr.Reason)
	require.Zero(t, pty.startCalls)
}
