package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBuildDependencyViewsBounded_RefusesRelatedOnlyFanOut(t *testing.T) {
	ctx := context.Background()
	svc, _ := setupOfficeTest(t)
	task := mustSeedTask(t, svc, "Related root")
	relations := newMockRelationRepo()
	for i := 0; i < maxDependencyFanOut+1; i++ {
		peerID := fmt.Sprintf("related-peer-%05d", i)
		if err := relations.AddRelation(ctx, task.ID, peerID); err != nil {
			t.Fatalf("seed related task %s: %v", peerID, err)
		}
	}
	svc.SetRelationRepository(relations)
	svc.tasks = &errTaskRepoOnGetByIDs{TaskRepository: svc.tasks}

	views, err := svc.BuildDependencyViewsBounded(ctx, []*models.Task{task})
	if !errors.Is(err, ErrDependencyFanOutExceeded) {
		t.Fatalf("err = %v, want ErrDependencyFanOutExceeded", err)
	}
	if views != nil {
		t.Errorf("views = %+v, want nil on refusal", views)
	}
}

func TestBuildDependencyViewsBounded_CombinesRelatedAndDependencyFanOut(t *testing.T) {
	ctx := context.Background()
	svc, _ := setupOfficeTest(t)
	task := mustSeedTask(t, svc, "Mixed root")
	blockers := &mockBlockerRepo{}
	relations := newMockRelationRepo()
	svc.SetBlockerRepository(blockers)
	svc.SetRelationRepository(relations)

	const dependencyCount = maxDependencyFanOut / 2
	const relationCount = maxDependencyFanOut - dependencyCount + 1
	for i := 0; i < dependencyCount; i++ {
		id := fmt.Sprintf("mixed-pred-%05d", i)
		if err := blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{
			TaskID: task.ID, BlockerTaskID: id,
		}); err != nil {
			t.Fatalf("seed blocker %s: %v", id, err)
		}
	}
	for i := 0; i < relationCount; i++ {
		id := fmt.Sprintf("mixed-related-%05d", i)
		if err := relations.AddRelation(ctx, task.ID, id); err != nil {
			t.Fatalf("seed related task %s: %v", id, err)
		}
	}
	svc.tasks = &errTaskRepoOnGetByIDs{TaskRepository: svc.tasks}

	views, err := svc.BuildDependencyViewsBounded(ctx, []*models.Task{task})
	if !errors.Is(err, ErrDependencyFanOutExceeded) {
		t.Fatalf("err = %v, want ErrDependencyFanOutExceeded", err)
	}
	if views != nil {
		t.Errorf("views = %+v, want nil on refusal", views)
	}
}
