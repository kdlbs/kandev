package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestCursorMCPAuthenticationCommandUsesPOSIXExecAndQuotesInputs(t *testing.T) {
	got, err := cursorMCPAuthenticationCommand("/Applications/Cursor's Tools/cursor-agent", "plugin-fixture's-server", "darwin")
	require.NoError(t, err)
	require.Equal(t, "exec '/Applications/Cursor'\\''s Tools/cursor-agent' mcp login 'plugin-fixture'\\''s-server'", got)
}

func TestCursorMCPAuthenticationCommandRejectsUnverifiedWindowsShell(t *testing.T) {
	_, err := cursorMCPAuthenticationCommand(`C:\Cursor Tools\cursor-agent.exe`, "plugin-fixture-server", "windows")
	require.ErrorIs(t, err, ErrCursorMCPAuthenticationUnsupported)
	require.NotErrorIs(t, err, ErrCursorMCPRecoveryUnavailable)
}

func TestCursorMCPAgentConfigAcceptsPassthroughCursorStrategy(t *testing.T) {
	config := &testAgent{
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			MCPStrategy: mcpconfig.CursorStrategy{},
		}},
		runtimeConfig: &agents.RuntimeConfig{},
	}

	require.True(t, cursorMCPAgentConfig(config))
}

func TestCursorMCPRecoveryUnavailableSentinelRemainsDistinct(t *testing.T) {
	require.False(t, errors.Is(ErrCursorMCPAuthenticationUnsupported, ErrCursorMCPRecoveryUnavailable))
}

func TestCursorMCPRecoverySnapshotReplacesOnlyTheRetriedServer(t *testing.T) {
	manager, eventBus := newPrepareEventsTestManager(t, "cursor-recovery-profile")
	execution := &AgentExecution{
		TaskID: "task-mcp", SessionID: "session-mcp", WorkspacePath: "/workspace",
		PrepareResult: &EnvPrepareResult{Success: false, Steps: []PrepareStep{
			{Name: "Environment", Kind: "executor_environment", Status: PrepareStepCompleted},
			{Name: "Discovery A", Kind: PrepareStepKindAgentMCPDiscovery, MCPServerID: "server-a", Status: PrepareStepCompleted},
			{Name: "Selection A", Kind: PrepareStepKindAgentMCPSelection, MCPServerID: "server-a", Status: PrepareStepCompleted},
			{Name: "Verification A", Kind: PrepareStepKindAgentMCPVerification, MCPServerID: "server-a", Status: PrepareStepFailed},
			{Name: "Verification B", Kind: PrepareStepKindAgentMCPVerification, MCPServerID: "server-b", Status: PrepareStepFailed},
		}},
	}
	recorder := manager.newPreparationAttemptRecorder(execution.TaskID, execution.SessionID)
	seedCursorMCPRecoverySteps(execution, recorder, "server-a")
	appendCursorMCPProgress(recorder, "server-a", PrepareStepKindAgentMCPApproval, PrepareStepCompleted, "", nil, nil)
	appendCursorMCPProgress(recorder, "server-a", PrepareStepKindAgentMCPVerification, PrepareStepCompleted, "", nil, nil)

	manager.publishExecutionPrepareCompleted(execution, recorder, nil)

	require.True(t, execution.PrepareResult.Success)
	steps := execution.PrepareResult.Steps
	requirePrepareStep(t, steps, "Environment")
	requirePrepareStep(t, steps, "Discovery A")
	requirePrepareStep(t, steps, "Selection A")
	requirePrepareStep(t, steps, "Verification B")
	for _, step := range steps {
		require.NotEqual(t, "Verification A", step.Name)
	}
	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.True(t, completed[0].Success)
	requirePrepareStep(t, completed[0].Steps, "Verification B")
}

