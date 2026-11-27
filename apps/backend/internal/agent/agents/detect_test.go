package agents

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

// TestRunCommandCheck_SeparatesTimeoutFromFailure pins the discrimination the
// staged check depends on: a bound elapsing is reported as timedOut, the
// command's own nonzero exit is not, and a caller cancellation stays an error
// that neither case swallows.
func TestRunCommandCheck_SeparatesTimeoutFromFailure(t *testing.T) {
	installHermesACPCheckHelper(t)
	useHermesCheckCounter(t)

	hermesPath, err := exec.LookPath(hermesBin)
	if err != nil {
		t.Fatalf("LookPath(hermes): %v", err)
	}

	t.Run("success", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "available")

		found, timedOut, err := runCommandCheck(context.Background(), hermesPath, 10*time.Second, "acp", "--check")
		if err != nil {
			t.Fatalf("runCommandCheck error: %v", err)
		}
		if !found || timedOut {
			t.Errorf("found=%v timedOut=%v, want true/false", found, timedOut)
		}
	})

	t.Run("nonzero exit is not a timeout", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "unavailable")

		found, timedOut, err := runCommandCheck(context.Background(), hermesPath, 10*time.Second, "acp", "--check")
		if err != nil {
			t.Fatalf("runCommandCheck error: %v", err)
		}
		if found || timedOut {
			t.Errorf("found=%v timedOut=%v, want false/false", found, timedOut)
		}
	})

	t.Run("bound elapsing is a timeout", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "always-slow")

		// Long enough to cover spawning the helper, far short of its 2s stall.
		found, timedOut, err := runCommandCheck(context.Background(), hermesPath, 600*time.Millisecond, "acp", "--check")
		if err != nil {
			t.Fatalf("runCommandCheck error: %v", err)
		}
		if found || !timedOut {
			t.Errorf("found=%v timedOut=%v, want false/true", found, timedOut)
		}
	})

	t.Run("caller cancellation is an error", func(t *testing.T) {
		t.Setenv(hermesACPCheckHelperEnv, "available")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		found, timedOut, err := runCommandCheck(ctx, hermesPath, 10*time.Second, "acp", "--check")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runCommandCheck error = %v, want context.Canceled", err)
		}
		if found || timedOut {
			t.Errorf("found=%v timedOut=%v, want false/false on cancellation", found, timedOut)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		found, timedOut, err := runCommandCheck(context.Background(), filepath.Join(t.TempDir(), "absent-binary"), 10*time.Second)
		if err != nil {
			t.Fatalf("runCommandCheck error: %v", err)
		}
		if found || timedOut {
			t.Errorf("found=%v timedOut=%v, want false/false for a binary that cannot start", found, timedOut)
		}
	})
}

// @covers AC-AGENTS-INSTALL-DETECTION-001.1, AC-AGENTS-INSTALL-DETECTION-001.4
func TestRunCommandCheck_NonzeroExitBeforeDeadlineObservation(t *testing.T) {
	installHermesACPCheckHelper(t)
	t.Setenv(hermesACPCheckHelperEnv, "unavailable")
	hermesPath, err := exec.LookPath(hermesBin)
	if err != nil {
		t.Fatalf("LookPath(hermes): %v", err)
	}
	cmd := exec.Command(hermesPath, "acp", "--check")
	var exitErr *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exitErr) {
		t.Fatalf("helper error = %v, want a conclusive nonzero exit", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	if found, timedOut := commandCheckOutcome(ctx, exitErr.ProcessState, false); found || timedOut {
		t.Fatal("a deadline observed after a conclusive exit must not cause a retry")
	}
	if runtime.GOOS != "windows" {
		if found, timedOut := commandCheckOutcome(ctx, exitErr.ProcessState, true); found || timedOut {
			t.Fatal("a kill racing a naturally exited POSIX process must not cause a retry")
		}
	}
}

func TestRunCommandCheck_ExpiredBoundBeforeStart(t *testing.T) {
	installHermesACPCheckHelper(t)
	useHermesCheckCounter(t)
	t.Setenv(hermesACPCheckHelperEnv, "counted-unavailable")
	hermesPath, err := exec.LookPath(hermesBin)
	if err != nil {
		t.Fatalf("LookPath(hermes): %v", err)
	}
	found, timedOut, err := runCommandCheck(context.Background(), hermesPath, 0, "acp", "--check")
	if err != nil || found || !timedOut {
		t.Fatalf("found=%v timedOut=%v err=%v, want false/true/nil", found, timedOut, err)
	}
	if _, err := os.Stat(os.Getenv(hermesCheckCounterEnv)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("helper started after the bound expired: %v", err)
	}
}

func TestRunCommandCheck_SuccessBeforeDeadlineObservation(t *testing.T) {
	installHermesACPCheckHelper(t)
	t.Setenv(hermesACPCheckHelperEnv, "available")
	hermesPath, err := exec.LookPath(hermesBin)
	if err != nil {
		t.Fatalf("LookPath(hermes): %v", err)
	}
	cmd := exec.Command(hermesPath, "acp", "--check")
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper error = %v, want a conclusive success", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	for _, interrupted := range []bool{false, true} {
		found, timedOut := commandCheckOutcome(ctx, cmd.ProcessState, interrupted)
		if !found || timedOut {
			t.Errorf("interrupted=%v: found=%v timedOut=%v, want true/false", interrupted, found, timedOut)
		}
	}
}
