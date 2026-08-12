package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

type blockingCleanupInventorySessions struct {
	repository.SessionRepository
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (r *blockingCleanupInventorySessions) ListTaskSessions(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSession, error) {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.SessionRepository.ListTaskSessions(ctx, taskID)
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
				var activeJobs int
				if err := repo.DB().QueryRowContext(ctx, `
					SELECT COUNT(*) FROM task_resource_cleanup_jobs
					WHERE task_id = ? AND state != ?
				`, taskID, models.TaskResourceCleanupStateCancelled).Scan(&activeJobs); err != nil {
					t.Fatalf("count cleanup intents: %v", err)
				}
				if activeJobs != 0 {
					t.Fatalf("active cleanup intents = %d, want none after inventory failure", activeJobs)
				}
			})
		}
	}
}

func TestDirectTaskMutationReservesCleanupBarrierBeforeInventory(t *testing.T) {
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
	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			taskID := "task-barrier-before-inventory-" + action.name
			seedCleanupTaskAndSession(t, repo, taskID, "session-before-inventory")
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			svc.sessions = &blockingCleanupInventorySessions{
				SessionRepository: repo,
				entered:           entered,
				release:           release,
			}
			done := make(chan error, 1)
			go func() { done <- action.run(ctx, svc, taskID) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("task mutation did not reach inventory capture")
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "session-racing-inventory", TaskID: taskID,
				State: models.TaskSessionStateCreated,
			}); err == nil {
				t.Fatal("session creation crossed the prepared cleanup barrier")
			}
			releaseOnce.Do(func() { close(release) })
			if err := <-done; err != nil {
				t.Fatalf("%s task: %v", action.name, err)
			}
		})
	}
}
