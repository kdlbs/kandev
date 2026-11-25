package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const cursorOptionsTestNativeID = "cursor-native-session"

func TestInitializeAndPromptWithLayersCursorModelScopedReplay(t *testing.T) {
	for _, tc := range []struct {
		name, evidence, reject string
		applySupported         bool
	}{
		{name: "correlated catalog before stream event", evidence: "reply", applySupported: true},
		{name: "notification before lifecycle dispatch", evidence: "update", applySupported: true},
		{name: "catalog from current setup", evidence: "setup", applySupported: true},
		{name: "unverified catalog"},
		{name: "advertised value rejection", evidence: "reply", reject: "reasoning_effort", applySupported: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testCursorStartupReplay(t, tc.evidence, tc.reject, tc.applySupported)
		})
	}
}

func testCursorStartupReplay(t *testing.T, evidence, reject string, applySupported bool) {
	t.Helper()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	seen := configOptionRecorder(mock, reject)
	setupModel, runtimeModel := "opus", "sonnet"
	if evidence == "setup" {
		setupModel, runtimeModel = "sonnet", ""
	}
	configureCursorCatalogResponses(mock, setupModel, evidence)

	log := newSessionTestLogger()
	events := &MockEventBusWithTracking{}
	sm := NewSessionManager(log, newTestStopCh(t))
	sm.SetDependencies(NewEventPublisher(events, log), nil, nil, nil)
	client := createTestClient(t, mock.server.URL)
	disconnected := connectAgentStream(t, mock, client)
	t.Cleanup(func() {
		client.Close()
		select {
		case <-disconnected:
		case <-time.After(5 * time.Second):
			t.Error("agent stream did not finish draining")
		}
	})
	execution := &AgentExecution{
		ID: "cursor-exec", TaskID: "task", SessionID: "session", AgentID: acpcompat.CursorAgentID,
		ACPSessionID: cursorOptionsTestNativeID, WorkspacePath: t.TempDir(), agentctl: client,
	}
	config := &testAgent{id: acpcompat.CursorAgentID, enabled: true, runtimeConfig: &agents.RuntimeConfig{
		Cmd: agents.NewCommand("cursor-agent"), Protocol: agent.ProtocolACP,
		SessionConfig: agents.SessionConfig{NativeSessionResume: true},
	}}

	ready := false
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := sm.InitializeAndPromptWithLayers(
		ctx, execution, config, "", nil, nil, func(string) error { ready = true; return nil },
		"", "", map[string]string{"fast": "false"},
		runtimeModel, "", map[string]string{"fast": "false", "reasoning_effort": "high"},
		StartModelPolicy{},
	)
	require.NoError(t, err)
	require.True(t, ready)
	require.Equal(t, cursorOptionsTestNativeID, execution.ACPSessionID)
	require.Contains(t, mock.getActionLog(), "agent.session.load")
	_, called := seen.Load("fast")
	require.False(t, called, "stale fast option reached agentctl")
	value, called := seen.Load("reasoning_effort")
	require.Equal(t, applySupported, called)
	if called {
		require.Equal(t, "high", value)
	}
	var failures []string
	for _, event := range events.getStreamEvents() {
		if event.Data == nil {
			continue
		}
		if data, ok := event.Data.Data.(map[string]any); ok {
			if failed, ok := data["workflow_session_config_failures"].([]string); ok {
				failures = append(failures, failed...)
			}
		}
	}
	if reject == "" {
		require.Empty(t, failures)
	} else {
		require.Equal(t, []string{reject}, failures)
	}
}

func configureCursorCatalogResponses(mock *mockAgentServer, setupModel, evidence string) {
	base := mock.handler
	models := []streams.SessionModelInfo{{ModelID: "opus"}, {ModelID: "sonnet"}}
	options := func(model string) []streams.ConfigOption {
		selected := []streams.ConfigOption{
			{Type: "select", ID: "model", Category: "model", CurrentValue: model},
			{Type: "select", ID: "reasoning_effort", CurrentValue: "medium"},
		}
		if model == "opus" {
			selected = append(selected, streams.ConfigOption{Type: "select", ID: "fast", CurrentValue: "false"})
		}
		return selected
	}
	state := func(model string, generation uint64) map[string]any {
		return map[string]any{
			"session_id": cursorOptionsTestNativeID, "current_model_id": model,
			"models": models, "config_options": options(model),
			"config_options_complete": true, "session_settings_generation": generation,
		}
	}
	mock.handler = func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.session.load":
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success": true, "model_state": state(setupModel, 1),
			})
			return response
		case "agent.session.set_model":
			body := map[string]any{"effective": "sonnet", "confirmed": true, "config_id": "model"}
			if evidence == "reply" {
				body["model_state"] = state("sonnet", 2)
			}
			response, _ := ws.NewResponse(msg.ID, msg.Action, body)
			return response
		default:
			return base(msg)
		}
	}
	if evidence == "update" {
		mock.eventsAfterResponse = func(msg ws.Message) []agentctl.AgentEvent {
			if msg.Action != "agent.session.set_model" {
				return nil
			}
			return []agentctl.AgentEvent{{
				Type: streams.EventTypeSessionModels, SessionID: cursorOptionsTestNativeID,
				CurrentModelID: "sonnet", SessionModels: models, ConfigOptions: options("sonnet"),
				SessionSettingsGeneration: 2,
				Data:                      map[string]any{"config_options_complete": true, "config_options_source": "provider_update"},
			}}
		}
	}
}
