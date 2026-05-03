package agents

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestWithNpxRunnable_MatchesWhenNpxOnPath confirms the npx-fallback returns
// available with the "npx <pkg>" tag whenever npx is on PATH. The settings
// page renders this as "Detected at npx <pkg>", giving users a truthful hint
// that the package isn't globally installed but launches via npx -y.
func TestWithNpxRunnable_MatchesWhenNpxOnPath(t *testing.T) {
	if !npxOnPath() {
		t.Skip("npx not on PATH; skipping fallback test")
	}

	found, matched, err := WithNpxRunnable("@example/pkg")(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true when node/npx is on PATH")
	}
	if matched != "npx @example/pkg" {
		t.Errorf("matched = %q, want %q", matched, "npx @example/pkg")
	}
}

// TestDetect_PrefersGlobalBinaryOverNpxFallback pins the first-match-wins
// contract: when both the global binary and npx are available, the global
// install reports its real path. This is what makes the UI's "Detected at
// /usr/local/bin/<bin>" hint accurate for users who actually installed.
func TestDetect_PrefersGlobalBinaryOverNpxFallback(t *testing.T) {
	if !npxOnPath() {
		t.Skip("npx not on PATH; skipping ordering test")
	}
	// `ls` is guaranteed on PATH in any POSIX-ish CI/dev env.
	if _, err := exec.LookPath("ls"); err != nil {
		t.Skip("ls not on PATH; skipping")
	}

	result, err := Detect(context.Background(),
		WithCommand("ls"),
		WithNpxRunnable("@example/pkg"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Available {
		t.Fatalf("expected Available=true")
	}
	if strings.HasPrefix(result.MatchedPath, "npx ") {
		t.Errorf("MatchedPath = %q, want a real path (global ls), not the npx tag", result.MatchedPath)
	}
}

// TestDetect_FallsBackToNpxWhenGlobalMissing covers the headline case: the
// agent's binary isn't installed globally, but node is — Detect should still
// return Available=true with the npx tag so the agent shows as "Installed"
// on the settings page.
func TestDetect_FallsBackToNpxWhenGlobalMissing(t *testing.T) {
	if !npxOnPath() {
		t.Skip("npx not on PATH; skipping fallback ordering test")
	}

	result, err := Detect(context.Background(),
		WithCommand("definitely-not-a-real-binary-xyz"),
		WithNpxRunnable("@example/pkg"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Available {
		t.Fatalf("expected Available=true via npx fallback")
	}
	if result.MatchedPath != "npx @example/pkg" {
		t.Errorf("MatchedPath = %q, want %q", result.MatchedPath, "npx @example/pkg")
	}
}

func npxOnPath() bool {
	_, err := exec.LookPath("npx")
	return err == nil
}