func TestCursorMCPRecoverySeedsAllPersistedStepsWhenResultIsNotLoaded(t *testing.T) {
	manager, execution, _ := newCursorMCPRecoveryFixture(t)
	execution.PrepareResult = nil
	execution.setMetadataValue("prepare_result", SerializePrepareResult(&EnvPrepareResult{Steps: []PrepareStep{
		{Name: "Environment", Kind: "executor_environment", Status: PrepareStepCompleted},
		{Name: "Verification A", Kind: PrepareStepKindAgentMCPVerification, MCPServerID: "server-a", Status: PrepareStepFailed},
		{Name: "Verification B", Kind: PrepareStepKindAgentMCPVerification, MCPServerID: "server-b", Status: PrepareStepFailed},
	}}))
	recorder := manager.newPreparationAttemptRecorder(execution.TaskID, execution.SessionID)

	seedCursorMCPRecoverySteps(execution, recorder, "server-a")

	steps := recorder.Steps()
	require.Len(t, steps, 2)
	require.Equal(t, "Environment", steps[0].Name)
	require.Equal(t, "Verification B", steps[1].Name)
}

func TestCursorProjectMCPRejectsEscapingParentDirectorySymlink(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	writeCursorMCPRecoveryPlugin(t, filepath.Join(os.Getenv("HOME"), ".cursor"))
	external := t.TempDir()
	require.NoError(t, os.Symlink(external, filepath.Join(execution.WorkspacePath, ".cursor")))
	manager.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		return mcpconfig.CursorNativeInventory{}, nil
	}

	err := manager.reconcileAndMaterializeCursorProjectMCP(
		context.Background(), execution, agents.NewCursorACP(), profile,
		string(executor.NameLocal), agents.NewCursorACP().Runtime().ProjectMCPStrategy,
	)

	require.ErrorContains(t, err, "unsafe Cursor MCP config path")
	_, err = os.Lstat(filepath.Join(external, "mcp.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCursorMCPAuthPreparationStatusSeparatesSkippedAndFailed(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*testing.T, *Manager, *AgentExecution, *AgentProfileInfo)
		wantStatus PrepareStepStatus
		wantCode   string
		wantErr    bool
	}{
		{
			name: "auth disabled",
			setup: func(_ *testing.T, _ *Manager, _ *AgentExecution, profile *AgentProfileInfo) {
				profile.CursorMCPAuthEnabled = false
			},
			wantStatus: PrepareStepSkipped,
			wantCode:   "disabled",
		},
		{
			name:       "remote executor",
			setup:      func(_ *testing.T, _ *Manager, _ *AgentExecution, _ *AgentProfileInfo) {},
			wantStatus: PrepareStepSkipped,
			wantCode:   "ineligible",
		},
		{
			name: "different runtime home",
			setup: func(t *testing.T, _ *Manager, execution *AgentExecution, _ *AgentProfileInfo) {
				execution.setRuntimeEnvironment(map[string]string{"HOME": t.TempDir()})
			},
			wantStatus: PrepareStepSkipped,
			wantCode:   "home_mismatch",
		},
		{
			name: "enabled bridge failure",
			setup: func(t *testing.T, _ *Manager, _ *AgentExecution, _ *AgentProfileInfo) {
				projects := filepath.Join(os.Getenv("HOME"), ".cursor", "projects")
				require.NoError(t, os.MkdirAll(filepath.Dir(projects), 0o700))
				require.NoError(t, os.WriteFile(projects, []byte("blocked"), 0o600))
			},
			wantStatus: PrepareStepFailed,
			wantCode:   "unavailable",
		},
		{
			name:       "unresolved profile",
			setup:      func(_ *testing.T, _ *Manager, _ *AgentExecution, _ *AgentProfileInfo) {},
			wantStatus: PrepareStepFailed,
			wantCode:   "unavailable",
			wantErr:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager, execution, profile := newCursorMCPRecoveryFixture(t)
			profile.CursorMCPAuthEnabled = true
			executorType := string(executor.NameLocal)
			if tc.name == "remote executor" {
				executorType = "ssh"
			}
			tc.setup(t, manager, execution, profile)
			if tc.name == "unresolved profile" {
				profile = nil
			}
			status, code, err := manager.prepareCursorMCPAuthWithStatus(execution, profile, executorType, mcpconfig.CursorStrategy{})
			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantCode, code)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMaterializeCursorMCPPreparationSurfacesOptionalCredentialBridgeFailure(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	profile.CursorMCPAuthEnabled = true
	profile.CursorPluginsMCPEnabled = false
	projects := filepath.Join(os.Getenv("HOME"), ".cursor", "projects")
	require.NoError(t, os.MkdirAll(filepath.Dir(projects), 0o700))
	require.NoError(t, os.WriteFile(projects, []byte("blocked"), 0o600))
	agentConfig, ok := manager.registry.Get("cursor-acp")
	require.True(t, ok)
	recorder := manager.newPreparationAttemptRecorder(execution.TaskID, execution.SessionID)

	err := manager.materializeRuntimeProjectMCPWithPreparation(context.Background(), execution, agentConfig, profile,
		string(executor.NameLocal), recorder)

	require.NoError(t, err, "optional credential bridge errors degrade MCP readiness")
	steps := recorder.Steps()
	require.Len(t, steps, 1)
	require.Equal(t, PrepareStepKindAgentMCPCredentials, steps[0].Kind)
	require.Equal(t, PrepareStepFailed, steps[0].Status)
	require.Equal(t, "unavailable", steps[0].FailureCode)
}

func TestRetryCursorMCPConnectionLoadsSameACPSessionWithoutPromptReplay(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
	runner := &cursorMCPRecoveryRecordingRunner{}
	manager.SetCursorNativeMCPCommandRunner(runner)

	server := newMockAgentServer(t)
	defer server.Close()
	loadPayloads := make(chan map[string]any, 1)
	server.handler = func(msg ws.Message) *ws.Message {
		if msg.Action != "agent.session.load" {
			return server.defaultHandler(msg)
		}
		var payload map[string]any
		_ = json.Unmarshal(msg.Payload, &payload)
		loadPayloads <- payload
		response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
		return response
	}
	client := createTestClient(t, server.server.URL)
	defer client.Close()
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	require.NoError(t, client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil))
	select {
	case <-server.wsConnected:
	case <-time.After(time.Second):
		t.Fatal("agent stream did not connect")
	}
	execution.agentctl = client

	result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")

	require.NoError(t, err)
	require.Equal(t, "ready", result.Status, "native commands: %v", runner.commands())
	select {
	case payload := <-loadPayloads:
		require.Equal(t, execution.ACPSessionID, payload["session_id"])
	case <-time.After(time.Second):
		t.Fatal("same ACP session was not loaded")
	}
	require.Equal(t, []string{"agent.initialize", "agent.session.load"}, server.getActionLog())
	require.Equal(t, []string{"stop", "configure", "start"}, server.getHTTPActionLog())
	require.Equal(t, "cursor-acp-session", execution.ACPSessionID)
	require.Equal(t, []string{"mcp enable plugin-harness-figma", "mcp list-tools plugin-harness-figma"}, runner.commands())
}

func TestRetryCursorMCPConnectionDoesNotReportReadyWhenSameSessionLoadFails(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
	runner := &cursorMCPRecoveryRecordingRunner{}
	manager.SetCursorNativeMCPCommandRunner(runner)

	server := newMockAgentServer(t)
	defer server.Close()
	server.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.session.load" {
			response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "load failed", nil)
			return response
		}
		return server.defaultHandler(msg)
	}
	client := createTestClient(t, server.server.URL)
	defer client.Close()
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	require.NoError(t, client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil))
	select {
	case <-server.wsConnected:
	case <-time.After(time.Second):
		t.Fatal("agent stream did not connect")
	}
	execution.agentctl = client

	result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")

	require.NoError(t, err)
	require.Equal(t, "unavailable", result.Status)
	require.Equal(t, "cursor-acp-session", execution.ACPSessionID, "a failed load must retain the saved ACP identity")
	require.Equal(t, []string{"agent.initialize", "agent.session.load"}, server.getActionLog())
	require.Equal(t, []string{"stop", "configure", "start"}, server.getHTTPActionLog())
	require.NotContains(t, server.getActionLog(), "agent.session.new")
	require.NotContains(t, server.getActionLog(), "agent.prompt")
	var verificationStatus PrepareStepStatus
	for _, step := range execution.PrepareResult.Steps {
		if step.Kind == PrepareStepKindAgentMCPVerification && step.MCPServerID == "plugin-harness-figma" {
			verificationStatus = step.Status
		}
	}
	require.Equal(t, PrepareStepFailed, verificationStatus)
	require.Equal(t, []string{"mcp enable plugin-harness-figma", "mcp list-tools plugin-harness-figma"}, runner.commands())
}

