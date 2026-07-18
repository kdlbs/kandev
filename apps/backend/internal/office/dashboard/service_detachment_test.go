package dashboard_test

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type recordingTaskDetacher struct {
	taskIDs []string
	err     error
}

func (d *recordingTaskDetacher) DetachTask(_ context.Context, taskID string) (*taskmodels.Task, error) {
	d.taskIDs = append(d.taskIDs, taskID)
	return &taskmodels.Task{ID: taskID}, d.err
}

func TestUpdateTaskParentIDUsesCanonicalDetacherWhenCleared(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "child", "workspace", "Child", "todo", 1)
	if _, err := deps.db.Exec(`UPDATE tasks SET parent_id = 'parent' WHERE id = 'child'`); err != nil {
		t.Fatalf("set parent: %v", err)
	}
	detacher := &recordingTaskDetacher{}
	deps.svc.SetTaskDetacher(detacher)

	if err := deps.svc.UpdateTaskParentID(context.Background(), "child", ""); err != nil {
		t.Fatalf("UpdateTaskParentID: %v", err)
	}

	if len(detacher.taskIDs) != 1 || detacher.taskIDs[0] != "child" {
		t.Fatalf("DetachTask calls = %#v, want [child]", detacher.taskIDs)
	}
	var parentID string
	if err := deps.db.Get(&parentID, `SELECT parent_id FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select parent: %v", err)
	}
	if parentID != "parent" {
		t.Fatalf("parent_id = %q, direct Office update bypassed canonical detacher", parentID)
	}
}

func TestUpdateTaskParentIDFailsClosedWhenDetacherIsNotConfigured(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "child", "workspace", "Child", "todo", 1)
	if _, err := deps.db.Exec(`UPDATE tasks SET parent_id = 'parent' WHERE id = 'child'`); err != nil {
		t.Fatalf("set parent: %v", err)
	}

	if err := deps.svc.UpdateTaskParentID(context.Background(), "child", ""); err == nil {
		t.Fatal("UpdateTaskParentID error = nil, want missing detacher error")
	}

	var parentID string
	if err := deps.db.Get(&parentID, `SELECT parent_id FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select parent: %v", err)
	}
	if parentID != "parent" {
		t.Fatalf("parent_id = %q, want parent", parentID)
	}
}

func TestUpdateTaskParentIDKeepsNonEmptyReparentingInOffice(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "child", "workspace", "Child", "todo", 1)
	detacher := &recordingTaskDetacher{}
	deps.svc.SetTaskDetacher(detacher)

	if err := deps.svc.UpdateTaskParentID(context.Background(), "child", "new-parent"); err != nil {
		t.Fatalf("UpdateTaskParentID: %v", err)
	}

	if len(detacher.taskIDs) != 0 {
		t.Fatalf("DetachTask calls = %#v, want none", detacher.taskIDs)
	}
	var parentID string
	if err := deps.db.Get(&parentID, `SELECT parent_id FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select parent: %v", err)
	}
	if parentID != "new-parent" {
		t.Fatalf("parent_id = %q, want new-parent", parentID)
	}
}
