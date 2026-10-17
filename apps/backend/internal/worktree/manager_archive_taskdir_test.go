package worktree

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func archiveTaskDirectoryFixture(t *testing.T) (*Manager, *Worktree, *TaskDirectoryCleanup) {
	t.Helper()
	cfg := newTestConfig(t)
	manager, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	wt, err := manager.Create(context.Background(), CreateRequest{
		TaskID: "task-directory", SessionID: "session-directory", TaskTitle: "Directory cleanup",
		RepositoryID: "repo-directory", RepositoryPath: initGitRepoWithRemote(t), BaseBranch: "main", TaskDirName: "task-directory", RepoName: "repo-directory",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wt.Path)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("agent output"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := manager.CaptureTaskDirectoryCleanup(context.Background(), wt)
	if err != nil || target == nil {
		t.Fatalf("capture: %+v %v", target, err)
	}
	if err := manager.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatal(err)
	}
	return manager, wt, target
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.1
// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.3
func TestCleanupTaskDirectoryIdentityAndNoFollow(t *testing.T) {
	for _, scenario := range []string{"recursive", "copied-marker-replacement", "foreign-marker", "root-symlink", "parent-symlink", "missing-store", "base-target", "outside-target", "cancelled", "missing-marker-retry"} {
		t.Run(scenario, func(t *testing.T) {
			manager, _, target := archiveTaskDirectoryFixture(t)
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "keep.txt")
			if err := os.WriteFile(sentinel, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(target.Path, storageworkspaces.OwnershipMarkerFilename)
			marker, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch scenario {
			case "recursive":
				if err := os.Symlink(outside, filepath.Join(target.Path, "link")); err != nil {
					t.Fatal(err)
				}
			case "copied-marker-replacement", "root-symlink":
				replaceArchiveTaskDirectory(t, target, marker, outside, scenario)

			case "parent-symlink":
				replaceArchiveTaskParent(t, target, marker, outside)
			case "foreign-marker":
				owner := target.Owner
				owner.TaskID = "different-task"
				changed, err := json.Marshal(owner)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(markerPath, changed, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing-store":
				manager.store = nil
			case "base-target":
				target.Path = filepath.Dir(target.Path)
			case "outside-target":
				target.Path = outside
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "missing-marker-retry":
				if err := os.Remove(markerPath); err != nil {
					t.Fatal(err)
				}
			}
			removed, err := manager.CleanupTaskDirectory(ctx, target)
			success := scenario == "recursive" || scenario == "missing-marker-retry"
			assertArchiveTaskDirectoryResult(t, manager, ctx, target, removed, err, success)

			if got, err := os.ReadFile(sentinel); err != nil || string(got) != "outside" {
				t.Fatalf("symlink target altered: %q %v", got, err)
			}
		})
	}
}

func replaceArchiveTaskDirectory(t *testing.T, target *TaskDirectoryCleanup, marker []byte, outside, scenario string) {
	t.Helper()
	if err := os.Rename(target.Path, target.Path+"-original"); err != nil {
		t.Fatal(err)
	}
	if scenario == "root-symlink" {
		if err := os.Symlink(outside, target.Path); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.Mkdir(target.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target.Path, storageworkspaces.OwnershipMarkerFilename), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "replacement.txt"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertArchiveTaskDirectoryResult(t *testing.T, manager *Manager, ctx context.Context, target *TaskDirectoryCleanup, removed bool, err error, success bool) {
	t.Helper()
	if !success {
		if err == nil || removed {
			t.Fatalf("unsafe root removal: %v %v", removed, err)
		}
		if _, err := os.Lstat(target.Path); err != nil {
			t.Fatalf("protected path removed: %v", err)
		}
		return
	}
	if err != nil || !removed {
		t.Fatalf("cleanup: %v %v", removed, err)
	}
	if _, err := os.Lstat(target.Path); !os.IsNotExist(err) {
		t.Fatalf("root remains: %v", err)
	}
	if removed, err := manager.CleanupTaskDirectory(ctx, target); err != nil || !removed {
		t.Fatalf("idempotent cleanup: %v %v", removed, err)
	}
}

func replaceArchiveTaskParent(t *testing.T, target *TaskDirectoryCleanup, marker []byte, outside string) {
	t.Helper()
	base := filepath.Dir(target.Path)
	if err := os.Rename(base, base+"-original"); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(outside, filepath.Base(target.Path))
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, storageworkspaces.OwnershipMarkerFilename), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, base); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(base); err != nil {
			t.Error(err)
		}
		if err := os.Rename(base+"-original", base); err != nil {
			t.Error(err)
		}
	})
}

func TestCleanupTaskDirectoryRejectsInvalidInventoryPathBeforeLock(t *testing.T) {
	manager, _, target := archiveTaskDirectoryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.store.CreateWorktree(ctx, &Worktree{ID: "invalid-root-row", TaskID: target.TaskID, Path: target.Path, Status: StatusActive}); err != nil {
		t.Fatal(err)
	}
	removed, err := manager.CleanupTaskDirectory(ctx, target)
	if removed || err == nil || !strings.Contains(err.Error(), "inventory path") {
		t.Fatalf("invalid inventory must be rejected before locking: %v %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(target.Path, "notes.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveTaskDirectoryAfterOwnershipTransfer(t *testing.T) {
	ctx := context.Background()
	manager, wt, _ := archiveTaskDirectoryFixture(t)
	originalOwner := wt.TaskID
	var err error
	wt, err = manager.store.GetWorktreeByID(ctx, wt.ID)
	if err != nil {
		t.Fatal(err)
	}
	wt.TaskID = "successor-task"
	wt.TaskDirName = originalOwner
	if err := manager.store.UpdateWorktree(ctx, wt); err != nil {
		t.Fatal(err)
	}
	target, err := manager.CaptureTaskDirectoryCleanup(ctx, wt)
	if err != nil || target == nil {
		t.Fatalf("capture current owner with unchanged marker: %+v %v", target, err)
	}
	if removed, err := manager.CleanupTaskDirectory(ctx, target); err != nil || !removed {
		t.Fatalf("cleanup transferred task root: %v %v", removed, err)
	}
}
