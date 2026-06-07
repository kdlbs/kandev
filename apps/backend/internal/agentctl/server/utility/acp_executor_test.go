package utility

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func ptr[T any](v T) *T { return &v }

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
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp)

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
}

// The legacy `models` field still wins when present so existing agents are
// unaffected; configOptions is only consulted as a fallback.
func TestApplySessionProbeFields_PrefersLegacyModelsField(t *testing.T) {
	t.Parallel()

	modelCat := acp.SessionConfigOptionCategoryModel
	resp := acp.NewSessionResponse{
		Models: &acp.SessionModelState{
			CurrentModelId: "legacy",
			AvailableModels: []acp.ModelInfo{
				{ModelId: "legacy", Name: "Legacy"},
			},
		},
		ConfigOptions: []acp.SessionConfigOption{
			{Select: &acp.SessionConfigOptionSelect{
				Category:     &modelCat,
				CurrentValue: "fallback",
				Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: "fallback", Name: "Fallback"},
				}},
				Type: "select",
			}},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp)

	if got, want := out.CurrentModelID, "legacy"; got != want {
		t.Fatalf("CurrentModelID = %q, want %q", got, want)
	}
	if got, want := len(out.Models), 1; got != want {
		t.Fatalf("len(Models) = %d, want %d", got, want)
	}
	if got, want := out.Models[0].ID, "legacy"; got != want {
		t.Fatalf("Models[0].ID = %q, want %q", got, want)
	}
}

// A non-nil `Models` struct with an empty `AvailableModels` slice is
// schema-valid. The fallback must NOT fire in that case — otherwise
// `CurrentModelID` set from the legacy field gets clobbered by the
// configOptions value, mixing sources.
func TestApplySessionProbeFields_LegacyEmptyModelsBlocksFallback(t *testing.T) {
	t.Parallel()

	modelCat := acp.SessionConfigOptionCategoryModel
	resp := acp.NewSessionResponse{
		Models: &acp.SessionModelState{
			CurrentModelId:  "legacy-current",
			AvailableModels: nil,
		},
		ConfigOptions: []acp.SessionConfigOption{
			{Select: &acp.SessionConfigOptionSelect{
				Category:     &modelCat,
				CurrentValue: "fallback-current",
				Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: "fallback-current", Name: "Fallback"},
				}},
				Type: "select",
			}},
		},
	}

	out := &ProbeResponse{}
	applySessionProbeFields(out, resp)

	if got, want := out.CurrentModelID, "legacy-current"; got != want {
		t.Fatalf("CurrentModelID = %q, want %q (legacy must win, fallback must not fire)", got, want)
	}
	if len(out.Models) != 0 {
		t.Fatalf("Models = %+v, want empty (fallback should be skipped when legacy field is non-nil)", out.Models)
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
	applySessionProbeFields(out, resp)

	if got, want := len(out.Modes), 2; got != want {
		t.Fatalf("len(Modes) = %d, want %d", got, want)
	}
	if out.Modes[0].ID != "default" || out.Modes[1].ID != "bypass" {
		t.Fatalf("Modes = %+v, want [default bypass]", out.Modes)
	}
}
