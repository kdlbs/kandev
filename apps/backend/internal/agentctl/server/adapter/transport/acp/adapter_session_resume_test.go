package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type sessionResumeAgent struct {
	sessionRequestCaptureAgent
	resumeRequest  acpsdk.ResumeSessionRequest
	resumeError    error
	resumeResponse *acpsdk.ResumeSessionResponse
	resumeCalls    int
	loadResponse   acpsdk.LoadSessionResponse
}

func (a *sessionResumeAgent) LoadSession(ctx context.Context, req acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	_, err := a.sessionRequestCaptureAgent.LoadSession(ctx, req)
	return a.loadResponse, err
}

func (a *sessionResumeAgent) ResumeSession(_ context.Context, req acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	a.resumeCalls++
	a.resumeRequest = req
	if a.resumeResponse != nil {
		return *a.resumeResponse, a.resumeError
	}
	var response acpsdk.ResumeSessionResponse
	if err := json.Unmarshal([]byte(`{
		"modes":{"currentModeId":"code","availableModes":[{"id":"code","name":"Code"}]},
		"configOptions":[{"type":"select","id":"model","name":"Model","category":"model",
			"currentValue":"saved-model","options":[{"value":"saved-model","name":"Saved model"}]}],
		"_meta":{"restored":true}
	}`), &response); err != nil {
		return response, err
	}
	return response, a.resumeError
}

func newSessionResumeAdapter(t *testing.T, load, resume bool) (*Adapter, *sessionResumeAgent) {
	t.Helper()
	toAgent, fromClient := io.Pipe()
	t.Cleanup(func() { _ = toAgent.Close(); _ = fromClient.Close() })
	toClient, fromAgent := io.Pipe()
	t.Cleanup(func() { _ = toClient.Close(); _ = fromAgent.Close() })
	fake := &sessionResumeAgent{}
	conn := acpsdk.NewClientSideConnection(acpclient.NewClient(), fromClient, toClient)
	_ = acpsdk.NewAgentSideConnection(fake, fromAgent, toAgent)
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })
	a.agentID = codexAgentID
	a.dialect = newACPDialect(codexAgentID)
	a.acpConn = conn
	a.cfg.WorkDir = t.TempDir()
	a.capabilities.LoadSession = load
	a.capabilities.McpCapabilities = acpsdk.McpCapabilities{Http: true, Sse: true}
	if resume {
		a.capabilities.SessionCapabilities.Resume = &acpsdk.SessionResumeCapabilities{}
	}
	return a, fake
}

func TestLoadSessionPrefersAdvertisedResume(t *testing.T) {
	for _, load := range []bool{true, false} {
		t.Run(fmt.Sprintf("load_supported_%t", load), func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, load, true)
			servers := []types.McpServer{
				{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
				{Name: "kandev", Type: "sse", URL: "http://localhost:10005/sse"},
			}
			if err := a.LoadSession(t.Context(), "saved-session", servers); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			if fake.resumeCalls != 1 || fake.loadRequest.SessionId != "" || fake.sessionCounter != 0 {
				t.Fatalf("want one resume without replay or replacement, got resume=%d load=%q new=%d",
					fake.resumeCalls, fake.loadRequest.SessionId, fake.sessionCounter)
			}
			if fake.resumeRequest.SessionId != "saved-session" || fake.resumeRequest.Cwd != a.cfg.WorkDir {
				t.Fatalf("resume lost session identity or cwd: %+v", fake.resumeRequest)
			}
			assertCapturedKandevTransport(t, fake.resumeRequest.McpServers, "http")
			if a.GetSessionID() != "saved-session" || a.isLoadingSession {
				t.Fatal("resume did not settle the saved session")
			}
			events := drainEvents(a)
			models := findSessionModelsEvent(t, events)
			if models.CurrentModelID != "saved-model" || len(models.ConfigOptions) != 1 {
				t.Fatalf("resume lost model/config state: %+v", models)
			}
			if state := a.GetSessionModelState(); state == nil || state.CurrentModelID != "saved-model" {
				t.Fatalf("cached model state = %+v", state)
			}
			var resumed, mode bool
			for _, event := range events {
				resumed = resumed || event.SessionStatus == streams.SessionStatusResumed
				mode = mode || event.CurrentModeID == "code"
			}
			if !resumed || !mode {
				t.Fatalf("resume events missing: resumed=%t mode=%t", resumed, mode)
			}
		})
	}
}

