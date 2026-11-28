package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceWIPSessionlessReleaseDefaultsAndUpdates(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "Release"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if workspace.WIPSessionlessReleaseEnabled ||
		workspace.WIPSessionlessReleaseGraceMinutes != models.DefaultWIPSessionlessReleaseGraceMinutes {
		t.Fatalf("default policy = enabled:%v grace:%d", workspace.WIPSessionlessReleaseEnabled, workspace.WIPSessionlessReleaseGraceMinutes)
	}

	enabled := true
	grace := 25
	updated, err := svc.UpdateWorkspace(ctx, workspace.ID, &UpdateWorkspaceRequest{
		WIPSessionlessReleaseEnabled:      &enabled,
		WIPSessionlessReleaseGraceMinutes: &grace,
	})
	if err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}
	reloaded, err := svc.GetWorkspace(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	for _, got := range []*models.Workspace{updated, reloaded} {
		if !got.WIPSessionlessReleaseEnabled || got.WIPSessionlessReleaseGraceMinutes != grace {
			t.Fatalf("policy = enabled:%v grace:%d", got.WIPSessionlessReleaseEnabled, got.WIPSessionlessReleaseGraceMinutes)
		}
	}

	// 200000000 minutes overflows time.Duration and would move the release
	// cutoff into the future.
	for _, invalid := range []int{0, models.MaxWIPSessionlessReleaseGraceMinutes + 1, 200000000} {
		if _, err := svc.UpdateWorkspace(ctx, workspace.ID, &UpdateWorkspaceRequest{WIPSessionlessReleaseGraceMinutes: &invalid}); !errors.Is(err, ErrWorkspaceWIPReleaseGraceInvalid) {
			t.Fatalf("grace update %d = %v, want ErrWorkspaceWIPReleaseGraceInvalid", invalid, err)
		}
	}
}
