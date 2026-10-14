package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.1
func TestArchiveTaskCleanupRemovesNonEmptyTaskDir(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, _, source, wt := createArchiveReclaimFixture(t)
	root := filepath.Dir(wt.Path)
	other := filepath.Join(filepath.Dir(root), "other-task")
	for _, path := range []string{filepath.Join(root, "artifacts", "nested"), other} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(root, "notes.txt"), filepath.Join(root, "artifacts", "nested", "output.txt"), filepath.Join(other, "sentinel.txt")} {
		if err := os.WriteFile(path, []byte("agent output"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archiveDirectoryTestTask(t, svc, repo, wt.TaskID, false)
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchive)
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("cleanup = %s: %s", job.State, job.LastError)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree remains: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("nonempty task directory remains: %v", err)
	}
	for _, path := range []string{source, filepath.Dir(root), filepath.Join(other, "sentinel.txt")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated path removed: %s: %v", path, err)
		}
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, wt.TaskID)
	if err != nil || env == nil || len(env.Repos) != 1 {
		t.Fatalf("branch recovery metadata lost: %+v: %v", env, err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.2
func TestArchiveReclaimRemovesNonEmptyTaskDir(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	root := filepath.Dir(wt.Path)
	loose := filepath.Join(root, "notes.txt")
	dirty := filepath.Join(wt.Path, "untracked.txt")
	for _, path := range []string{loose, dirty} {
		if err := os.WriteFile(path, []byte("keep until reclamation"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archiveDirectoryTestTask(t, svc, repo, wt.TaskID, false)
	if _, err := os.Stat(loose); err != nil {
		t.Fatalf("dirty root removed: %v", err)
	}
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("task directory remains after final reclaim: %v", err)
	}
}

type failingTaskDirectoryCleaner struct{ *worktree.Manager }

func (f *failingTaskDirectoryCleaner) CleanupTaskDirectory(ctx context.Context, target *worktree.TaskDirectoryCleanup) (bool, error) {
	remaining, err := f.GetAllByTaskID(ctx, target.TaskID)
	if err != nil {
		return false, err
	}
	if len(remaining) > 0 {
		return f.Manager.CleanupTaskDirectory(ctx, target)
	}
	// Model a recursive remover which removed the marker before an I/O failure.
	if err := os.Remove(filepath.Join(target.Path, storageworkspaces.OwnershipMarkerFilename)); err != nil {
		return false, err
	}
	return false, errors.New("injected task directory removal failure")
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.3
func TestArchiveTaskDirectoryRetriesAfterRestart(t *testing.T) {
	for _, reclaim := range []bool{false, true} {
		t.Run(map[bool]string{false: "archive", true: "reclaim"}[reclaim], func(t *testing.T) {
			ctx := context.Background()
			svc, _, repo, manager, _, wt := createArchiveReclaimFixture(t)
			root := filepath.Dir(wt.Path)
			loose := filepath.Join(root, "notes.txt")
			if err := os.WriteFile(loose, []byte("agent output"), 0o600); err != nil {
				t.Fatal(err)
			}
			dirty := filepath.Join(wt.Path, "untracked.txt")
			if reclaim {
				if err := os.WriteFile(dirty, []byte("dirty"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			svc.SetWorktreeCleanup(&failingTaskDirectoryCleaner{Manager: manager})
			archiveDirectoryTestTask(t, svc, repo, wt.TaskID, !reclaim)
			trigger := models.TaskResourceCleanupTriggerArchive
			if reclaim {
				trigger = models.TaskResourceCleanupTriggerArchiveReclaim
				if err := os.Remove(dirty); err != nil {
					t.Fatal(err)
				}
				job := latestCleanupJob(t, repo, wt.TaskID, trigger)
				if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
					t.Fatal(err)
				}
				if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
					t.Fatal("root failure did not reach reclaim retry")
				}
			}
			job := latestCleanupJob(t, repo, wt.TaskID, trigger)
			if job.State != models.TaskResourceCleanupStateRetryWait {
				t.Fatalf("root failure state = %s, %s", job.State, job.LastError)
			}
			if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
				t.Fatalf("worktree not removed: %v", err)
			}
			if _, err := os.Stat(loose); err != nil {
				t.Fatal(err)
			}
			restarted := newServiceOverRepository(t, repo)
			restarted.StopTaskResourceCleanupWorker()
			restarted.SetWorktreeCleanup(manager)
			if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
				t.Fatal(err)
			}
			if err := restarted.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("retry left root behind: %v", err)
			}
		})
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.2
func TestArchiveTaskDirectoryPreservesDirtySiblingUntilFinalReclaim(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, manager, source, primary := createArchiveReclaimFixture(t)
	sibling, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID: primary.TaskID, SessionID: primary.SessionID, TaskTitle: "Dirty sibling",
		RepositoryID: primary.RepositoryID, RepositoryPath: source, BaseBranch: "main",
		TaskDirName: primary.TaskID, RepoName: "repo-archive-reclaim-flow", BranchSlug: "dirty", CheckoutBranch: "feature/dirty",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(primary.Path)
	loose := filepath.Join(root, "notes.txt")
	dirty := filepath.Join(sibling.Path, "untracked.txt")
	for _, path := range []string{loose, dirty} {
		if err := os.WriteFile(path, []byte("task output"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archiveDirectoryTestTask(t, svc, repo, primary.TaskID, false)
	if _, err := os.Stat(primary.Path); !os.IsNotExist(err) {
		t.Fatalf("clean sibling remains: %v", err)
	}
	for _, path := range []string{loose, dirty} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dirty sibling root removed: %v", err)
		}
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	job := latestCleanupJob(t, repo, primary.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("root remains after last sibling: %v", err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.2
func TestArchiveTaskDirectoryDefersWhileEnvironmentIsInUse(t *testing.T) {
	for _, borrower := range []bool{false, true} {
		t.Run(map[bool]string{false: "live-runtime", true: "borrower"}[borrower], func(t *testing.T) {
			ctx := context.Background()
			svc, _, repo, manager, _, wt := createArchiveReclaimFixture(t)
			root := filepath.Dir(wt.Path)
			loose := filepath.Join(root, "notes.txt")
			if err := os.WriteFile(loose, []byte("keep while in use"), 0o600); err != nil {
				t.Fatal(err)
			}
			target, err := manager.CaptureTaskDirectoryCleanup(ctx, wt)
			if err != nil {
				t.Fatal(err)
			}
			env, err := repo.GetTaskEnvironmentByTaskID(ctx, wt.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.CleanupWorktreesPreservingBranches(ctx, []*worktree.Worktree{wt}); err != nil {
				t.Fatal(err)
			}
			if borrower {
				if err := repo.CreateTask(ctx, &models.Task{ID: "borrower-task", WorkspaceID: "ws-" + wt.TaskID, WorkflowID: "wf-" + wt.TaskID, Title: "Borrower"}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "borrower-session", TaskID: "borrower-task", TaskEnvironmentID: env.ID, State: models.TaskSessionStateRunning}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{SessionID: wt.SessionID, TaskID: wt.TaskID, Runtime: "standalone", Status: "running"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := svc.cleanupArchiveTaskDirectory(ctx, wt.TaskID, env, target); !errors.Is(err, errArchiveTaskDirectoryDeferred) {
				t.Fatalf("in-use root must defer: %v", err)
			}
			if _, err := os.Stat(loose); err != nil {
				t.Fatalf("in-use root removed: %v", err)
			}
			if borrower {
				session, err := repo.GetTaskSession(ctx, "borrower-session")
				if err != nil {
					t.Fatal(err)
				}
				session.State = models.TaskSessionStateCompleted
				if err := repo.UpdateTaskSession(ctx, session); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := repo.DeleteExecutorRunningBySessionID(ctx, wt.SessionID); err != nil {
					t.Fatal(err)
				}
			}
			if err := svc.cleanupArchiveTaskDirectory(ctx, wt.TaskID, env, target); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("unused root remains: %v", err)
			}
		})
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.4
func TestCascadeArchiveRemovesDescendantTaskDirectory(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	parentID := "archive-directory-parent"
	if err := repo.CreateTask(ctx, &models.Task{ID: parentID, WorkspaceID: "ws-" + wt.TaskID, WorkflowID: "wf-" + wt.TaskID, Title: "Archive parent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateTask(ctx, wt.TaskID, &UpdateTaskRequest{ParentID: &parentID}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wt.Path)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("agent output"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.StopTaskResourceCleanupWorker()
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	handoff.SetTaskEventPublisher(svc)
	if _, err := handoff.ArchiveTaskTree(ctx, parentID, true); err != nil {
		t.Fatal(err)
	}
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerCascadeArchive)
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("descendant root remains after cascade: %v", err)
	}
}

func archiveDirectoryTestTask(t *testing.T, svc *Service, repo *sqliterepo.Repository, taskID string, expectFailure bool) {
	t.Helper()
	ctx := context.Background()
	svc.StopTaskResourceCleanupWorker()
	if err := svc.ArchiveTask(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	job := latestCleanupJob(t, repo, taskID, models.TaskResourceCleanupTriggerArchive)
	err := svc.processTaskResourceCleanupJob(ctx, job.ID)
	if expectFailure && err == nil {
		t.Fatal("injected archive directory failure did not reach retry")
	}
	if !expectFailure && err != nil {
		t.Fatal(err)
	}
}
