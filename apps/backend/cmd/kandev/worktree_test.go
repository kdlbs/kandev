package main

import (
	"context"
	"os/exec"
	"testing"
)

// TestDetectBranchRemote_ReturnsConfiguredUpstream covers the happy path: a
// branch with an explicit `branch.<name>.remote` config returns that remote
// (covers fork-workflow repos whose primary remote is named "upstream",
// "github", etc., where hard-coding "origin" would push to the wrong place).
func TestDetectBranchRemote_ReturnsConfiguredUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")
	mustGit(t, dir, "checkout", "-b", "feature")
	// Mimic what `git push --set-upstream <remote> <branch>` would write.
	mustGit(t, dir, "config", "branch.feature.remote", "upstream")

	if got := detectBranchRemote(context.Background(), dir, "feature"); got != "upstream" {
		t.Errorf("got %q, want upstream", got)
	}
}

func TestDetectBranchRemote_NoUpstreamFallsBackToOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init", "--quiet")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")
	mustGit(t, dir, "checkout", "-b", "feature")
	// No branch.feature.remote config — git config returns non-zero.

	if got := detectBranchRemote(context.Background(), dir, "feature"); got != defaultGitRemote {
		t.Errorf("got %q, want %s", got, defaultGitRemote)
	}
}

func TestDetectBranchRemote_NonGitDirFallsBackToOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// `git config` in a non-git dir errors out, so detectBranchRemote
	// should fall back to the default remote rather than propagate the error.
	if got := detectBranchRemote(context.Background(), t.TempDir(), "feature"); got != defaultGitRemote {
		t.Errorf("got %q, want %s", got, defaultGitRemote)
	}
}

// mustGit runs `git -C dir <args...>` and fails the test on non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	// Disable system-level config so user/host config can't interfere.
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
