package service

import (
	"context"
	"errors"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

func TestArchiveTaskTree_RetentionRaceReleasesCommittedChildMembership(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.addTask("peer", "", "ws-1")
	tasks.taskEnvironments = map[string]*models.TaskEnvironment{
		"env-shared": {ID: "env-shared", TaskID: "child"},
	}
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "root",
		MaterializedEnvironmentID: "env-shared",
		OwnedByKandev:             true,
		CleanupPolicy:             orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:             orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members["g1"] = map[string]string{
		"root":  orchmodels.WorkspaceMemberRoleOwner,
		"child": orchmodels.WorkspaceMemberRoleMember,
		"peer":  orchmodels.WorkspaceMemberRoleMember,
	}
	svc := NewHandoffService(&retentionRaceCascadeRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, groups, nil)

	out, err := svc.ArchiveTaskTree(context.Background(), "root", true)
	if !errors.Is(err, ErrTaskArchiveHeld) {
		t.Fatalf("ArchiveTaskTree error = %v, want retention hold", err)
	}
	if len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "child" {
		t.Errorf("archived tasks = %v, want child", out.ArchivedTaskIDs)
	}
	if _, active := groups.members["g1"]["child"]; active {
		t.Error("archived child remains an active workspace-group member")
	}
	if _, active := groups.members["g1"]["root"]; !active {
		t.Error("held root lost its workspace-group membership")
	}
	env, err := tasks.GetTaskEnvironment(context.Background(), "env-shared")
	if err != nil {
		t.Errorf("GetTaskEnvironment: %v", err)
		return
	}
	if env.TaskID != "root" {
		t.Errorf("shared environment owner = %q, want held root", env.TaskID)
	}
}
