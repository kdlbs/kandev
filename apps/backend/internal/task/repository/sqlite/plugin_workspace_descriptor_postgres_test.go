package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func seedDescriptorEnvironmentPostgres(t *testing.T, repo *Repository, n int) {
	t.Helper()
	ctx := context.Background()
	workspaceID, workflowID := "workspace-descriptor", "workflow-descriptor"
	if n == 1 {
		if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Descriptor"}); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
		if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Descriptor"}); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	taskID := fmt.Sprintf("task-descriptor-%d", n)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step-descriptor", Title: taskID, Priority: "medium"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	seedDescriptorEnvironmentRows(t, repo, n, taskID)
}

func newDescriptorPostgresRepos(t *testing.T) (*Repository, *Repository) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	secondDB := openSecondPostgresConnection(t, dsn, db)
	second, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatalf("init second postgres repo: %v", err)
	}
	return repo, second
}

func TestPluginWorkspaceDescriptorImmutablePostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseDescriptorImmutability(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginWorkspaceDescriptorUniquePerInstallationPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseDescriptorUniqueness(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginWorkspaceDescriptorRacePostgres(t *testing.T) {
	repo, second := newDescriptorPostgresRepos(t)
	exerciseDescriptorRace(t, repo, second, seedDescriptorEnvironmentPostgres)
}
