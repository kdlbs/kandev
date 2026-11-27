package acp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type setModeHandler func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error)
type setConfigOptionHandler func(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error)

type setModeTestAgent struct {
	*sessionRequestCaptureAgent
	connection     *acpsdk.AgentSideConnection
	handler        setModeHandler
	configHandler  setConfigOptionHandler
	legacyRequests chan acpsdk.SetSessionModeRequest
	configRequests chan acpsdk.SetSessionConfigOptionRequest
}

func (a *setModeTestAgent) SetSessionMode(ctx context.Context, request acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	if a.legacyRequests != nil {
		a.legacyRequests <- request
	}
	return a.handler(ctx, request)
}

func (a *setModeTestAgent) SetSessionConfigOption(ctx context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	if a.configRequests != nil {
		a.configRequests <- request
	}
	if a.configHandler != nil {
		return a.configHandler(ctx, request)
	}
	return a.sessionRequestCaptureAgent.SetSessionConfigOption(ctx, request)
}

func newSetModeTestAdapter(t *testing.T, handler setModeHandler) (*Adapter, *setModeTestAgent, chan string) {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	adapter := newTestAdapter()
	processed := make(chan string, 8)
	client := acpclient.NewClient(acpclient.WithUpdateHandler(func(notification acpsdk.SessionNotification) {
		marker := ""
		switch {
		case notification.Update.CurrentModeUpdate != nil:
			marker = string(notification.Update.CurrentModeUpdate.CurrentModeId)
		case notification.Update.ConfigOptionUpdate != nil:
			marker = "config_option_update"
		default:
			return
		}
		if event := adapter.convertNotification(notification); event != nil {
			adapter.sendUpdate(*event)
		}
		processed <- marker
	}))
	connection := acpsdk.NewClientSideConnection(client, clientToAgentWriter, agentToClientReader)
	agent := &setModeTestAgent{
		sessionRequestCaptureAgent: &sessionRequestCaptureAgent{},
		handler:                    handler,
	}
	agent.connection = acpsdk.NewAgentSideConnection(agent, agentToClientWriter, clientToAgentReader)
	adapter.acpConn = connection
	adapter.acpClient = client
	adapter.sessionID = "session-1"
	adapter.availableModes = []streams.SessionModeInfo{
		{ID: "default"}, {ID: "acceptEdits"}, {ID: "plan"}, {ID: "bypassPermissions"},
	}
	adapter.noteCurrentMode("session-1", "default")
	t.Cleanup(func() {
		_ = adapter.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})
	return adapter, agent, processed
}

func groupedModeConfigOption(current string) acpsdk.SessionConfigOption {
	category := acpsdk.SessionConfigOptionCategoryMode
	choices := acpsdk.SessionConfigSelectOptionsGrouped{{
		Group: "safety",
		Name:  "Safety",
		Options: []acpsdk.SessionConfigSelectOption{
			{Value: "default", Name: "Default"},
			{Value: "acceptEdits", Name: "Accept edits"},
			{Value: "plan", Name: "Plan"},
			{Value: "bypassPermissions", Name: "Bypass permissions"},
		},
	}}
	return acpsdk.SessionConfigOption{Select: &acpsdk.SessionConfigOptionSelect{
		Type: "select", Id: "claude_permission_mode", Name: "Permission mode",
		Category: &category, CurrentValue: acpsdk.SessionConfigValueId(current),
		Options: acpsdk.SessionConfigSelectOptions{Grouped: &choices},
	}}
}

func sessionConfigSnapshot(mode, model, effort string) []acpsdk.SessionConfigOption {
	selectOption := func(id, name, category, current string, values ...string) acpsdk.SessionConfigOption {
		optionCategory := acpsdk.SessionConfigOptionCategory(category)
		choices := make(acpsdk.SessionConfigSelectOptionsUngrouped, 0, len(values))
		for _, value := range values {
			choices = append(choices, acpsdk.SessionConfigSelectOption{Value: acpsdk.SessionConfigValueId(value), Name: value})
		}
		return acpsdk.SessionConfigOption{Select: &acpsdk.SessionConfigOptionSelect{
			Type: "select", Id: acpsdk.SessionConfigId(id), Name: name, Category: &optionCategory,
			CurrentValue: acpsdk.SessionConfigValueId(current),
			Options:      acpsdk.SessionConfigSelectOptions{Ungrouped: &choices},
		}}
	}
	return []acpsdk.SessionConfigOption{
		groupedModeConfigOption(mode),
		selectOption("model", "Model", "model", model, "model-a", "model-b"),
		selectOption("reasoning_effort", "Reasoning effort", "reasoning_effort", effort, "low", "high"),
	}
}

