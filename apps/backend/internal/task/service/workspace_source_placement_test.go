package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkspaceRepositoryRelativePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		layout    string
		placement WorkspaceRepositoryPlacement
		entry     string
		want      string
	}{
		{
			name:      "kandev directory in repository layout",
			layout:    WorkspaceLayoutRepository,
			placement: WorkspacePlacementKandevDirectory,
			entry:     "payments-main",
			want:      "repo-main/kandev/payments-main",
		},
		{
			name:      "current root in repository layout",
			layout:    WorkspaceLayoutRepository,
			placement: WorkspacePlacementCurrentRoot,
			entry:     "payments-main",
			want:      "repo-main/payments-main",
		},
		{
			name:      "expands task root",
			layout:    WorkspaceLayoutRepository,
			placement: WorkspacePlacementExpandRoot,
			entry:     "payments-main",
			want:      "payments-main",
		},
		{
			name:      "parent layout has no repository prefix",
			layout:    WorkspaceLayoutTaskRoot,
			placement: WorkspacePlacementCurrentRoot,
			entry:     "payments-main",
			want:      "payments-main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := workspaceRepositoryRelativePath(&models.TaskEnvironment{
				WorkspacePath:   "/tasks/task-1/repo-main",
				WorkspaceLayout: tt.layout,
			}, tt.placement, tt.entry)
			if err != nil {
				t.Fatalf("workspaceRepositoryRelativePath: %v", err)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkspaceRepositoryRelativePathRejectsUnsafeEntry(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"", ".", "..", "nested/name", "/absolute"} {
		entry := entry
		t.Run(entry, func(t *testing.T) {
			t.Parallel()
			_, err := workspaceRepositoryRelativePath(&models.TaskEnvironment{
				WorkspacePath:   "/tasks/task-1/repo-main",
				WorkspaceLayout: WorkspaceLayoutRepository,
			}, WorkspacePlacementCurrentRoot, entry)
			if !errors.Is(err, ErrInvalidWorkspaceRepositoryPlacement) {
				t.Fatalf("error = %v, want ErrInvalidWorkspaceRepositoryPlacement", err)
			}
		})
	}
}

func TestEffectiveTaskEnvironmentWorkspaceLayoutUsesPhysicalRoots(t *testing.T) {
	t.Parallel()

	workspacePath := filepath.Join("/tasks", "task-1")
	if got := EffectiveTaskEnvironmentWorkspaceLayout(&models.TaskEnvironment{
		WorkspacePath:   workspacePath,
		WorkspaceLayout: WorkspaceLayoutRepository,
		TaskDirName:     "task-1",
		Repos: []*models.TaskEnvironmentRepo{{
			WorktreePath: filepath.Join(workspacePath, "primary"),
		}},
	}); got != WorkspaceLayoutTaskRoot {
		t.Fatalf("stale repository layout = %q, want %q", got, WorkspaceLayoutTaskRoot)
	}

	primaryPath := filepath.Join(workspacePath, "primary")
	if got := EffectiveTaskEnvironmentWorkspaceLayout(&models.TaskEnvironment{
		WorkspacePath:   primaryPath,
		WorkspaceLayout: WorkspaceLayoutTaskRoot,
		TaskDirName:     "task-1",
		Repos: []*models.TaskEnvironmentRepo{
			{WorktreePath: filepath.Join(primaryPath, "attached")},
			{WorktreePath: primaryPath},
		},
	}); got != WorkspaceLayoutRepository {
		t.Fatalf("nested repository layout = %q, want %q", got, WorkspaceLayoutRepository)
	}
}

func TestValidateWorkspaceRepositoryPlacement(t *testing.T) {
	t.Parallel()

	if err := validateWorkspaceRepositoryPlacement(WorkspacePlacementKandevDirectory); err != nil {
		t.Fatalf("valid placement rejected: %v", err)
	}
	if err := validateWorkspaceRepositoryPlacement("unknown"); !errors.Is(err, ErrInvalidWorkspaceRepositoryPlacement) {
		t.Fatalf("error = %v, want ErrInvalidWorkspaceRepositoryPlacement", err)
	}
}

func TestPreflightWorkspaceRepositoryDestinationsRejectsOccupiedAndCaseCollidingPaths(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "New")
	if err := os.WriteFile(existing, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := preflightWorkspaceRepositoryDestinations([]string{filepath.Join(root, "new")})
	if !errors.Is(err, ErrWorkspaceSourceConflict) || !errors.Is(err, worktree.ErrWorkspacePathOccupied) {
		t.Fatalf("case-colliding destination error = %v, want workspace conflict and occupied path", err)
	}
}