func TestProviderRestoredLoadSettingsProvenanceSurvivesDelayedNotifications(t *testing.T) {
	a, _ := newSessionResumeAdapter(t, true, true)
	ctx := streams.WithSessionSettingsPolicy(t.Context(), streams.SessionSettingsPolicyProviderRestored)
	if err := a.LoadSession(ctx, "saved-session", nil); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	initial := drainEvents(a)
	var initialMode, initialModels *streams.AgentEvent
	for i := range initial {
		event := &initial[i]
		switch event.Type {
		case streams.EventTypeSessionMode:
			initialMode = event
		case streams.EventTypeSessionModels:
			initialModels = event
		}
	}
	if initialMode == nil || initialModels == nil {
		t.Fatalf("load settings reports missing: %+v", initial)
	}
	for _, event := range []*streams.AgentEvent{initialMode, initialModels} {
		if event.SessionSettingsPolicy != streams.SessionSettingsPolicyProviderRestored || event.SessionSettingsGeneration == 0 {
			t.Fatalf("initial load report lacks host provenance: %+v", event)
		}
	}

	// A load notification can remain queued until after LoadSession returns.
	a.handleACPUpdate(makeNotification("saved-session", acpsdk.SessionUpdate{
		CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
			SessionUpdate: "current_mode_update", CurrentModeId: "provider-update",
		},
	}), 0)
	delayed := drainEvents(a)
	if len(delayed) != 1 || delayed[0].Type != streams.EventTypeSessionMode {
		t.Fatalf("delayed mode report = %+v", delayed)
	}
	if delayed[0].SessionSettingsPolicy != streams.SessionSettingsPolicyProviderRestored ||
		delayed[0].SessionSettingsGeneration <= initialMode.SessionSettingsGeneration {
		t.Fatalf("delayed load report lost immutable provenance: %+v initial=%+v", delayed[0], initialMode)
	}

	// Explicit selector outcomes are host-created events with a later source
	// generation and no provider-restored marker.
	a.emitSetModelEvent("saved-session", "explicit-model", nil, nil)
	explicit := drainEvents(a)
	if len(explicit) != 1 || explicit[0].SessionSettingsPolicy != "" ||
		explicit[0].SessionSettingsGeneration <= delayed[0].SessionSettingsGeneration {
		t.Fatalf("explicit model outcome was not distinguished from load reports: %+v delayed=%+v", explicit, delayed[0])
	}
}

func TestProviderRestoredLoadWithoutSettingsEmitsEmptySnapshots(t *testing.T) {
	a, fake := newSessionResumeAdapter(t, true, true)
	fake.resumeResponse = &acpsdk.ResumeSessionResponse{}
	ctx := streams.WithSessionSettingsPolicy(t.Context(), streams.SessionSettingsPolicyProviderRestored)
	if err := a.LoadSession(ctx, "saved-session", nil); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	events := drainEvents(a)
	var mode, models *streams.AgentEvent
	for i := range events {
		switch events[i].Type {
		case streams.EventTypeSessionMode:
			mode = &events[i]
		case streams.EventTypeSessionModels:
			models = &events[i]
		}
	}
	if mode == nil || models == nil {
		t.Fatalf("provider-restored load omitted empty settings snapshots: %+v", events)
	}
	for _, event := range []*streams.AgentEvent{mode, models} {
		if event.SessionSettingsPolicy != streams.SessionSettingsPolicyProviderRestored || event.SessionSettingsGeneration == 0 {
			t.Errorf("empty load report lacks host provenance: %+v", event)
		}
	}
	if mode.CurrentModeID != "" || models.CurrentModelID != "" || len(models.SessionModels) != 0 || len(models.ConfigOptions) != 0 {
		t.Fatalf("empty provider report contains settings: mode=%+v models=%+v", mode, models)
	}
	settled, _ := models.Data["config_options_settled"].(bool)
	if !settled {
		t.Fatal("empty provider model snapshot must be marked settled so it clears prior effective state")
	}
}

