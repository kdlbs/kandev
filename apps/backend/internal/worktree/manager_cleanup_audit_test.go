package worktree

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestClassifyWorktreeRegistrationOwnership_AllowsDetachedPathIdentity(t *testing.T) {
	registrations := []worktreeRegistration{{
		path:   "/tasks/task/repo",
		head:   "abc123",
		branch: "refs/heads/feature/task",
	}}

	got, err := classifyWorktreeRegistrationOwnership(
		registrations, "/tasks/task/repo", "", "abc123",
		worktreeRegistrationOwnershipOptions{allowAnyBranchAtPath: true},
	)
	if err != nil {
		t.Fatalf("classify detached registration: %v", err)
	}
	if got != worktreeRegistrationOwned {
		t.Fatalf("classification = %v, want owned", got)
	}
}

func TestVerifyCleanupBranchRedundant_DoesNotUseDetachedRepositoryHEAD(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-detached-base", "session-detached-base", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-detached-base", "session-detached-base")

	// A detached repository HEAD must not be treated as a containing base. The
	// branch has no explicit BaseBranch, so cleanup must fail closed when the
	// only candidate is the repository's detached HEAD.
	wt.BaseBranch = ""
	runGit(t, wt.Path, "commit", "--allow-empty", "-m", "unique detached branch")
	runGit(t, wt.RepositoryPath, "checkout", "--detach", wt.Branch)

	delete, err := mgr.verifyCleanupBranchRedundant(context.Background(), wt, "refs/heads/"+wt.Branch)
	if err == nil && delete {
		t.Fatal("cleanup accepted detached repository HEAD as a containing base")
	}
}
