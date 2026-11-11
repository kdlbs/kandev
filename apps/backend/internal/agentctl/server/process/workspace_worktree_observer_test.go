package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestWorktreeObserverPublishesOnlyInRootLinkedWorktrees(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "primary")
	initGitRepoAt(t, primary)
	linked := filepath.Join(taskRoot, ".claude", "worktrees", "fix-b")
	addObserverWorktree(t, primary, linked, "fix-b")
	outside := filepath.Join(t.TempDir(), "outside")
	addObserverWorktree(t, primary, outside, "outside")
	detached := filepath.Join(taskRoot, ".claude", "worktrees", "detached")
	if err := os.MkdirAll(filepath.Dir(detached), 0o755); err != nil {
		t.Fatalf("mkdir detached parent: %v", err)
	}
	runObserverGit(t, primary, "worktree", "add", "--detach", detached, "main")

	wt := NewWorkspaceTrackerForRepo(primary, "primary", newTestLogger(t))
	wt.SetWorkspaceRoot(taskRoot)
	sub := wt.SubscribeWorkspaceStream()
	defer func() { wt.UnsubscribeWorkspaceStream(sub) }()

	wt.observeWorktreeInventory(context.Background())
	inventory := nextWorktreeInventory(t, sub)
	if inventory.RepositoryName != "primary" {
		t.Fatalf("repository name = %q", inventory.RepositoryName)
	}
	if len(inventory.Entries) != 1 {
		t.Fatalf("entries = %+v, want only the in-root named worktree", inventory.Entries)
	}
	entry := inventory.Entries[0]
	if entry.Path != linked || entry.Branch != "fix-b" || entry.HeadSHA == "" {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	// A second observation with no change must not re-publish.
	wt.observeWorktreeInventory(context.Background())
	select {
	case message := <-sub:
		t.Fatalf("unchanged inventory re-published: %+v", message)
	default:
	}

	// Checking out a branch inside the detached worktree is caught on the
	// periodic full-list tick.
	runObserverGit(t, detached, "checkout", "-b", "detached-fixed")
	wt.worktreeObserver.ticks = worktreeFullListEveryNTicks - 1
	wt.observeWorktreeInventory(context.Background())
	inventory = nextWorktreeInventory(t, sub)
	if len(inventory.Entries) != 2 {
		t.Fatalf("entries after branch checkout = %+v, want two", inventory.Entries)
	}
}

func TestWorktreeObserverIgnoresNonRepositoryTracker(t *testing.T) {
	taskRoot := t.TempDir()
	wt := NewWorkspaceTrackerForRepo(taskRoot, "", newTestLogger(t))
	wt.SetWorkspaceRoot(taskRoot)
	sub := wt.SubscribeWorkspaceStream()
	defer func() { wt.UnsubscribeWorkspaceStream(sub) }()
	wt.observeWorktreeInventory(context.Background())
	select {
	case message := <-sub:
		t.Fatalf("non-repository tracker published inventory: %+v", message)
	default:
	}
}

func addObserverWorktree(t *testing.T, primary, path, branch string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir worktree parent: %v", err)
	}
	runObserverGit(t, primary, "worktree", "add", "-b", branch, path, "main")
}

func runObserverGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func nextWorktreeInventory(t *testing.T, sub types.WorkspaceStreamSubscriber) *types.WorktreeInventory {
	t.Helper()
	for {
		select {
		case message, ok := <-sub:
			if !ok {
				t.Fatal("workspace stream closed")
			}
			if message.Type == types.WorkspaceMessageTypeWorktreeInventory && message.WorktreeInventory != nil {
				return message.WorktreeInventory
			}
		default:
			t.Fatal("expected a worktree inventory message")
		}
	}
}

func TestWorktreeObserverReplaysLatestInventoryToNewSubscriber(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "primary")
	initGitRepoAt(t, primary)
	linked := filepath.Join(taskRoot, ".claude", "worktrees", "fix-b")
	addObserverWorktree(t, primary, linked, "fix-b")

	wt := NewWorkspaceTrackerForRepo(primary, "primary", newTestLogger(t))
	wt.SetWorkspaceRoot(taskRoot)
	wt.observeWorktreeInventory(context.Background())

	sub := wt.SubscribeWorkspaceStream()
	defer func() { wt.UnsubscribeWorkspaceStream(sub) }()
	inventory := nextWorktreeInventory(t, sub)
	if len(inventory.Entries) != 1 || inventory.Entries[0].Path != linked {
		t.Fatalf("replayed inventory = %+v", inventory)
	}
}
