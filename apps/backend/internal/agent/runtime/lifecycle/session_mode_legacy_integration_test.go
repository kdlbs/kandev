package lifecycle

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	acpadapter "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.3, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.4, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.8, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.9, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.10, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.11
func TestLegacyConfirmedModeLifecycle(t *testing.T) {
	t.Run("fresh start admits matching mode without provider mutation", func(t *testing.T) {
		runLegacyConfirmedModeStart(t, []string{"default"}, "default", "", false, "default", 0)
	})

	t.Run("native load admits its own matching report", func(t *testing.T) {
		runLegacyConfirmedModeStart(t, nil, "default", "saved-session", true, "default", 0)
	})

	t.Run("silent different mode holds the prompt", func(t *testing.T) {
		runLegacyConfirmedModeStart(t, []string{"default"}, "default", "", false, "ask", 1)
	})

	t.Run("load without its own mode report cannot reuse cached mode", func(t *testing.T) {
		runLegacyConfirmedModeStart(t, nil, "", "saved-session", true, "default", 1)
	})

	t.Run("context reset admits a prompt with the replacement report", func(t *testing.T) {
		runLegacyConfirmedModeReset(t, []string{"default", "default"}, 0, true)
	})

	t.Run("context reset without a replacement report holds the prompt", func(t *testing.T) {
		runLegacyConfirmedModeReset(t, []string{"default", ""}, 1, false)
	})
}

