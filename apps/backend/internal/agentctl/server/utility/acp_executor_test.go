package utility

import "testing"

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
