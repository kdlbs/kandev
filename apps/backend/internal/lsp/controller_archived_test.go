package lsp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type archivedControllerTasks struct {
	*fakeControllerTasks
	archivedAt time.Time
}

func (f *archivedControllerTasks) GetTask(_ context.Context, taskID string) (*models.Task, error) {
	f.record("task:" + taskID)
	return &models.Task{ID: taskID, ArchivedAt: &f.archivedAt}, nil
}

func TestArchivedTaskStartDoesNotEnsureTaskResources(t *testing.T) {
	tasks := &archivedControllerTasks{
		fakeControllerTasks: &fakeControllerTasks{},
		archivedAt:          time.Unix(150, 0).UTC(),
	}
	runtimes := &fakeLSPRuntimes{host: newFakeLSPHost()}
	controller := NewController(ControllerConfig{
		Tasks: tasks, Store: newMemoryLSPStore(), Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(8), Clock: func() time.Time { return time.Unix(100, 0).UTC() },
	})

	snapshot, err := controller.Start(
		context.Background(), "task-1", "go",
		Origin{Initiator: InitiatorUser, Reason: "user_control"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Phase != PhaseWaitingForTask || runtimes.ensureCalls != 0 || controller.capacity.Active() != 0 {
		t.Fatalf("archived start=%#v ensure=%d active=%d",
			snapshot, runtimes.ensureCalls, controller.capacity.Active())
	}
}
