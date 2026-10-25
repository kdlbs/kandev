package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	orchmodels "github.com/kandev/kandev/internal/office/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

type replacementBlockerRepo struct {
	*mockBlockerRepo
	replaceErr error
}

func (r *replacementBlockerRepo) ReplaceTaskBlockers(
	_ context.Context,
	taskID string,
	blockerTaskIDs []string,
) error {
	if r.replaceErr != nil {
		return r.replaceErr
	}
	next := make([]*orchmodels.TaskBlocker, 0, len(r.blockers)+len(blockerTaskIDs))
	for _, blocker := range r.blockers {
		if blocker.TaskID != taskID {
			next = append(next, blocker)
		}
	}
	for _, blockerTaskID := range blockerTaskIDs {
		next = append(next, &orchmodels.TaskBlocker{
			TaskID:        taskID,
			BlockerTaskID: blockerTaskID,
		})
	}
	r.blockers = next
	return nil
}

func TestReplaceDependencies_ReplacesSetAndPublishesChangedTasks(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	blockers := &replacementBlockerRepo{mockBlockerRepo: &mockBlockerRepo{}}
	svc.SetBlockerRepository(blockers)
	ctx := context.Background()
	oldPredecessor := mustSeedTask(t, svc, "Old predecessor")
	newPredecessor := mustSeedTask(t, svc, "New predecessor")
	dependent := mustSeedTask(t, svc, "Dependent")
	if err := blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{
		TaskID: dependent.ID, BlockerTaskID: oldPredecessor.ID,
	}); err != nil {
		t.Fatalf("seed dependency: %v", err)
	}
	eventBus, ok := svc.eventBus.(*MockEventBus)
	if !ok {
		t.Fatal("test service does not use MockEventBus")
	}
	eventBus.ClearEvents()

	if err := svc.ReplaceDependencies(ctx, dependent.ID, []string{newPredecessor.ID}); err != nil {
		t.Fatalf("ReplaceDependencies: %v", err)
	}

	got, err := svc.GetBlockers(ctx, dependent.ID)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(got) != 1 || got[0] != newPredecessor.ID {
		t.Fatalf("blockers = %v, want [%s]", got, newPredecessor.ID)
	}

	updatedIDs := map[string]bool{}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskUpdated {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("task.updated data type = %T", event.Data)
		}
		if id, ok := data["task_id"].(string); ok {
			updatedIDs[id] = true
		}
	}
	if len(updatedIDs) != 3 {
		t.Fatalf("updated task IDs = %v, want dependent and both changed peers", updatedIDs)
	}
	for _, id := range []string{dependent.ID, oldPredecessor.ID, newPredecessor.ID} {
		if !updatedIDs[id] {
			t.Errorf("task.updated missing for %s", id)
		}
	}
	_ = repo
}

func TestReplaceDependencies_RejectsDuplicateWithoutChangingSet(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	blockers := &replacementBlockerRepo{mockBlockerRepo: &mockBlockerRepo{}}
	svc.SetBlockerRepository(blockers)
	ctx := context.Background()
	predecessor := mustSeedTask(t, svc, "Predecessor")
	dependent := mustSeedTask(t, svc, "Dependent")
	if err := svc.AddDependency(ctx, dependent.ID, predecessor.ID); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	err := svc.ReplaceDependencies(ctx, dependent.ID, []string{predecessor.ID, predecessor.ID})
	if !errors.Is(err, ErrInvalidDependencySet) {
		t.Fatalf("error = %v, want ErrInvalidDependencySet", err)
	}
	got, err := svc.GetBlockers(ctx, dependent.ID)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(got) != 1 || got[0] != predecessor.ID {
		t.Fatalf("blockers after rejected replacement = %v, want [%s]", got, predecessor.ID)
	}
}

func TestReplaceDependencies_RejectsCycleWithoutChangingSet(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	blockers := &replacementBlockerRepo{mockBlockerRepo: &mockBlockerRepo{}}
	svc.SetBlockerRepository(blockers)
	ctx := context.Background()
	a := mustSeedTask(t, svc, "A")
	b := mustSeedTask(t, svc, "B")
	c := mustSeedTask(t, svc, "C")
	if err := svc.AddDependency(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("AddDependency(a,b): %v", err)
	}
	if err := svc.AddDependency(ctx, b.ID, c.ID); err != nil {
		t.Fatalf("AddDependency(b,c): %v", err)
	}

	err := svc.ReplaceDependencies(ctx, c.ID, []string{a.ID})
	var cycle *CycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("error = %v, want CycleError", err)
	}
	got, err := svc.GetBlockers(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("cycle rejection changed blockers = %v", got)
	}
}

func TestReplaceDependencies_ReturnsUnavailableWithoutReplacementCapability(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	dependent := mustSeedTask(t, svc, "Dependent")
	predecessor := mustSeedTask(t, svc, "Predecessor")

	err := svc.ReplaceDependencies(context.Background(), dependent.ID, []string{predecessor.ID})
	if !errors.Is(err, ErrDependencyRepositoryUnavailable) {
		t.Fatalf("error = %v, want ErrDependencyRepositoryUnavailable", err)
	}
}

func TestReplaceDependencies_PreservesNotFoundAuthorizationContract(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedForeignPair(t, repo)
	svc.SetBlockerRepository(&replacementBlockerRepo{mockBlockerRepo: &mockBlockerRepo{}})

	err := svc.ReplaceDependencies(ctxAs("user-a"), "task-b", []string{"task-b2"})
	if !errors.Is(err, taskrepo.ErrTaskNotFound) {
		t.Fatalf("foreign replacement error = %v, want ErrTaskNotFound", err)
	}
}
