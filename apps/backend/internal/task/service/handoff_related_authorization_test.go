package service

import (
	"context"
	"errors"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestGetTaskRelations_FiltersForeignEdges(t *testing.T) {
	ctx := context.Background()
	tasks := newFakeTaskRepo()
	tasks.addTask("foreign-parent", "", "workspace-b")
	tasks.addTask("target", "foreign-parent", "workspace-a")
	tasks.addTask("same-child", "target", "workspace-a")
	tasks.addTask("foreign-child", "target", "workspace-b")
	tasks.addTask("same-sibling", "foreign-parent", "workspace-a")
	tasks.addTask("foreign-sibling", "foreign-parent", "workspace-b")
	tasks.addTask("same-blocker", "", "workspace-a")
	tasks.addTask("foreign-blocker", "", "workspace-b")
	tasks.addTask("same-blocked", "", "workspace-a")
	tasks.addTask("foreign-blocked", "", "workspace-b")

	blockers := &fakeBlockerRepo{}
	for _, blocker := range []string{"same-blocker", "foreign-blocker"} {
		if err := blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{TaskID: "target", BlockerTaskID: blocker}); err != nil {
			t.Fatalf("create target blocker %q: %v", blocker, err)
		}
	}
	for _, blocked := range []string{"same-blocked", "foreign-blocked"} {
		if err := blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{TaskID: blocked, BlockerTaskID: "target"}); err != nil {
			t.Fatalf("create blocked task %q: %v", blocked, err)
		}
	}

	svc := newPhase4Service(t, tasks, blockers, newCascadeWSGroupRepo())
	related, err := svc.GetTaskRelations(ctx, "workspace-a", "target")
	if err != nil {
		t.Fatalf("list compact related tasks: %v", err)
	}

	if related.Parent != nil {
		t.Fatalf("foreign parent leaked: parent=%+v", related.Parent)
	}
	assertRelatedTaskIDs(t, related.Children, "same-child")
	assertRelatedTaskIDs(t, related.Siblings, "same-sibling")
	assertRelatedTaskIDs(t, related.Blockers, "same-blocker")
	assertRelatedTaskIDs(t, related.BlockedBy, "same-blocked")
	for _, task := range append([]pluginsdk.RelationTask{related.Task}, append(related.Children, append(related.Siblings, append(related.Blockers, related.BlockedBy...)...)...)...) {
		if task.WorkspaceID != "workspace-a" {
			t.Fatalf("cross-workspace task projected: %+v", task)
		}
	}
}

func TestGetTaskRelations_HidesUnavailableTarget(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("foreign", "", "workspace-b")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	for _, taskID := range []string{"foreign", "missing"} {
		_, err := svc.GetTaskRelations(context.Background(), "workspace-a", taskID)
		if !errors.Is(err, repository.ErrTaskNotFound) {
			t.Errorf("GetTaskRelations(%q) error = %v, want ErrTaskNotFound", taskID, err)
		}
	}
}

func assertRelatedTaskIDs(t *testing.T, tasks []pluginsdk.RelationTask, want ...string) {
	t.Helper()
	if len(tasks) != len(want) {
		t.Fatalf("related task count = %d, want %d: %+v", len(tasks), len(want), tasks)
	}
	for i, task := range tasks {
		if task.ID != want[i] {
			t.Fatalf("related task[%d] = %q, want %q", i, task.ID, want[i])
		}
	}
}
