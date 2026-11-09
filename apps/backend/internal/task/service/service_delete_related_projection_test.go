package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

type failingTaskDependencyCleaner struct {
	*mockBlockerRepo
}

func (*failingTaskDependencyCleaner) DeleteTaskBlockersForTask(context.Context, string) error {
	return errors.New("cleanup failed")
}

func TestDeleteDependencyEdgesForTaskRefreshesRelatedPeersOnEarlyReturns(t *testing.T) {
	for _, tc := range []struct {
		name     string
		blockers BlockerRepository
	}{
		{name: "no blocker repository"},
		{name: "blocker repository without cleaner", blockers: &mockBlockerRepo{}},
		{name: "blocker cleanup failure", blockers: &failingTaskDependencyCleaner{mockBlockerRepo: &mockBlockerRepo{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := setupOfficeTest(t)
			ctx := context.Background()
			deleted := mustSeedTask(t, svc, "Deleted")
			peer := mustSeedTask(t, svc, "Related peer")
			relations := newMockRelationRepo()
			if err := relations.AddRelation(ctx, deleted.ID, peer.ID); err != nil {
				t.Fatalf("add relation: %v", err)
			}
			svc.SetRelationRepository(relations)
			svc.SetBlockerRepository(tc.blockers)
			eventBus := NewMockEventBus()
			svc.eventBus = eventBus

			svc.deleteDependencyEdgesForTask(ctx, deleted.ID)

			for _, event := range eventBus.GetPublishedEvents() {
				if event.Type != events.TaskUpdated {
					continue
				}
				data, ok := event.Data.(map[string]interface{})
				if !ok || data["task_id"] != peer.ID {
					continue
				}
				related, ok := data["related"].([]DependencyRef)
				if !ok || len(related) != 0 {
					t.Fatalf("peer related projection = %#v, want empty after relation cleanup", data["related"])
				}
				return
			}
			t.Fatalf("missing task.updated for surviving related peer %s", peer.ID)
		})
	}
}

var _ taskDependencyCleaner = (*failingTaskDependencyCleaner)(nil)

func TestDeleteWorkspaceRemovesDependencyEdgesOfCascadedTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-edges", Name: "Edges"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-edges", WorkspaceID: "ws-edges", Name: "Board"})
	for _, id := range []string{"task-a", "task-b"} {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: "ws-edges", WorkflowID: "wf-edges", WorkflowStepID: "step-edges", Title: id,
		}); err != nil {
			t.Fatalf("CreateTask %s: %v", id, err)
		}
	}
	relations := newMockRelationRepo()
	if err := relations.AddRelation(ctx, "task-a", "task-b"); err != nil {
		t.Fatalf("add relation: %v", err)
	}
	blockers := &mockBlockerRepo{blockers: []*orchmodels.TaskBlocker{{TaskID: "task-b", BlockerTaskID: "task-a"}}}
	svc.SetRelationRepository(relations)
	svc.SetBlockerRepository(blockers)

	if err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-edges", "Edges"); err != nil {
		t.Fatalf("DeleteWorkspaceWithConfirmName: %v", err)
	}

	for _, id := range []string{"task-a", "task-b"} {
		if related, _ := relations.ListRelationsForTask(ctx, id); len(related) != 0 {
			t.Fatalf("relations for %s = %v, want none", id, related)
		}
	}
	if len(blockers.blockers) != 0 {
		t.Fatalf("blockers = %d, want none", len(blockers.blockers))
	}
}
