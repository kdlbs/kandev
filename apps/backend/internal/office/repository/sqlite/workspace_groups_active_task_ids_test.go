package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestGetActiveWorkspaceGroupTaskIDs(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	insertWGTask(t, repo, "task-active", false)
	insertWGTask(t, repo, "task-released", false)
	insertWGTask(t, repo, "task-none", false)

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "task-active",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g); err != nil {
		t.Fatalf("create workspace group: %v", err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, g.ID, "task-active", models.WorkspaceMemberRoleOwner); err != nil {
		t.Fatalf("add active member: %v", err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, g.ID, "task-released", ""); err != nil {
		t.Fatalf("add released member: %v", err)
	}
	if err := repo.ReleaseWorkspaceGroupMember(ctx, g.ID, "task-released", models.WorkspaceReleaseReasonArchived, ""); err != nil {
		t.Fatalf("release member: %v", err)
	}

	got, err := repo.GetActiveWorkspaceGroupTaskIDs(ctx, []string{"task-active", "task-released", "task-none"})
	if err != nil {
		t.Fatalf("GetActiveWorkspaceGroupTaskIDs: %v", err)
	}
	if !got["task-active"] {
		t.Error("active member reported false")
	}
	if got["task-released"] {
		t.Error("released member reported true")
	}
	if got["task-none"] {
		t.Error("task with no membership reported true")
	}
}

func TestGetActiveWorkspaceGroupTaskIDsEmptyInput(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	got, err := repo.GetActiveWorkspaceGroupTaskIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetActiveWorkspaceGroupTaskIDs(nil): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for empty input, got %#v", got)
	}
}