func TestRetryCursorMCPConnectionRejectsBusySessionBeforeNativeCommands(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
	runner := &cursorMCPRecoveryRecordingRunner{}
	manager.SetCursorNativeMCPCommandRunner(runner)
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 1
	execution.promptCompletionGeneration = 0
	execution.promptLifecycleMu.Unlock()

	_, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")

	require.ErrorIs(t, err, ErrCursorMCPRecoverySessionBusy)
	require.Empty(t, runner.commands())
}

func TestRetryCursorMCPConnectionFailsClosedForPassthroughWithoutOwnedChatID(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
	runner := &cursorMCPRecoveryRecordingRunner{}
	manager.SetCursorNativeMCPCommandRunner(runner)
	execution.PassthroughProcessID = "cursor-tui-process"
	execution.ACPSessionID = ""

	result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")

	require.NoError(t, err)
	require.Equal(t, "unavailable", result.Status)
	require.Equal(t, "session_reload_unsupported", result.ReasonCode)
	require.Empty(t, runner.commands())
}

func TestRetryCursorMCPConnectionRejectsChangedProfilePolicyOrOwnership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *AgentExecution, *AgentProfileInfo)
	}{
		{
			name: "deselected profile source",
			change: func(_ *testing.T, _ *AgentExecution, profile *AgentProfileInfo) {
				profile.MCPSelectedServers = nil
			},
		},
		{
			name: "executor policy",
			change: func(_ *testing.T, execution *AgentExecution, _ *AgentProfileInfo) {
				execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{})
			},
		},
		{
			name: "owned definition changed",
			change: func(t *testing.T, execution *AgentExecution, _ *AgentProfileInfo) {
				configPath := filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json")
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(configPath, []byte(strings.ReplaceAll(string(data), "fixture.example", "changed.example")), 0o600))
			},
		},
		{
			name: "source definition changed",
			change: func(t *testing.T, _ *AgentExecution, _ *AgentProfileInfo) {
				configPath := filepath.Join(os.Getenv("HOME"), ".cursor", "plugins", "local", "harness", "mcp.json")
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(configPath, []byte(strings.ReplaceAll(string(data), "fixture.example", "changed.example")), 0o600))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, execution, profile := newCursorMCPRecoveryFixture(t)
			prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
			runner := &cursorMCPRecoveryRecordingRunner{}
			manager.SetCursorNativeMCPCommandRunner(runner)
			tc.change(t, execution, profile)

			result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")

			if err != nil {
				require.ErrorIs(t, err, ErrCursorMCPRecoveryUnavailable)
			} else {
				require.Equal(t, "unavailable", result.Status)
			}
			require.Empty(t, runner.commands())
		})
	}
}

