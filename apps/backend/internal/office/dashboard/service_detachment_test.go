package dashboard_test

import (
	"context"
	"encoding/json"
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
	insertTestTask(t, deps.db, "new-parent", "workspace", "New parent", "todo", 1)
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

// The Office non-empty reparent path must apply the same composite workspace
// semantics as the canonical detach: an inherit_parent subtask re-parented to
// a new parent keeps its materialized workspace as shared_group instead of
// silently inheriting the new parent's.
func TestUpdateTaskParentIDNormalizesInheritedWorkspaceMode(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "child", "workspace", "Child", "todo", 1)
	insertTestTask(t, deps.db, "new-parent", "workspace", "New parent", "todo", 1)
	if _, err := deps.db.Exec(`UPDATE tasks SET parent_id = 'parent', metadata = '{"workspace":{"mode":"inherit_parent","group_id":"group-1"}}' WHERE id = 'child'`); err != nil {
		t.Fatalf("set parent/metadata: %v", err)
	}

	if err := deps.svc.UpdateTaskParentID(context.Background(), "child", "new-parent"); err != nil {
		t.Fatalf("UpdateTaskParentID: %v", err)
	}

	var parentID string
	if err := deps.db.Get(&parentID, `SELECT parent_id FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select parent: %v", err)
	}
	if parentID != "new-parent" {
		t.Fatalf("parent_id = %q, want new-parent", parentID)
	}

	var metadata string
	if err := deps.db.Get(&metadata, `SELECT metadata FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	var parsed struct {
		Workspace struct {
			Mode    string `json:"mode"`
			GroupID string `json:"group_id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
		t.Fatalf("parse metadata %q: %v", metadata, err)
	}
	if parsed.Workspace.Mode != "shared_group" {
		t.Fatalf("workspace mode = %q, want shared_group", parsed.Workspace.Mode)
	}
	if parsed.Workspace.GroupID != "group-1" {
		t.Fatalf("workspace group_id = %q, want group-1", parsed.Workspace.GroupID)
	}
}

// A repeated PATCH with the same parent_id must not flip workspace semantics:
// the task was not re-parented, so an inherit_parent mode is preserved.
func TestUpdateTaskParentIDSameParentDoesNotNormalizeMetadata(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "child", "workspace", "Child", "todo", 1)
	insertTestTask(t, deps.db, "parent", "workspace", "Parent", "todo", 1)
	if _, err := deps.db.Exec(`UPDATE tasks SET parent_id = 'parent', metadata = '{"workspace":{"mode":"inherit_parent","group_id":"group-1"}}' WHERE id = 'child'`); err != nil {
		t.Fatalf("set parent/metadata: %v", err)
	}

	if err := deps.svc.UpdateTaskParentID(context.Background(), "child", "parent"); err != nil {
		t.Fatalf("UpdateTaskParentID: %v", err)
	}

	var metadata string
	if err := deps.db.Get(&metadata, `SELECT metadata FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select metadata: %v", err)
	}
	var parsed struct {
		Workspace struct {
			Mode string `json:"mode"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
		t.Fatalf("parse metadata %q: %v", metadata, err)
	}
	if parsed.Workspace.Mode != "inherit_parent" {
		t.Fatalf("workspace mode = %q, want inherit_parent (same-parent update must not normalize)", parsed.Workspace.Mode)
	}
}

// A non-empty parent_id that does not exist must be rejected instead of
// silently writing a dangling parent reference.
func TestUpdateTaskParentIDRejectsNonExistentParent(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "child", "workspace", "Child", "todo", 1)
	if _, err := deps.db.Exec(`UPDATE tasks SET parent_id = 'parent' WHERE id = 'child'`); err != nil {
		t.Fatalf("set parent: %v", err)
	}

	if err := deps.svc.UpdateTaskParentID(context.Background(), "child", "missing-parent"); err == nil {
		t.Fatal("UpdateTaskParentID error = nil, want missing-parent error")
	}

	var parentID string
	if err := deps.db.Get(&parentID, `SELECT parent_id FROM tasks WHERE id = 'child'`); err != nil {
		t.Fatalf("select parent: %v", err)
	}
	if parentID != "parent" {
		t.Fatalf("parent_id = %q, want unchanged 'parent'", parentID)
	}
}
