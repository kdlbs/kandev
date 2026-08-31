package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCleanupWorktrees_RecoversAfterPathOnlyRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-partial", "session-partial", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-partial", "session-partial")

	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree path without shared Git metadata: %v", err)
	}
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("recover partial cleanup: %v", err)
	}

	assertNoCleanupRegistration(t, wt.RepositoryPath, wt.Path)
	assertCleanupBranchAbsent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_PartialFailureRemainsRetryable(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-retry", "session-retry", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-retry", "session-retry")

	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove worktree path without shared Git metadata: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.CleanupWorktrees(cancelled, []*Worktree{wt}); err == nil {
		t.Fatal("cleanup with unavailable Git metadata writer returned nil")
	}
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("retry partial cleanup: %v", err)
	}
	assertNoCleanupRegistration(t, wt.RepositoryPath, wt.Path)
	assertCleanupBranchAbsent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_IsIdempotentAfterVerifiedRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-idempotent", "session-idempotent", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-idempotent", "session-idempotent")

	for attempt := 1; attempt <= 2; attempt++ {
		if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
			t.Fatalf("cleanup attempt %d: %v", attempt, err)
		}
	}
	assertNoCleanupRegistration(t, wt.RepositoryPath, wt.Path)
	assertCleanupBranchAbsent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
}

func TestCleanupWorktrees_RefusesUniqueWork(t *testing.T) {
	tests := []struct {
		name string
		add  func(*testing.T, *Worktree)
	}{
		{
			name: "unmerged commit",
			add: func(t *testing.T, wt *Worktree) {
				t.Helper()
				runGit(t, wt.Path, "commit", "--allow-empty", "-m", "unique local work")
			},
		},
		{
			name: "untracked file",
			add: func(t *testing.T, wt *Worktree) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(wt.Path, "unique.txt"), []byte("keep me\n"), 0o644); err != nil {
					t.Fatalf("write unique file: %v", err)
				}
			},
		},
		{
			name: "tracked modification",
			add: func(t *testing.T, wt *Worktree) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(wt.Path, "README.md"), []byte("keep this edit\n"), 0o644); err != nil {
					t.Fatalf("modify tracked file: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr, store := newReferenceCleanupTestManager(t)
			taskID := "task-unique-" + strings.ReplaceAll(test.name, " ", "-")
			sessionID := "session-unique-" + strings.ReplaceAll(test.name, " ", "-")
			seedReferenceCleanupSession(t, store, taskID, sessionID, models.TaskSessionStateCompleted)
			wt := createReferenceCleanupWorktree(t, mgr, taskID, sessionID)
			test.add(t, wt)

			err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt})
			assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
			if test.name == "unmerged commit" {
				if err != nil {
					t.Fatalf("cleanup preserving unique commit: %v", err)
				}
				if _, statErr := os.Lstat(wt.Path); !os.IsNotExist(statErr) {
					t.Fatalf("clean worktree path remains after branch preservation: %v", statErr)
				}
				assertWorktreeReferenceStatus(t, store, wt.ID, StatusDeleted)
				return
			}
			if err == nil {
				t.Fatal("cleanup of uncommitted work returned nil")
			}
			if _, statErr := os.Lstat(wt.Path); statErr != nil {
				t.Fatalf("worktree containing untracked work was removed: %v", statErr)
			}
			assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
		})
	}
}

func TestCleanupWorktrees_PreservesUnrelatedWorktreeAndBranch(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-target", "session-target", models.TaskSessionStateCompleted)
	target := createReferenceCleanupWorktree(t, mgr, "task-target", "session-target")

	unrelatedPath := filepath.Join(t.TempDir(), "unrelated-worktree")
	runGit(t, target.RepositoryPath, "worktree", "add", "-b", "feature/unrelated", unrelatedPath, "main")
	runGit(t, unrelatedPath, "commit", "--allow-empty", "-m", "unrelated unique work")
	unrelatedHead := strings.TrimSpace(runGit(t, unrelatedPath, "rev-parse", "HEAD"))

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{target}); err != nil {
		t.Fatalf("cleanup target worktree: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, unrelatedPath, "rev-parse", "HEAD")); got != unrelatedHead {
		t.Fatalf("unrelated worktree HEAD = %q, want %q", got, unrelatedHead)
	}
	assertCleanupBranchPresent(t, target.RepositoryPath, "feature/unrelated")
	assertWorktreeReferenceStatus(t, store, target.ID, StatusDeleted)
}

func TestCleanupWorktrees_RefusesUnrelatedReplacementAtRecordedPath(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-replaced", "session-replaced", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-replaced", "session-replaced")

	runGit(t, wt.RepositoryPath, "worktree", "remove", "--force", wt.Path)
	if err := os.MkdirAll(wt.Path, 0o755); err != nil {
		t.Fatalf("create replacement directory: %v", err)
	}
	sentinel := filepath.Join(wt.Path, "unrelated.txt")
	if err := os.WriteFile(sentinel, []byte("unrelated\n"), 0o644); err != nil {
		t.Fatalf("write replacement sentinel: %v", err)
	}

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err == nil {
		t.Fatal("cleanup of unrelated replacement returned nil")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "unrelated\n" {
		t.Fatalf("replacement sentinel changed: contents=%q err=%v", got, err)
	}
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
	assertWorktreeReferenceStatus(t, store, wt.ID, StatusActive)
}

func assertNoCleanupRegistration(t *testing.T, repoPath, worktreePath string) {
	t.Helper()
	out := runGit(t, repoPath, "worktree", "list", "--porcelain")
	if strings.Contains(out, worktreePath) {
		t.Fatalf("worktree registration remains for %q:\n%s", worktreePath, out)
	}
}

func assertCleanupBranchPresent(t *testing.T, repoPath, branch string) {
	t.Helper()
	if got := strings.TrimSpace(runGit(t, repoPath, "branch", "--list", branch)); got == "" {
		t.Fatalf("branch %q was deleted", branch)
	}
}

func assertCleanupBranchAbsent(t *testing.T, repoPath, branch string) {
	t.Helper()
	if got := strings.TrimSpace(runGit(t, repoPath, "branch", "--list", branch)); got != "" {
		t.Fatalf("branch %q remains: %q", branch, got)
	}
}