func TestSessionSettingsGenerationContinuesAcrossLoadAndReset(t *testing.T) {
	a, fake := newSessionResumeAdapter(t, true, true)
	restored := streams.WithSessionSettingsPolicy(t.Context(), streams.SessionSettingsPolicyProviderRestored)
	if err := a.LoadSession(restored, "saved-session", nil); err != nil {
		t.Fatalf("initial LoadSession: %v", err)
	}
	initial := drainEvents(a)
	initialMax := maxSessionSettingsGeneration(initial)
	if initialMax == 0 {
		t.Fatalf("initial load emitted no settings generations: %+v", initial)
	}

	var newResponse acpsdk.NewSessionResponse
	if err := json.Unmarshal([]byte(`{
		"sessionId":"reset-session",
		"modes":{"currentModeId":"plan","availableModes":[{"id":"plan","name":"Plan"}]},
		"configOptions":[{"type":"select","id":"model","name":"Model","category":"model",
			"currentValue":"reset-model","options":[{"value":"reset-model","name":"Reset model"}]}]
	}`), &newResponse); err != nil {
		t.Fatalf("decode session/new response: %v", err)
	}
	fake.newResponse = &newResponse
	resetSessionID, err := a.ResetSession(t.Context(), nil)
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if resetSessionID != "reset-session" {
		t.Fatalf("ResetSession ID = %q, want reset-session", resetSessionID)
	}
	resetEvents := drainEvents(a)
	resetMode := findSessionModeEvent(t, resetEvents)
	resetModels := findSessionModelsEvent(t, resetEvents)
	if resetMode.SessionSettingsGeneration <= initialMax ||
		resetModels.SessionSettingsGeneration <= resetMode.SessionSettingsGeneration {
		t.Fatalf("session/new settings generations did not continue after load: load=%d resetMode=%+v resetModels=%+v",
			initialMax, resetMode, resetModels)
	}
	if resetMode.SessionSettingsPolicy != "" || resetModels.SessionSettingsPolicy != "" {
		t.Fatalf("session/new inherited provider-restored policy: mode=%+v models=%+v", resetMode, resetModels)
	}

	// A queued notification from the superseded session must remain rejected
	// without consuming a generation belonging to the current session.
	a.handleACPUpdate(makeNotification("saved-session", acpsdk.SessionUpdate{
		CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
			SessionUpdate: "current_mode_update", CurrentModeId: "stale-mode",
		},
	}), 0)
	if stale := drainEvents(a); len(stale) != 0 {
		t.Fatalf("old-session settings report escaped after reset: %+v", stale)
	}
	a.handleACPUpdate(makeNotification(resetSessionID, acpsdk.SessionUpdate{
		CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
			SessionUpdate: "current_mode_update", CurrentModeId: "current-mode",
		},
	}), 0)
	current := drainEvents(a)
	if len(current) != 1 || current[0].Type != streams.EventTypeSessionMode ||
		current[0].SessionSettingsGeneration <= resetModels.SessionSettingsGeneration {
		t.Fatalf("current-session report generation did not continue: prior=%+v event=%+v", resetModels, current)
	}

	if err := a.LoadSession(restored, "next-session", nil); err != nil {
		t.Fatalf("subsequent LoadSession: %v", err)
	}
	loaded := drainEvents(a)
	loadedMode := findSessionModeEvent(t, loaded)
	loadedModels := findSessionModelsEvent(t, loaded)
	if loadedMode.SessionSettingsGeneration <= current[0].SessionSettingsGeneration ||
		loadedModels.SessionSettingsGeneration <= loadedMode.SessionSettingsGeneration {
		t.Fatalf("subsequent session/load reset generations: previous=%+v loadedMode=%+v loadedModels=%+v",
			current[0], loadedMode, loadedModels)
	}
	a.handleACPUpdate(makeNotification(resetSessionID, acpsdk.SessionUpdate{
		CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
			SessionUpdate: "current_mode_update", CurrentModeId: "late-reset-mode",
		},
	}), 0)
	if stale := drainEvents(a); len(stale) != 0 {
		t.Fatalf("old reset-session report escaped after load: %+v", stale)
	}
}

func maxSessionSettingsGeneration(events []streams.AgentEvent) uint64 {
	var max uint64
	for _, event := range events {
		if event.SessionSettingsGeneration > max {
			max = event.SessionSettingsGeneration
		}
	}
	return max
}

func findSessionModeEvent(t *testing.T, events []streams.AgentEvent) streams.AgentEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == streams.EventTypeSessionMode {
			return event
		}
	}
	t.Fatalf("session_mode event missing: %+v", events)
	return streams.AgentEvent{}
}

