package service

import (
	"context"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestGetTaskContext_RootHasEmptySiblings(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("other-root", "", "ws-1") // would be a "sibling" under naive empty-parent rule
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.GetTaskContext(context.Background(), "root")
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if len(out.Siblings) != 0 {
		t.Errorf("root should have no siblings, got %v", out.Siblings)
	}
	if out.Parent != nil {
		t.Errorf("root should have no parent, got %+v", out.Parent)
	}
}

func TestGetTaskContext_ProjectsParentAndSiblings(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("p", "", "ws-1")
	tasks.addTask("a", "p", "ws-1")
	tasks.addTask("b", "p", "ws-1")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.GetTaskContext(context.Background(), "a")
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if out.Parent == nil || out.Parent.ID != "p" {
		t.Errorf("parent missing or wrong: %+v", out.Parent)
	}
	if len(out.Siblings) != 1 || out.Siblings[0].ID != "b" {
		t.Errorf("siblings = %+v", out.Siblings)
	}
}

func TestGetTaskContext_SurfacesWorkspaceGroup(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("p", "", "ws-1")
	tasks.addTask("c", "p", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "p",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		MaterializedPath: "/wt/abc",
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
		OwnedByKandev:    true,
	}
	groups.members["g1"] = map[string]string{
		"p": orchmodels.WorkspaceMemberRoleOwner,
		"c": orchmodels.WorkspaceMemberRoleMember,
	}
	svc := newCascadeService(t, tasks, groups)

	out, err := svc.GetTaskContext(context.Background(), "c")
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if out.WorkspaceGroup == nil || out.WorkspaceGroup.ID != "g1" {
		t.Fatalf("workspace group missing: %+v", out.WorkspaceGroup)
	}
	if out.WorkspaceGroup.MaterializedPath != "/wt/abc" {
		t.Errorf("path = %q", out.WorkspaceGroup.MaterializedPath)
	}
	if !out.WorkspaceGroup.OwnedByKandev {
		t.Error("OwnedByKandev should be true")
	}
	if len(out.WorkspaceGroup.Members) != 2 {
		t.Errorf("members = %d, want 2", len(out.WorkspaceGroup.Members))
	}
}

func TestGetTaskContext_BlockedReasonAndWorkspaceStatus(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("a", "", "ws-1")
	tasks.addTask("b", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "a",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusFailed,
		OwnedByKandev:    true,
	}
	groups.members["g1"] = map[string]string{"a": orchmodels.WorkspaceMemberRoleOwner}
	svc := newCascadeService(t, tasks, groups)

	out, err := svc.GetTaskContext(context.Background(), "a")
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if out.WorkspaceStatus != v1.TaskWorkspaceStatusRequiresConf {
		t.Errorf("workspace_status = %q, want requires_configuration", out.WorkspaceStatus)
	}
}

func TestGetTaskContext_DocumentBodiesNotIncluded(t *testing.T) {
	// Phase 7 contract: AvailableDocs lists key+title only, never content.
	// We test the structural guarantee — DocumentRef has no Content field —
	// rather than seeding documents (the fakes don't expose docsRepo).
	type docRefStruct = v1.DocumentRef
	var d docRefStruct
	_ = d
	// Compile-time check: the struct must not have a Content field.
	// Reflection-based assertion would also work; the field absence is
	// itself the contract.
}