func reportConfigOptionsFromAgent(agent *setModeTestAgent, options []acpsdk.SessionConfigOption) error {
	return agent.connection.SessionUpdate(context.Background(), acpsdk.SessionNotification{
		SessionId: "session-1",
		Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{
			SessionUpdate: "config_option_update",
			ConfigOptions: options,
		}},
	})
}

func TestSetModeUsesAdvertisedModeConfigOptionAndAuthoritativeClamp(t *testing.T) {
	requested := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	legacy := make(chan acpsdk.SetSessionModeRequest, 1)
	response := groupedModeConfigOption("default")
	agentHandler := func(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{response}}, nil
	}
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.configRequests = requested
	agent.legacyRequests = legacy
	agent.configHandler = agentHandler
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})

	result, err := adapter.SetMode(context.Background(), "bypassPermissions")
	if err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if !result.Confirmed || result.Effective != "default" {
		t.Fatalf("SetMode result = %+v, want authoritative clamped mode", result)
	}
	select {
	case request := <-requested:
		if request.ValueId == nil || string(request.ValueId.ConfigId) != "claude_permission_mode" || string(request.ValueId.Value) != "bypassPermissions" {
			t.Fatalf("config request = %+v, want advertised mode config ID and requested value", request)
		}
	default:
		t.Fatal("SetMode did not use the advertised mode config option")
	}
	select {
	case request := <-legacy:
		t.Fatalf("clamped config option fell back to legacy mode request: %+v", request)
	default:
	}
}

func TestSetModeDoesNotInventCurrentValueWhenConfigResponseOmitsMode(t *testing.T) {
	requested := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.configRequests = requested
	agent.configHandler = func(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{}, nil
	}
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})

	result, err := adapter.SetMode(context.Background(), "acceptEdits")
	if err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if result.Confirmed || result.Effective != "" {
		t.Fatalf("SetMode result = %+v, want unconfirmed with no invented current value", result)
	}
	select {
	case <-requested:
	default:
		t.Fatal("SetMode did not use the advertised mode config option")
	}
}

func TestSetConfigOptionModeUsesAuthoritativeModeFlow(t *testing.T) {
	requests := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("mode config option unexpectedly fell back to session/set_mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.configRequests = requests
	agent.configHandler = func(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{
			groupedModeConfigOption(string(request.ValueId.Value)),
		}}, nil
	}
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})

	if err := adapter.SetConfigOption(context.Background(), "claude_permission_mode", "acceptEdits"); err != nil {
		t.Fatalf("SetConfigOption: %v", err)
	}
	select {
	case request := <-requests:
		if request.ValueId == nil || string(request.ValueId.ConfigId) != "claude_permission_mode" || string(request.ValueId.Value) != "acceptEdits" {
			t.Fatalf("config request = %+v, want advertised mode option and value", request)
		}
	default:
		t.Fatal("SetConfigOption did not enter the mode config-option path")
	}
	if got := adapter.currentModeSnapshot().mode; got != "acceptEdits" {
		t.Fatalf("cached mode = %q, want authoritative response value", got)
	}
}

func TestNewSessionInitializesModeOnlyConfigOptionBeforeSetMode(t *testing.T) {
	requests := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("advertised mode option unexpectedly used session/set_mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	modeOption := groupedModeConfigOption("default")
	agent.newResponse = &acpsdk.NewSessionResponse{
		SessionId:     "mode-only-session",
		ConfigOptions: []acpsdk.SessionConfigOption{modeOption},
	}
	agent.configRequests = requests
	agent.configHandler = func(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{
			groupedModeConfigOption(string(request.ValueId.Value)),
		}}, nil
	}

	if _, err := adapter.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	assertInitializedModeConfig(t, adapter, "default", "bypassPermissions")
	result, err := adapter.SetMode(context.Background(), "acceptEdits")
	if err != nil || !result.Applied() {
		t.Fatalf("SetMode = %+v, %v, want confirmed advertised mode", result, err)
	}
	request := <-requests
	if request.ValueId == nil || string(request.ValueId.ConfigId) != "claude_permission_mode" || string(request.ValueId.Value) != "acceptEdits" {
		t.Fatalf("SetMode request = %+v, want the advertised mode config ID", request)
	}
}