func runLegacyConfirmedModeStart(
	t *testing.T,
	newModeReports []string,
	loadMode string,
	existingSessionID string,
	nativeResume bool,
	selectedMode string,
	wantProviderModeRequests int,
) {
	t.Helper()
	fixture := newLegacyModeLifecycleBridge(t, newModeReports, loadMode)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mgr := newTestManager(t)
	// This fixture owns the WS stream so its disconnect callback can be joined
	// deterministically; initialization should use the already-connected stream.
	mgr.sessionManager.streamManager = nil
	execution := &AgentExecution{
		ID: "exec-legacy-mode", TaskID: "task-legacy-mode", SessionID: "session-legacy-mode",
		AgentID: "auggie", AgentProfileID: "profile-legacy-mode", AgentCommand: "auggie --model test",
		WorkspacePath: t.TempDir(), ACPSessionID: existingSessionID,
		Status: v1.AgentStatusRunning, agentctl: fixture.client,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	if existingSessionID != "" {
		execution.SetModeState(&CachedModeState{
			CurrentModeID:  "default",
			AvailableModes: []streams.SessionModeInfo{{ID: "default"}, {ID: "ask"}},
		})
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	promptDispatched := make(chan struct{}, 1)
	execution.setInitialPromptDispatchCallbacks(nil, func() {
		promptDispatched <- struct{}{}
		execution.promptDoneCh <- PromptCompletionSignal{StopReason: "test-complete"}
	}, nil)
	agentConfig := &testAgent{id: "auggie", enabled: true, runtimeConfig: &agents.RuntimeConfig{
		Cmd: agents.NewCommand("auggie"), Protocol: agent.ProtocolACP,
		SessionConfig:  agents.SessionConfig{NativeSessionResume: nativeResume},
		ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
	}}

	err := mgr.sessionManager.InitializeAndPromptWithLayers(
		ctx, execution, agentConfig, "continue the task", nil, nil,
		func(string) error { return nil }, "", "ask", nil, "", selectedMode, nil, StartModelPolicy{},
	)
	if wantProviderModeRequests == 0 {
		if err != nil {
			t.Fatalf("InitializeAndPromptWithLayers: %v", err)
		}
		select {
		case <-promptDispatched:
		case <-ctx.Done():
			t.Fatalf("initial prompt was not admitted: %v", ctx.Err())
		}
	} else {
		if err == nil {
			t.Fatal("InitializeAndPromptWithLayers succeeded without confirmed mode")
		}
		select {
		case <-promptDispatched:
			t.Fatal("initial prompt was admitted after mode confirmation failed")
		default:
		}
	}

	modeCalls, prompts := fixture.providerCounts()
	if modeCalls != wantProviderModeRequests {
		t.Fatalf("provider mode mutation calls = %d, want %d", modeCalls, wantProviderModeRequests)
	}
	wantPrompts := 0
	if wantProviderModeRequests == 0 {
		wantPrompts = 1
	}
	if prompts != wantPrompts {
		t.Fatalf("provider prompts = %d, want %d", prompts, wantPrompts)
	}
	if got := fixture.modeSelections(); !slices.Equal(got, []string{selectedMode}) {
		t.Fatalf("selected mode requests = %v, want only runtime-selected %q", got, selectedMode)
	}
	assertLegacyModeLifecycleActionOrder(t, fixture.mock, existingSessionID != "", wantPrompts == 1)
}

func runLegacyConfirmedModeReset(t *testing.T, newModeReports []string, wantProviderModeRequests int, wantPrompt bool) {
	t.Helper()
	fixture := newLegacyModeLifecycleBridge(t, newModeReports, "default")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := fixture.adapter.Initialize(ctx); err != nil {
		t.Fatalf("adapter.Initialize: %v", err)
	}
	previousSessionID, err := fixture.adapter.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("adapter.NewSession: %v", err)
	}

	mgr := newTestManager(t)
	// The fixture owns the stream and closes it after reset assertions.
	mgr.sessionManager.streamManager = nil
	execution := &AgentExecution{
		ID: "exec-legacy-mode-reset", TaskID: "task-legacy-mode-reset", SessionID: "session-legacy-mode-reset",
		AgentID: "auggie", AgentProfileID: "profile-legacy-mode", AgentCommand: "auggie --model test",
		WorkspacePath: t.TempDir(), ACPSessionID: previousSessionID,
		Status: v1.AgentStatusRunning, sessionInitialized: true, agentctl: fixture.client,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.SetModeState(&CachedModeState{
		CurrentModeID:  "default",
		AvailableModes: []streams.SessionModeInfo{{ID: "default"}, {ID: "ask"}},
	})
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	err = mgr.ResetAgentContext(ctx, execution.ID)
	if wantPrompt && err != nil {
		t.Fatalf("ResetAgentContext: %v", err)
	}
	if !wantPrompt && err == nil {
		t.Fatal("ResetAgentContext succeeded without a mode report from the replacement session")
	}
	if execution.ACPSessionID == previousSessionID {
		t.Fatalf("reset session id = %q; want a replacement for %q", execution.ACPSessionID, previousSessionID)
	}

	if wantPrompt {
		result, promptErr := mgr.sessionManager.SendPrompt(ctx, execution, "after context reset", true, nil, true)
		if promptErr != nil || result == nil || result.StopReason != PromptStopReasonDispatched {
			t.Fatalf("post-reset SendPrompt = %+v, %v; want admitted prompt", result, promptErr)
		}
	} else {
		if _, promptErr := mgr.sessionManager.SendPrompt(ctx, execution, "after context reset", true, nil, true); promptErr == nil {
			t.Fatal("prompt was admitted after context reset failed mode confirmation")
		}
	}

	modeCalls, prompts := fixture.providerCounts()
	if modeCalls != wantProviderModeRequests {
		t.Fatalf("provider mode mutation calls = %d, want %d", modeCalls, wantProviderModeRequests)
	}
	wantPromptCount := 0
	if wantPrompt {
		wantPromptCount = 1
	}
	if prompts != wantPromptCount {
		t.Fatalf("provider prompts = %d, want %d", prompts, wantPromptCount)
	}
	if got := fixture.modeSelections(); !slices.Equal(got, []string{"default"}) {
		t.Fatalf("selected mode requests = %v, want [default]", got)
	}
	assertLegacyModeResetActionOrder(t, fixture.mock, wantPrompt)
}

func assertLegacyModeLifecycleActionOrder(t *testing.T, mock *mockAgentServer, loaded, prompt bool) {
	t.Helper()
	actions := mock.getActionLog()
	sessionAction := "agent.session.new"
	if loaded {
		sessionAction = "agent.session.load"
	}
	sessionIndex, modeIndex, promptIndex := -1, -1, -1
	for i, action := range actions {
		switch action {
		case sessionAction:
			sessionIndex = i
		case "agent.session.set_mode":
			modeIndex = i
		case "agent.prompt":
			promptIndex = i
		}
	}
	if sessionIndex < 0 || modeIndex <= sessionIndex {
		t.Fatalf("actions = %v, want %s before confirmed selected mode", actions, sessionAction)
	}
	if prompt && (promptIndex <= modeIndex) {
		t.Fatalf("actions = %v, want selected mode before provider prompt", actions)
	}
	if !prompt && promptIndex >= 0 {
		t.Fatalf("actions = %v, unexpected prompt after mode confirmation failed", actions)
	}
}

func assertLegacyModeResetActionOrder(t *testing.T, mock *mockAgentServer, prompt bool) {
	t.Helper()
	actions := mock.getActionLog()
	resetIndex, modeIndex, promptIndex := -1, -1, -1
	for i, action := range actions {
		switch action {
		case "agent.session.reset":
			resetIndex = i
		case "agent.session.set_mode":
			modeIndex = i
		case "agent.prompt":
			promptIndex = i
		}
	}
	if resetIndex < 0 || modeIndex <= resetIndex {
		t.Fatalf("actions = %v, want reset before selected mode", actions)
	}
	if prompt && promptIndex <= modeIndex {
		t.Fatalf("actions = %v, want mode confirmation before prompt", actions)
	}
	if !prompt && promptIndex >= 0 {
		t.Fatalf("actions = %v, unexpected prompt after reset mode confirmation failed", actions)
	}
}

type legacyModeLifecycleBridge struct {
	adapter  *acpadapter.Adapter
	client   *agentctl.Client
	mock     *mockAgentServer
	provider *legacyModeLifecycleACPAgent
}

type legacyModeLifecycleACPAgent struct {
	acpsdk.Agent
	mu               sync.Mutex
	newModeReports   []string
	loadModeReport   string
	newSessionCount  int
	modeRequests     []string
	modeSelections   []string
	promptSessionIDs []string
}

var _ acpsdk.Agent = (*legacyModeLifecycleACPAgent)(nil)

func newLegacyModeLifecycleBridge(t *testing.T, newModeReports []string, loadModeReport string) *legacyModeLifecycleBridge {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	provider := &legacyModeLifecycleACPAgent{
		newModeReports: append([]string(nil), newModeReports...),
		loadModeReport: loadModeReport,
	}
	adapter := acpadapter.NewAdapter(&shared.Config{AgentID: "auggie", WorkDir: t.TempDir()}, newSessionTestLogger())
	if err := adapter.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
		t.Fatalf("adapter.Connect: %v", err)
	}
	agentConnection := acpsdk.NewAgentSideConnection(provider, agentToClientWriter, clientToAgentReader)
	mock := newMockAgentServer(t)
	mock.handler = func(msg ws.Message) *ws.Message {
		requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		failed := func(err error) *ws.Message {
			response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
			return response
		}
		success := func(payload any) *ws.Message {
			response, _ := ws.NewResponse(msg.ID, msg.Action, payload)
			return response
		}
		switch msg.Action {
		case "agent.initialize":
			if err := adapter.Initialize(requestCtx); err != nil {
				return failed(err)
			}
			return success(agentctl.InitializeResponse{Success: true, AgentInfo: &agentctl.AgentInfo{Name: "auggie", Version: "test"}})
		case "agent.session.new":
			sessionID, err := adapter.NewSession(requestCtx, nil)
			if err != nil {
				return failed(err)
			}
			return success(agentctl.NewSessionResponse{Success: true, SessionID: sessionID})
		case "agent.session.load":
			var request struct {
				SessionID string `json:"session_id"`
			}
			if err := msg.ParsePayload(&request); err != nil {
				return failed(err)
			}
			if err := adapter.LoadSession(requestCtx, request.SessionID, nil); err != nil {
				return failed(err)
			}
			return success(map[string]any{"success": true, "session_id": request.SessionID})
		case "agent.session.reset":
			sessionID, err := adapter.ResetSession(requestCtx, nil)
			if err != nil {
				return failed(err)
			}
			return success(agentctl.NewSessionResponse{Success: true, SessionID: sessionID})
		case "agent.session.set_mode":
			var request struct {
				ModeID string `json:"mode_id"`
			}
			if err := msg.ParsePayload(&request); err != nil {
				return failed(err)
			}
			provider.recordModeSelection(request.ModeID)
			result, err := adapter.SetMode(requestCtx, request.ModeID)
			if err != nil {
				return failed(err)
			}
			return success(result)
		case "agent.prompt":
			var request struct {
				Text             string                 `json:"text"`
				Attachments      []v1.MessageAttachment `json:"attachments"`
				PromptGeneration uint64                 `json:"prompt_generation"`
			}
			if err := msg.ParsePayload(&request); err != nil {
				return failed(err)
			}
			if err := adapter.Prompt(requestCtx, request.Text, request.Attachments, request.PromptGeneration); err != nil {
				return failed(err)
			}
			return success(map[string]any{"success": true})
		default:
			return mock.defaultHandler(msg)
		}
	}

	client := createTestClient(t, mock.server.URL)
	streamCtx, cancelStream := context.WithCancel(context.Background())
	disconnected := make(chan struct{})
	var disconnectedOnce sync.Once
	if err := client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, func(error) {
		disconnectedOnce.Do(func() { close(disconnected) })
	}); err != nil {
		cancelStream()
		client.Close()
		mock.Close()
		_ = adapter.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
		t.Fatalf("connect agent stream: %v", err)
	}
	select {
	case <-mock.wsConnected:
	case <-time.After(5 * time.Second):
		t.Fatal("agent stream never connected")
	}
	updatesDone := make(chan struct{})
	go func() {
		defer close(updatesDone)
		for range adapter.Updates() {
		}
	}()

	bridge := &legacyModeLifecycleBridge{adapter: adapter, client: client, mock: mock, provider: provider}
	t.Cleanup(func() {
		cancelStream()
		client.Close()
		mock.Close()
		waitLegacyModeFixtureDone(t, disconnected, "agentctl client stream")
		_ = adapter.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
		waitLegacyModeFixtureDone(t, updatesDone, "adapter update drain")
		if clientConnection := adapter.GetACPConnection(); clientConnection != nil {
			waitLegacyModeFixtureDone(t, clientConnection.Done(), "ACP client connection")
		}
		waitLegacyModeFixtureDone(t, agentConnection.Done(), "ACP agent connection")
	})
	return bridge
}

