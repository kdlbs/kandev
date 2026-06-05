package utility

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"
)

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

func TestIsOpenCodeACPCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command []string
		want    bool
	}{
		{name: "opencode acp", command: []string{"opencode", "acp"}, want: true},
		{name: "path opencode acp", command: []string{filepath.Join("/usr/local/bin", "opencode"), "acp"}, want: true},
		{name: "opencode non acp", command: []string{"opencode", "run"}, want: false},
		{name: "too short", command: []string{"opencode"}, want: false},
		{name: "other acp", command: []string{"claude", "acp"}, want: false},
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

func TestEnvironWithNoColorOverridesExistingValue(t *testing.T) {
	t.Parallel()

	got := environWithNoColor([]string{"PATH=/usr/bin", "NO_COLOR=0", "HOME=/tmp"})
	want := []string{"PATH=/usr/bin", "HOME=/tmp", "NO_COLOR=1"}
	if !slices.Equal(got, want) {
		t.Fatalf("environWithNoColor() = %#v, want %#v", got, want)
	}
}

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
