package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type recordingPriorityTaskRepository struct {
	repository.TaskRepository
	db             *sqliterepo.Repository
	priorityWrites int
	fullWrites     int
}

func (r *recordingPriorityTaskRepository) UpdateTaskPriority(ctx context.Context, taskID, priority string) error {
	r.priorityWrites++
	_, err := r.db.DB().ExecContext(ctx, `UPDATE tasks SET priority = ? WHERE id = ?`, priority, taskID)
	return err
}

func (r *recordingPriorityTaskRepository) UpdateTask(ctx context.Context, task *models.Task) error {
	r.fullWrites++
	return r.TaskRepository.UpdateTask(ctx, task)
}

func TestUpdateTask_PriorityOnlyUsesFieldScopedRepositoryWrite(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)

	recording := &recordingPriorityTaskRepository{TaskRepository: repo, db: repo}
	svc.tasks = recording
	eventBus.ClearEvents()

	priority := "high"
	updated, err := svc.UpdateTask(ctx, "task-123", &UpdateTaskRequest{Priority: &priority})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Priority != priority {
		t.Fatalf("priority = %q, want %q", updated.Priority, priority)
	}
	if recording.priorityWrites != 1 {
		t.Fatalf("priority writes = %d, want 1", recording.priorityWrites)
	}
	if recording.fullWrites != 0 {
		t.Fatalf("full task writes = %d, want 0 for a priority-only update", recording.fullWrites)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 1 {
		t.Fatalf("published events = %d, want 1", got)
	}
}

var _ repository.TaskPriorityRepository = (*recordingPriorityTaskRepository)(nil)
