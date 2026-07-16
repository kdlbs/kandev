package utility

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func ptr[T any](v T) *T { return &v }

func TestProbeConfigOptions_PreservesDescriptions(t *testing.T) {
	t.Parallel()

	options := acp.SessionConfigSelectOptionsUngrouped{{
		Value:       "high",
		Name:        "High",
		Description: ptr("More thorough reasoning."),
	}}

	converted := probeConfigOptions([]acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{
			Type:         "select",
			Id:           "reasoning_effort",
			Name:         "Reasoning effort",
			Description:  ptr("Controls reasoning depth."),
			CurrentValue: "high",
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &options},
		},
	}})

	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("marshal config options: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal config options: %v", err)
	}
	if got := payload[0]["description"]; got != "Controls reasoning depth." {
		t.Errorf("option description = %#v, want %q", got, "Controls reasoning depth.")
	}
	values := payload[0]["options"].([]any)
	if got := values[0].(map[string]any)["description"]; got != "More thorough reasoning." {
		t.Errorf("value description = %#v, want %q", got, "More thorough reasoning.")
	}
}

func TestResolveProbeCommand_AllowsEveryListedBinary(t *testing.T) {
	t.Parallel()

	for _, name := range slices.Sorted(maps.Keys(allowedProbeCommands)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := resolveProbeCommand(name); got != name {
				t.Fatalf("resolveProbeCommand(%q) = %q, want %q", name, got, name)
			}
			path := filepath.Join("/usr/local/bin", name)
			if got := resolveProbeCommand(path); got != name {
				t.Fatalf("resolveProbeCommand(%q) = %q, want %q", path, got, name)
			}
		})
	}
}

func TestResolveProbeCommand_RejectsUnknown(t *testing.T) {
	t.Parallel()
	if got := resolveProbeCommand("claude"); got != "" {
		t.Fatalf("resolveProbeCommand(claude) = %q, want empty", got)
	}
}

