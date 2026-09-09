package sqlite_test

import (
	"context"
	"fmt"
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

// TestGetActiveWorkspaceGroupTaskIDsChunksAcrossHostParamLimit proves the
// chunked read merges correctly when the caller-supplied task ID count
// exceeds the per-statement bind-parameter chunk size (500 internally): a
// boot or board load for a large workflow must not error the whole read,
// and a hit must still be found regardless of which chunk it falls into.
func TestGetActiveWorkspaceGroupTaskIDsChunksAcrossHostParamLimit(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	insertWGTask(t, repo, "task-active-chunked", false)

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "task-active-chunked",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g); err != nil {
		t.Fatalf("create workspace group: %v", err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, g.ID, "task-active-chunked", models.WorkspaceMemberRoleOwner); err != nil {
		t.Fatalf("add active member: %v", err)
	}

	// Pad well past the internal chunk size (500) so the read spans at
	// least two chunks, with the real hit placed last so it lands in the
	// final chunk.
	taskIDs := make([]string, 0, 1001)
	for i := 0; i < 1000; i++ {
		taskIDs = append(taskIDs, fmt.Sprintf("padding-task-%d", i))
	}
	taskIDs = append(taskIDs, "task-active-chunked")

	got, err := repo.GetActiveWorkspaceGroupTaskIDs(ctx, taskIDs)
	if err != nil {
		t.Fatalf("GetActiveWorkspaceGroupTaskIDs with %d IDs: %v", len(taskIDs), err)
	}
	if !got["task-active-chunked"] {
		t.Errorf("active member reported false across chunked read")
	}
	if len(got) != 1 {
		t.Errorf("expected exactly one hit across chunked read, got %d: %#v", len(got), got)
	}
}