func TestResetSessionReplacesLegacyModesWithModeOnlyConfigCatalog(t *testing.T) {
	requests := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("advertised mode option unexpectedly used session/set_mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.newResponse = &acpsdk.NewSessionResponse{
		SessionId: "legacy-mode-session",
		Modes: &acpsdk.SessionModeState{
			CurrentModeId: "legacy-only",
			AvailableModes: []acpsdk.SessionMode{
				{Id: "legacy-only", Name: "Legacy only"},
			},
		},
	}
	if _, err := adapter.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	agent.newResponse = &acpsdk.NewSessionResponse{
		SessionId:     "mode-option-session",
		ConfigOptions: []acpsdk.SessionConfigOption{groupedModeConfigOption("default")},
	}
	agent.configRequests = requests
	agent.configHandler = func(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{
			groupedModeConfigOption(string(request.ValueId.Value)),
		}}, nil
	}

	if _, err := adapter.ResetSession(context.Background(), nil); err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	assertInitializedModeConfig(t, adapter, "default", "bypassPermissions")
	for _, mode := range adapter.availableModes {
		if mode.ID == "legacy-only" {
			t.Fatal("reset retained the prior session's legacy mode catalog")
		}
	}
	if _, err := adapter.SetMode(context.Background(), "legacy-only"); err == nil {
		t.Fatal("reset accepted a mode absent from its config-option catalog")
	}
	result, err := adapter.SetMode(context.Background(), "acceptEdits")
	if err != nil || !result.Applied() {
		t.Fatalf("SetMode after reset = %+v, %v, want confirmed advertised mode", result, err)
	}
	request := <-requests
	if request.ValueId == nil || string(request.ValueId.ConfigId) != "claude_permission_mode" || string(request.ValueId.Value) != "acceptEdits" {
		t.Fatalf("SetMode after reset request = %+v, want mode config option", request)
	}
}

func TestLoadSessionInitializesModeOnlyConfigOptionBeforeSetMode(t *testing.T) {
	requests := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("advertised mode option unexpectedly used session/set_mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	adapter.capabilities.LoadSession = true
	agent.loadResponse = &acpsdk.LoadSessionResponse{
		ConfigOptions: []acpsdk.SessionConfigOption{groupedModeConfigOption("default")},
	}
	agent.configRequests = requests
	agent.configHandler = func(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{
			groupedModeConfigOption(string(request.ValueId.Value)),
		}}, nil
	}

	if err := adapter.LoadSession(context.Background(), "mode-only-loaded-session", nil); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	assertInitializedModeConfig(t, adapter, "default", "bypassPermissions")
	result, err := adapter.SetMode(context.Background(), "acceptEdits")
	if err != nil || !result.Applied() {
		t.Fatalf("SetMode after load = %+v, %v, want confirmed advertised mode", result, err)
	}
	request := <-requests
	if request.ValueId == nil || string(request.ValueId.ConfigId) != "claude_permission_mode" || string(request.ValueId.Value) != "acceptEdits" {
		t.Fatalf("SetMode after load request = %+v, want mode config option", request)
	}
}

func assertInitializedModeConfig(t *testing.T, adapter *Adapter, current string, option string) {
	t.Helper()
	if got := adapter.currentModeSnapshot().mode; got != current {
		t.Fatalf("initialized current mode = %q, want %q", got, current)
	}
	if len(adapter.availableConfigOptions) == 0 {
		t.Fatal("mode-only config option was not cached")
	}
	config, ok := modeConfigOption(adapter.availableConfigOptions)
	if !ok || config.ID != "claude_permission_mode" || !configOptionAdvertisesValue(config, option) {
		t.Fatalf("initialized mode config = %+v, want advertised ID and %q choice", config, option)
	}
	if len(adapter.availableModes) != len(config.Options) {
		t.Fatalf("available modes = %+v, want choices from config option %+v", adapter.availableModes, config.Options)
	}
}