// TestIsOpenCodeACPCommand verifies that the fallback only applies to
// OpenCode's ACP transport command.
func TestIsOpenCodeACPCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command []string
		want    bool
	}{
		{name: "opencode acp", command: []string{openCodeCommand, openCodeACPSubcommand}, want: true},
		{name: "path opencode acp", command: []string{filepath.Join("/usr/local/bin", openCodeCommand), openCodeACPSubcommand}, want: true},
		{name: "opencode non acp", command: []string{openCodeCommand, "run"}, want: false},
		{name: "too short", command: []string{openCodeCommand}, want: false},
		{name: "other acp", command: []string{"claude", openCodeACPSubcommand}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isOpenCodeACPCommand(tt.command); got != tt.want {
				t.Fatalf("isOpenCodeACPCommand(%v) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

// TestParseOpenCodeModelsOutput verifies parsing, deduplication, and filtering
// of non-model lines from OpenCode CLI output.
func TestParseOpenCodeModelsOutput(t *testing.T) {
	t.Parallel()

	got := parseOpenCodeModelsOutput("\nAvailable models:\nopenai/gpt-5.5\nanthropic/claude-sonnet-4-5\nloading models\nopenrouter/anthropic/claude-sonnet-4\nopenai/gpt-5.5\n")
	want := []ProbeModel{
		{ID: "openai/gpt-5.5", Name: "openai/gpt-5.5"},
		{ID: "anthropic/claude-sonnet-4-5", Name: "anthropic/claude-sonnet-4-5"},
		{ID: "openrouter/anthropic/claude-sonnet-4", Name: "openrouter/anthropic/claude-sonnet-4"},
	}
	if !slices.EqualFunc(got, want, func(a, b ProbeModel) bool {
		return a.ID == b.ID && a.Name == b.Name && a.Description == b.Description
	}) {
		t.Fatalf("parseOpenCodeModelsOutput() = %#v, want %#v", got, want)
	}
}

// TestEnvironWithNoColorOverridesExistingValue verifies that NO_COLOR=1 wins
// over any pre-existing environment value.
func TestEnvironWithNoColorOverridesExistingValue(t *testing.T) {
	t.Parallel()

	got := environWithNoColor([]string{"PATH=/usr/bin", "NO_COLOR=0", "HOME=/tmp"})
	want := []string{"PATH=/usr/bin", "HOME=/tmp", "NO_COLOR=1"}
	if !slices.Equal(got, want) {
		t.Fatalf("environWithNoColor() = %#v, want %#v", got, want)
	}
}

// TestIsOpenCodeModelID verifies the lightweight format guard for OpenCode
// model IDs parsed from CLI output.
func TestIsOpenCodeModelID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id   string
		want bool
	}{
		{id: "openai/gpt-5.5", want: true},
		{id: "openrouter/anthropic/claude-sonnet-4", want: true},
		{id: "Available models:", want: false},
		{id: "loading models", want: false},
		{id: "", want: false},
		{id: "openai /gpt-5.5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			if got := isOpenCodeModelID(tt.id); got != tt.want {
				t.Fatalf("isOpenCodeModelID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestSanitizeInferenceChunk_DropsPiVersionBanner(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("pi v0.74.0")
	if got != "" {
		t.Fatalf("sanitizeInferenceChunk() = %q, want empty string", got)
	}
}

func TestSanitizeInferenceChunk_PreservesNormalText(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("fix: avoid duplicate commit message generation")
	want := "fix: avoid duplicate commit message generation"
	if got != want {
		t.Fatalf("sanitizeInferenceChunk() = %q, want %q", got, want)
	}
}

func TestSanitizeInferenceChunk_RemovesBannerLineFromMultilineChunk(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("pi v0.74.0\nfix: tighten prompt parsing")
	want := "fix: tighten prompt parsing"
	if got != want {
		t.Fatalf("sanitizeInferenceChunk() = %q, want %q", got, want)
	}
}

func TestSanitizeInferenceChunk_EmptyInput(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("")
	if got != "" {
		t.Fatalf("sanitizeInferenceChunk() = %q, want empty string", got)
	}
}

func TestSanitizeInferenceChunk_BannerWithWhitespace(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("  pi v0.74.0  ")
	if got != "" {
		t.Fatalf("sanitizeInferenceChunk() = %q, want empty string", got)
	}
}

func TestSanitizeInferenceChunk_RemovesBannerLineAtEnd(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("fix: tighten prompt parsing\npi v0.74.0")
	want := "fix: tighten prompt parsing"
	if got != want {
		t.Fatalf("sanitizeInferenceChunk() = %q, want %q", got, want)
	}
}

func TestSanitizeInferenceChunk_RemovesMultipleBannerLines(t *testing.T) {
	t.Parallel()

	got := sanitizeInferenceChunk("pi v0.74.0\nfix: tighten prompt parsing\npi v1.0.0")
	want := "fix: tighten prompt parsing"
	if got != want {
		t.Fatalf("sanitizeInferenceChunk() = %q, want %q", got, want)
	}
}

// Reproduces the regression behind "Claude advertised no models": newer
// claude-agent-acp (v0.42+) drops the unstable `models` / `modes` fields and
// publishes the same data through `configOptions[]`. The probe must fall back
// to that shape so the inference-agents endpoint still surfaces the model
// list.
func TestApplySessionProbeFields_FallsBackToConfigOptions(t *testing.T) {
	t.Parallel()

	modelCat := acp.SessionConfigOptionCategoryModel
	thoughtCat := acp.SessionConfigOptionCategoryThoughtLevel
	resp := acp.NewSessionResponse{
		ConfigOptions: []acp.SessionConfigOption{
			{Select: &acp.SessionConfigOptionSelect{
				Category:     &modelCat,
				CurrentValue: "opus",
				Id:           "model",
				Name:         "Model",
				Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: "default", Name: "Default (recommended)", Description: ptr("Sonnet 4.6")},
					{Value: "opus", Name: "Opus", Description: ptr("Opus 4.7")},
					{Value: "haiku", Name: "Haiku"},
				}},
				Type: "select",
			}},
			{Select: &acp.SessionConfigOptionSelect{
				Category:     &thoughtCat,
				CurrentValue: "medium",
				Id:           "reasoning_effort",
				Name:         "Reasoning effort",
				Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: "low", Name: "Low"},
					{Value: "medium", Name: "Medium"},
					{Value: "high", Name: "High"},
				}},
				Type: "select",
			}},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp, "")

	if got, want := out.CurrentModelID, "opus"; got != want {
		t.Fatalf("CurrentModelID = %q, want %q", got, want)
	}
	if got, want := len(out.Models), 3; got != want {
		t.Fatalf("len(Models) = %d, want %d", got, want)
	}
	if got, want := out.Models[1].ID, "opus"; got != want {
		t.Fatalf("Models[1].ID = %q, want %q", got, want)
	}
	if got, want := out.Models[0].Description, "Sonnet 4.6"; got != want {
		t.Fatalf("Models[0].Description = %q, want %q", got, want)
	}
	if got, want := len(out.ConfigOptions), 2; got != want {
		t.Fatalf("len(ConfigOptions) = %d, want %d", got, want)
	}
	if got, want := out.ConfigOptions[1].ID, "reasoning_effort"; got != want {
		t.Fatalf("ConfigOptions[1].ID = %q, want %q", got, want)
	}
	if got, want := out.ConfigOptions[1].Options[2].Name, "High"; got != want {
		t.Fatalf("ConfigOptions[1].Options[2].Name = %q, want %q", got, want)
	}
}

