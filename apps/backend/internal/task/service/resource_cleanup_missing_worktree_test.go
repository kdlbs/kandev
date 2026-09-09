package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

type missingWorktreeCleanupFixture struct {
	taskID      string
	manager     *worktree.Manager
	repositoryA string
	worktreeA   *worktree.Worktree
	repositoryB string
	worktreeB   *worktree.Worktree
}

func TestPrepareTaskResourceCleanup_MissingWorktree(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	fixture := newMissingWorktreeCleanupFixture(t, repo, "task-prepare-missing-worktree")
	taskSvc.SetWorktreeCleanup(fixture.manager)

	const operationID = "cascade_delete:missing-worktree:task-prepare-missing-worktree"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx,
		fixture.taskID,
		models.TaskResourceCleanupTriggerCascadeDelete,
		operationID,
		true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}

	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", err)
	}
	if job.State != models.TaskResourceCleanupState("prepared") {
		t.Fatalf("cleanup state = %q, want prepared", job.State)
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
		t.Fatalf("decode cleanup snapshot: %v", err)
	}
	if len(snapshot.Worktrees) != 2 {
		t.Fatalf("snapshot worktrees = %d, want both repositories: %+v", len(snapshot.Worktrees), snapshot.Worktrees)
	}
	if len(snapshot.WorktreeHeadOIDs) != 1 {
		t.Fatalf("snapshot worktree identities = %#v, want only the surviving branch", snapshot.WorktreeHeadOIDs)
	}
	if _, ok := snapshot.WorktreeHeadOIDs[fixture.worktreeA.ID]; ok {
		t.Fatalf("snapshot invented an identity for absent worktree %q: %#v", fixture.worktreeA.ID, snapshot.WorktreeHeadOIDs)
	}
	if got := snapshot.WorktreeHeadOIDs[fixture.worktreeB.ID]; got == "" {
		t.Fatalf("snapshot omitted surviving worktree %q identity", fixture.worktreeB.ID)
	}

	task, err := repo.GetTask(ctx, fixture.taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ArchivedAt != nil {
		t.Fatal("preparation mutated the task before the lifecycle mutation")
	}
}

