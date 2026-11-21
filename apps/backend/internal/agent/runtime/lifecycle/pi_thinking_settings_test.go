package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestPiThinkingStartupFailures(t *testing.T) {
	tests := []struct {
		name          string
		agentID       string
		result        agentctl.ModeResult
		refused       bool
		wantError     string
		wantCode      string
		wantReason    string
		wantEffective string
	}{
		{name: "unconfirmed", result: agentctl.ModeResult{Requested: "medium"}, wantError: "confirmed", wantCode: "thinking_level_unconfirmed", wantReason: "confirmation_missing"},
		{name: "confirmed without effective mode", result: agentctl.ModeResult{Requested: "medium", Confirmed: true}, wantError: "confirmed", wantCode: "thinking_level_unconfirmed", wantReason: "confirmation_missing"},
		{name: "clamped", result: agentctl.ModeResult{Requested: "medium", Effective: "off", Confirmed: true}, wantError: "not applied", wantCode: "thinking_level_mismatch", wantReason: "effective_mismatch", wantEffective: "off"},
		{name: "refused", refused: true, wantError: "apply requested thinking level", wantCode: "thinking_level_failed", wantReason: "application_failed"},
		{name: "other provider still requires confirmation", agentID: "codex", result: agentctl.ModeResult{Requested: "medium"}, wantError: "confirmed", wantCode: "permission_mode_unconfirmed", wantReason: "confirmation_missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(msg ws.Message) *ws.Message {
				if msg.Action == "agent.session.set_mode" {
					if tc.refused {
						response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "mode refused", nil)
						return response
					}
					response, _ := ws.NewResponse(msg.ID, msg.Action, tc.result)
					return response
				}
				return mock.defaultHandler(msg)
			}

			log := newSessionTestLogger()
			stopCh := newTestStopCh(t)
			sm := NewSessionManager(log, stopCh)
			sm.SetDependencies(NewEventPublisher(&MockEventBusWithTracking{}, log), nil, nil, nil)
			client := createTestClient(t, mock.server.URL)
			disconnected := connectAgentStream(t, mock, client)
			t.Cleanup(func() {
				closeStopChOnce(stopCh)
				client.Close()
				select {
				case <-disconnected:
				case <-time.After(5 * time.Second):
					t.Error("agent stream did not finish draining during cleanup")
				}
			})
			agentID := tc.agentID
			if agentID == "" {
				agentID = "pi-acp"
			}
			execution := &AgentExecution{
				AgentID: agentID, ID: "exec-mode-unapplied", TaskID: "task-mode-unapplied", SessionID: "session-mode-unapplied",
				TaskScope: TaskLaunchScopeTask, WorkspacePath: "/workspace", agentctl: client,
				promptDoneCh: make(chan PromptCompletionSignal, 1),
			}
			agentConfig := &testAgent{id: agentID, enabled: true, runtimeConfig: &agents.RuntimeConfig{
				Cmd: agents.NewCommand(agentID), Protocol: agent.ProtocolACP,
				SessionConfig: agents.SessionConfig{}, ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
			}}

			err := sm.InitializeAndPromptWithLayers(
				context.Background(), execution, agentConfig, "do work", nil, nil,
				func(string) error { return nil }, "", "medium", nil, "", "", nil, StartModelPolicy{},
			)
			t.Logf("bootstrap error: %v", err)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("mode error = %v, want %q", err, tc.wantError)
			}
			var failure *BootstrapFailure
			if !errors.As(err, &failure) {
				t.Fatalf("mode error %T does not preserve typed bootstrap evidence: %v", err, err)
			}
			if failure.Code != tc.wantCode {
				t.Errorf("cause code = %q, want %q", failure.Code, tc.wantCode)
			}
			requested, effective := failure.RequestedSetting, failure.EffectiveSetting
			if tc.agentID == "codex" {
				requested, effective = failure.RequestedMode, failure.EffectiveMode
			}
			if failure.Reason != tc.wantReason || requested != "medium" || effective != tc.wantEffective {
				t.Errorf("mode evidence = (%q, %q, %q), want (%q, plan, %q)",
					failure.Reason, failure.RequestedMode, failure.EffectiveMode, tc.wantReason, tc.wantEffective)
			}
			if failure.PromptNotSent == nil || !*failure.PromptNotSent {
				t.Error("prompt_not_sent evidence = false or unknown, want true")
			}
			for _, action := range mock.getActionLog() {
				if action == "agent.prompt" {
					t.Fatal("initial prompt was dispatched with an unapplied explicit mode")
				}
			}
		})
	}
}

