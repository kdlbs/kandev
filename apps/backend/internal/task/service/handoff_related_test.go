package service

import (
	"context"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// setDescription and setState mutate a task the fake repo already holds so a
// test can model a CREATED sibling (no session) that carries dependency
// metadata in its description. They reach into the fake directly because
// addTask only seeds id/parent/workspace.
func (f *fakeTaskRepo) setDescription(id, desc string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t := f.tasks[id]; t != nil {
		t.Description = desc
	}
}

func (f *fakeTaskRepo) setState(id string, state v1.TaskState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t := f.tasks[id]; t != nil {
		t.State = state
	}
}

// TestListRelated_CreatedSiblingDescription is the core regression for
// issue #1772: a running subtask must be able to read the description of an
// authorized CREATED sibling that has no session yet, through the bounded
// list_related_tasks projection.
func TestListRelated_CreatedSiblingDescription(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("runner", "parent", "ws-1") // the running caller
	tasks.addTask("dep", "parent", "ws-1")    // CREATED sibling, never started
	tasks.setState("runner", v1.TaskStateInProgress)
	tasks.setState("dep", v1.TaskStateCreated)
	tasks.setDescription("dep", "Depends on: 01 [foundation], 03 [canvas]")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "runner")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if len(out.Siblings) != 1 || out.Siblings[0].ID != "dep" {
		t.Fatalf("siblings = %+v, want single sibling dep", out.Siblings)
	}
	sib := out.Siblings[0]
	if sib.State != string(v1.TaskStateCreated) {
		t.Errorf("sibling state = %q, want CREATED", sib.State)
	}
	if sib.Description != "Depends on: 01 [foundation], 03 [canvas]" {
		t.Errorf("sibling description = %q, want the Depends on line", sib.Description)
	}
}

// TestListRelated_ProjectsDescriptionAcrossRelations verifies the
// description flows onto self, parent, and children too, not just siblings.
func TestListRelated_ProjectsDescriptionAcrossRelations(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("self", "parent", "ws-1")
	tasks.addTask("child", "self", "ws-1")
	tasks.setDescription("parent", "parent desc")
	tasks.setDescription("self", "self desc")
	tasks.setDescription("child", "child desc")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "self")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if out.Task.Description != "self desc" {
		t.Errorf("self description = %q", out.Task.Description)
	}
	if out.Parent == nil || out.Parent.Description != "parent desc" {
		t.Errorf("parent description = %+v", out.Parent)
	}
	if len(out.Children) != 1 || out.Children[0].Description != "child desc" {
		t.Errorf("children = %+v", out.Children)
	}
}

// TestListRelated_OmitsEmptyDescription confirms the projection stays lean
// when a related task has no description (omitempty keeps the MCP output
// usable on workflows with many historical tasks).
func TestListRelated_OmitsEmptyDescription(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("self", "parent", "ws-1")
	tasks.addTask("sib", "parent", "ws-1")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "self")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if len(out.Siblings) != 1 || out.Siblings[0].Description != "" {
		t.Errorf("sibling description should be empty, got %+v", out.Siblings)
	}
}
