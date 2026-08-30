package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type archivedBranchMaintainer interface {
	MaintainArchivedBranches(context.Context, int) (BranchCleanupReceipt, error)
}

type unarchivingMaintenanceStore struct {
	*SQLiteStore
	taskID string
	checks int
}

func (s *unarchivingMaintenanceStore) IsArchivedBranchCandidate(
	ctx context.Context, worktreeID string,
) (bool, error) {
	s.checks++
	if s.checks == 2 {
		if _, err := s.db.ExecContext(ctx, `UPDATE tasks SET archived_at = NULL WHERE id = ?`, s.taskID); err != nil {
			return false, err
		}
	}
	return s.SQLiteStore.IsArchivedBranchCandidate(ctx, worktreeID)
}

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

func TestCleanupWorktreesPreservingBranches_DeletesExactExpectedLocalRef(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-exact-ref", "session-exact-ref", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-exact-ref", "session-exact-ref")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "git-commands.log")
	wrapper := filepath.Join(binDir, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nexec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := mgr.CleanupWorktreesPreservingBranches(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("exact-ref cleanup: %v", err)
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read git command log: %v", err)
	}
	wantCommand := "update-ref -d refs/heads/" + wt.Branch + " " + wantHead
	if !strings.Contains(string(commands), wantCommand) {
		t.Fatalf("git commands = %q, want atomic exact-ref deletion %q", commands, wantCommand)
	}
	if strings.Contains(string(commands), "branch -d") || strings.Contains(string(commands), "branch -D") {
		t.Fatalf("git commands used branch deletion instead of expected-SHA deletion: %q", commands)
	}
}

