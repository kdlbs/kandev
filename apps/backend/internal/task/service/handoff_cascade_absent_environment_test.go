package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// absentCanonicalEnvironmentGroup builds a shared group whose canonical
// environment row is gone. materializedPath distinguishes the two shapes the
// affected inventory actually contains: a group materialized with zero
// worktrees (empty path) and one that recorded a real path before its
// environment row was deleted. Neither may block archive, and neither is
// authority to delete a resource.
func absentCanonicalEnvironmentGroup(
	groups *fakeWSGroupRepoCascade, groupID, envID, materializedPath string,
) {
	groups.groups[groupID] = &orchmodels.WorkspaceGroup{
		ID: groupID, WorkspaceID: "ws-1", OwnerTaskID: "root",
		MaterializedEnvironmentID: envID,
		MaterializedPath:          materializedPath,
		OwnedByKandev:             true,
		CleanupPolicy:             orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:             orchmodels.WorkspaceCleanupStatusActive,
	}
	groups.members[groupID] = map[string]string{
		"root":  orchmodels.WorkspaceMemberRoleOwner,
		"child": orchmodels.WorkspaceMemberRoleMember,
	}
}

// AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.6: a positively absent canonical
// environment carries no ownership, so there is nothing to transfer and the
// archive completes.
func TestArchiveTaskTree_SucceedsWhenCanonicalEnvironmentAbsent(t *testing.T) {
	for _, tc := range []struct {
		name             string
		materializedPath string
		lookupErr        error
	}{
		{"typed sentinel, no worktrees", "", fmt.Errorf("%w: %s", repository.ErrTaskEnvironmentNotFound, "env-gone")},
		{"typed sentinel, path recorded", "/tmp/ws/root", fmt.Errorf("%w: %s", repository.ErrTaskEnvironmentNotFound, "env-gone")},
		{"nil row, nil error", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("root", "", "ws-1")
			tasks.addTask("child", "root", "ws-1")
			if tc.lookupErr != nil {
				tasks.taskEnvironmentErrs["env-gone"] = tc.lookupErr
			}
			groups := newCascadeWSGroupRepo()
			absentCanonicalEnvironmentGroup(groups, "g1", "env-gone", tc.materializedPath)
			svc := newCascadeService(t, tasks, groups)

			if _, err := svc.ArchiveTaskTree(context.Background(), "root", false); err != nil {
				t.Fatalf("ArchiveTaskTree: %v", err)
			}
			got, _ := tasks.GetTask(context.Background(), "root")
			if got.ArchivedAt == nil {
				t.Fatal("root should be archived")
			}
			if owner := groups.groups["g1"].OwnerTaskID; owner != "root" {
				t.Fatalf("group owner = %q, want unchanged %q (no stewardship moved)", owner, "root")
			}
		})
	}
}

// AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.7: an uncertain signal is not
// absence. A generic lookup failure must still fail the archive rather than be
// read as "nothing to transfer".
func TestArchiveTaskTree_FailsWhenEnvironmentLookupErrors(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	tasks.taskEnvironmentErrs["env-shared"] = errors.New("database is locked")
	groups := newCascadeWSGroupRepo()
	absentCanonicalEnvironmentGroup(groups, "g1", "env-shared", "")
	svc := newCascadeService(t, tasks, groups)

	_, err := svc.ArchiveTaskTree(context.Background(), "root", false)
	if err == nil {
		t.Fatal("archive should fail on an uncertain environment lookup error")
	}
	if errors.Is(err, repository.ErrTaskEnvironmentNotFound) {
		t.Fatalf("error should not be classified as absence: %v", err)
	}
	got, _ := tasks.GetTask(context.Background(), "root")
	if got.ArchivedAt != nil {
		t.Fatal("root must not be archived when ownership cannot be resolved")
	}
	if owner := groups.groups["g1"].OwnerTaskID; owner != "root" {
		t.Fatalf("group owner = %q, want unchanged", owner)
	}
}

// DeleteTaskTree shares transferWorkspaceGroupEnvironmentOwnership with
// ArchiveTaskTree, but the absent-environment tolerance is scoped to archive
// only (AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.6/.7 cover archive; delete
// is a destructive path this plan never evaluated). A positively absent
// environment must still fail delete, the same as before the archive fix.
func TestDeleteTaskTree_FailsWhenCanonicalEnvironmentAbsent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lookupErr error
	}{
		{"typed sentinel", fmt.Errorf("%w: %s", repository.ErrTaskEnvironmentNotFound, "env-gone")},
		{"nil row, nil error", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("root", "", "ws-1")
			tasks.addTask("child", "root", "ws-1")
			if tc.lookupErr != nil {
				tasks.taskEnvironmentErrs["env-gone"] = tc.lookupErr
			}
			groups := newCascadeWSGroupRepo()
			absentCanonicalEnvironmentGroup(groups, "g1", "env-gone", "")
			svc := newCascadeService(t, tasks, groups)

			if _, err := svc.DeleteTaskTree(context.Background(), "root", false); err == nil {
				t.Fatal("delete should fail closed on an absent canonical environment")
			}
			if got, _ := tasks.GetTask(context.Background(), "root"); got == nil {
				t.Fatal("root must not be deleted when ownership cannot be resolved")
			}
			if owner := groups.groups["g1"].OwnerTaskID; owner != "root" {
				t.Fatalf("group owner = %q, want unchanged", owner)
			}
		})
	}
}