func TestUnsolicitedConfigOptionUpdateEmitsModeEvent(t *testing.T) {
	adapter, _, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("unsolicited config update unexpectedly invoked a mode request")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})
	adapter.availableModes = []streams.SessionModeInfo{{ID: "stale-session-mode"}}
	notification := acpsdk.SessionNotification{
		SessionId: "session-1",
		Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{
			SessionUpdate: "config_option_update",
			ConfigOptions: []acpsdk.SessionConfigOption{groupedModeConfigOption("acceptEdits")},
		}},
	}
	adapter.handleACPUpdate(notification, 0)
	events := drainEvents(adapter)
	var modeEvent, configEvent *AgentEvent
	for i := range events {
		switch events[i].Type {
		case streams.EventTypeSessionMode:
			modeEvent = &events[i]
		case streams.EventTypeSessionModels:
			configEvent = &events[i]
		}
	}
	if modeEvent == nil || modeEvent.CurrentModeID != "acceptEdits" || len(modeEvent.AvailableModes) != 4 {
		t.Fatalf("unsolicited mode event = %+v; config event = %+v", modeEvent, configEvent)
	}
	if !legacyModesAdvertise(modeEvent.AvailableModes, "bypassPermissions") || legacyModesAdvertise(modeEvent.AvailableModes, "stale-session-mode") {
		t.Fatalf("unsolicited mode choices = %+v, want current config-option choices", modeEvent.AvailableModes)
	}
	if configEvent == nil || currentModeFromConfig(configEvent.ConfigOptions) != "acceptEdits" {
		t.Fatalf("unsolicited config event = %+v, want full mode snapshot", configEvent)
	}
	if got := adapter.currentModeSnapshot().mode; got != "acceptEdits" {
		t.Fatalf("cached current mode = %q, want unsolicited provider mode", got)
	}
}

func TestUnsolicitedConfigOptionUpdateFromStaleSessionIsIgnored(t *testing.T) {
	adapter, _, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("stale config update unexpectedly invoked a mode request")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	adapter.availableConfigOptions = convertACPConfigOptions(sessionConfigSnapshot("default", "model-a", "low"))
	before := cloneConfigOptions(adapter.availableConfigOptions)
	notification := acpsdk.SessionNotification{
		SessionId: "replaced-session",
		Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{
			SessionUpdate: "config_option_update",
			ConfigOptions: []acpsdk.SessionConfigOption{groupedModeConfigOption("acceptEdits")},
		}},
	}
	adapter.handleACPUpdate(notification, 0)
	if events := drainEvents(adapter); len(events) != 0 {
		t.Fatalf("stale config update emitted events: %+v", events)
	}
	if got := currentModeFromConfig(adapter.availableConfigOptions); got != currentModeFromConfig(before) {
		t.Fatalf("stale config update changed cached mode from %q to %q", currentModeFromConfig(before), got)
	}
	if got := adapter.currentModeSnapshot().mode; got != "default" {
		t.Fatalf("stale config update changed current mode to %q", got)
	}
}

func TestSetModeDoesNotFallbackAfterProviderConfigOptionError(t *testing.T) {
	requested := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	legacy := make(chan acpsdk.SetSessionModeRequest, 1)
	providerErr := "provider denied mode"
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.configRequests = requested
	agent.legacyRequests = legacy
	agent.configHandler = func(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("%s", providerErr)
	}
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})

	_, err := adapter.SetMode(context.Background(), "bypassPermissions")
	if err == nil || !strings.Contains(err.Error(), providerErr) {
		t.Fatalf("SetMode error = %v, want provider denial", err)
	}
	select {
	case <-requested:
	default:
		t.Fatal("SetMode did not attempt the advertised mode config option")
	}
	select {
	case request := <-legacy:
		t.Fatalf("provider denial fell back to legacy mode request: %+v", request)
	default:
	}
}

