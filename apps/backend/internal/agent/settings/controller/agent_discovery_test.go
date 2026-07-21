package controller

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
)

func TestConfigOptionDTOsPreserveDescriptions(t *testing.T) {
	dtos := configOptionDTOs([]hostutility.ConfigOption{{
		Type:         "select",
		ID:           "reasoning_effort",
		Name:         "Reasoning effort",
		Description:  "Controls reasoning depth.",
		CurrentValue: "high",
		Options: []hostutility.ConfigOptionChoice{{
			Value:       "high",
			Name:        "High",
			Description: "More thorough reasoning.",
		}},
	}})

	raw, err := json.Marshal(dtos)
	if err != nil {
		t.Fatalf("marshal config option DTOs: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal config option DTOs: %v", err)
	}
	if got := payload[0]["description"]; got != "Controls reasoning depth." {
		t.Errorf("option description = %#v, want provider description", got)
	}
	values := payload[0]["options"].([]any)
	if got := values[0].(map[string]any)["description"]; got != "More thorough reasoning." {
		t.Errorf("value description = %#v, want provider description", got)
	}
}