func TestRetryCursorMCPConnectionRechecksDisableAfterEnableBeforeVerify(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	writeCursorMCPRecoveryPlugin(t, filepath.Join(os.Getenv("HOME"), ".cursor"))
	manager.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		return mcpconfig.CursorNativeInventory{}, nil
	}
	require.NoError(t, manager.reconcileAndMaterializeCursorProjectMCP(
		context.Background(), execution, agents.NewCursorACP(), profile,
		string(executor.NameLocal), agents.NewCursorACP().Runtime().ProjectMCPStrategy,
	))

	runner := &cursorMCPRecoveryBarrierRunner{enableStarted: make(chan struct{}), releaseEnable: make(chan struct{})}
	manager.SetCursorNativeMCPCommandRunner(runner)
	type retryOutcome struct {
		result CursorMCPRetryResult
		err    error
	}
	done := make(chan retryOutcome, 1)
	go func() {
		result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")
		done <- retryOutcome{result: result, err: err}
	}()
	<-runner.enableStarted
	writeCursorMCPDisabledForPath(t, filepath.Join(os.Getenv("HOME"), ".cursor"), execution.WorkspacePath, []string{"plugin-harness-figma"})
	close(runner.releaseEnable)

	outcome := <-done
	require.NoError(t, outcome.err)
	require.Equal(t, "unavailable", outcome.result.Status)
	require.Equal(t, []string{"mcp enable plugin-harness-figma"}, runner.commands())
}