func TestSetModeConfirmsClaudeConfigOptionUpdateWithoutCurrentModeUpdate(t *testing.T) {
	var adapter *Adapter
	var agent *setModeTestAgent
	var processed chan string
	adapter, agent, processed = newSetModeTestAdapter(t, func(ctx context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		if err := reportConfigOptionsFromAgent(agent, []acpsdk.SessionConfigOption{groupedModeConfigOption("acceptEdits")}); err != nil {
			return acpsdk.SetSessionModeResponse{}, err
		}
		select {
		case <-processed:
		case <-ctx.Done():
			return acpsdk.SetSessionModeResponse{}, ctx.Err()
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})
	// This bridge exposes the legacy session mode API alongside config updates.
	// No mode config option is cached, so SetMode must use the legacy request.
	adapter.availableModes = []streams.SessionModeInfo{{ID: "default"}, {ID: "acceptEdits"}}

	result, err := adapter.SetMode(context.Background(), "acceptEdits")
	if err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if !result.Confirmed || result.Effective != "acceptEdits" {
		t.Fatalf("SetMode result = %+v, want mode from config_option_update", result)
	}
}

func reportModeFromAgent(agent *setModeTestAgent, mode string) error {
	return agent.connection.SessionUpdate(context.Background(), acpsdk.SessionNotification{
		SessionId: "session-1",
		Update: acpsdk.SessionUpdate{
			CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
				SessionUpdate: "current_mode_update",
				CurrentModeId: acpsdk.SessionModeId(mode),
			},
		},
	})
}

