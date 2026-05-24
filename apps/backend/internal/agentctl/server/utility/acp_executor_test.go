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
