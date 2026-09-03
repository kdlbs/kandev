package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// blockingReferenceStore pauses cleanup after removeWorktree has copied the
// inventory record. This is reviewer-requested contract coverage for the
// snapshot boundary.
type blockingReferenceStore struct {
	*SQLiteStore
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func (s *blockingReferenceStore) CountActiveWorktreeReferences(
	ctx context.Context,
	_ string,
	_ []string,
) (int, error) {
	close(s.entered)
	select {
	case <-s.release:
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *blockingReferenceStore) unblock() {
	s.releaseOnce.Do(func() { close(s.release) })
}

// postRemovalSwapDirectoryHandle moves the audited directory away before the
// second verification. It lets the test prove that parent cleanup is skipped
// when the lexical path no longer identifies the audited directory.
type postRemovalSwapDirectoryHandle struct {
	path            string
	replacementPath string
	verifyCalls     int
}

func (h *postRemovalSwapDirectoryHandle) Close() error { return nil }

func (h *postRemovalSwapDirectoryHandle) VerifyPath(string) error {
	h.verifyCalls++
	if h.verifyCalls == 1 {
		return nil
	}
	return errors.New("pinned directory no longer matches lexical path")
}

func (h *postRemovalSwapDirectoryHandle) IsValidWorktree() bool { return true }

func (h *postRemovalSwapDirectoryHandle) RemoveDirectory(context.Context) error {
	return os.Rename(h.path, h.replacementPath)
}

func (h *postRemovalSwapDirectoryHandle) ReadFile(string) ([]byte, error) {
	return nil, os.ErrNotExist
}

func (h *postRemovalSwapDirectoryHandle) WriteFile(string, []byte, os.FileMode) error {
	return errors.New("fake directory handle does not support writes")
}

// Reviewer-requested contract coverage: the post-removal identity check must
// run before the best-effort parent-directory cleanup.
func TestRemoveWorktreeDir_RevalidatesBeforeParentCleanup(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	tasksBase, err := mgr.config.ExpandedTasksBasePath()
	if err != nil {
		t.Fatalf("expand tasks base path: %v", err)
	}
	taskDir := filepath.Join(tasksBase, "task-post-removal")
	worktreePath := filepath.Join(taskDir, "repository")
	replacementPath := filepath.Join(t.TempDir(), "replacement")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create worktree path: %v", err)
	}

	handle := &postRemovalSwapDirectoryHandle{
		path:            worktreePath,
		replacementPath: replacementPath,
	}
	err = mgr.removeWorktreeDir(
		context.Background(),
		worktreePath,
		filepath.Join(t.TempDir(), "repository.git"),
		handle,
	)
	if err == nil || !strings.Contains(err.Error(), "before parent cleanup") {
		t.Fatalf("removeWorktreeDir error = %v, want post-removal identity failure", err)
	}
	if handle.verifyCalls != 2 {
		t.Fatalf("VerifyPath calls = %d, want 2", handle.verifyCalls)
	}
	if _, err := os.Stat(replacementPath); err != nil {
		t.Fatalf("renamed original worktree missing: %v", err)
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Fatalf("parent task directory was removed after identity failure: %v", err)
	}
}

// Reviewer-requested contract coverage: cleanup must use the copied record
// even when a caller mutates its worktree projection while cleanup waits.
func TestCleanupWorktrees_UsesSnapshotWhenCallerMutatesPath(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-snapshot", "session-snapshot", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-snapshot", "session-snapshot")
	originalPath := wt.Path

	blockingStore := &blockingReferenceStore{
		SQLiteStore: store,
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	mgr.store = blockingStore
	t.Cleanup(blockingStore.unblock)

	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- mgr.CleanupWorktrees(context.Background(), []*Worktree{wt})
	}()
	<-blockingStore.entered

	replacementPath := originalPath + "-caller-replacement"
	if err := os.MkdirAll(replacementPath, 0o755); err != nil {
		blockingStore.unblock()
		t.Fatalf("create caller replacement path: %v", err)
	}
	sentinel := filepath.Join(replacementPath, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("preserve me\n"), 0o644); err != nil {
		blockingStore.unblock()
		t.Fatalf("write caller replacement sentinel: %v", err)
	}
	wt.Path = replacementPath
	blockingStore.unblock()

	if err := <-cleanupDone; err != nil {
		t.Fatalf("cleanup after caller projection mutation: %v", err)
	}
	if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
		t.Fatalf("snapshotted worktree path remains, stat error = %v", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve me\n" {
		t.Fatalf("caller replacement changed: contents=%q err=%v", got, err)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}
