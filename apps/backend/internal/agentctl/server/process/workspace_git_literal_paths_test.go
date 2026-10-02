package process

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitLiteralPatchSelection(t *testing.T) {
	for _, pair := range literalSelectionNames {
		for _, layer := range []string{"unstaged", "staged", "mixed"} {
			t.Run(layer+"/"+pair[0], func(t *testing.T) {
				skipNativeInvalidLiteralPath(t, pair[0])
				dir := setupLiteralSelectionRepo(t, pair)
				if layer != "unstaged" {
					runGit(t, dir, "add", "-A")
				}
				if layer == "mixed" {
					writeFile(t, dir, pair[0], "base-0\nselected-marker\nselected-worktree\n")
					writeFile(t, dir, pair[1], "base-1\nunselected-marker\nunselected-worktree\n")
				}
				tracker := NewWorkspaceTracker(dir, newTestLogger(t))
				t.Cleanup(tracker.Stop)
				status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
				if err != nil {
					t.Fatal(err)
				}
				file, ok := status.Files[pair[0]]
				if !ok || file.Path != pair[0] || len(status.Files) != 2 || file.DiffState != gitStatusDiffReady {
					t.Fatalf("incorrect membership/readiness: %+v", status.Files)
				}
				assertLiteralPatch(t, file.Diff, "selected-marker", "")
				wantAdditions := 1
				if layer == "mixed" {
					wantAdditions = 2
					assertNumstatFacet(t, file.StagedChange, 1, "selected-marker")
					assertNumstatFacet(t, file.UnstagedChange, 1, "selected-worktree")
					assertLiteralPatch(t, file.StagedChange.Diff, "selected-marker", "+selected-worktree")
					assertLiteralPatch(t, file.UnstagedChange.Diff, "selected-worktree", "+selected-marker")
				}
				if file.Additions != wantAdditions || file.Deletions != 0 {
					t.Errorf("counts +%d -%d, want +%d -0", file.Additions, file.Deletions, wantAdditions)
				}
			})
		}
	}
}

func assertLiteralPatch(t *testing.T, patch, selected, forbidden string) {
	t.Helper()
	if !strings.Contains(patch, "+"+selected) || strings.Contains(patch, "unselected-") || (forbidden != "" && strings.Contains(patch, forbidden)) {
		t.Errorf("patch must contain selected %q only (forbidden %q): %q", selected, forbidden, patch)
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitLiteralCachedFallback(t *testing.T) {
	for _, setting := range []string{"0", "1"} {
		t.Run(setting, func(t *testing.T) {
			checkLiteralCachedFallback(t, setting)
		})
	}
}

func checkLiteralCachedFallback(t *testing.T, setting string) {
	t.Helper()
	pair := [2]string{"new[ab].txt", "newa.txt"}
	dir := setupLiteralSelectionRepo(t, pair)
	runGit(t, dir, "add", "-A")
	t.Setenv("GIT_LITERAL_PATHSPECS", setting)
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	update := &types.GitStatusUpdate{Files: map[string]types.FileInfo{
		pair[0]: {Path: pair[0], Status: "modified", Staged: true},
		pair[1]: {Path: pair[1], Status: "modified", Staged: true},
	}}
	if err := tracker.enrichWithStagedDiff(context.Background(), update, "HEAD", types.GitStatusUpdate{}); err != nil {
		t.Fatal(err)
	}
	file := update.Files[pair[0]]
	if file.DiffState != gitStatusDiffReady || file.Additions != 1 {
		t.Fatalf("cached detail = %+v, want ready +1", file)
	}
	assertLiteralPatch(t, file.Diff, "selected-marker", "")
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
func TestWorkspaceGitLiteralPathspecEnvironment(t *testing.T) {
	pair := [2]string{"new[ab].txt", "newa.txt"}
	dir := setupLiteralSelectionRepo(t, pair)
	runGit(t, dir, "add", "-A")
	writeFile(t, dir, pair[0], "base-0\nselected-marker\nselected-worktree\n")
	writeFile(t, dir, pair[1], "base-1\nunselected-marker\nunselected-worktree\n")
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	status, err := tracker.GetGitStatusWithDetails(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	file := status.Files[pair[0]]
	assertLiteralPatch(t, file.Diff, "selected-worktree", "")
	assertNumstatFacet(t, file.StagedChange, 1, "selected-marker")
	assertNumstatFacet(t, file.UnstagedChange, 1, "selected-worktree")
	assertLiteralPatch(t, file.StagedChange.Diff, "selected-marker", "+selected-worktree")
	assertLiteralPatch(t, file.UnstagedChange.Diff, "selected-worktree", "+selected-marker")
	if os.Getenv("GIT_LITERAL_PATHSPECS") != "1" {
		t.Fatal("selected diff changed the process environment")
	}
	patch, _, err := capDiffOutput(context.Background(), dir, "diff", "HEAD", "--", pair[0])
	if err != nil {
		t.Fatal(err)
	}
	assertLiteralPatch(t, patch, "selected-worktree", "")
}