func TestPreflightWorkspaceRepositoryDestinationsRejectsProposedCaseCollision(t *testing.T) {
	root := t.TempDir()
	err := preflightWorkspaceRepositoryDestinations([]string{filepath.Join(root, "api"), filepath.Join(root, "API")})
	if !errors.Is(err, ErrWorkspaceSourceConflict) || !errors.Is(err, worktree.ErrWorkspacePathOccupied) {
		t.Fatalf("proposed collision error = %v, want workspace conflict and occupied path", err)
	}
}

func TestPreviewWorkspaceRepositoryPlacementScopesRepositoryReferencesToTaskWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "ws-placement-owned", Name: "Owned", OwnerID: "user-placement"},
		{ID: "ws-placement-same-user", Name: "Same user", OwnerID: "user-placement"},
		{ID: "ws-placement-other-user", Name: "Other user", OwnerID: "other-user"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-placement-auth", WorkspaceID: "ws-placement-owned", Name: "Placement"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-placement-auth", WorkspaceID: "ws-placement-owned", WorkflowID: "wf-placement-auth",
		Title: "Placement", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	workspacePath := t.TempDir()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-placement-auth", TaskID: "task-placement-auth", ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: workspacePath, WorkspaceLayout: WorkspaceLayoutTaskRoot, TaskDirName: "task-placement-auth",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	for _, entity := range []*models.Repository{
		{ID: "repo-placement-owned", WorkspaceID: "ws-placement-owned", Name: "owned-api", DefaultBranch: "main"},
		{ID: "repo-placement-same-user", WorkspaceID: "ws-placement-same-user", Name: "same-user-secret", DefaultBranch: "main"},
		{ID: "repo-placement-other-user", WorkspaceID: "ws-placement-other-user", Name: "other-user-secret", DefaultBranch: "main"},
	} {
		if err := repo.CreateRepository(ctx, entity); err != nil {
			t.Fatalf("CreateRepository(%s): %v", entity.ID, err)
		}
	}

	preview, err := svc.PreviewWorkspaceRepositoryPlacement(ctxAs("user-placement"), "task-placement-auth", []WorkspaceSourceInput{{
		Kind: WorkspaceSourceRepository, RepositoryID: "repo-placement-owned",
	}}, WorkspacePlacementCurrentRoot)
	if err != nil {
		t.Fatalf("owned repository preview: %v", err)
	}
	if len(preview.Sources) != 1 || preview.Sources[0].RepositoryName != "owned-api" {
		t.Fatalf("owned preview sources = %+v, want owned-api", preview.Sources)
	}
	if err := os.Mkdir(filepath.Join(workspacePath, "owned-api"), 0o755); err != nil {
		t.Fatalf("create occupied illustrative destination: %v", err)
	}
	capabilityPreview, err := svc.PreviewWorkspaceRepositoryPlacement(ctxAs("user-placement"), "task-placement-auth", []WorkspaceSourceInput{{
		Kind: WorkspaceSourceRepository, RepositoryID: "repo-placement-owned",
	}}, "")
	if err != nil {
		t.Fatalf("capability preview: %v", err)
	}
	if capabilityPreview.Placement != "" || len(capabilityPreview.SupportedPlacements) != 3 {
		t.Fatalf("capability preview = %+v, want no selected placement and all capability options", capabilityPreview)
	}
	if capabilityPreview.SupportedPlacements[1].Placement != WorkspacePlacementCurrentRoot ||
		!capabilityPreview.SupportedPlacements[1].Enabled {
		t.Fatalf("current-root capability = %+v, want enabled", capabilityPreview.SupportedPlacements[1])
	}

	for _, repositoryID := range []string{"repo-placement-same-user", "repo-placement-other-user"} {
		_, err := svc.PreviewWorkspaceRepositoryPlacement(ctxAs("user-placement"), "task-placement-auth", []WorkspaceSourceInput{{
			Kind: WorkspaceSourceRepository, RepositoryID: repositoryID,
		}}, WorkspacePlacementCurrentRoot)
		if !errors.Is(err, ErrTaskReferenceNotFound) {
			t.Fatalf("foreign repository %s error = %v, want ErrTaskReferenceNotFound", repositoryID, err)
		}
	}
}
