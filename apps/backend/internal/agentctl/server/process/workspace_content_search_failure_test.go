//go:build !windows

package process

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

// Content search resolves its file list from a live `git ls-files` behind the
// shared Git admission semaphore, so under contention that command can be
// starved at admission or killed at its execution deadline. Both failures must
// reach the caller as errors.
//
// A swallowed failure here would be indistinguishable from a genuinely empty
// workspace: the palette would report "No content matches found" for a file
// that exists on disk, and any test asserting on that match would fail with no
// diagnosable cause. These pin the propagation at the tracker, which is where
// the error is born and where it could be dropped; Manager.SearchWorkspaceContent
// returns the first tracker error unchanged.

// TestSearchContentReportsGitAdmissionStarvation covers a caller that gives up
// waiting for a Git slot that never frees.
func TestSearchContentReportsGitAdmissionStarvation(t *testing.T) {
	tracker := newContentSearchFailureTracker(t)

	restoreCap := subproc.Git().SetCapForTest(1)
	t.Cleanup(restoreCap)
	hold, err := subproc.AcquireGit(context.Background(), subproc.GitInteractive)
	if err != nil {
		t.Fatalf("hold git slot: %v", err)
	}
	t.Cleanup(hold)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)
	results, err := tracker.SearchContent(ctx, "needle", 10)
	if err == nil {
		t.Fatalf("SearchContent returned nil error with %d results while Git admission "+
			"was starved; a starved search must not be reported as an empty workspace",
			len(results))
	}
}

// TestSearchContentReportsGitCommandTimeout covers a Git process that wedges
// past gitCommandTimeout and is killed.
func TestSearchContentReportsGitCommandTimeout(t *testing.T) {
	tracker := newContentSearchFailureTracker(t)

	previousTimeout := gitCommandTimeout
	gitCommandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { gitCommandTimeout = previousTimeout })
	shimDir := installSleepGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	results, err := tracker.SearchContent(context.Background(), "needle", 10)
	if err == nil {
		t.Fatalf("SearchContent returned nil error with %d results after the file-list "+
			"command was killed at its deadline; a failed search must not be reported "+
			"as an empty workspace", len(results))
	}
}

// newContentSearchFailureTracker builds a tracker over a repository holding one
// match and asserts the match is findable while Git is healthy, so a later
// empty result cannot be blamed on the fixture.
func newContentSearchFailureTracker(t *testing.T) *WorkspaceTracker {
	t.Helper()
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	writeFile(t, repoDir, "match.txt", "workspace content search needle\n")

	tracker := NewWorkspaceTrackerForRepo(repoDir, "repo", newTestLogger(t))
	baseline, err := tracker.SearchContent(context.Background(), "needle", 10)
	if err != nil {
		t.Fatalf("baseline SearchContent returned error: %v", err)
	}
	if len(baseline) != 1 {
		t.Fatalf("baseline result count = %d, want 1", len(baseline))
	}
	return tracker
}
