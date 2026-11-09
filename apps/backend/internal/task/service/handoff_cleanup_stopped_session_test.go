package service

import (
	"context"
	"testing"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// A member session whose runtime has stopped keeps its executors_running row
// for resume; workspace deletion must clean the group without waiting on it.
func TestCleanupWorkspaceGroupsDoesNotWaitOnStoppedMemberSession(t *testing.T) {
	groups := newCascadeWSGroupRepo()
	ctx := context.Background()
	if err := groups.CreateWorkspaceGroup(ctx, &orchmodels.WorkspaceGroup{
		ID:               "group-owned",
		WorkspaceID:      "ws-delete",
		OwnerTaskID:      "task-1",
		MaterializedPath: "/tmp/kandev-owned-group",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}); err != nil {
		t.Fatalf("create owned group: %v", err)
	}
	if err := groups.AddWorkspaceGroupMember(ctx, "group-owned", "task-1", "owner"); err != nil {
		t.Fatalf("add group member: %v", err)
	}
	sessions := newFakeSessionReader()
	sessions.sessions["task-1"] = []*models.TaskSession{{ID: "session-1", TaskID: "task-1"}}
	cleaner := &fakeWorkspaceCleaner{}
	svc := NewHandoffService(nil, nil, nil, nil, groups, nil)
	svc.SetSessionReader(sessions)
	svc.SetWorkspaceCleaner(cleaner)

	// Shorter than the group-idle poll interval: any wait would hit the deadline.
	cleanupCtx, cancel := context.WithTimeout(ctx, workspaceGroupCleanupPollInterval/2)
	defer cancel()
	start := time.Now()
	if err := svc.CleanupWorkspaceGroups(cleanupCtx, "ws-delete"); err != nil {
		t.Fatalf("cleanup workspace groups: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= workspaceGroupCleanupPollInterval {
		t.Fatalf("cleanup waited %v on a stopped session", elapsed)
	}
	if len(cleaner.plainFolders) != 1 {
		t.Fatalf("plain folder cleanups = %#v, want one", cleaner.plainFolders)
	}
}