func TestRetryCursorMCPConnectionFencesNativeIOWhenNewPreparationStarts(t *testing.T) {
	for _, blockedCommand := range []string{"enable", "list-tools"} {
		t.Run(blockedCommand, func(t *testing.T) {
			manager, execution, profile := newCursorMCPRecoveryFixture(t)
			prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
			runner := &cursorMCPRecoveryCommandBarrierRunner{
				blockedCommand: blockedCommand,
				commandStarted: make(chan struct{}, 1),
				releaseCommand: make(chan struct{}),
			}
			manager.SetCursorNativeMCPCommandRunner(runner)
			type retryOutcome struct {
				result CursorMCPRetryResult
				err    error
			}
			done := make(chan retryOutcome, 1)
			go func() {
				result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")
				done <- retryOutcome{result: result, err: err}
			}()
			select {
			case <-runner.commandStarted:
			case <-time.After(time.Second):
				close(runner.releaseCommand)
				t.Fatal("native MCP command did not start")
			}

			nextGeneration := beginCursorProjectMCPPreparation(cursorProjectMCPWorkspaceKey(execution.WorkspacePath))
			defer finishCursorProjectMCPPreparation(cursorProjectMCPWorkspaceKey(execution.WorkspacePath), nextGeneration)
			close(runner.releaseCommand)
			outcome := <-done

			require.NoError(t, outcome.err)
			require.Equal(t, "unavailable", outcome.result.Status)
			wantCommands := []string{"mcp enable plugin-harness-figma"}
			if blockedCommand == "list-tools" {
				wantCommands = append(wantCommands, "mcp list-tools plugin-harness-figma")
			}
			require.Equal(t, wantCommands, runner.commands())
			require.Equal(t, "cursor-acp-session", execution.ACPSessionID)
		})
	}
}

type cursorMCPRecoveryBarrierRunner struct {
	mu            sync.Mutex
	calls         []string
	enableStarted chan struct{}
	releaseEnable chan struct{}
}

type cursorMCPRecoveryRecordingRunner struct {
	mu    sync.Mutex
	calls []string
}

type cursorMCPRecoveryCommandBarrierRunner struct {
	mu             sync.Mutex
	calls          []string
	blockedCommand string
	commandStarted chan struct{}
	releaseCommand chan struct{}
}