// auggie 0.29.x hasn't migrated to configOptions[category=model] and still
// emits the pre-v0.13.5 top-level `models` field. The kdlbs SDK fork keeps
// parsing it as acp.LegacyModels; the probe must fall through to that surface
// so the inference-agents endpoint still surfaces auggie's model list.
func TestApplySessionProbeFields_FallsBackToLegacyModels(t *testing.T) {
	t.Parallel()

	opusDesc := "Great for complex coding"
	resp := acp.NewSessionResponse{
		LegacyModels: &acp.LegacyModels{
			CurrentModelId: "claude-opus-4-7",
			AvailableModels: []acp.LegacyModelInfo{
				{ModelId: "claude-opus-4-7", Name: "Opus 4.7", Description: &opusDesc},
				{ModelId: "claude-sonnet-4-5", Name: "Sonnet 4.5"},
			},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp, "")

	if got, want := out.CurrentModelID, "claude-opus-4-7"; got != want {
		t.Fatalf("CurrentModelID = %q, want %q", got, want)
	}
	if got, want := len(out.Models), 2; got != want {
		t.Fatalf("len(Models) = %d, want %d", got, want)
	}
	if got, want := out.Models[0].ID, "claude-opus-4-7"; got != want {
		t.Fatalf("Models[0].ID = %q, want %q", got, want)
	}
	if got, want := out.Models[0].Description, "Great for complex coding"; got != want {
		t.Fatalf("Models[0].Description = %q, want %q", got, want)
	}
}

func TestApplySessionProbeFields_GrokLegacyModelsExposeReasoning(t *testing.T) {
	t.Parallel()

	resp := acp.NewSessionResponse{
		LegacyModels: &acp.LegacyModels{
			CurrentModelId: "grok-4.5",
			AvailableModels: []acp.LegacyModelInfo{{
				ModelId: "grok-4.5",
				Name:    "Grok 4.5",
				Meta: map[string]any{
					"supportsReasoningEffort": true,
					"reasoningEffort":         "high",
					"reasoningEfforts":        []any{"high", "medium", "low"},
				},
			}},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp, "grok-acp")

	if got, want := len(out.ConfigOptions), 2; got != want {
		t.Fatalf("len(ConfigOptions) = %d, want model + reasoning", got)
	}
	if got, want := out.ConfigOptions[1].ID, "reasoning_effort"; got != want {
		t.Fatalf("ConfigOptions[1].ID = %q, want %q", got, want)
	}
}

// When both surfaces are present the typed configOptions list wins — the
// legacy field is only a fallback for unmigrated agents.
func TestApplySessionProbeFields_TypedConfigOptionsBeatLegacyModels(t *testing.T) {
	t.Parallel()

	modelCat := acp.SessionConfigOptionCategoryModel
	resp := acp.NewSessionResponse{
		ConfigOptions: []acp.SessionConfigOption{
			{Select: &acp.SessionConfigOptionSelect{
				Category:     &modelCat,
				CurrentValue: "opus",
				Id:           "model",
				Name:         "Model",
				Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: "opus", Name: "Opus"},
				}},
				Type: "select",
			}},
		},
		LegacyModels: &acp.LegacyModels{
			CurrentModelId:  "claude-opus-4-7",
			AvailableModels: []acp.LegacyModelInfo{{ModelId: "claude-opus-4-7", Name: "Opus 4.7"}},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp, "")

	if got, want := out.CurrentModelID, "opus"; got != want {
		t.Fatalf("CurrentModelID = %q, want %q (typed configOptions must win)", got, want)
	}
	if got, want := len(out.Models), 1; got != want {
		t.Fatalf("len(Models) = %d, want %d", got, want)
	}
	if got, want := out.Models[0].ID, "opus"; got != want {
		t.Fatalf("Models[0].ID = %q, want %q", got, want)
	}
}