func TestLoadSessionFallsBackToReplayOnlyWhenResumeUnsupported(t *testing.T) {
	for _, advertised := range []bool{true, false} {
		t.Run(fmt.Sprintf("resume_advertised_%t", advertised), func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, true, advertised)
			fake.resumeError = acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionResume)
			fake.loadResponse.LegacyModels = &acpsdk.LegacyModels{
				CurrentModelId:  "legacy-model",
				AvailableModels: []acpsdk.LegacyModelInfo{{ModelId: "legacy-model", Name: "Legacy model"}},
			}
			if err := a.LoadSession(t.Context(), "saved-session", nil); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			if fake.loadRequest.SessionId != "saved-session" || fake.sessionCounter != 0 {
				t.Fatalf("want replay of saved session, got load=%q new=%d", fake.loadRequest.SessionId, fake.sessionCounter)
			}
			if advertised != (fake.resumeCalls == 1) {
				t.Fatalf("resume calls=%d advertised=%t", fake.resumeCalls, advertised)
			}
			if event := findSessionModelsEvent(t, drainEvents(a)); event.CurrentModelID != "legacy-model" {
				t.Fatalf("load fallback lost legacy model state: %+v", event)
			}
		})
	}
}

func TestLoadSessionResumesLegacyModelsAcrossAgents(t *testing.T) {
	for _, agentID := range []string{"auggie", claudeAgentID, "unknown-agent"} {
		t.Run(agentID, func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, true, true)
			a.agentID = agentID
			a.dialect = newACPDialect(agentID)
			fake.resumeResponse = &acpsdk.ResumeSessionResponse{}
			if err := json.Unmarshal([]byte(`{"models":{
				"currentModelId":"legacy-model",
				"availableModels":[{"modelId":"legacy-model","name":"Legacy model"}]
			}}`), fake.resumeResponse); err != nil {
				t.Fatal(err)
			}
			if err := a.LoadSession(t.Context(), "saved-session", nil); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			if fake.resumeCalls != 1 || fake.loadRequest.SessionId != "" || fake.sessionCounter != 0 {
				t.Fatalf("want resume without replay, got resume=%d load=%q new=%d",
					fake.resumeCalls, fake.loadRequest.SessionId, fake.sessionCounter)
			}
			models := findSessionModelsEvent(t, drainEvents(a))
			if models.CurrentModelID != "legacy-model" || len(models.SessionModels) != 1 {
				t.Fatalf("resume lost legacy model state: %+v", models)
			}
			if state := a.GetSessionModelState(); state == nil || len(state.Models) != 1 || state.Models[0].ModelID != "legacy-model" {
				t.Fatalf("cached model state = %+v", state)
			}
		})
	}
}

func TestLoadSessionDoesNotLoadWhenReplayUnsupported(t *testing.T) {
	a, fake := newSessionResumeAdapter(t, false, true)
	a.dialect = newACPDialect("auggie")
	a.agentID = "auggie"
	a.sessionID = "prior-session"
	fake.resumeError = acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionResume)
	if err := a.LoadSession(t.Context(), "saved-session", nil); err == nil {
		t.Fatal("expected unsupported session loading")
	}
	if fake.resumeCalls != 1 || fake.loadRequest.SessionId != "" || fake.sessionCounter != 0 {
		t.Fatal("failed resume retried an unsupported method")
	}
	if a.GetSessionID() != "prior-session" || a.isLoadingSession {
		t.Fatal("failed resume changed identity or retained replay suppression")
	}
}

func TestLoadSessionResumeFailurePreservesIdentity(t *testing.T) {
	for _, cause := range []error{
		acpsdk.NewInternalError(map[string]any{"error": "context deadline exceeded"}),
		acpsdk.NewAuthRequired(nil),
		acpsdk.NewRequestCancelled(nil),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			a, fake := newSessionResumeAdapter(t, true, true)
			fake.resumeError = cause
			a.sessionID = "prior-session"
			if err := a.LoadSession(t.Context(), "saved-session", nil); err == nil {
				t.Fatal("expected resume error")
			}
			if fake.resumeCalls != 1 || fake.loadRequest.SessionId != "" || fake.sessionCounter != 0 {
				t.Fatalf("inconclusive error retried: resume=%d load=%q new=%d",
					fake.resumeCalls, fake.loadRequest.SessionId, fake.sessionCounter)
			}
			if a.GetSessionID() != "prior-session" || a.isLoadingSession {
				t.Fatal("failed resume changed identity or retained replay suppression")
			}
		})
	}
}
