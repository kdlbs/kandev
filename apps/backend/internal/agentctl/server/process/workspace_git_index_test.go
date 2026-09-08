package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotGitIndexCleansUp(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(wt.Stop)

	snapshotPath, snapshotCleanup, err := snapshotGitIndex(context.Background(), wt.gitIndexPath)
	if err != nil {
		t.Fatalf("snapshotGitIndex() error = %v", err)
	}
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot index is not readable before cleanup: %v", err)
	}

	snapshotCleanup()
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("snapshot index after cleanup error = %v, want not-exist", err)
	}
}

func TestSnapshotGitIndexCancellationCleansUp(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(wt.Stop)

	baseCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx := &cancelAfterErrChecksContext{Context: baseCtx, remaining: 2, cancel: cancel}

	_, snapshotCleanup, err := snapshotGitIndex(ctx, wt.gitIndexPath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshotGitIndex() error = %v, want %v", err, context.Canceled)
	}
	if snapshotCleanup != nil {
		t.Fatal("snapshotGitIndex() returned cleanup after cancellation")
	}

	assertNoIndexSnapshots(t, filepath.Dir(wt.gitIndexPath))
}

func TestSnapshotGitIndexErrorCleansUp(t *testing.T) {
	dir := t.TempDir()
	missingIndex := filepath.Join(dir, "missing-index")

	_, snapshotCleanup, err := snapshotGitIndex(context.Background(), missingIndex)
	if err == nil {
		t.Fatal("snapshotGitIndex() returned nil error for a missing index")
	}
	if snapshotCleanup != nil {
		t.Fatal("snapshotGitIndex() returned cleanup after an error")
	}

	assertNoIndexSnapshots(t, dir)
}

func TestSnapshotGitIndexSupportsLinkedWorktree(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	worktreeDir := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoDir, "worktree", "add", "-b", "snapshot-worktree", worktreeDir)

	gitEntry, err := os.Stat(filepath.Join(worktreeDir, ".git"))
	if err != nil {
		t.Fatalf("linked worktree .git entry: %v", err)
	}
	if !gitEntry.Mode().IsRegular() {
		t.Fatalf("linked worktree .git entry mode = %v, want regular file", gitEntry.Mode())
	}

	wt := NewWorkspaceTracker(worktreeDir, newTestLogger(t))
	t.Cleanup(wt.Stop)
	if wt.gitIndexPath == "" {
		t.Fatal("linked worktree did not resolve a git index path")
	}

	snapshotPath, snapshotCleanup, err := snapshotGitIndex(context.Background(), wt.gitIndexPath)
	if err != nil {
		t.Fatalf("snapshotGitIndex() for linked worktree error = %v", err)
	}
	t.Cleanup(snapshotCleanup)
	if filepath.IsAbs(snapshotPath) && strings.HasPrefix(snapshotPath, worktreeDir+string(filepath.Separator)) {
		t.Fatalf("snapshot index unexpectedly lives in the worktree: %q", snapshotPath)
	}
}

func assertNoIndexSnapshots(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".kandev-index-snapshot-*"))
	if err != nil {
		t.Fatalf("find index snapshots: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary index snapshots remain: %v", matches)
	}
}