func TestPiThinkingStartupWinningIntent(t *testing.T) {
	tests := []struct{ name, profileMode, profileThinking, runtimeMode, runtimeThinking, want, model string }{
		{"canonical wins within layer", "medium", "low", "high", "off", "off", ""},
		{"higher legacy source wins", "medium", "low", "off", "", "off", ""},
		{"profile canonical", "medium", "off", "", "", "off", ""},
		{"provider default", "", "", "", "", "", ""},
		{"previous model catalog cannot veto selection", "", "medium", "", "", "medium", "thinking-model"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			modeRequests := make(chan string, 2)
			mock.handler = func(msg ws.Message) *ws.Message {
				if tc.model != "" && msg.Action == "agent.session.new" {
					response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
						"success": true, "session_id": "test-session-123",
						"model_state": streams.SessionModelState{
							CurrentModelID: "off-only",
							Models:         []streams.SessionModelInfo{{ModelID: "off-only"}, {ModelID: tc.model}},
							ConfigOptions: []streams.ConfigOption{{
								Type: "select", ID: "thought_level", Category: "thought_level", CurrentValue: "off",
								Options: []streams.ConfigOptionValue{{Value: "off"}},
							}},
						},
					})
					return response
				}
				// The peer acknowledges SetModel while the catalog update is still queued.
				if msg.Action == "agent.session.set_mode" {
					var request struct {
						ModeID string `json:"mode_id"`
					}
					_ = msg.ParsePayload(&request)
					modeRequests <- request.ModeID
					response, _ := ws.NewResponse(msg.ID, msg.Action, agentctl.ModeResult{
						Requested: request.ModeID, Effective: request.ModeID, Confirmed: true,
					})
					return response
				}
				return mock.defaultHandler(msg)
			}

			log := newSessionTestLogger()
			stopCh := newTestStopCh(t)
			sm := NewSessionManager(log, stopCh)
			sm.SetDependencies(NewEventPublisher(&MockEventBusWithTracking{}, log), nil, nil, nil)
			client := createTestClient(t, mock.server.URL)
			disconnected := connectAgentStream(t, mock, client)
			t.Cleanup(func() {
				closeStopChOnce(stopCh)
				client.Close()
				select {
				case <-disconnected:
				case <-time.After(5 * time.Second):
					t.Error("agent stream did not finish draining during cleanup")
				}
			})
			execution := &AgentExecution{
				AgentID: "pi-acp", ID: "exec-mode-override", TaskID: "task-mode-override", SessionID: "session-mode-override",
				WorkspacePath: "/workspace", agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1),
			}
			promptDispatched := make(chan struct{}, 1)
			execution.setInitialPromptDispatchCallbacks(nil, func() {
				promptDispatched <- struct{}{}
				execution.promptDoneCh <- PromptCompletionSignal{StopReason: "test-complete"}
			}, nil)
			agentConfig := &testAgent{id: "pi-acp", enabled: true, runtimeConfig: &agents.RuntimeConfig{
				Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP,
				SessionConfig: agents.SessionConfig{}, ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
			}}

			err := sm.InitializeAndPromptWithLayers(
				context.Background(), execution, agentConfig, "do work", nil, nil,
				func(string) error { return nil }, tc.model, tc.profileMode, map[string]string{"thought_level": tc.profileThinking}, "", tc.runtimeMode, map[string]string{"thought_level": tc.runtimeThinking}, StartModelPolicy{},
			)
			if err != nil {
				t.Fatalf("InitializeAndPromptWithLayers: %v", err)
			}
			select {
			case <-promptDispatched:
			case <-time.After(3 * time.Second):
				t.Fatal("initial prompt was not dispatched")
			}
			select {
			case mode := <-modeRequests:
				if mode != tc.want {
					t.Fatalf("applied mode = %q, want winning runtime override", mode)
				}
			default:
				if tc.want != "" {
					t.Fatal("winning mode was not applied")
				}
			}
			select {
			case mode := <-modeRequests:
				t.Fatalf("applied a second mode %q; profile mode must not be applied before its override", mode)
			default:
			}
			actions := mock.getActionLog()
			modeIndex, promptIndex, modelIndex := -1, -1, -1
			for i, action := range actions {
				if action == "agent.session.set_mode" && modeIndex == -1 {
					modeIndex = i
				}
				if action == "agent.session.set_model" {
					modelIndex = i
				}
				if action == "agent.prompt" {
					promptIndex = i
				}
			}
			if tc.model != "" && (modelIndex < 0 || modelIndex >= modeIndex) {
				t.Fatalf("actions = %v, want model before thinking", actions)
			}
			if (tc.want != "" && (modeIndex < 0 || modeIndex >= promptIndex)) || promptIndex < 0 {
				t.Fatalf("actions = %v, want winning mode before prompt", actions)
			}

		})
	}
}

