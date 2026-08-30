package worktree

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCleanupWorktrees_PreservesBranchWhenRequested(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-archive", "session-archive", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-archive", "session-archive")
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "local-only archive work")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("branch-preserving cleanup: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree path remains after cleanup, stat error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", wt.Branch)); got != wantHead {
		t.Fatalf("preserved branch head = %q, want %q", got, wantHead)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktreesPreservingBranches_RetainsLegacyUnknownOwner(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-legacy", "session-legacy", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-legacy", "session-legacy")
	wt.BranchOwner = BranchOwnerUnknown
	if err := store.UpdateWorktree(context.Background(), wt); err != nil {
		t.Fatalf("mark legacy metadata: %v", err)
	}

	receipt, err := mgr.CleanupWorktreesWithReceipt(context.Background(), []*Worktree{wt})
	if err != nil {
		t.Fatalf("legacy cleanup: %v", err)
	}
	if receipt.Deleted != 0 || receipt.RetainedReasons[RetainedUnknownOwner] != 1 {
		t.Fatalf("legacy receipt = %+v", receipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got == "" {
		t.Fatal("legacy branch was deleted")
	}
}

func TestCleanupWorktreesPreservingBranches_RetainsExternalBranch(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-external", "session-external", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-external", "session-external")
	wt.BranchOwner = BranchOwnerExternal
	if err := store.UpdateWorktree(context.Background(), wt); err != nil {
		t.Fatalf("mark external metadata: %v", err)
	}

	receipt, err := mgr.CleanupWorktreesWithReceipt(context.Background(), []*Worktree{wt})
	if err != nil {
		t.Fatalf("external cleanup: %v", err)
	}
	if receipt.RetainedReasons[RetainedExternalOwner] != 1 {
		t.Fatalf("external receipt = %+v", receipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got == "" {
		t.Fatal("external branch was deleted")
	}
}

func TestProtectedBranchNameRecognizesEquivalentRefForms(t *testing.T) {
	for _, protectedRef := range []string{"main", "origin/main", "refs/heads/main", "refs/remotes/origin/main"} {
		if !protectedBranchName("main", protectedRef) {
			t.Errorf("main was not protected by %q", protectedRef)
		}
	}
	if protectedBranchName("feature/task", "origin/main") {
		t.Fatal("feature branch was treated as the protected main branch")
	}
}

func TestCleanupWorktreesPreservingBranches_RetainsAmbiguousOwner(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-ambiguous", "session-ambiguous", models.TaskSessionStateCompleted)
	seedReferenceCleanupSession(t, store, "task-ambiguous-peer", "session-ambiguous-peer", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-ambiguous", "session-ambiguous")
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug, worktree_id,
			worktree_path, worktree_branch, worktree_branch_owner,
			worktree_integration_ref, position, status, created_at, updated_at
		) VALUES (
			'ambiguous-peer', 'env-ambiguous-peer', 'repository', 'peer', 'ambiguous-peer',
			'/missing', ?, 'kandev', 'main', 0, 'deleted', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`, wt.Branch); err != nil {
		t.Fatalf("insert ambiguous owner: %v", err)
	}

	receipt, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt})
	if err != nil {
		t.Fatalf("ambiguous cleanup: %v", err)
	}
	if receipt.RetainedReasons[RetainedAmbiguousOwner] != 1 {
		t.Fatalf("ambiguous receipt = %+v", receipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got == "" {
		t.Fatal("ambiguous branch was deleted")
	}
}

func TestCleanupWorktreesPreservingBranches_DeletesOnlyLocalRef(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-remote", "session-remote", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-remote", "session-remote")
	head := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	remoteRef := "refs/remotes/origin/" + wt.Branch
	runGit(t, wt.RepositoryPath, "update-ref", remoteRef, head)

	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("remote ref cleanup: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", "--verify", remoteRef)); got != head {
		t.Fatalf("remote ref head = %q, want %q", got, head)
	}
}

func TestCleanupWorktreesPreservingBranches_ConcurrentCleanupDeletesOnce(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-race", "session-race", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-race", "session-race")

	var wg sync.WaitGroup
	receipts := make(chan BranchCleanupReceipt, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := mgr.CleanupWorktreesWithReceipt(context.Background(), []*Worktree{wt})
			receipts <- receipt
			errs <- err
		}()
	}
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent cleanup: %v", err)
		}
	}
	deleted := 0
	for receipt := range receipts {
		deleted += receipt.Deleted
	}
	if deleted != 1 {
		t.Fatalf("concurrent deleted count = %d, want 1", deleted)
	}
}

func TestCleanupWorktreesWithReceipt_DeduplicatesWorktreeIDs(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-dedup", "session-dedup", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-dedup", "session-dedup")

	receipt, err := mgr.CleanupWorktreesWithReceipt(context.Background(), []*Worktree{wt, wt})
	if err != nil {
		t.Fatalf("deduplicated cleanup: %v", err)
	}
	if receipt.Attempted != 1 || receipt.Deleted != 1 {
		t.Fatalf("deduplicated receipt = %+v, want one attempted deletion", receipt)
	}
}

func TestCleanupWorktreesPreservingBranches_RemovesFullyMergedManagedBranch(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-merged", "session-merged", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-merged", "session-merged")

	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("merged branch cleanup: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree path remains after cleanup, stat error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got != "" {
		t.Fatalf("fully merged managed branch remains after cleanup: %q", got)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestRemoveByID_RemovesFullyMergedManagedBranch(t *testing.T) {
	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-direct",
		SessionID:      "session-direct",
		TaskTitle:      "Direct cleanup",
		RepositoryID:   "repository",
		RepositoryPath: initGitRepoWithRemote(t),
		BaseBranch:     "main",
		IntegrationRef: "main",
		TaskDirName:    "task-direct",
		RepoName:       "repository",
	})
	if err != nil {
		t.Fatalf("create direct worktree: %v", err)
	}

	if err := mgr.RemoveByID(context.Background(), wt.ID, false); err != nil {
		t.Fatalf("RemoveByID: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got != "" {
		t.Fatalf("fully merged direct branch remains: %q", got)
	}
}

func TestCleanupWorktrees_RemovesBranchByDefault(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-delete", "session-delete", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-delete", "session-delete")

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got != "" {
		t.Fatalf("default cleanup preserved branch %q", got)
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}
