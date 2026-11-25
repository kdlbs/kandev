package acp

import (
	"context"
	"fmt"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func piThinkingSnapshot(current string, values ...string) []acpsdk.SessionConfigOption {
	category := acpsdk.SessionConfigOptionCategory("thought_level")
	choices := make(acpsdk.SessionConfigSelectOptionsUngrouped, 0, len(values))
	for _, value := range values {
		choices = append(choices, acpsdk.SessionConfigSelectOption{Value: acpsdk.SessionConfigValueId(value), Name: value})
	}
	return []acpsdk.SessionConfigOption{{Select: &acpsdk.SessionConfigOptionSelect{
		Type: "select", Id: "thinking_level", Name: "Thinking level", Category: &category,
		CurrentValue: acpsdk.SessionConfigValueId(current), Options: acpsdk.SessionConfigSelectOptions{Ungrouped: &choices},
	}}}
}

// @covers AC-AGENTS-SESSION-SETTING-SEMANTICS-001.2, .3, .8
func TestPiThinkingSettingsUsesCurrentModelConfig(t *testing.T) {
	adapter, peer, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, fmt.Errorf("unexpected legacy mutation")
	})
	adapter.agentID = "pi-acp"
	adapter.availableModes = []streams.SessionModeInfo{{ID: "medium"}}
	adapter.emitAuthoritativeConfigOptions("session-1", "model", piThinkingSnapshot("off", "off"), nil, false)
	peer.configHandler = func(_ context.Context, req acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		if req.ValueId.ConfigId != "thinking_level" || req.ValueId.Value != "off" {
			t.Errorf("unexpected request: %+v", req)
		}
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: piThinkingSnapshot("off", "off")}, nil
	}
	if len(adapter.availableModes) != 1 || adapter.availableModes[0].ID != "off" {
		t.Errorf("stale modes: %+v", adapter.availableModes)
	}
	result, err := adapter.SetMode(context.Background(), "off")
	if err != nil || !result.Confirmed || result.Effective != "off" {
		t.Fatalf("SetMode(off) = %+v, %v", result, err)
	}
	if _, err := adapter.SetMode(context.Background(), "medium"); err == nil {
		t.Fatal("accepted unsupported thinking level")
	}
}

func TestPiThinkingSettingsEmptySnapshotRemovesChoices(t *testing.T) {
	a := newTestAdapter()
	a.agentID = "pi-acp"
	a.sessionID = "session-1"
	a.availableModes = []streams.SessionModeInfo{{ID: "medium"}}
	a.availableConfigOptions = convertACPConfigOptions(piThinkingSnapshot("medium", "medium"))
	event := a.convertNotification(acpsdk.SessionNotification{SessionId: "session-1", Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{ConfigOptions: []acpsdk.SessionConfigOption{}}}})
	if event == nil || len(a.availableModes) != 0 || len(a.availableConfigOptions) != 0 {
		t.Fatalf("empty snapshot retained choices: %+v", a.availableModes)
	}
}

func TestPiThinkingSettingsCanonicalClampIsRejected(t *testing.T) {
	adapter, peer, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, fmt.Errorf("legacy route is forbidden")
	})
	adapter.agentID = "pi-acp"
	adapter.availableConfigOptions = convertACPConfigOptions(piThinkingSnapshot("off", "off", "medium"))
	peer.configHandler = func(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: piThinkingSnapshot("off", "off", "medium")}, nil
	}
	if err := adapter.SetConfigOption(context.Background(), "thinking_level", "medium"); err == nil {
		t.Fatal("clamped thinking selection was accepted")
	}
}

func TestPiThinkingSettingsAmbiguousConfigCannotUseLegacyAlias(t *testing.T) {
	config := convertACPConfigOptions(piThinkingSnapshot("off", "off"))
	other := config[0]
	other.ID = "another_thinking_level"
	config = append(config, other)
	_, _, err := selectSessionModeOption(config, []streams.SessionModeInfo{{ID: "off"}}, "off", "", "pi-acp")
	if err == nil {
		t.Fatal("ambiguous thinking options accepted through legacy alias")
	}
}

