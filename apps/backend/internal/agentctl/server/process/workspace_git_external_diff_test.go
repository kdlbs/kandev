package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var workspaceExternalModes = []string{"ordinary", "configured", "environment", "both"}

type workspaceExternalFixture struct {
	dir     string
	paths   map[string]string
	tracker *WorkspaceTracker
	helper  [2]cumulativeExternalHelper
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.46
func TestWorkspaceGitExternalHelpers(t *testing.T) {
	for _, mode := range workspaceExternalModes {
		for _, layer := range []string{"unstaged", "staged", "mixed"} {
			t.Run(mode+"/"+layer, func(t *testing.T) {
				fixture := newWorkspaceExternalFixture(t, mode, layer)
				status := readWorkspaceExternal(t, fixture, func(ctx context.Context) (types.GitStatusUpdate, error) {
					return fixture.tracker.GetGitStatusWithDetails(ctx, true)
				})
				assertWorkspaceExternalStatus(t, fixture, status, layer)
			})
		}
	}
}

func newWorkspaceExternalFixture(t *testing.T, mode, layer string) workspaceExternalFixture {
	t.Helper()
	isolatePlainPatchEnvironment(t)
	dir, paths := seedPlainPatchRepo(t, layer)
	env := configurePlainPatchColor(t, dir, "diff-always")
	setPlainPatchLiveColor(t, "always")
	env = append(env, "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=color.ui", "GIT_CONFIG_VALUE_0=always",
		"GIT_CONFIG_KEY_1=color.diff", "GIT_CONFIG_VALUE_1=always")
	helper := [2]cumulativeExternalHelper{newCumulativeExternalHelper(t), newCumulativeExternalHelper(t)}
	// The helper's private entry gate must be in the captured child environment.
	env = append(env, "KANDEV_TEST_CUMULATIVE_EXTERNAL_HELPER=1")
	env = configureWorkspaceExternal(t, dir, "new[ab].txt", mode, env, helper)
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	tracker.SetGitEnvironment(env)
	setPlainPatchLiveColor(t, "false")
	t.Setenv("GIT_EXTERNAL_DIFF", "")
	return workspaceExternalFixture{dir: dir, paths: paths, tracker: tracker, helper: helper}
}

func configureWorkspaceExternal(t *testing.T, dir, path, mode string, env []string, helper [2]cumulativeExternalHelper) []string {
	t.Helper()
	if mode == "configured" || mode == "both" {
		runGit(t, dir, "config", "diff.external", helper[0].command)
		proveWorkspaceExternal(t, dir, path, env, helper[0])
	}
	if mode == "environment" || mode == "both" {
		if mode == "both" {
			runGit(t, dir, "config", "--unset", "diff.external")
		}
		env = append(append([]string(nil), env...), "GIT_EXTERNAL_DIFF="+helper[1].command)
		proveWorkspaceExternal(t, dir, path, env, helper[1])
		if mode == "both" {
			runGit(t, dir, "config", "diff.external", helper[0].command)
		}
	}
	return env
}

func proveWorkspaceExternal(t *testing.T, dir, path string, env []string, helper cumulativeExternalHelper) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "diff", "--ext-diff", "HEAD", "--", path)
	cmd.Dir, cmd.Env = dir, append([]string(nil), env...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "helper positive control: %s", output)
	require.Equal(t, "CUSTOM DIFF OUTPUT\n", string(output))
	data, err := os.ReadFile(helper.sentinel)
	require.NoError(t, err)
	require.Equal(t, "executed\n", string(data))
	require.NoError(t, os.Remove(helper.sentinel))
	t.Log("actual executable external-helper positive control passed")
}

func workspaceExternalReadGuard(t *testing.T, fixture workspaceExternalFixture) func() {
	t.Helper()
	before := workspaceExternalEvidence(t, fixture)
	env, live := fixture.tracker.gitEnvironmentSnapshot(), os.Environ()
	return func() {
		t.Helper()
		for _, helper := range fixture.helper {
			helper.assertAbsent(t)
		}
		assert.Equal(t, before, workspaceExternalEvidence(t, fixture), "detail read mutated repository")
		assert.Equal(t, env, fixture.tracker.gitEnvironmentSnapshot())
		assert.Equal(t, live, os.Environ())
	}
}