func (r *cursorMCPRecoveryRecordingRunner) Run(_ context.Context, _ string, args []string, _ string, _ map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	if len(args) != 3 || args[0] != "mcp" {
		return mcpconfig.NativeMCPCommandResult{}, mcpconfig.ErrNativeMCPExecutableUnavailable
	}
	r.mu.Lock()
	r.calls = append(r.calls, "mcp "+args[1]+" "+args[2])
	r.mu.Unlock()
	if args[1] == "enable" {
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0}, nil
	}
	return mcpconfig.NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("Tools for " + args[2] + " (1):\n- fixture_tool ()\n")}, nil
}

func (r *cursorMCPRecoveryRecordingRunner) commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func prepareCursorMCPRecoveryWorkspace(t *testing.T, manager *Manager, execution *AgentExecution, profile *AgentProfileInfo) {
	t.Helper()
	writeCursorMCPRecoveryPlugin(t, filepath.Join(os.Getenv("HOME"), ".cursor"))
	manager.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		return mcpconfig.CursorNativeInventory{}, nil
	}
	require.NoError(t, manager.reconcileAndMaterializeCursorProjectMCP(
		context.Background(), execution, agents.NewCursorACP(), profile,
		string(executor.NameLocal), agents.NewCursorACP().Runtime().ProjectMCPStrategy,
	))
}

func (r *cursorMCPRecoveryBarrierRunner) Run(_ context.Context, _ string, args []string, _ string, _ map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	r.mu.Lock()
	r.calls = append(r.calls, "mcp "+args[1]+" "+args[2])
	r.mu.Unlock()
	if args[1] == "enable" {
		close(r.enableStarted)
		<-r.releaseEnable
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0}, nil
	}
	return mcpconfig.NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("Tools for " + args[2] + " (1):\n- fixture_tool ()\n")}, nil
}

func (r *cursorMCPRecoveryBarrierRunner) commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *cursorMCPRecoveryCommandBarrierRunner) Run(_ context.Context, _ string, args []string, _ string, _ map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	command := "mcp " + args[1] + " " + args[2]
	r.mu.Lock()
	r.calls = append(r.calls, command)
	r.mu.Unlock()
	if args[1] == r.blockedCommand {
		r.commandStarted <- struct{}{}
		<-r.releaseCommand
	}
	if args[1] == "enable" {
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0}, nil
	}
	return mcpconfig.NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("Tools for " + args[2] + " (1):\n- fixture_tool ()\n")}, nil
}

func (r *cursorMCPRecoveryCommandBarrierRunner) commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func newCursorMCPRecoveryFixture(t *testing.T) (*Manager, *AgentExecution, *AgentProfileInfo) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	profile := &AgentProfileInfo{
		ProfileID: "cursor-recovery-profile", AgentName: "cursor-acp",
		CursorPluginsMCPEnabled: true, MCPSelectionMode: "selected",
		MCPSelectedServers: []string{"plugin-harness-figma"},
	}
	manager := newTestManager(t)
	manager.profileResolver = &mockPassthroughProfileResolver{profile: profile}
	execution := &AgentExecution{
		ID: "cursor-recovery-execution", TaskID: "cursor-recovery-task", TaskEnvironmentID: "cursor-recovery-env",
		SessionID: "cursor-recovery-session", AgentProfileID: profile.ProfileID, ACPSessionID: "cursor-acp-session",
		ExecutorType: string(executor.NameLocal), WorkspacePath: workspace, Status: v1.AgentStatusRunning,
		AgentCommand: "cursor-agent", AgentArgs: []string{"acp"},
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false},
	}
	execution.setRuntimeEnvironment(map[string]string{"HOME": home})
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	require.NoError(t, manager.executionStore.Add(execution))
	return manager, execution, profile
}

func writeCursorMCPRecoveryPlugin(t *testing.T, cursorHome string) {
	t.Helper()
	plugin := filepath.Join(cursorHome, "plugins", "local", "harness")
	require.NoError(t, os.MkdirAll(plugin, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(plugin, "mcp.json"),
		[]byte(`{"mcpServers":{"figma":{"url":"https://fixture.example/mcp"}}}`), 0o600))
}
