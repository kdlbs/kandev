package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// stubWorktreeRecovery implements WorktreeProvider + BranchStatusProber.
type stubWorktreeRecovery struct {
	worktrees []*worktree.Worktree
	statuses  map[string]string // branch → status
}

func (s *stubWorktreeRecovery) OnTaskDeleted(context.Context, string) error { return nil }
func (s *stubWorktreeRecovery) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return s.worktrees, nil
}
func (s *stubWorktreeRecovery) BranchRecoveryStatus(_ context.Context, _, branch string) string {
	return s.statuses[branch]
}

// stubTaskRepoRepo overrides just the two methods RecoverTaskBranches uses.
type stubTaskRepoRepo struct {
	repository.TaskRepoRepository
	repos   []*models.TaskRepository
	updated []*models.TaskRepository
}

func (s *stubTaskRepoRepo) ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error) {
	return s.repos, nil
}
func (s *stubTaskRepoRepo) UpdateTaskRepository(_ context.Context, tr *models.TaskRepository) error {
	s.updated = append(s.updated, tr)
	return nil
}

func recoveryTestService(t *testing.T, wt *stubWorktreeRecovery, repos *stubTaskRepoRepo) *Service {
	t.Helper()
	svc, _, _ := createTestService(t)
	svc.SetWorktreeCleanup(wt)
	svc.taskRepos = repos
	return svc
}

func TestRecoverTaskBranches_RestoresCheckoutBranchWhenBranchSurvives(t *testing.T) {
	wt := &stubWorktreeRecovery{
		worktrees: []*worktree.Worktree{{
			RepositoryID:   "repo-1",
			RepositoryPath: "/repos/one",
			Branch:         "feature/old-work",
		}},
		statuses: map[string]string{"feature/old-work": worktree.BranchStatusRemote},
	}
	repos := &stubTaskRepoRepo{
		repos: []*models.TaskRepository{{TaskID: "task-1", RepositoryID: "repo-1"}},
	}
	svc := recoveryTestService(t, wt, repos)

	out := svc.RecoverTaskBranches(context.Background(), "task-1")
	if len(out) != 1 {
		t.Fatalf("recovery entries = %d, want 1", len(out))
	}
	if out[0].Status != worktree.BranchStatusRemote || out[0].Branch != "feature/old-work" {
		t.Errorf("recovery = %+v, want remote feature/old-work", out[0])
	}
	if len(repos.updated) != 1 || repos.updated[0].CheckoutBranch != "feature/old-work" {
		t.Fatalf("checkout_branch not restored: updated = %+v", repos.updated)
	}
}

func TestRecoverTaskBranches_MissingBranchReportsWithoutUpdate(t *testing.T) {
	wt := &stubWorktreeRecovery{
		worktrees: []*worktree.Worktree{{
			RepositoryID:   "repo-1",
			RepositoryPath: "/repos/one",
			Branch:         "feature/never-pushed",
		}},
		statuses: map[string]string{"feature/never-pushed": worktree.BranchStatusMissing},
	}
	repos := &stubTaskRepoRepo{
		repos: []*models.TaskRepository{{TaskID: "task-1", RepositoryID: "repo-1"}},
	}
	svc := recoveryTestService(t, wt, repos)

	out := svc.RecoverTaskBranches(context.Background(), "task-1")
	if len(out) != 1 || out[0].Status != worktree.BranchStatusMissing {
		t.Fatalf("recovery = %+v, want single missing entry", out)
	}
	if len(repos.updated) != 0 {
		t.Errorf("checkout_branch must not be written for a missing branch: %+v", repos.updated)
	}
}

func TestRecoverTaskBranches_NeverOverwritesExistingCheckoutBranch(t *testing.T) {
	wt := &stubWorktreeRecovery{
		worktrees: []*worktree.Worktree{{
			RepositoryID:   "repo-1",
			RepositoryPath: "/repos/one",
			Branch:         "feature/old-work",
		}},
		statuses: map[string]string{"feature/old-work": worktree.BranchStatusLocal},
	}
	repos := &stubTaskRepoRepo{
		repos: []*models.TaskRepository{{
			TaskID: "task-1", RepositoryID: "repo-1", CheckoutBranch: "pr-head-branch",
		}},
	}
	svc := recoveryTestService(t, wt, repos)

	svc.RecoverTaskBranches(context.Background(), "task-1")
	if len(repos.updated) != 0 {
		t.Errorf("existing checkout_branch (PR head) must not be overwritten: %+v", repos.updated)
	}
}

func TestRecoverTaskBranches_PicksNewestWorktreePerRepo(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-time.Minute)
	wt := &stubWorktreeRecovery{
		worktrees: []*worktree.Worktree{
			{RepositoryID: "repo-1", RepositoryPath: "/repos/one", Branch: "feature/first-session", CreatedAt: old},
			{RepositoryID: "repo-1", RepositoryPath: "/repos/one", Branch: "feature/latest-session", CreatedAt: recent},
		},
		statuses: map[string]string{
			"feature/first-session":  worktree.BranchStatusLocal,
			"feature/latest-session": worktree.BranchStatusLocal,
		},
	}
	repos := &stubTaskRepoRepo{
		repos: []*models.TaskRepository{{TaskID: "task-1", RepositoryID: "repo-1"}},
	}
	svc := recoveryTestService(t, wt, repos)

	out := svc.RecoverTaskBranches(context.Background(), "task-1")
	if len(out) != 1 || out[0].Branch != "feature/latest-session" {
		t.Fatalf("recovery = %+v, want single entry for the newest branch", out)
	}
	if len(repos.updated) != 1 || repos.updated[0].CheckoutBranch != "feature/latest-session" {
		t.Fatalf("checkout_branch = %+v, want feature/latest-session", repos.updated)
	}
}