func TestSetModeUsesReportArrivingBeforeRPCResponse(t *testing.T) {
	var adapter *Adapter
	var agent *setModeTestAgent
	var processed chan string
	adapter, agent, processed = newSetModeTestAdapter(t, func(ctx context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		if err := reportModeFromAgent(agent, "default"); err != nil {
			return acpsdk.SetSessionModeResponse{}, err
		}
		select {
		case <-processed:
		case <-ctx.Done():
			return acpsdk.SetSessionModeResponse{}, ctx.Err()
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})

	type result struct {
		modeResult streams.ModeResult
		err        error
	}
	resultCh := make(chan result, 1)
	go func() {
		modeResult, err := adapter.SetMode(context.Background(), "bypassPermissions")
		resultCh <- result{modeResult: modeResult, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("SetMode: %v", result.err)
		}
		if !result.modeResult.Confirmed || result.modeResult.Effective != "default" {
			t.Fatalf("SetMode result = %+v, want the early clamp report", result.modeResult)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetMode did not finish after the early report")
	}

	events := drainEvents(adapter)
	if len(events) < 2 {
		t.Fatalf("mode update events = %+v, want provider and SetMode events", events)
	}
	last := events[len(events)-1]
	if last.CurrentModeID != "default" || last.RequestedModeID != "bypassPermissions" {
		t.Fatalf("SetMode event = %+v, want the reported clamp and request", last)
	}
}

func TestConcurrentSetModeRequestsCannotShareAReport(t *testing.T) {
	var adapter *Adapter
	var agent *setModeTestAgent
	var processed chan string
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{}, 1)
	adapter, agent, processed = newSetModeTestAdapter(t, func(ctx context.Context, request acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		switch string(request.ModeId) {
		case "plan":
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return acpsdk.SetSessionModeResponse{}, ctx.Err()
			}
			if err := reportModeFromAgent(agent, "plan"); err != nil {
				return acpsdk.SetSessionModeResponse{}, err
			}
			select {
			case <-processed:
			case <-ctx.Done():
				return acpsdk.SetSessionModeResponse{}, ctx.Err()
			}
		case "bypassPermissions":
			secondEntered <- struct{}{}
		default:
			return acpsdk.SetSessionModeResponse{}, context.Canceled
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})

	type result struct {
		mode streams.ModeResult
		err  error
	}
	firstResult := make(chan result, 1)
	go func() {
		mode, err := adapter.SetMode(context.Background(), "plan")
		firstResult <- result{mode: mode, err: err}
	}()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first SetMode request did not reach the agent")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.SetMode(canceled, "bypassPermissions"); err == nil {
		t.Fatal("concurrent canceled SetMode request unexpectedly proceeded")
	}
	select {
	case <-secondEntered:
		t.Fatal("concurrent request reached the agent while the first mode change was active")
	default:
	}

	close(releaseFirst)
	select {
	case got := <-firstResult:
		if got.err != nil || !got.mode.Confirmed || got.mode.Effective != "plan" {
			t.Fatalf("first SetMode result = %+v, error = %v", got.mode, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first SetMode request did not settle")
	}

	second, err := adapter.SetMode(context.Background(), "bypassPermissions")
	if err != nil {
		t.Fatalf("second SetMode: %v", err)
	}
	select {
	case <-secondEntered:
	default:
		t.Fatal("second SetMode request did not reach the agent")
	}
	if second.Confirmed || second.Effective != "" {
		t.Fatalf("second SetMode result = %+v, want no confirmation from the first request's report", second)
	}
}

func TestConcurrentACPAdaptersKeepSessionModesIsolated(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	var firstAgent, secondAgent *setModeTestAgent
	first, agent, _ := newSetModeTestAdapter(t, func(ctx context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		entered <- "first"
		select {
		case <-release:
		case <-ctx.Done():
			return acpsdk.SetSessionModeResponse{}, ctx.Err()
		}
		if err := reportModeFromAgent(firstAgent, "plan"); err != nil {
			return acpsdk.SetSessionModeResponse{}, err
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})
	firstAgent = agent
	second, agent, _ := newSetModeTestAdapter(t, func(ctx context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		entered <- "second"
		select {
		case <-release:
		case <-ctx.Done():
			return acpsdk.SetSessionModeResponse{}, ctx.Err()
		}
		if err := reportModeFromAgent(secondAgent, "bypassPermissions"); err != nil {
			return acpsdk.SetSessionModeResponse{}, err
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})
	secondAgent = agent

	type outcome struct {
		result streams.ModeResult
		err    error
	}
	firstResult := make(chan outcome, 1)
	secondResult := make(chan outcome, 1)
	go func() {
		result, err := first.SetMode(context.Background(), "plan")
		firstResult <- outcome{result: result, err: err}
	}()
	go func() {
		result, err := second.SetMode(context.Background(), "bypassPermissions")
		secondResult <- outcome{result: result, err: err}
	}()
	seen := map[string]bool{}
	for range 2 {
		select {
		case name := <-entered:
			seen[name] = true
		case <-time.After(2 * time.Second):
			t.Fatal("both session mode requests did not reach their own ACP process")
		}
	}
	close(release)
	for name, resultCh := range map[string]<-chan outcome{"first": firstResult, "second": secondResult} {
		select {
		case got := <-resultCh:
			if got.err != nil || !got.result.Confirmed {
				t.Fatalf("%s SetMode result = %+v, error = %v", name, got.result, got.err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s SetMode did not settle", name)
		}
	}
	if !seen["first"] || !seen["second"] {
		t.Fatalf("mode requests entered %v; want both isolated ACP processes", seen)
	}
	if got := first.currentModeSnapshot().mode; got != "plan" {
		t.Fatalf("first session effective mode = %q, want plan", got)
	}
	if got := second.currentModeSnapshot().mode; got != "bypassPermissions" {
		t.Fatalf("second session effective mode = %q, want bypassPermissions", got)
	}
}

func TestSetModeRejectsModeMissingFromAgentCapabilities(t *testing.T) {
	requests := make(chan acpsdk.SetSessionModeRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.legacyRequests = requests
	adapter.availableModes = []streams.SessionModeInfo{{ID: "default"}}

	if _, err := adapter.SetMode(context.Background(), "bypassPermissions"); err == nil {
		t.Fatal("SetMode accepted a mode the agent did not advertise")
	}
	select {
	case request := <-requests:
		t.Fatalf("unsupported mode reached the agent: %+v", request)
	default:
	}
}

func TestLateTimedOutModeReportCannotConfirmNextRequest(t *testing.T) {
	secondEntered := make(chan struct{})
	adapter, agent, processed := newSetModeTestAdapter(t, func(_ context.Context, request acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		if string(request.ModeId) == "bypassPermissions" {
			close(secondEntered)
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})

	first, err := adapter.SetMode(context.Background(), "plan")
	if err != nil || first.Confirmed {
		t.Fatalf("first SetMode = %+v, %v; want timeout", first, err)
	}
	type outcome struct {
		result streams.ModeResult
		err    error
	}
	secondResult := make(chan outcome, 1)
	go func() {
		result, err := adapter.SetMode(context.Background(), "bypassPermissions")
		secondResult <- outcome{result, err}
	}()
	select {
	case <-secondEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("second request did not reach agent")
	}
	if err := reportModeFromAgent(agent, "plan"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("late first report was not processed")
	}
	select {
	case got := <-secondResult:
		if got.err != nil || got.result.Confirmed || got.result.Effective != "" {
			t.Fatalf("second SetMode = %+v, %v; stale report confirmed it", got.result, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second request did not settle")
	}
}