func workspaceExternalEvidence(t *testing.T, fixture workspaceExternalFixture) map[string]string {
	t.Helper()
	evidence := plainPatchReadEvidence(t, fixture.dir, fixture.paths)
	index, err := os.ReadFile(filepath.Join(fixture.dir, ".git", "index"))
	require.NoError(t, err)
	evidence["raw-index"] = string(index)
	evidence["dirty"] = runGit(t, fixture.dir, "--no-optional-locks", "status", "--porcelain=v1", "-z")
	evidence["patch"] = runGit(t, fixture.dir, "--no-optional-locks", "diff", "--no-ext-diff", "--no-color", "--binary", "HEAD")
	return evidence
}

func readWorkspaceExternal(t *testing.T, fixture workspaceExternalFixture, read func(context.Context) (types.GitStatusUpdate, error)) types.GitStatusUpdate {
	t.Helper()
	defer workspaceExternalReadGuard(t, fixture)()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := read(ctx)
	require.NoError(t, err)
	return status
}

func assertWorkspaceExternalStatus(t *testing.T, fixture workspaceExternalFixture, status types.GitStatusUpdate, layer string) {
	t.Helper()
	require.Equal(t, gitStatusStateReady, status.StatusState)
	require.True(t, status.FilesComplete)
	require.Equal(t, gitStatusDetailReady, status.DetailState)
	require.NotEmpty(t, status.TrackerID)
	require.Equal(t, strings.TrimSpace(runGit(t, fixture.dir, "rev-parse", "HEAD")), status.HeadCommit)
	require.Len(t, status.Files, len(fixture.paths))
	require.Len(t, status.Modified, len(fixture.paths))
	for path, tag := range fixture.paths {
		file, ok := status.Files[path]
		require.True(t, ok, "exact file key %q", path)
		assertPlainPatchFile(t, file, path, tag, layer)
		assert.NotContains(t, file.Diff, "CUSTOM DIFF OUTPUT")
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.46
func TestWorkspaceGitExternalHelperCachedFallback(t *testing.T) {
	for _, mode := range workspaceExternalModes {
		t.Run(mode, func(t *testing.T) {
			fixture := newWorkspaceExternalFixture(t, mode, "staged")
			defer workspaceExternalReadGuard(t, fixture)()
			update := &types.GitStatusUpdate{Files: make(map[string]types.FileInfo)}
			for path := range fixture.paths {
				update.Files[path] = types.FileInfo{Path: path, Status: "modified", Staged: true}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			head := strings.TrimSpace(runGit(t, fixture.dir, "rev-parse", "HEAD"))
			require.NoError(t, fixture.tracker.enrichWithStagedDiff(ctx, update, head, types.GitStatusUpdate{}))
			for path, tag := range fixture.paths {
				assertPlainPatchFile(t, update.Files[path], path, tag, "staged")
			}
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.1, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.46
func TestWorkspaceGitExternalHelperCacheAndDirtyReads(t *testing.T) {
	for _, mode := range workspaceExternalModes {
		t.Run(mode, func(t *testing.T) {
			fixture := newWorkspaceExternalFixture(t, mode, "mixed")
			first := readWorkspaceExternal(t, fixture, func(ctx context.Context) (types.GitStatusUpdate, error) {
				return fixture.tracker.GetGitStatusWithDetails(ctx, true)
			})
			assertWorkspaceExternalStatus(t, fixture, first, "mixed")
			cached := readWorkspaceExternal(t, fixture, func(ctx context.Context) (types.GitStatusUpdate, error) {
				return fixture.tracker.GetGitStatusWithDetails(ctx, false)
			})
			replayed := readWorkspaceExternal(t, fixture, fixture.tracker.GetGitStatusReplay)
			require.Equal(t, first, cached)
			require.Equal(t, first, replayed)
			for path, tag := range fixture.paths {
				writeFile(t, fixture.dir, path, plainPatchContents(tag, "index-stage", "worktree-line")+"dirty-successor"+tag+"\n")
			}
			fresh := readWorkspaceExternal(t, fixture, func(ctx context.Context) (types.GitStatusUpdate, error) {
				return fixture.tracker.GetGitStatusWithDetails(ctx, true)
			})
			assertWorkspaceExternalDirty(t, fixture, first, fresh)
		})
	}
}

func assertWorkspaceExternalDirty(t *testing.T, fixture workspaceExternalFixture, first, fresh types.GitStatusUpdate) {
	t.Helper()
	require.Equal(t, gitStatusDetailReady, fresh.DetailState)
	require.Equal(t, first.TrackerID, fresh.TrackerID)
	require.Greater(t, fresh.SnapshotRevision, first.SnapshotRevision)
	require.Len(t, fresh.Files, len(fixture.paths))
	for path, tag := range fixture.paths {
		file := fresh.Files[path]
		require.Equal(t, path, file.Path)
		require.Equal(t, "modified", file.Status)
		require.Equal(t, 3, file.Additions)
		require.Equal(t, 2, file.Deletions)
		require.Equal(t, gitStatusDiffReady, file.DiffState)
		require.NotNil(t, file.StagedChange)
		require.NotNil(t, file.UnstagedChange)
		require.Equal(t, 2, file.UnstagedChange.Additions)
		require.Equal(t, 1, file.UnstagedChange.Deletions)
		assertPlainPatchText(t, file.StagedChange.Diff, path, tag, "staged")
		assert.Contains(t, file.Diff, "+dirty-successor"+tag+"\n")
		assert.Contains(t, file.UnstagedChange.Diff, "+dirty-successor"+tag+"\n")
		assert.NotEqual(t, first.Files[path].Diff, file.Diff)
		assert.NotContains(t, file.Diff, "CUSTOM DIFF OUTPUT")
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.46
func TestWorkspaceGitExternalHelperBudgets(t *testing.T) {
	for _, mode := range workspaceExternalModes {
		for _, limit := range []string{"per_file", "total_bytes"} {
			t.Run(mode+"/"+limit, func(t *testing.T) {
				fixture := newWorkspaceExternalFixture(t, mode, "staged")
				count, lines := 1, 12000
				if limit == "total_bytes" {
					count, lines = 12, 7600
				}
				for i := 0; i < count; i++ {
					path := fmt.Sprintf("budget-%02d.txt", i)
					fixture.paths[path] = "-budget"
					writeFile(t, fixture.dir, path, strings.Repeat("external budget built-in sentinel\n", lines))
				}
				runGit(t, fixture.dir, "add", "-A")
				status := readWorkspaceExternal(t, fixture, func(ctx context.Context) (types.GitStatusUpdate, error) {
					return fixture.tracker.GetGitStatusWithDetails(ctx, true)
				})
				assertWorkspaceExternalBudget(t, fixture, status, limit)
			})
		}
	}
}

func assertWorkspaceExternalBudget(t *testing.T, fixture workspaceExternalFixture, status types.GitStatusUpdate, limit string) {
	t.Helper()
	require.Equal(t, gitStatusStateReady, status.StatusState)
	require.True(t, status.FilesComplete)
	require.Equal(t, gitStatusDetailReady, status.DetailState)
	require.Len(t, status.Files, len(fixture.paths))
	skipped, total := 0, 0
	for path, file := range status.Files {
		assert.Equal(t, path, file.Path)
		assert.Equal(t, gitStatusDiffReady, file.DiffState)
		assert.LessOrEqual(t, len(file.Diff), maxDiffOutputSize)
		assert.NotContains(t, file.Diff, "CUSTOM DIFF OUTPUT")
		total += len(file.Diff)
		if file.DiffSkipReason == diffSkipReasonBudgetExceeded {
			skipped++
			assert.Empty(t, file.Diff)
		} else if strings.HasPrefix(path, "budget-") {
			assert.True(t, strings.HasPrefix(file.Diff, "diff --git a/"+path+" b/"+path+"\n"))
			assert.Contains(t, file.Diff, "+external budget built-in sentinel\n")
		}
	}
	if limit == "per_file" {
		file := status.Files["budget-00.txt"]
		assert.Equal(t, diffSkipReasonTruncated, file.DiffSkipReason)
		assert.Len(t, file.Diff, maxDiffOutputSize)
		assert.Equal(t, 12000, file.Additions)
		assert.Equal(t, 0, file.Deletions)
		return
	}
	assert.Positive(t, skipped)
	assert.GreaterOrEqual(t, total, maxTotalDiffBytes)
	assert.LessOrEqual(t, total, maxTotalDiffBytes+maxDiffOutputSize)
}