func TestTaskLifecycleCleanup_MissingWorktree(t *testing.T) {
	tests := []struct {
		name           string
		trigger        models.TaskResourceCleanupTrigger
		mutate         func(context.Context, *Service, *sqliterepo.Repository, string) error
		preserveBranch bool
	}{
		{
			name:    "direct archive",
			trigger: models.TaskResourceCleanupTriggerArchive,
			mutate: func(ctx context.Context, svc *Service, _ *sqliterepo.Repository, taskID string) error {
				return svc.ArchiveTask(ctx, taskID)
			},
			preserveBranch: true,
		},
		{
			name:    "direct delete",
			trigger: models.TaskResourceCleanupTriggerDelete,
			mutate: func(ctx context.Context, svc *Service, _ *sqliterepo.Repository, taskID string) error {
				return svc.DeleteTask(ctx, taskID)
			},
		},
		{
			name:    "cascade archive",
			trigger: models.TaskResourceCleanupTriggerCascadeArchive,
			mutate: func(ctx context.Context, svc *Service, repo *sqliterepo.Repository, taskID string) error {
				handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
				handoff.SetTaskResourceCleaner(svc)
				_, err := handoff.ArchiveTaskTree(ctx, taskID, true)
				return err
			},
			preserveBranch: true,
		},
		{
			name:    "cascade delete",
			trigger: models.TaskResourceCleanupTriggerCascadeDelete,
			mutate: func(ctx context.Context, svc *Service, repo *sqliterepo.Repository, taskID string) error {
				handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
				handoff.SetTaskResourceCleaner(svc)
				_, err := handoff.DeleteTaskTree(ctx, taskID, true)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			taskSvc, repo := setupOfficeTest(t)
			taskSvc.StopTaskResourceCleanupWorker()
			fixture := newMissingWorktreeCleanupFixture(t, repo, "task-lifecycle-missing-worktree")
			taskSvc.SetWorktreeCleanup(fixture.manager)
			if test.preserveBranch {
				taskSvc.SetEnvironmentDestroyer(&archiveManagerEnvironmentDestroyer{mgr: fixture.manager})
			} else {
				taskSvc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: fixture.manager})
			}

			if err := test.mutate(ctx, taskSvc, repo, fixture.taskID); err != nil {
				t.Fatalf("lifecycle mutation: %v", err)
			}

			job := latestCleanupJob(t, repo, fixture.taskID, test.trigger)
			if job.State != models.TaskResourceCleanupStatePending {
				t.Fatalf("cleanup state after mutation = %q, want pending", job.State)
			}
			if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
				t.Fatalf("processTaskResourceCleanupJob: %v", err)
			}
			job, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
			if err != nil {
				t.Fatalf("reload cleanup job: %v", err)
			}
			if job.State != models.TaskResourceCleanupStateSucceeded {
				t.Fatalf("cleanup state = %q, want succeeded", job.State)
			}

			if _, statErr := os.Stat(fixture.worktreeA.Path); !os.IsNotExist(statErr) {
				t.Errorf("absent worktree path was not absent after cleanup: %v", statErr)
			}
			if _, statErr := os.Stat(fixture.worktreeB.Path); !os.IsNotExist(statErr) {
				t.Errorf("healthy worktree path remains after cleanup: %v", statErr)
			}
			assertNoWorktreeRegistration(t, fixture.repositoryA, fixture.worktreeA.Path)
			assertNoWorktreeRegistration(t, fixture.repositoryB, fixture.worktreeB.Path)

			branchOutput := strings.TrimSpace(string(runGitTestCmd(
				t, fixture.repositoryB, "branch", "--list", fixture.worktreeB.Branch,
			)))
			if test.preserveBranch && branchOutput == "" {
				t.Fatalf("archive cleanup deleted surviving branch %q", fixture.worktreeB.Branch)
			}
			if !test.preserveBranch && branchOutput != "" {
				t.Fatalf("delete cleanup retained branch %q", fixture.worktreeB.Branch)
			}
		})
	}
}

func TestTaskLifecycleCleanup_MissingWorktree_ReplacementBranchStaysRetryable(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	fixture := newMissingWorktreeCleanupFixture(t, repo, "task-replacement-after-preparation")
	taskSvc.SetWorktreeCleanup(fixture.manager)
	taskSvc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: fixture.manager})
	const operationID = "delete:replacement-after-preparation"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, fixture.taskID, models.TaskResourceCleanupTriggerDelete, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	runGitTestCmd(t, fixture.repositoryA, "branch", fixture.worktreeA.Branch, "main")
	if err := taskSvc.StartPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		t.Fatalf("StartPreparedTaskResourceCleanup: %v", err)
	}
	job := latestCleanupJob(t, repo, fixture.taskID, models.TaskResourceCleanupTriggerDelete)
	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("processTaskResourceCleanupJob error = nil, want replacement-branch safety error")
	}
	job, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload cleanup job: %v", err)
	}
	if job.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("cleanup state = %q, want retry_wait", job.State)
	}
	if got := strings.TrimSpace(string(runGitTestCmd(
		t, fixture.repositoryA, "branch", "--list", fixture.worktreeA.Branch,
	))); got == "" {
		t.Fatalf("replacement branch %q was deleted after preparation", fixture.worktreeA.Branch)
	}
}

