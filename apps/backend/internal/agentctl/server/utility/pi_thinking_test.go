package utility

import (
	acp "github.com/coder/acp-go-sdk"
	"testing"
)

func TestPiThinkingProbeReplacesLegacyModes(t *testing.T) {
	category := acp.SessionConfigOptionCategory("thought_level")
	choices := acp.SessionConfigSelectOptionsUngrouped{{Value: "off", Name: "Off"}}
	response := acp.NewSessionResponse{Modes: &acp.SessionModeState{CurrentModeId: "medium", AvailableModes: []acp.SessionMode{{Id: "medium", Name: "Medium"}}}, ConfigOptions: []acp.SessionConfigOption{{Select: &acp.SessionConfigOptionSelect{Type: "select", Id: "thought_level", Category: &category, CurrentValue: "off", Options: acp.SessionConfigSelectOptions{Ungrouped: &choices}}}}}
	out := &ProbeResponse{}
	applySessionProbeFields(out, response, "pi-acp")
	if out.CurrentModeID != "off" || len(out.Modes) != 1 || out.Modes[0].ID != "off" {
		t.Fatalf("stale thinking choices: %+v", out.Modes)
	}
}