func TestPiThinkingSettingsEmptySnapshotClearsPublishedMode(t *testing.T) {
	for _, response := range []bool{false, true} {
		t.Run(fmt.Sprint(response), func(t *testing.T) {
			a := newTestAdapter()
			a.agentID = "pi-acp"
			a.sessionID = "session-1"
			a.availableModes = []streams.SessionModeInfo{{ID: "medium"}}
			a.noteCurrentMode("session-1", "medium")
			if response {
				a.emitAuthoritativeConfigOptions("session-1", "model", nil, nil, false)
			} else {
				a.handleACPUpdate(acpsdk.SessionNotification{SessionId: "session-1", Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{ConfigOptions: []acpsdk.SessionConfigOption{}}}}, 0)
			}
			var mode *AgentEvent
			for _, event := range drainEvents(a) {
				if event.Type == streams.EventTypeSessionMode {
					mode = &event
				}
			}
			if mode == nil || mode.CurrentModeID != "" || len(mode.AvailableModes) != 0 || a.currentModeSnapshot().mode != "" {
				t.Fatalf("stale published thinking: event=%+v snapshot=%+v", mode, a.currentModeSnapshot())
			}
		})
	}
}

func TestPiThinkingNotificationKeepsGenerationWhenResponseOvertakesPublication(t *testing.T) {
	a := newTestAdapter()
	a.agentID = "pi-acp"
	a.sessionID = "session-1"
	earlier := a.convertNotification(acpsdk.SessionNotification{
		SessionId: "session-1",
		Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{
			ConfigOptions: piThinkingSnapshot("medium", "off", "medium"),
		}},
	})
	if earlier == nil {
		t.Fatal("notification did not produce a settings event")
	}
	// The RPC response commits a newer snapshot before the notification
	// worker reaches publication.
	a.emitAuthoritativeConfigOptions("session-1", "model", piThinkingSnapshot("off", "off"), nil, false)
	mode := a.modeEventForConfigSnapshot(*earlier)
	if mode == nil || mode.CurrentModeID != "medium" || len(mode.AvailableModes) != 2 {
		t.Fatalf("paired mode event did not use the notification snapshot: %+v", mode)
	}
	a.stampSessionSettingsReports("session-1", mode, earlier)
	if mode.SessionSettingsGeneration != earlier.SessionSettingsGeneration {
		t.Fatal("paired mode and model reports have different generations")
	}

	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeSessionModels {
			if earlier.SessionSettingsGeneration == 0 || earlier.SessionSettingsGeneration >= event.SessionSettingsGeneration {
				t.Fatalf("older notification generation = %d, newer response generation = %d", earlier.SessionSettingsGeneration, event.SessionSettingsGeneration)
			}
			return
		}
	}
	t.Fatal("response did not publish a settings event")
}

func TestPiThinkingSettingsModelChangeWithoutSnapshotInvalidatesChoices(t *testing.T) {
	a, peer, _ := newSetModeTestAdapter(t, nil)
	a.agentID = "pi-acp"
	config := convertACPConfigOptions(piThinkingSnapshot("medium", "off", "medium"))
	config = append(config, streams.ConfigOption{ID: "model", Type: "select", Category: "model", CurrentValue: "old"})
	a.availableConfigOptions = config
	a.availableModes = []streams.SessionModeInfo{{ID: "medium"}}
	a.noteCurrentMode("session-1", "medium")
	peer.configHandler = func(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{}, nil
	}
	if err := a.SetConfigOption(context.Background(), "model", "new"); err != nil {
		t.Fatal(err)
	}
	var modelEvent, modeEvent *AgentEvent
	for _, event := range drainEvents(a) {
		switch event.Type {
		case streams.EventTypeSessionMode:
			modeEvent = &event
		case streams.EventTypeSessionModels:
			modelEvent = &event
		}
	}
	if modelEvent == nil || modelEvent.CurrentModelID != "new" {
		t.Fatalf("missing model convergence: %+v", modelEvent)
	}
	if _, ok := a.modeConfigOption(modelEvent.ConfigOptions); ok {
		t.Fatalf("model event retained the previous thinking option: %+v", modelEvent.ConfigOptions)
	}
	if modeEvent == nil || modeEvent.CurrentModeID != "" || len(modeEvent.AvailableModes) != 0 ||
		modeEvent.SessionSettingsGeneration != modelEvent.SessionSettingsGeneration {
		t.Fatalf("mode event did not clear the previous thinking choices: %+v", modeEvent)
	}
	if a.currentModeSnapshot().mode != "" || len(a.availableModes) != 0 {
		t.Fatal("adapter retained the previous thinking choices")
	}
}
