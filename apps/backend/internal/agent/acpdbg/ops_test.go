package acpdbg

import "testing"

func TestBuildProbeResult_FallsBackToConfigOptionModels(t *testing.T) {
	t.Parallel()

	initResp := Frame{"result": map[string]any{
		"protocolVersion": float64(1),
		"agentInfo":       map[string]any{"name": "claude-agent-acp", "version": "0.42.0"},
	}}
	newResp := Frame{"result": map[string]any{
		"sessionId": "session-1",
		"configOptions": []any{
			map[string]any{
				"category":     "model",
				"currentValue": "default",
				"type":         "select",
				"options": []any{
					map[string]any{"value": "default", "name": "Default"},
					map[string]any{"value": "opus", "name": "Opus"},
				},
			},
			map[string]any{
				"category":     "mode",
				"currentValue": "plan",
				"type":         "select",
				"options": []any{
					map[string]any{"value": "default", "name": "Default"},
					map[string]any{"value": "plan", "name": "Plan Mode"},
				},
			},
		},
	}}

	got := buildProbeResult(initResp, newResp)

	if got.CurrentModelID != "default" {
		t.Fatalf("CurrentModelID = %q, want default", got.CurrentModelID)
	}
	if len(got.Models) != 2 || got.Models[0] != "default" || got.Models[1] != "opus" {
		t.Fatalf("Models = %+v, want [default opus]", got.Models)
	}
	if got.CurrentModeID != "plan" {
		t.Fatalf("CurrentModeID = %q, want plan", got.CurrentModeID)
	}
	if len(got.Modes) != 2 || got.Modes[0] != "default" || got.Modes[1] != "plan" {
		t.Fatalf("Modes = %+v, want [default plan]", got.Modes)
	}
}

func TestBuildProbeResult_PrefersLegacyModels(t *testing.T) {
	t.Parallel()

	got := buildProbeResult(Frame{}, Frame{"result": map[string]any{
		"models": map[string]any{
			"currentModelId":  "legacy",
			"availableModels": []any{map[string]any{"modelId": "legacy"}},
		},
		"configOptions": []any{map[string]any{
			"category":     "model",
			"currentValue": "fallback",
			"type":         "select",
			"options":      []any{map[string]any{"value": "fallback"}},
		}},
	}})

	if got.CurrentModelID != "legacy" {
		t.Fatalf("CurrentModelID = %q, want legacy", got.CurrentModelID)
	}
	if len(got.Models) != 1 || got.Models[0] != "legacy" {
		t.Fatalf("Models = %+v, want [legacy]", got.Models)
	}
}

func TestBuildProbeResult_PrefersLegacyModes(t *testing.T) {
	t.Parallel()

	got := buildProbeResult(Frame{}, Frame{"result": map[string]any{
		"modes": map[string]any{
			"currentModeId":  "legacy-mode",
			"availableModes": []any{map[string]any{"id": "legacy-mode"}},
		},
		"configOptions": []any{map[string]any{
			"category":     "mode",
			"currentValue": "fallback-mode",
			"type":         "select",
			"options":      []any{map[string]any{"value": "fallback-mode"}},
		}},
	}})

	if got.CurrentModeID != "legacy-mode" {
		t.Fatalf("CurrentModeID = %q, want legacy-mode", got.CurrentModeID)
	}
	if len(got.Modes) != 1 || got.Modes[0] != "legacy-mode" {
		t.Fatalf("Modes = %+v, want [legacy-mode]", got.Modes)
	}
}

func TestBuildProbeResult_FallsBackToConfigOptionGroupedModels(t *testing.T) {
	t.Parallel()

	got := buildProbeResult(Frame{}, Frame{"result": map[string]any{
		"sessionId": "session-grouped",
		"configOptions": []any{
			map[string]any{
				"category":     "model",
				"currentValue": "opus",
				"type":         "select",
				"options": []any{
					map[string]any{
						"group": "Anthropic",
						"options": []any{
							map[string]any{"value": "opus", "name": "Opus"},
							map[string]any{"value": "sonnet", "name": "Sonnet"},
						},
					},
					map[string]any{
						"group": "Other",
						"options": []any{
							map[string]any{"value": "haiku", "name": "Haiku"},
						},
					},
				},
			},
		},
	}})

	if got.CurrentModelID != "opus" {
		t.Fatalf("CurrentModelID = %q, want opus", got.CurrentModelID)
	}
	wantModels := map[string]bool{"opus": true, "sonnet": true, "haiku": true}
	if len(got.Models) != 3 {
		t.Fatalf("len(Models) = %d, want 3: %v", len(got.Models), got.Models)
	}
	for _, model := range got.Models {
		if !wantModels[model] {
			t.Fatalf("unexpected model %q in %v", model, got.Models)
		}
	}
}
