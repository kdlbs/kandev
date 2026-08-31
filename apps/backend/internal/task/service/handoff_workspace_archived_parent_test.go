package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

// REGRESSION: an inherit_parent child created under an already-archived
// parent must be rejected outright ("born stranded") rather than silently
// attached to a workspace group whose owner's runtime resources (worktree,
// container/sandbox) are already torn down by archive — even though the
// owner's task_environments row itself survives archive. This is the case
// where the parent has no workspace group yet, so lookupOrCreateParentGroup
// would otherwise take the create-new-group branch.
func TestAttachWorkspacePolicy_InheritParentRefusesArchivedParent_NoExistingGroup(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	archivedAt := time.Now()
	tasks.tasks["parent"].ArchivedAt = &archivedAt
	tasks.addTask("child", "parent", "ws-1")
	groups := newFakeWSGroupRepo()
	svc := newPhase4Service(t, tasks, &fakeBlockerRepo{}, groups)

	err := svc.AttachWorkspacePolicy(context.Background(), "child", "parent", WorkspacePolicy{Mode: "inherit_parent"})
	if err == nil {
		t.Fatal("expected error attaching inherit_parent child to an archived parent")
	}
	if !strings.Contains(err.Error(), "parent") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("error %q does not name the archived parent", err.Error())
	}
	if len(groups.groups) != 0 {
		t.Fatalf("no workspace group should have been created; got %d", len(groups.groups))
	}
}

// REGRESSION (the actual bug): a parent that already has other children
// already has a workspace group. Before this fix, lookupOrCreateParentGroup
// only checked ArchivedAt on the create-new-group branch, so once a group
// existed, a NEW inherit_parent child attached to an archived parent's group
// with no error at all — a task that is dead workspace-wise the moment it is
// created, indistinguishable on the board from a normal launchable card.
func TestAttachWorkspacePolicy_InheritParentRefusesArchivedParent_ExistingGroup(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("child1", "parent", "ws-1")
	groups := newFakeWSGroupRepo()
	svc := newPhase4Service(t, tasks, &fakeBlockerRepo{}, groups)

	pol := WorkspacePolicy{Mode: "inherit_parent"}
	if err := svc.AttachWorkspacePolicy(context.Background(), "child1", "parent", pol); err != nil {
		t.Fatalf("attach child1: %v", err)
	}
	if len(groups.groups) != 1 {
		t.Fatalf("expected 1 group after first attach, got %d", len(groups.groups))
	}

	archivedAt := time.Now()
	tasks.tasks["parent"].ArchivedAt = &archivedAt
	tasks.addTask("child2", "parent", "ws-1")

	err := svc.AttachWorkspacePolicy(context.Background(), "child2", "parent", pol)
	if err == nil {
		t.Fatal("expected error attaching a second inherit_parent child after the parent was archived")
	}
	if !strings.Contains(err.Error(), "parent") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("error %q does not name the archived parent", err.Error())
	}
	for gid, members := range groups.members {
		if _, ok := members["child2"]; ok {
			t.Fatalf("child2 must not be joined to group %s of an archived parent", gid)
		}
	}
}
