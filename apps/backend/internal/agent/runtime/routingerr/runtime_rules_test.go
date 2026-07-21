package routingerr

import (
	"strings"
	"testing"
)

func TestMatchRuntimeEnvironmentRules_NpxENOTEMPTY(t *testing.T) {
	stderr := strings.Join([]string{
		"npm error code ENOTEMPTY",
		"npm error syscall rename",
		"npm error path /Users/cfl/.npm/_npx/d820eb7d96bc2600/node_modules/@anthropic-ai/claude-agent-sdk-darwin-arm64",
		"npm error dest /Users/cfl/.npm/_npx/d820eb7d96bc2600/node_modules/@anthropic-ai/.claude-agent-sdk-darwin-arm64-n6izKK54",
		"npm error errno -66",
		"npm error ENOTEMPTY: directory not empty",
	}, "\n")

	got, ok := matchRuntimeEnvironmentRules(stderr)
	if !ok {
		t.Fatalf("expected match, got none")
	}
	if got.Code != CodeNpxCacheCorrupted {
		t.Fatalf("Code = %q, want %q", got.Code, CodeNpxCacheCorrupted)
	}
	if got.ClassifierRule != "npm.enotempty.npx.v1" {
		t.Fatalf("ClassifierRule = %q, want npm.enotempty.npx.v1", got.ClassifierRule)
	}
	if got.Confidence != ConfHigh {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, ConfHigh)
	}
	wantPath := "/Users/cfl/.npm/_npx/d820eb7d96bc2600"
	if got.RemediationPath != wantPath {
		t.Fatalf("RemediationPath = %q, want %q", got.RemediationPath, wantPath)
	}
}

func TestMatchRuntimeEnvironmentRules_NoMatch(t *testing.T) {
	cases := []string{
		"",
		"some unrelated error",
		"npm error code EACCES",
		"npm error code ENOTEMPTY without any _npx path",
	}
	for _, in := range cases {
		if _, ok := matchRuntimeEnvironmentRules(in); ok {
			t.Errorf("expected no match for %q", in)
		}
	}
}

func TestExtractNpxCachePath_AllowsSpacesInHomePath(t *testing.T) {
	// macOS user dirs commonly contain spaces; the regex must not stop
	// at whitespace or the rule silently degrades on those machines.
	text := "npm error path /Users/John Doe/.npm/_npx/abc123def4/node_modules/foo"
	got := extractNpxCachePath(text)
	want := "/Users/John Doe/.npm/_npx/abc123def4"
	if got != want {
		t.Errorf("extractNpxCachePath = %q, want %q", got, want)
	}
}

func TestClassify_NpxCacheCorrupted_WiredThroughClassify(t *testing.T) {
	resetInjection()
	exit := 190
	in := Input{
		Phase:      PhaseSessionInit,
		ProviderID: "claude-acp",
		ExitCode:   &exit,
		Stderr: strings.Join([]string{
			"npm error code ENOTEMPTY",
			"npm error path /Users/cfl/.npm/_npx/d820eb7d96bc2600/node_modules/@anthropic-ai/claude-agent-sdk-darwin-arm64",
			"npm error ENOTEMPTY: directory not empty",
		}, "\n"),
	}
	e := Classify(in)
	if e == nil {
		t.Fatal("expected non-nil Error")
	}
	if e.Code != CodeNpxCacheCorrupted {
		t.Fatalf("Code = %q, want %q", e.Code, CodeNpxCacheCorrupted)
	}
	if !e.AutoRetryable {
		t.Errorf("AutoRetryable = false, want true")
	}
	if !e.FallbackAllowed {
		t.Errorf("FallbackAllowed = false, want true")
	}
	if e.RemediationPath != "/Users/cfl/.npm/_npx/d820eb7d96bc2600" {
		t.Errorf("RemediationPath = %q, want /Users/cfl/.npm/_npx/d820eb7d96bc2600", e.RemediationPath)
	}
	if e.Phase != PhaseSessionInit {
		t.Errorf("Phase = %q, want %q", e.Phase, PhaseSessionInit)
	}
	if e.ExitCode == nil || *e.ExitCode != 190 {
		t.Errorf("ExitCode lost, got %v", e.ExitCode)
	}
}
