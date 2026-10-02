package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitMalformedNumstatDetailsUnavailable(t *testing.T) {
	for _, phase := range []string{"unstaged", "staged", "mixed"} {
		for _, suffix := range []string{"1\t0\tbroken.txt", "1\t0\t\x00old\x00", "bad\t0\tbroken.txt\x00", "0\t999999999999999999999999\tbroken.txt\x00"} {
			t.Run(phase+"/"+suffix, func(t *testing.T) {
				dir := setupNumstatQualityRepo(t, phase)
				installNumstatOutputShim(t, phase, "1\t0\thealthy.txt\x00"+suffix)
				tracker := NewWorkspaceTracker(dir, newTestLogger(t))
				t.Cleanup(tracker.Stop)
				status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
				if !errors.Is(err, errGitStatusDetailsUnavailable) || status.DetailState != gitStatusDetailUnavailable ||
					status.ErrorCode != gitStatusErrorDetailsUnavailable || !status.FilesComplete || status.StatusState != gitStatusStateReady {
					t.Fatalf("malformed %s output published: err=%v status=%+v", phase, err, status)
				}
				assertNumstatQualityState(t, status.Files["healthy.txt"], phase, gitStatusDiffReady)
				assertNumstatQualityState(t, status.Files["broken.txt"], phase, gitStatusDiffUnavailable)
			})
		}
	}
}

func setupNumstatQualityRepo(t *testing.T, phase string) string {
	t.Helper()
	dir := setupNumstatPathRepo(t, "healthy.txt", "base\n")
	writeFile(t, dir, "broken.txt", "base\n")
	runGit(t, dir, "add", "broken.txt")
	runGit(t, dir, "commit", "-m", "Add sibling")
	for _, path := range []string{"healthy.txt", "broken.txt"} {
		if phase != "unstaged" {
			writeFile(t, dir, path, "base\nSTAGED_MARKER\n")
			runGit(t, dir, "add", "--", path)
		}
		writeFile(t, dir, path, "base\nSTAGED_MARKER\nUNSTAGED_MARKER\n")
	}
	return dir
}

func assertNumstatQualityState(t *testing.T, file types.FileInfo, phase, want string) {
	t.Helper()
	state, diff := file.DiffState, file.Diff
	switch phase {
	case "staged":
		if file.StagedChange == nil {
			t.Fatal("missing staged facet")
		}
		state, diff = file.StagedChange.DiffState, file.StagedChange.Diff
	case "mixed":
		if file.UnstagedChange == nil {
			t.Fatal("missing unstaged facet")
		}
		state, diff = file.UnstagedChange.DiffState, file.UnstagedChange.Diff
	}
	if state != want || (want == gitStatusDiffReady && !strings.Contains(diff, "MARKER")) {
		t.Errorf("%s details state=%q patch=%q, want %q", phase, state, diff, want)
	}
	if phase != "unstaged" && (file.DiffState != gitStatusDiffReady || !strings.Contains(file.Diff, "UNSTAGED_MARKER")) {
		t.Errorf("ready flattened details lost: %+v", file)
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitEmptyNumstatDetailsReady(t *testing.T) {
	dir := setupNumstatPathRepo(t, "healthy.txt", "base\n")
	installNumstatOutputShim(t, "unstaged", "")
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil || status.DetailState != gitStatusDetailReady || len(status.Files) != 0 {
		t.Fatalf("legitimate empty numstat: err=%v status=%+v", err, status)
	}
}

func installNumstatOutputShim(t *testing.T, phase, output string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Git stdout injection uses the existing POSIX PATH shim pattern")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	payload := filepath.Join(dir, "numstat")
	if err := os.WriteFile(payload, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := fmt.Sprintf(`#!/bin/sh
numstat=0
nul=0
cached=0
last=''
for arg in "$@"; do
  case "$arg" in --numstat) numstat=1 ;; -z) nul=1 ;; --cached) cached=1 ;; esac
  last="$arg"
done
phase=unstaged
if [ "$cached" = 1 ]; then phase=staged; elif [ "$last" = -z ]; then phase=mixed; fi
if [ "$numstat" = 1 ] && [ "$nul" = 1 ] && [ "$phase" = %s ]; then
  exec cat %s
fi
exec %s "$@"
`, quote(phase), quote(payload), quote(realGit))
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