func waitLegacyModeFixtureDone(t *testing.T, done <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("%s did not stop during cleanup", name)
	}
}

func legacyModeLifecycleModeState(current string) *acpsdk.SessionModeState {
	return &acpsdk.SessionModeState{
		CurrentModeId: acpsdk.SessionModeId(current),
		AvailableModes: []acpsdk.SessionMode{
			{Id: "default", Name: "Default"},
			{Id: "ask", Name: "Ask"},
		},
	}
}

func (a *legacyModeLifecycleACPAgent) Initialize(_ context.Context, request acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion:   request.ProtocolVersion,
		AgentCapabilities: acpsdk.AgentCapabilities{LoadSession: true},
	}, nil
}

func (a *legacyModeLifecycleACPAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	a.mu.Lock()
	a.newSessionCount++
	newSessionCount := a.newSessionCount
	currentMode := "default"
	if newSessionCount <= len(a.newModeReports) {
		currentMode = a.newModeReports[newSessionCount-1]
	}
	a.mu.Unlock()
	return acpsdk.NewSessionResponse{
		SessionId: acpsdk.SessionId(fmt.Sprintf("provider-session-%d", newSessionCount)),
		Modes:     legacyModeLifecycleModeState(currentMode),
	}, nil
}

func (a *legacyModeLifecycleACPAgent) LoadSession(context.Context, acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	return acpsdk.LoadSessionResponse{Modes: legacyModeLifecycleModeState(a.loadModeReport)}, nil
}

func (a *legacyModeLifecycleACPAgent) SetSessionMode(_ context.Context, request acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	a.mu.Lock()
	a.modeRequests = append(a.modeRequests, string(request.ModeId))
	a.mu.Unlock()
	return acpsdk.SetSessionModeResponse{}, nil
}

func (a *legacyModeLifecycleACPAgent) Prompt(_ context.Context, request acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	a.mu.Lock()
	a.promptSessionIDs = append(a.promptSessionIDs, string(request.SessionId))
	a.mu.Unlock()
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *legacyModeLifecycleACPAgent) recordModeSelection(mode string) {
	a.mu.Lock()
	a.modeSelections = append(a.modeSelections, mode)
	a.mu.Unlock()
}

func (a *legacyModeLifecycleBridge) providerCounts() (modeCalls, prompts int) {
	a.provider.mu.Lock()
	defer a.provider.mu.Unlock()
	return len(a.provider.modeRequests), len(a.provider.promptSessionIDs)
}

func (a *legacyModeLifecycleBridge) modeSelections() []string {
	a.provider.mu.Lock()
	defer a.provider.mu.Unlock()
	return append([]string(nil), a.provider.modeSelections...)
}