// Grouped select-option payloads are flattened group-by-group so the
// fallback works regardless of whether the agent groups its options.
func TestApplySessionProbeFields_FlattensGroupedConfigOptions(t *testing.T) {
	t.Parallel()

	modeCat := acp.SessionConfigOptionCategoryMode
	resp := acp.NewSessionResponse{
		ConfigOptions: []acp.SessionConfigOption{
			{Select: &acp.SessionConfigOptionSelect{
				Category:     &modeCat,
				CurrentValue: "default",
				Options: acp.SessionConfigSelectOptions{Grouped: &acp.SessionConfigSelectOptionsGrouped{
					{Group: "safe", Name: "Safe", Options: []acp.SessionConfigSelectOption{
						{Value: "default", Name: "Default"},
					}},
					{Group: "danger", Name: "Danger", Options: []acp.SessionConfigSelectOption{
						{Value: "bypass", Name: "Bypass"},
					}},
				}},
				Type: "select",
			}},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp, "")

	if got, want := len(out.Modes), 2; got != want {
		t.Fatalf("len(Modes) = %d, want %d", got, want)
	}
	if out.Modes[0].ID != "default" || out.Modes[1].ID != "bypass" {
		t.Fatalf("Modes = %+v, want [default bypass]", out.Modes)
	}
}

func TestRemoveEnvEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  []string
		key  string
		want []string
	}{
		{
			name: "removes single match",
			env:  []string{"ACP_BACKEND=windsurf", "PATH=/usr/bin"},
			key:  "ACP_BACKEND",
			want: []string{"PATH=/usr/bin"},
		},
		{
			name: "removes all matches",
			env:  []string{"ACP_BACKEND=a", "PATH=/usr/bin", "ACP_BACKEND=b"},
			key:  "ACP_BACKEND",
			want: []string{"PATH=/usr/bin"},
		},
		{
			name: "no match returns unchanged",
			env:  []string{"PATH=/usr/bin", "HOME=/root"},
			key:  "ACP_BACKEND",
			want: []string{"PATH=/usr/bin", "HOME=/root"},
		},
		{
			name: "prefix-only key not removed",
			env:  []string{"ACP_BACKEND_URL=http://x", "ACP_BACKEND=windsurf"},
			key:  "ACP_BACKEND",
			want: []string{"ACP_BACKEND_URL=http://x"},
		},
		{
			name: "empty env",
			env:  []string{},
			key:  "ACP_BACKEND",
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RemoveEnvEntry(tc.env, tc.key)
			if !slices.Equal(got, tc.want) {
				t.Errorf("RemoveEnvEntry(%v, %q) = %v, want %v", tc.env, tc.key, got, tc.want)
			}
		})
	}
}

func TestSanitizeEnvForAgent(t *testing.T) {
	// Core case: strip listed vars while keeping the rest. This covers the
	// main RemoveEnvEntry + sanitizeEnvForAgent path.
	t.Setenv("ACP_BACKEND", "windsurf")
	t.Setenv("TEST_KEEP_ME", "yes")

	env := sanitizeEnvForAgent(&InferenceConfigDTO{
		StripEnv: []string{"ACP_BACKEND"},
	})
	for _, e := range env {
		if strings.HasPrefix(e, "ACP_BACKEND=") {
			t.Errorf("ACP_BACKEND not stripped: %q", e)
		}
	}
	found := false
	for _, e := range env {
		if e == "TEST_KEEP_ME=yes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TEST_KEEP_ME was stripped but should have been kept")
	}
}