func TestCleanupWorktreesPreservingBranches_HeadRaceRemainsRetryable(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-head-race", "session-head-race", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-head-race", "session-head-race")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", wantHead+"^{tree}"))
	racingHead := strings.TrimSpace(runGit(t, wt.Path, "commit-tree", tree, "-p", wantHead, "-m", "racing head"))

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	binDir := t.TempDir()
	wrapper := filepath.Join(binDir, "git")
	branchRef := "refs/heads/" + wt.Branch
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-c\" ] && [ \"$3\" = \"update-ref\" ] && [ \"$4\" = \"-d\" ]; then\n" +
		"  \"" + realGit + "\" -C \"$PWD\" update-ref \"" + branchRef + "\" \"" + racingHead + "\"\n" +
		"fi\n" +
		"exec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	receipt, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt})
	if err != nil {
		t.Fatalf("head-race cleanup: %v", err)
	}
	if receipt.Deleted != 0 || receipt.RetainedReasons[RetainedHeadChanged] != 1 {
		t.Fatalf("head-race receipt = %+v, want head-changed retention", receipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", branchRef)); got != racingHead {
		t.Fatalf("racing branch head = %q, want %q", got, racingHead)
	}
	archived, err := store.GetWorktreeByID(ctx, wt.ID)
	if err != nil {
		t.Fatalf("load raced worktree: %v", err)
	}
	if archived.RecoveryHeadSHA != "" {
		t.Fatalf("raced recovery head = %q, want empty so maintenance can retry", archived.RecoveryHeadSHA)
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

func TestMaintainArchivedBranches_CompactsAfterIntegrationAndRestoresExactHead(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-maintenance", "session-maintenance", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-maintenance", "session-maintenance")
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO repositories (id, workspace_id, name, local_path, created_at, updated_at)
		VALUES (?, 'workspace', 'repository', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, wt.RepositoryID, wt.RepositoryPath); err != nil {
		t.Fatalf("persist repository path: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE task_sessions SET base_branch = ? WHERE id = ?
	`, wt.BaseBranch, wt.SessionID); err != nil {
		t.Fatalf("persist session base branch: %v", err)
	}
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "archive before integration")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))

	if _, err := store.db.ExecContext(ctx, `
		UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?
	`, wt.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	archiveReceipt, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt})
	if err != nil {
		t.Fatalf("archive cleanup: %v", err)
	}
	if archiveReceipt.RetainedReasons[RetainedNotIntegrated] != 1 {
		t.Fatalf("archive receipt = %+v, want not-integrated retention", archiveReceipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", wt.Branch)); got != wantHead {
		t.Fatalf("archive retained branch head = %q, want %q", got, wantHead)
	}

	// Simulate the later local integration that the archive path could not
	// observe. Storage maintenance must revisit the durable archived row rather
	// than scan branch names.
	runGit(t, wt.RepositoryPath, "update-ref", "refs/heads/main", wantHead)
	maintainer, ok := any(mgr).(archivedBranchMaintainer)
	if !ok {
		t.Fatal("worktree manager does not implement bounded archived-branch maintenance")
	}
	maintenanceReceipt, err := maintainer.MaintainArchivedBranches(ctx, 1)
	if err != nil {
		t.Fatalf("archived branch maintenance: %v", err)
	}
	if maintenanceReceipt.Attempted != 1 || maintenanceReceipt.Deleted != 1 {
		t.Fatalf("maintenance receipt = %+v, want one attempted deletion", maintenanceReceipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got != "" {
		t.Fatalf("integrated archived branch remains after maintenance: %q", got)
	}

	if _, err := store.db.ExecContext(ctx, `
		UPDATE tasks SET archived_at = NULL WHERE id = ?
	`, wt.TaskID); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	restored, err := mgr.Create(ctx, CreateRequest{
		TaskID:         wt.TaskID,
		SessionID:      wt.SessionID,
		WorktreeID:     wt.ID,
		TaskTitle:      "Archived maintenance",
		RepositoryID:   wt.RepositoryID,
		RepositoryPath: wt.RepositoryPath,
		BaseBranch:     wt.BaseBranch,
		IntegrationRef: wt.IntegrationRef,
		TaskDirName:    wt.TaskDirName,
		RepoName:       "repository",
	})
	if err != nil {
		t.Fatalf("restore after maintenance: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, restored.Path, "rev-parse", "HEAD")); got != wantHead {
		t.Fatalf("restored HEAD = %q, want exact archived head %q", got, wantHead)
	}
	persistedRestored, err := store.GetWorktreeByID(ctx, restored.ID)
	if err != nil {
		t.Fatalf("load restored worktree: %v", err)
	}
	if persistedRestored.BranchCompactedAt != nil {
		t.Fatalf("restored worktree kept stale compaction marker: %v", persistedRestored.BranchCompactedAt)
	}
}

func TestMaintainArchivedBranches_UnarchiveAfterSelectionRetainsBranch(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	seedReferenceCleanupSession(t, store, "task-unarchive-race", "session-unarchive-race", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-unarchive-race", "session-unarchive-race")
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO repositories (id, workspace_id, name, local_path, created_at, updated_at)
		VALUES (?, 'workspace', 'repository', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, wt.RepositoryID, wt.RepositoryPath); err != nil {
		t.Fatalf("persist repository path: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE task_sessions SET base_branch = ? WHERE id = ?`, wt.BaseBranch, wt.SessionID); err != nil {
		t.Fatalf("persist session base branch: %v", err)
	}
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "archive before unarchive race")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, wt.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt}); err != nil {
		t.Fatalf("archive cleanup: %v", err)
	}
	runGit(t, wt.RepositoryPath, "update-ref", "refs/heads/main", wantHead)

	mgr.store = &unarchivingMaintenanceStore{SQLiteStore: store, taskID: wt.TaskID}
	receipt, err := mgr.MaintainArchivedBranches(ctx, 1)
	if err != nil {
		t.Fatalf("maintenance during unarchive race: %v", err)
	}
	if receipt.Deleted != 0 || receipt.RetainedReasons[RetainedArchiveStateChanged] != 1 {
		t.Fatalf("unarchive-race receipt = %+v, want archive-state retention", receipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", wt.Branch)); got != wantHead {
		t.Fatalf("unarchive race changed branch head = %q, want %q", got, wantHead)
	}
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
