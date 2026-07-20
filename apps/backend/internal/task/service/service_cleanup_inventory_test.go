package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

type failingCleanupInventoryWorktree struct {
	err error
}

type failingActiveCleanupSessions struct {
	repository.SessionRepository
	err error
}

func (r failingActiveCleanupSessions) ListActiveTaskSessionsByTaskID(context.Context, string) ([]*models.TaskSession, error) {
	return nil, r.err
}

func (c failingCleanupInventoryWorktree) OnTaskDeleted(context.Context, string) error { return nil }

func (c failingCleanupInventoryWorktree) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, c.err
}

func TestTaskMutationAbortsOnAuthoritativeCleanupInventoryReadError(t *testing.T) {
	inventoryErr := errors.New("cleanup inventory unavailable")
	actions := []struct {
		name string
		run  func(context.Context, *Service, string) error
	}{
		{name: "archive", run: func(ctx context.Context, svc *Service, taskID string) error {
			return svc.ArchiveTask(ctx, taskID)
		}},
		{name: "delete", run: func(ctx context.Context, svc *Service, taskID string) error {
			return svc.DeleteTask(ctx, taskID)
		}},
	}
	inventories := []struct {
		name   string
		inject func(*Service, repository.SessionRepository)
	}{
		{name: "sessions", inject: func(svc *Service, sessions repository.SessionRepository) {
			svc.sessions = failingListTaskSessionsRepo{SessionRepository: sessions, err: inventoryErr}
		}},
		{name: "active_sessions", inject: func(svc *Service, sessions repository.SessionRepository) {
			svc.SetExecutionStopper(newRecordingTaskExecutionStopper())
			svc.sessions = failingActiveCleanupSessions{SessionRepository: sessions, err: inventoryErr}
		}},
		{name: "worktrees", inject: func(svc *Service, _ repository.SessionRepository) {
			svc.SetWorktreeCleanup(failingCleanupInventoryWorktree{err: inventoryErr})
		}},
		{name: "environment", inject: func(svc *Service, _ repository.SessionRepository) {
			svc.taskEnvironments = &stubEnvRepo{getErr: inventoryErr}
		}},
	}

	for _, action := range actions {
		for _, inventory := range inventories {
			t.Run(action.name+"/"+inventory.name, func(t *testing.T) {
				svc, _, repo := createTestService(t)
				ctx := context.Background()
				taskID := "task-" + action.name + "-" + inventory.name
				seedCleanupTaskAndSession(t, repo, taskID, "session-"+taskID)
				// Keep the current failing implementation from launching a cleanup
				// goroutine after it incorrectly mutates the task.
				svc.cleanupWorkerWake = make(chan struct{}, 1)
				inventory.inject(svc, repo)

				err := action.run(ctx, svc, taskID)
				if !errors.Is(err, inventoryErr) {
					t.Fatalf("%s error = %v, want inventory error", action.name, err)
				}
				task, getErr := repo.GetTask(ctx, taskID)
				if getErr != nil {
					t.Fatalf("task missing after inventory error: %v", getErr)
				}
				if task.ArchivedAt != nil {
					t.Fatal("task archived after inventory error")
				}
				var jobs int
				if err := repo.DB().QueryRowContext(ctx, `
					SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE task_id = ?
				`, taskID).Scan(&jobs); err != nil {
					t.Fatalf("count cleanup intents: %v", err)
				}
				if jobs != 0 {
					t.Fatalf("cleanup intents = %d, want none before authoritative inventory succeeds", jobs)
				}
			})
		}
	}
}