func TestPiThinkingStartupProviderRestoredOmitsAliases(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	var mu sync.Mutex
	var loadedSessionID string
	var configWrites []struct{ id, value, policy string }
	var promptTexts []string
	mock.handler = func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.session.load":
			var request struct {
				SessionID string `json:"session_id"`
			}
			_ = msg.ParsePayload(&request)
			mu.Lock()
			loadedSessionID = request.SessionID
			mu.Unlock()
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success":    true,
				"session_id": request.SessionID,
				"model_state": streams.SessionModelState{
					CurrentModelID: "provider-default",
					Models: []streams.SessionModelInfo{
						{ModelID: "profile-model"}, {ModelID: "runtime-model"},
					},
					ConfigOptions: []streams.ConfigOption{
						{ID: "primary_model", CurrentValue: "provider-default"},
						{ID: "PRIMARY_MODEL", Category: "model", CurrentValue: "provider-default"},
						{ID: "permission_level", CurrentValue: "default"},
						{ID: "permission_mode", Category: "mode", CurrentValue: "default"},
						{ID: "reasoning_effort", Category: "reasoning", CurrentValue: "medium"},
						{ID: "thought_level", Type: "select", Category: "thought_level", CurrentValue: "off", Options: []streams.ConfigOptionValue{{Value: "off"}}},
					},
					ConfigOptionsSettled: true,
				},
			})
			return response
		case "agent.session.set_config_option":
			var request struct {
				ConfigID              string `json:"config_id"`
				Value                 string `json:"value"`
				SessionSettingsPolicy string `json:"session_settings_policy"`
			}
			_ = msg.ParsePayload(&request)
			mu.Lock()
			configWrites = append(configWrites, struct{ id, value, policy string }{request.ConfigID, request.Value, request.SessionSettingsPolicy})
			mu.Unlock()
		case "agent.prompt":
			var request struct {
				Text string `json:"text"`
			}
			_ = msg.ParsePayload(&request)
			mu.Lock()
			promptTexts = append(promptTexts, request.Text)
			mu.Unlock()
		}
		return mock.defaultHandler(msg)
	}

	log := newSessionTestLogger()
	stopCh := newTestStopCh(t)
	sm := NewSessionManager(log, stopCh)
	eventBus := &MockEventBusWithTracking{}
	sm.SetDependencies(NewEventPublisher(eventBus, log), nil, nil, nil)
	client := createTestClient(t, mock.server.URL)
	disconnected := connectAgentStream(t, mock, client)
	t.Cleanup(func() {
		closeStopChOnce(stopCh)
		client.Close()
		select {
		case <-disconnected:
		case <-time.After(5 * time.Second):
			t.Error("agent stream did not finish draining during cleanup")
		}
	})
	execution := &AgentExecution{
		AgentID: "pi-acp", ID: "exec-provider-restored", TaskID: "task-provider-restored", SessionID: "session-provider-restored",
		ACPSessionID: "saved-native-token", TaskScope: TaskLaunchScopeTask,
		SessionSettingsPolicy:           SessionSettingsPolicyProviderRestored,
		SessionSettingsProjectionPolicy: SessionSettingsPolicyProviderRestored,
		WorkspacePath:                   "/workspace", agentctl: client,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	agentConfig := &testAgent{id: "pi-acp", enabled: true, runtimeConfig: &agents.RuntimeConfig{
		Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP,
		SessionConfig:  agents.SessionConfig{NativeSessionResume: true},
		ResourceLimits: agents.ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
	}}
	profileOptions := map[string]string{
		"thought_level": "medium", "primary_model": "saved-profile-model", "permission_mode": "saved-profile-mode",
		"permission_level": "saved-profile-level", "reasoning_effort": "profile-effort",
	}
	runtimeOptions := map[string]string{
		"thought_level": "high", "primary_model": "saved-runtime-model", "permission_mode": "saved-runtime-mode",
		"permission_level": "saved-runtime-level", "reasoning_effort": "runtime-effort",
	}
	profileOptionsBefore := map[string]string{}
	for id, value := range profileOptions {
		profileOptionsBefore[id] = value
	}
	runtimeOptionsBefore := map[string]string{}
	for id, value := range runtimeOptions {
		runtimeOptionsBefore[id] = value
	}
	readyCalls := 0
	err := sm.InitializeAndPromptWithLayers(
		context.Background(), execution, agentConfig, "", nil, nil,
		func(string) error { readyCalls++; return nil },
		"profile-model", "saved-profile-mode", profileOptions,
		"runtime-model", "saved-runtime-mode", runtimeOptions,
		StartModelPolicy{RequireExactModel: true},
	)
	if err != nil {
		t.Fatalf("InitializeAndPromptWithLayers: %v", err)
	}
	if !execution.isSessionInitialized() {
		t.Fatal("provider-restored session was not initialized")
	}
	if readyCalls != 1 {
		t.Fatalf("mark ready calls = %d, want 1", readyCalls)
	}
	mu.Lock()
	gotLoadedSessionID := loadedSessionID
	gotConfigWrites := append([]struct{ id, value, policy string }(nil), configWrites...)
	mu.Unlock()
	if gotLoadedSessionID != "saved-native-token" {
		t.Fatalf("session/load token = %q, want saved-native-token", gotLoadedSessionID)
	}
	for _, action := range mock.getActionLog() {
		if action == "agent.session.new" || action == "agent.session.set_model" || action == "agent.session.set_mode" {
			t.Errorf("provider-restored startup must not issue %q", action)
		}
	}
	if len(gotConfigWrites) != 2 || gotConfigWrites[0] != (struct{ id, value, policy string }{"reasoning_effort", "profile-effort", string(streams.SessionSettingsPolicyProviderRestored)}) || gotConfigWrites[1] != (struct{ id, value, policy string }{"reasoning_effort", "runtime-effort", string(streams.SessionSettingsPolicyProviderRestored)}) {
		t.Errorf("startup config writes = %v, want profile then runtime reasoning_effort with provider_restored provenance", gotConfigWrites)
	}
	if !reflect.DeepEqual(profileOptions, profileOptionsBefore) || !reflect.DeepEqual(runtimeOptions, runtimeOptionsBefore) {
		t.Fatal("startup filtering mutated the saved profile/runtime options")
	}
	var sessionModelEvents []*AgentStreamEventData
	for _, payload := range eventBus.getStreamEvents() {
		if payload.Data != nil && payload.Data.Type == streams.EventTypeSessionModels {
			sessionModelEvents = append(sessionModelEvents, payload.Data)
		}
	}
	if len(sessionModelEvents) == 0 {
		t.Fatal("expected provider-restored lifecycle session_models reports")
	}
	for _, event := range sessionModelEvents {
		if event.SessionSettingsPolicy != streams.SessionSettingsPolicyProviderRestored {
			t.Errorf("session_models settings policy = %q, want provider_restored", event.SessionSettingsPolicy)
		}
	}
	manager := newTestManager(t)
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("register recovered execution for later selectors: %v", err)
	}
	if err := manager.SetSessionModel(context.Background(), execution.ID, "user-selected-model"); err != nil {
		t.Fatalf("later explicit model selection: %v", err)
	}
	if err := manager.SetSessionMode(context.Background(), execution.ID, "", "user-selected-mode"); err != nil {
		t.Fatalf("later explicit mode selection: %v", err)
	}
	dispatched := make(chan struct{}, 1)
	if _, err := sm.SendPromptWithDispatchCallback(context.Background(), execution, "next prompt", false, nil, true, func() {
		dispatched <- struct{}{}
	}); err != nil {
		t.Fatalf("send next prompt: %v", err)
	}
	select {
	case <-dispatched:
	case <-time.After(3 * time.Second):
		t.Fatal("next prompt did not dispatch")
	}
	mu.Lock()
	gotPrompts := append([]string(nil), promptTexts...)
	mu.Unlock()
	if !reflect.DeepEqual(gotPrompts, []string{"next prompt"}) {
		t.Fatalf("prompt texts = %v, want [next prompt]", gotPrompts)
	}
	actions := mock.getActionLog()
	modelSelections, modeSelections := 0, 0
	for _, action := range actions {
		if action == "agent.session.set_model" {
			modelSelections++
		}
		if action == "agent.session.set_mode" {
			modeSelections++
		}
	}
	if modelSelections != 1 || modeSelections != 1 {
		t.Fatalf("explicit selector actions = model:%d mode:%d, want one each after recovery", modelSelections, modeSelections)
	}
}
