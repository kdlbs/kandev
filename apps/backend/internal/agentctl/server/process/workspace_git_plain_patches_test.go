package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var plainPatchColorCases = []string{
	"ordinary", "disabled", "diff-always", "ui-always", "ui-always-diff-disabled", "captured-always",
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45
func TestWorkspaceGitPlainPatches(t *testing.T) {
	for _, color := range plainPatchColorCases {
		for _, layer := range []string{"unstaged", "staged", "mixed"} {
			t.Run(color+"/"+layer, func(t *testing.T) {
				isolatePlainPatchEnvironment(t)
				dir, paths := seedPlainPatchRepo(t, layer)
				captured := configurePlainPatchColor(t, dir, color)
				tracker := NewWorkspaceTracker(dir, newTestLogger(t))
				t.Cleanup(tracker.Stop)
				tracker.SetGitEnvironment(captured)
				if color == "captured-always" {
					setPlainPatchLiveColor(t, "false")
				}
				before := plainPatchReadEvidence(t, dir, paths)
				envBefore := tracker.gitEnvironmentSnapshot()
				liveBefore := os.Environ()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				status, err := tracker.GetGitStatusWithDetails(ctx, true)
				require.NoError(t, err)
				require.Equal(t, gitStatusStateReady, status.StatusState)
				require.True(t, status.FilesComplete)
				require.Equal(t, gitStatusDetailReady, status.DetailState)
				require.Len(t, status.Files, len(paths))
				require.Len(t, status.Modified, len(paths))
				for path, tag := range paths {
					file, ok := status.Files[path]
					require.True(t, ok, "exact file key %q", path)
					assertPlainPatchFile(t, file, path, tag, layer)
				}
				require.Equal(t, before, plainPatchReadEvidence(t, dir, paths), "read mutated repository")
				require.Equal(t, envBefore, tracker.gitEnvironmentSnapshot())
				require.Equal(t, liveBefore, os.Environ())
			})
		}
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45
func TestWorkspaceGitPlainCachedFallback(t *testing.T) {
	for _, color := range plainPatchColorCases {
		t.Run(color, func(t *testing.T) {
			isolatePlainPatchEnvironment(t)
			dir, paths := seedPlainPatchRepo(t, "staged")
			captured := configurePlainPatchColor(t, dir, color)
			tracker := NewWorkspaceTracker(dir, newTestLogger(t))
			t.Cleanup(tracker.Stop)
			tracker.SetGitEnvironment(captured)
			if color == "captured-always" {
				setPlainPatchLiveColor(t, "false")
			}
			before := plainPatchReadEvidence(t, dir, paths)
			envBefore, liveBefore := tracker.gitEnvironmentSnapshot(), os.Environ()
			update := &types.GitStatusUpdate{Files: make(map[string]types.FileInfo)}
			for path := range paths {
				update.Files[path] = types.FileInfo{Path: path, Status: "modified", Staged: true}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			head := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
			require.NoError(t, tracker.enrichWithStagedDiff(ctx, update, head, types.GitStatusUpdate{}))
			for path, tag := range paths {
				assertPlainPatchFile(t, update.Files[path], path, tag, "staged")
			}
			require.Equal(t, before, plainPatchReadEvidence(t, dir, paths))
			require.Equal(t, envBefore, tracker.gitEnvironmentSnapshot())
			require.Equal(t, liveBefore, os.Environ())
		})
	}
}

func isolatePlainPatchEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			require.NoError(t, os.Unsetenv(key))
			t.Cleanup(func() { require.NoError(t, os.Setenv(key, value)) })
		}
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
}

func seedPlainPatchRepo(t *testing.T, layer string) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "core.autocrlf", "false")
	runGit(t, dir, "config", "core.quotePath", "false")
	paths := map[string]string{
		"new[ab].txt": "-selected", "newa.txt": "-sibling", "日本語.txt": "-unicode",
		"ansi.txt": "-literal-\x1b[31mcontent\x1b[0m",
	}
	if runtime.GOOS != "windows" {
		paths["escape\x1bname.txt"] = "-escape-name"
	}
	for path, tag := range paths {
		writeFile(t, dir, path, plainPatchContents(tag, "base-stage", "base-worktree"))
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "Plain workspace fixture")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	for path, tag := range paths {
		writeFile(t, dir, path, plainPatchContents(tag, "index-stage", "base-worktree"))
	}
	if layer != "unstaged" {
		runGit(t, dir, "add", "-A")
	}
	if layer == "mixed" {
		for path, tag := range paths {
			writeFile(t, dir, path, plainPatchContents(tag, "index-stage", "worktree-line"))
		}
	}
	return dir, paths
}

func plainPatchContents(tag, first, second string) string {
	return first + tag + "\n" + second + tag + "\ncontext" + tag + "\n"
}

