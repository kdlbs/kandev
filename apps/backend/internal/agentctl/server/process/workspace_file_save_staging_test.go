package process

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

type pendingStageResult struct {
	result *GitOperationResult
	err    error
}

func newSaveStagingFixture(t *testing.T, large bool) (string, *WorkspaceTracker, fileSaveCase) {
	t.Helper()
	dir, tracker := newFileSaveFixture(t)
	save := distinctFileSaves()[0]
	if large {
		line := strings.Repeat("saved ü content ", 16384)
		save.desired = strings.Replace(save.desired, "alpha new", line, 1)
		save.patch = strings.Replace(save.patch, "alpha new", line, 1)
		for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
			t.Setenv(key, dir)
		}
	}
	writeFile(t, dir, ".kandev-patch-user", "user-owned patch-like file\n")
	writeFile(t, dir, "addition.txt", "ordinary addition\n")
	if err := os.Remove(filepath.Join(dir, "beta.txt")); err != nil {
		t.Fatal(err)
	}
	return dir, tracker, save
}

func beginSaveStaging(ctx context.Context, tracker *WorkspaceTracker, save fileSaveCase, desired *string, pending *sync.WaitGroup) <-chan fileSaveResult {
	result := make(chan fileSaveResult, 1)
	pending.Add(1)
	go func() {
		defer pending.Done()
		hash, resolution, err := tracker.ApplyFileDiff(ctx, save.path, save.path, save.patch,
			fmt.Sprintf("%x", sha256.Sum256([]byte(save.original))), desired)
		result <- fileSaveResult{hash, resolution, err}
	}()
	return result
}

func beginStageAll(ctx context.Context, operator *GitOperator, pending *sync.WaitGroup) <-chan pendingStageResult {
	result := make(chan pendingStageResult, 1)
	pending.Add(1)
	go func() {
		defer pending.Done()
		staged, err := operator.Stage(ctx, nil)
		result <- pendingStageResult{staged, err}
	}()
	return result
}

func awaitStageAll(t *testing.T, ctx context.Context, pending <-chan pendingStageResult) {
	t.Helper()
	select {
	case got := <-pending:
		if got.err != nil || got.result == nil || !got.result.Success {
			t.Errorf("Stage All = %+v, %v; want success", got.result, got.err)
		}
	case <-ctx.Done():
		t.Fatalf("Stage All did not settle: %v", ctx.Err())
	}
}

func assertSaveStagingIndex(t *testing.T, dir, indexedAlpha string) {
	t.Helper()
	want := map[string]string{
		".kandev-patch-user": "user-owned patch-like file\n",
		"README.md":          "# Test Repo",
		"addition.txt":       "ordinary addition\n",
		"alpha.txt":          indexedAlpha,
		"neighbor.txt":       "neighbor stays intact\n",
	}
	paths := strings.Split(strings.TrimSuffix(runGit(t, dir, "ls-files", "-z"), "\x00"), "\x00")
	var wantPaths []string
	for path, content := range want {
		wantPaths = append(wantPaths, path)
		if got := runGit(t, dir, "show", ":"+path); got != content {
			t.Errorf("index blob %s differs: got %d bytes, want %d", path, len(got), len(content))
		}
	}
	slices.Sort(wantPaths)
	slices.Sort(paths)
	if !slices.Equal(paths, wantPaths) {
		t.Errorf("real index paths = %q; want exactly %q", paths, wantPaths)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var diskPaths []string
	for _, entry := range entries {
		if entry.Name() != ".git" {
			diskPaths = append(diskPaths, entry.Name())
		}
	}
	if !slices.Equal(diskPaths, wantPaths) {
		t.Errorf("working tree paths = %q; want exactly %q", diskPaths, wantPaths)
	}
	assertFileSaveBytes(t, dir, "neighbor.txt", want["neighbor.txt"])
	assertFileSaveBytes(t, dir, ".kandev-patch-user", want[".kandev-patch-user"])
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.5
func TestApplyFileDiff_StageAllOverlap(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(fmt.Sprintf("large_patch_and_worktree_temp=%t", large), func(t *testing.T) {
			dir, tracker, save := newSaveStagingFixture(t, large)
			ctx, release, pending := holdFileSaveAdmission(t)
			stage := beginStageAll(ctx, NewGitOperator(dir, newTestLogger(t), nil), pending)
			awaitQueuedFileSaves(t, ctx, 1)
			desired := &save.desired
			if large {
				desired = nil
			}
			saved := beginSaveStaging(ctx, tracker, save, desired, pending)
			awaitQueuedFileSaves(t, ctx, 2)
			release()
			awaitStageAll(t, ctx, stage)
			got := awaitFileSaveResult(t, ctx, saved)
			pending.Wait()
			assertSavedFile(t, dir, save, got)
			assertSaveStagingIndex(t, dir, save.original)
		})
	}
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.5
func TestApplyFileDiff_StageAllSequentialControl(t *testing.T) {
	dir, tracker, save := newSaveStagingFixture(t, false)
	hash, resolution, err := tracker.ApplyFileDiff(t.Context(), save.path, save.path, save.patch,
		fmt.Sprintf("%x", sha256.Sum256([]byte(save.original))), &save.desired)
	assertSavedFile(t, dir, save, fileSaveResult{hash, resolution, err})
	stage, err := NewGitOperator(dir, newTestLogger(t), nil).Stage(t.Context(), nil)
	if err != nil || stage == nil || !stage.Success {
		t.Fatalf("Stage All = %+v, %v; want success", stage, err)
	}
	assertSaveStagingIndex(t, dir, save.desired)
}
