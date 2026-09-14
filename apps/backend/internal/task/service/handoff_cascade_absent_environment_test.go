package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
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

// AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.7: a positively absent canonical
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

// AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.8: an uncertain signal is not
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
// only (AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.7/.8 cover archive; delete
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

type recordingAbsentEnvironmentWorkspaceCleaner struct {
	plainFolders        []string
	singleRepoWorktrees []string
	multiRepoRoots      []string
	remoteEnvironments  []string
}

func (c *recordingAbsentEnvironmentWorkspaceCleaner) ValidateManagedRoot(string) error {
	return nil
}

func (c *recordingAbsentEnvironmentWorkspaceCleaner) CleanupPlainFolder(_ context.Context, path string) error {
	c.plainFolders = append(c.plainFolders, path)
	return nil
}

func (c *recordingAbsentEnvironmentWorkspaceCleaner) CleanupSingleRepoWorktree(_ context.Context, worktreeID string) error {
	c.singleRepoWorktrees = append(c.singleRepoWorktrees, worktreeID)
	return nil
}

func (c *recordingAbsentEnvironmentWorkspaceCleaner) CleanupMultiRepoRoot(_ context.Context, rootPath string, _ []string) error {
	c.multiRepoRoots = append(c.multiRepoRoots, rootPath)
	return nil
}

func (c *recordingAbsentEnvironmentWorkspaceCleaner) CleanupRemoteEnvironment(_ context.Context, _, environmentID string) error {
	c.remoteEnvironments = append(c.remoteEnvironments, environmentID)
	return nil
}

func TestArchiveTaskTree_AbsentCanonicalEnvironmentPreservesSurvivingGroupResources(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	officeDB := sqlx.NewDb(repo.DB(), "sqlite3")
	officeRepo, err := officesqlite.NewWithDB(officeDB, officeDB, nil)
	if err != nil {
		t.Fatalf("NewWithDB(office): %v", err)
	}
	ws, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil || ws == nil {
		t.Fatalf("GetWorkspace: workspace=%#v err=%v", ws, err)
	}
	for _, task := range []*models.Task{
		{
			ID: "task-absent-env-root", WorkspaceID: ws.ID, WorkflowID: ws.OfficeWorkflowID,
			WorkflowStepID: "step", Title: "Root", Priority: models.TaskPriorityMedium,
		},
		{
			ID: "task-absent-env-child", WorkspaceID: ws.ID, WorkflowID: ws.OfficeWorkflowID,
			WorkflowStepID: "step", Title: "Surviving child", ParentID: "task-absent-env-root",
			Priority: models.TaskPriorityMedium,
		},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}

	const groupID = "group-absent-canonical-environment"
	const absentEnvironmentID = "environment-already-removed"
	group := &orchmodels.WorkspaceGroup{
		ID:                        groupID,
		WorkspaceID:               ws.ID,
		OwnerTaskID:               "task-absent-env-root",
		OwnershipGeneration:       7,
		MaterializedPath:          filepath.Join(t.TempDir(), "shared-workspace"),
		MaterializedEnvironmentID: absentEnvironmentID,
		MaterializedKind:          orchmodels.WorkspaceGroupKindSingleRepo,
		OwnedByKandev:             true,
		CleanupPolicy:             orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:             orchmodels.WorkspaceCleanupStatusActive,
		RestoreConfigJSON:         `{"kind":"single_repo","worktree_ids":{"repo-1":"shared-worktree"}}`,
	}
	if err := officeRepo.CreateWorkspaceGroup(ctx, group); err != nil {
		t.Fatalf("CreateWorkspaceGroup: %v", err)
	}
	for _, member := range []struct {
		taskID string
		role   string
	}{
		{"task-absent-env-root", orchmodels.WorkspaceMemberRoleOwner},
		{"task-absent-env-child", orchmodels.WorkspaceMemberRoleMember},
	} {
		if err := officeRepo.AddWorkspaceGroupMember(ctx, groupID, member.taskID, member.role); err != nil {
			t.Fatalf("AddWorkspaceGroupMember(%s): %v", member.taskID, err)
		}
	}

	cleaner := &recordingAbsentEnvironmentWorkspaceCleaner{}
	taskSvc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	handoff := NewHandoffService(repo, repo, nil, nil, officeRepo, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	handoff.SetWorkspaceCleaner(cleaner)

	if _, err := handoff.ArchiveTaskTree(ctx, "task-absent-env-root", false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}
	waitForCleanupDone(t, taskSvc)

	root, err := repo.GetTask(ctx, "task-absent-env-root")
	if err != nil {
		t.Fatalf("GetTask(root): %v", err)
	}
	if root.ArchivedAt == nil {
		t.Fatal("root task should be archived")
	}
	child, err := repo.GetTask(ctx, "task-absent-env-child")
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	if child.ArchivedAt != nil {
		t.Fatal("surviving child should remain active")
	}

	gotGroup, err := officeRepo.GetWorkspaceGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetWorkspaceGroup: %v", err)
	}
	if gotGroup.OwnerTaskID != group.OwnerTaskID {
		t.Fatalf("group owner = %q, want %q", gotGroup.OwnerTaskID, group.OwnerTaskID)
	}
	if gotGroup.OwnershipGeneration != group.OwnershipGeneration {
		t.Fatalf("group ownership generation = %d, want %d", gotGroup.OwnershipGeneration, group.OwnershipGeneration)
	}
	if gotGroup.CleanupStatus != orchmodels.WorkspaceCleanupStatusActive {
		t.Fatalf("group cleanup status = %q, want active while child remains", gotGroup.CleanupStatus)
	}
	if len(cleaner.plainFolders) != 0 || len(cleaner.singleRepoWorktrees) != 0 ||
		len(cleaner.multiRepoRoots) != 0 || len(cleaner.remoteEnvironments) != 0 {
		t.Fatalf("shared workspace cleanup calls = %#v, want none", cleaner)
	}
}