func configurePlainPatchColor(t *testing.T, dir, color string) []string {
	t.Helper()
	switch color {
	case "disabled":
		runGit(t, dir, "config", "color.ui", "false")
		runGit(t, dir, "config", "color.diff", "false")
	case "diff-always":
		runGit(t, dir, "config", "color.diff", "always")
	case "ui-always":
		runGit(t, dir, "config", "color.ui", "always")
	case "ui-always-diff-disabled":
		runGit(t, dir, "config", "color.ui", "always")
		runGit(t, dir, "config", "color.diff", "false")
	case "captured-always":
		setPlainPatchLiveColor(t, "always")
	}
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")
	return append([]string(nil), os.Environ()...)
}

func setPlainPatchLiveColor(t *testing.T, value string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "color.ui")
	t.Setenv("GIT_CONFIG_VALUE_0", value)
	t.Setenv("GIT_CONFIG_KEY_1", "color.diff")
	t.Setenv("GIT_CONFIG_VALUE_1", value)
}

func plainPatchReadEvidence(t *testing.T, dir string, paths map[string]string) map[string]string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	require.NoError(t, err)
	evidence := map[string]string{
		"config": string(config), "head": runGit(t, dir, "rev-parse", "HEAD"),
		"refs":  runGit(t, dir, "for-each-ref", "--format=%(refname) %(objectname)"),
		"index": runGit(t, dir, "ls-files", "--stage", "-z"),
	}
	for path := range paths {
		data, readErr := os.ReadFile(filepath.Join(dir, path))
		require.NoError(t, readErr)
		evidence["worktree:"+path] = string(data)
		evidence["index:"+path] = runGit(t, dir, "show", ":"+path)
	}
	return evidence
}

func assertPlainPatchFile(t *testing.T, file types.FileInfo, path, tag, layer string) {
	t.Helper()
	require.Equal(t, path, file.Path)
	require.Equal(t, "modified", file.Status)
	require.Equal(t, layer == "staged", file.Staged)
	require.Empty(t, file.OldPath)
	count := 1
	if layer == "mixed" {
		count = 2
	}
	require.Equal(t, count, file.Additions)
	require.Equal(t, count, file.Deletions)
	require.Equal(t, gitStatusDiffReady, file.DiffState)
	require.Empty(t, file.DiffSkipReason)
	assertPlainPatchText(t, file.Diff, path, tag, layer)
	if layer != "mixed" {
		require.Nil(t, file.StagedChange)
		require.Nil(t, file.UnstagedChange)
		return
	}
	for _, facet := range []*types.FileChangeFacet{file.StagedChange, file.UnstagedChange} {
		require.NotNil(t, facet)
		require.Equal(t, "modified", facet.Status)
		require.Equal(t, 1, facet.Additions)
		require.Equal(t, 1, facet.Deletions)
		require.Equal(t, gitStatusDiffReady, facet.DiffState)
		require.Empty(t, facet.DiffSkipReason)
		require.Empty(t, facet.OldPath)
	}
	assertPlainPatchText(t, file.StagedChange.Diff, path, tag, "staged")
	assertPlainPatchText(t, file.UnstagedChange.Diff, path, tag, "mixed-unstaged")
}

func assertPlainPatchText(t *testing.T, patch, path, tag, layer string) {
	t.Helper()
	a, b := "a/"+path, "b/"+path
	if strings.Contains(path, "\x1b") {
		a = `"` + strings.ReplaceAll(a, "\x1b", `\033`) + `"`
		b = `"` + strings.ReplaceAll(b, "\x1b", `\033`) + `"`
	}
	assert.True(t, strings.HasPrefix(patch, "diff --git "+a+" "+b+"\n"), "patch header: %q", patch)
	assert.Contains(t, patch, "--- "+a+"\n+++ "+b+"\n")
	var hunk string
	switch layer {
	case "mixed":
		hunk = fmt.Sprintf("@@ -1,3 +1,3 @@\n-base-stage%s\n-base-worktree%s\n+index-stage%s\n+worktree-line%s\n context%s\n", tag, tag, tag, tag, tag)
	case "mixed-unstaged":
		hunk = fmt.Sprintf("@@ -1,3 +1,3 @@\n index-stage%s\n-base-worktree%s\n+worktree-line%s\n context%s\n", tag, tag, tag, tag)
	default:
		hunk = fmt.Sprintf("@@ -1,3 +1,3 @@\n-base-stage%s\n+index-stage%s\n base-worktree%s\n context%s\n", tag, tag, tag, tag)
	}
	assert.True(t, strings.HasSuffix(patch, hunk), "patch = %q, want exact hunk %q", patch, hunk)
	if !strings.Contains(tag, "\x1b") {
		assert.NotContains(t, patch, "\x1b[")
	}
}
