package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestBindTaskRepositoryContributionDestinationPreservesLatestRow(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-contribution-bind")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-contribution-bind", WorkspaceID: "workspace-contribution-bind", Name: "Work",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-contribution-bind", WorkspaceID: "workspace-contribution-bind",
		WorkflowID: "workflow-contribution-bind", Title: "Bind",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-contribution-bind", WorkspaceID: "workspace-contribution-bind", Name: "kandev",
	}); err != nil {
		t.Fatal(err)
	}
	link := &models.TaskRepository{
		ID: "link-contribution-bind", TaskID: "task-contribution-bind", RepositoryID: "repo-contribution-bind",
		BaseBranch: "main", CheckoutBranch: "feature/task-owned", Position: 1,
		Metadata: map[string]interface{}{"existing": "value"},
	}
	if err := repo.CreateTaskRepository(ctx, link); err != nil {
		t.Fatal(err)
	}

	latest, err := repo.GetTaskRepository(ctx, link.ID)
	if err != nil {
		t.Fatal(err)
	}
	latest.BaseBranch = "release/concurrent"
	latest.CheckoutBranch = "feature/concurrent"
	latest.Position = 7
	latest.Metadata["concurrent"] = "preserved"
	if err := repo.UpdateTaskRepository(ctx, latest); err != nil {
		t.Fatal(err)
	}

	bound, changed, err := repo.BindTaskRepositoryContributionDestination(
		ctx, link.ID, link.TaskID, link.RepositoryID, sqliteTestContributionDestination(),
	)
	if err != nil || !changed {
		t.Fatalf("BindTaskRepositoryContributionDestination() = %#v, %v, %v", bound, changed, err)
	}
	if bound.BaseBranch != "release/concurrent" || bound.CheckoutBranch != "feature/concurrent" || bound.Position != 7 {
		t.Fatalf("binding changed concurrent row fields: %#v", bound)
	}
	if bound.Metadata["existing"] != "value" || bound.Metadata["concurrent"] != "preserved" {
		t.Fatalf("binding changed concurrent metadata: %#v", bound.Metadata)
	}
	if _, ok, err := models.LoadContributionDestination(bound.Metadata); err != nil || !ok {
		t.Fatalf("stored contribution destination: ok=%v err=%v metadata=%#v", ok, err, bound.Metadata)
	}

	if _, changed, err := repo.BindTaskRepositoryContributionDestination(
		ctx, link.ID, "other-task", link.RepositoryID, sqliteTestContributionDestination(),
	); err == nil || changed {
		t.Fatalf("mismatched task binding = changed %v, err %v", changed, err)
	}
}

func sqliteTestContributionDestination() *models.ContributionDestination {
	return &models.ContributionDestination{
		Version:  models.ContributionDestinationVersion,
		Provider: models.ContributionDestinationProviderGitHub,
		SourceRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "kdlbs/kandev", ProviderID: "100", RemoteURL: "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "agent/kandev", ProviderID: "200", RemoteURL: "https://github.com/agent/kandev.git",
		},
	}
}
