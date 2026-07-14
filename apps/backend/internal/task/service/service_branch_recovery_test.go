package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
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

func recoveryTestService(t *testing.T, wt *stubWorktreeRecovery, repos *stubTaskRepoRepo) (*Service, *MockEventBus) {
	t.Helper()
	svc, eventBus, _ := createTestService(t)
	svc.SetWorktreeCleanup(wt)
	svc.taskRepos = repos
	return svc, eventBus
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
	svc, eventBus := recoveryTestService(t, wt, repos)
	if err := svc.tasks.CreateTask(context.Background(), &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "t",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	eventBus.ClearEvents()

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
	// checkout_branch rides on task events — a successful restore must
	// re-publish task.updated so WS clients pick up the repository change.
	published := eventBus.GetPublishedEvents()
	found := false
	for _, ev := range published {
		if ev.Type == events.TaskUpdated {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a %s event after checkout_branch restore, got %d events", events.TaskUpdated, len(published))
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
	svc, _ := recoveryTestService(t, wt, repos)

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
	svc, _ := recoveryTestService(t, wt, repos)

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
	svc, _ := recoveryTestService(t, wt, repos)

	out := svc.RecoverTaskBranches(context.Background(), "task-1")
	if len(out) != 1 || out[0].Branch != "feature/latest-session" {
		t.Fatalf("recovery = %+v, want single entry for the newest branch", out)
	}
	if len(repos.updated) != 1 || repos.updated[0].CheckoutBranch != "feature/latest-session" {
		t.Fatalf("checkout_branch = %+v, want feature/latest-session", repos.updated)
	}
}
