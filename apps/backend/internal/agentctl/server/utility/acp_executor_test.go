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
