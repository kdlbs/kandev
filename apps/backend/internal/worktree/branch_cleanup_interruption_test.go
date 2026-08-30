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

type cancelAfterArchivedRecoveryStore struct {
	*SQLiteStore
	cancel context.CancelFunc
	once   sync.Once
}

func (s *cancelAfterArchivedRecoveryStore) PersistArchivedBranchRecoveryHead(
	ctx context.Context, worktreeID, expected, recoveryHead string,
) (bool, error) {
	persisted, err := s.SQLiteStore.PersistArchivedBranchRecoveryHead(
		ctx, worktreeID, expected, recoveryHead,
	)
	if err == nil && persisted {
		s.once.Do(s.cancel)
	}
	return persisted, err
}

func TestMaintainArchivedBranches_RetriesAfterRecoveryPersistInterruption(t *testing.T) {
	mgr, store, wt, wantHead := archivedIntegratedBranchForMaintenance(t, "interrupted")

	interruptedCtx, cancel := context.WithCancel(context.Background())
	mgr.store = &cancelAfterArchivedRecoveryStore{SQLiteStore: store, cancel: cancel}
	firstReceipt, firstErr := mgr.MaintainArchivedBranches(interruptedCtx, 1)
	if firstErr == nil {
		t.Fatal("interrupted maintenance error = nil, want context cancellation")
	}
	if firstReceipt.Deleted != 0 {
		t.Fatalf("interrupted maintenance receipt = %+v, want no deletion", firstReceipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", wt.Branch)); got != wantHead {
		t.Fatalf("branch head after interruption = %q, want %q", got, wantHead)
	}

	mgr.store = store
	secondReceipt, err := mgr.MaintainArchivedBranches(context.Background(), 1)
	if err != nil {
		t.Fatalf("retry archived branch maintenance: %v", err)
	}
	if secondReceipt.Attempted != 1 || secondReceipt.Deleted != 1 {
		t.Fatalf("retry receipt = %+v, want one attempted deletion", secondReceipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "branch", "--list", wt.Branch)); got != "" {
		t.Fatalf("integrated archived branch remains after retry: %q", got)
	}
	thirdReceipt, err := mgr.MaintainArchivedBranches(context.Background(), 1)
	if err != nil {
		t.Fatalf("maintenance after completed compaction: %v", err)
	}
	if thirdReceipt.Attempted != 0 {
		t.Fatalf("completed branch was selected again: %+v", thirdReceipt)
	}
}

func TestMaintainArchivedBranches_RestoresRefWhenBranchBecomesLiveDuringDelete(t *testing.T) {
	mgr, _, wt, wantHead := archivedIntegratedBranchForMaintenance(t, "live-race")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	binDir := t.TempDir()
	racePath := filepath.Join(t.TempDir(), "racing-worktree")
	wrapper := filepath.Join(binDir, "git")
	branchRef := "refs/heads/" + wt.Branch
	script := "#!/bin/sh\n" +
		"case \" $* \" in\n" +
		"  *\" update-ref -d " + branchRef + " \"*)\n" +
		"    \"" + realGit + "\" -C \"" + wt.RepositoryPath + "\" worktree add \"" + racePath + "\" \"" + wt.Branch + "\" >/dev/null\n" +
		"    ;;\n" +
		"esac\n" +
		"exec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command(realGit, "-C", wt.RepositoryPath, "worktree", "remove", "--force", racePath).Run()
	})
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	receipt, err := mgr.MaintainArchivedBranches(context.Background(), 1)
	if err != nil {
		t.Fatalf("maintenance with liveness race: %v", err)
	}
	if receipt.Deleted != 0 || receipt.RetainedReasons[RetainedLiveWorktree] != 1 {
		t.Fatalf("liveness-race receipt = %+v, want live-worktree retention", receipt)
	}
	if got := strings.TrimSpace(runGit(t, wt.RepositoryPath, "rev-parse", branchRef)); got != wantHead {
		t.Fatalf("branch head after liveness race = %q, want restored %q", got, wantHead)
	}
}

func archivedIntegratedBranchForMaintenance(
	t *testing.T, suffix string,
) (*Manager, *SQLiteStore, *Worktree, string) {
	t.Helper()
	mgr, store := newReferenceCleanupTestManager(t)
	ctx := context.Background()
	taskID := "task-maintenance-" + suffix
	sessionID := "session-maintenance-" + suffix
	seedReferenceCleanupSession(t, store, taskID, sessionID, models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, taskID, sessionID)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO repositories (id, workspace_id, name, local_path, created_at, updated_at)
		VALUES (?, 'workspace', 'repository', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, wt.RepositoryID, wt.RepositoryPath); err != nil {
		t.Fatalf("persist repository path: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE task_sessions SET base_branch = ? WHERE id = ?`, wt.BaseBranch, wt.SessionID); err != nil {
		t.Fatalf("persist session base branch: %v", err)
	}
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "archive before integration")
	wantHead := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD"))
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, wt.TaskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	archiveReceipt, err := mgr.CleanupWorktreesWithReceipt(ctx, []*Worktree{wt})
	if err != nil {
		t.Fatalf("archive cleanup: %v", err)
	}
	if archiveReceipt.RetainedReasons[RetainedNotIntegrated] != 1 {
		t.Fatalf("archive receipt = %+v, want not-integrated retention", archiveReceipt)
	}
	runGit(t, wt.RepositoryPath, "update-ref", "refs/heads/main", wantHead)
	return mgr, store, wt, wantHead
}