func TestTaskLifecycleCleanup_MissingWorktree_ReplacementCheckoutStaysRetryable(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	fixture := newMissingWorktreeCleanupFixture(t, repo, "task-replacement-checkout-after-preparation")
	taskSvc.SetWorktreeCleanup(fixture.manager)
	taskSvc.SetEnvironmentDestroyer(&archiveManagerEnvironmentDestroyer{mgr: fixture.manager})
	const operationID = "archive:replacement-checkout-after-preparation"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, fixture.taskID, models.TaskResourceCleanupTriggerArchive, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	if err := repo.ArchiveTask(ctx, fixture.taskID); err != nil {
		t.Fatalf("ArchiveTask mutation: %v", err)
	}
	runGitTestCmd(t, fixture.repositoryA, "branch", fixture.worktreeA.Branch, "main")
	runGitTestCmd(t, fixture.repositoryA, "worktree", "add", fixture.worktreeA.Path, fixture.worktreeA.Branch)
	if err := taskSvc.StartPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		t.Fatalf("StartPreparedTaskResourceCleanup: %v", err)
	}
	job := latestCleanupJob(t, repo, fixture.taskID, models.TaskResourceCleanupTriggerArchive)
	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("processTaskResourceCleanupJob error = nil, want replacement-checkout safety error")
	}
	job, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload cleanup job: %v", err)
	}
	if job.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("cleanup state = %q, want retry_wait", job.State)
	}
	if _, err := os.Stat(fixture.worktreeA.Path); err != nil {
		t.Fatalf("replacement checkout was removed after preparation: %v", err)
	}
	if got := strings.TrimSpace(string(runGitTestCmd(
		t, fixture.repositoryA, "branch", "--list", fixture.worktreeA.Branch,
	))); got == "" {
		t.Fatalf("replacement branch %q was removed after preparation", fixture.worktreeA.Branch)
	}
}

func newMissingWorktreeCleanupFixture(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
) *missingWorktreeCleanupFixture {
	t.Helper()
	ctx := context.Background()
	sessionID := "session-" + taskID
	environmentID := "env-" + taskID
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)

	repositoryA := initSimpleGitRepo(t)
	repositoryB := initSimpleGitRepo(t)
	workspaceID := "ws-" + taskID
	for id, path := range map[string]string{"repo-a-" + taskID: repositoryA, "repo-b-" + taskID: repositoryB} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: workspaceID, Name: id, SourceType: "local", LocalPath: path,
		}); err != nil {
			t.Fatalf("CreateRepository %s: %v", id, err)
		}
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Join(t.TempDir(), "environment"), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.TaskEnvironmentID = environmentID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	manager := newCleanupTestWorktreeManager(t, repo)
	worktreeA, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		TaskTitle: "Missing worktree cleanup", RepositoryID: "repo-a-" + taskID, RepositoryPath: repositoryA,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-a",
	})
	if err != nil {
		t.Fatalf("create worktree A: %v", err)
	}
	worktreeB, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		TaskTitle: "Missing worktree cleanup", RepositoryID: "repo-b-" + taskID, RepositoryPath: repositoryB,
		BaseBranch: "main", TaskDirName: taskID, RepoName: "repo-b",
	})
	if err != nil {
		t.Fatalf("create worktree B: %v", err)
	}
	runGitTestCmd(t, repositoryA, "worktree", "remove", "--force", worktreeA.Path)
	runGitTestCmd(t, repositoryA, "branch", "-D", worktreeA.Branch)

	return &missingWorktreeCleanupFixture{
		taskID: taskID, manager: manager,
		repositoryA: repositoryA, worktreeA: worktreeA,
		repositoryB: repositoryB, worktreeB: worktreeB,
	}
}

func latestCleanupJob(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
) *models.TaskResourceCleanupJob {
	t.Helper()
	var jobID string
	if err := repo.DB().QueryRowContext(context.Background(), `
		SELECT id FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = ?
		ORDER BY created_at DESC LIMIT 1
	`, taskID, trigger).Scan(&jobID); err != nil {
		t.Fatalf("load cleanup job: %v", err)
	}
	job, err := repo.GetTaskResourceCleanupJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	return job
}
