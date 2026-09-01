package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkflowCRUDOrderingVisibilityAndTransactionalReorder(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-workflow-crud")
	for _, workflow := range []*models.Workflow{
		{ID: "workflow-one", WorkspaceID: "workspace-workflow-crud", Name: "One", Source: "invalid", Style: "invalid"},
		{ID: "workflow-two", WorkspaceID: "workspace-workflow-crud", Name: "Two", Source: models.WorkflowSourceGitHub, SourcePath: ".kandev/two.yaml", Style: models.WorkflowStyleCustom},
		{ID: "workflow-hidden", WorkspaceID: "workspace-workflow-crud", Name: "Hidden", Hidden: true},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow(%s): %v", workflow.ID, err)
		}
	}
	got, err := repo.GetWorkflow(ctx, "workflow-one")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Source != models.WorkflowSourceManual || got.Style != models.WorkflowStyleKanban || got.SortOrder != 0 {
		t.Fatalf("normalized workflow = %+v", got)
	}
	visible, err := repo.ListWorkflows(ctx, "workspace-workflow-crud", false)
	if err != nil || strings.Join(workflowIDs(visible), ",") != "workflow-one,workflow-two" {
		t.Fatalf("visible workflows = %v, %v", workflowIDs(visible), err)
	}
	if err := repo.ReorderWorkflows(ctx, "workspace-workflow-crud", []string{"workflow-two", "workflow-one"}); err != nil {
		t.Fatalf("ReorderWorkflows: %v", err)
	}
	visible, err = repo.ListWorkflows(ctx, "workspace-workflow-crud", false)
	if err != nil || strings.Join(workflowIDs(visible), ",") != "workflow-two,workflow-one" {
		t.Fatalf("reordered workflows = %v, %v", workflowIDs(visible), err)
	}
	if err := repo.ReorderWorkflows(ctx, "workspace-workflow-crud", []string{"workflow-one", "missing"}); err == nil {
		t.Fatal("ReorderWorkflows accepted a missing workflow")
	}
	visible, err = repo.ListWorkflows(ctx, "workspace-workflow-crud", false)
	if err != nil || strings.Join(workflowIDs(visible), ",") != "workflow-two,workflow-one" {
		t.Fatalf("failed reorder was not rolled back: %v, %v", workflowIDs(visible), err)
	}

	got, err = repo.GetWorkflow(ctx, "workflow-two")
	if err != nil {
		t.Fatal(err)
	}
	got.Name, got.Description, got.Prompt, got.Hidden = "Updated", "description", "prompt", true
	if err := repo.UpdateWorkflow(ctx, got); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	all, err := repo.ListWorkflows(ctx, "workspace-workflow-crud", true)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListWorkflows(include hidden) = %v, %v", workflowIDs(all), err)
	}
	deleted, err := repo.DeleteWorkflowsByWorkspace(ctx, "workspace-workflow-crud", []string{"workflow-one"})
	if err != nil || deleted != 2 {
		t.Fatalf("DeleteWorkflowsByWorkspace = %d, %v", deleted, err)
	}
	if err := repo.DeleteWorkflow(ctx, "workflow-one"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
	if err := repo.DeleteWorkflow(ctx, "workflow-one"); err == nil {
		t.Fatal("second DeleteWorkflow returned nil")
	}
}

func TestTaskRepositoryCRUDOrderingBatchAndCascade(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-task-repo")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-task-repo", WorkspaceID: "workspace-task-repo", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"task-repo-one", "task-repo-two"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-task-repo", WorkflowID: "workflow-task-repo", Title: taskID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, repositoryID := range []string{"repo-one", "repo-two"} {
		if err := repo.CreateRepository(ctx, &models.Repository{ID: repositoryID, WorkspaceID: "workspace-task-repo", Name: repositoryID}); err != nil {
			t.Fatal(err)
		}
	}
	links := []*models.TaskRepository{
		{ID: "link-second", TaskID: "task-repo-one", RepositoryID: "repo-two", BaseBranch: "main", CheckoutBranch: "feature", BranchPolicyID: "policy-feature", BranchPolicyName: "Feature", BranchPolicyBaseBranch: "main", BranchPolicyBranchTemplate: "feature/{title}-{suffix}", BranchPolicyPullRequestTarget: "develop", Position: 2, Metadata: map[string]any{"role": "secondary"}},
		{ID: "link-first", TaskID: "task-repo-one", RepositoryID: "repo-one", BaseBranch: "main", Position: 1, Metadata: map[string]any{"role": "primary"}},
		{ID: "link-other", TaskID: "task-repo-two", RepositoryID: "repo-one", Position: 0},
	}
	for _, link := range links {
		if err := repo.CreateTaskRepository(ctx, link); err != nil {
			t.Fatalf("CreateTaskRepository(%s): %v", link.ID, err)
		}
	}
	got, err := repo.GetTaskRepository(ctx, "link-second")
	if err != nil || got.CheckoutBranch != "feature" || got.Metadata["role"] != "secondary" || got.BranchPolicyID != "policy-feature" || got.BranchPolicyName != "Feature" || got.BranchPolicyBaseBranch != "main" || got.BranchPolicyBranchTemplate != "feature/{title}-{suffix}" || got.BranchPolicyPullRequestTarget != "develop" {
		t.Fatalf("GetTaskRepository = %+v, %v", got, err)
	}
	got.Position, got.BaseBranch, got.CheckoutBranch = 0, "develop", "next"
	got.BranchPolicyID = "policy-hotfix"
	got.BranchPolicyName = "Hotfix"
	got.BranchPolicyBaseBranch = "develop"
	got.BranchPolicyBranchTemplate = "hotfix/{title}"
	got.BranchPolicyPullRequestTarget = "main"
	got.Metadata = map[string]any{"updated": true}
	if err := repo.UpdateTaskRepository(ctx, got); err != nil {
		t.Fatalf("UpdateTaskRepository: %v", err)
	}
	updated, err := repo.GetTaskRepository(ctx, "link-second")
	if err != nil || updated.BranchPolicyID != "policy-hotfix" || updated.BranchPolicyName != "Hotfix" || updated.BranchPolicyBaseBranch != "develop" || updated.BranchPolicyBranchTemplate != "hotfix/{title}" || updated.BranchPolicyPullRequestTarget != "main" {
		t.Fatalf("updated task repository snapshot = %+v, %v", updated, err)
	}
	primary, err := repo.GetPrimaryTaskRepository(ctx, "task-repo-one")
	if err != nil || primary.ID != "link-second" {
		t.Fatalf("GetPrimaryTaskRepository = %+v, %v", primary, err)
	}
	grouped, err := repo.ListTaskRepositoriesByTaskIDs(ctx, []string{"task-repo-one", "task-repo-two"})
	if err != nil || len(grouped["task-repo-one"]) != 2 || len(grouped["task-repo-two"]) != 1 {
		t.Fatalf("ListTaskRepositoriesByTaskIDs = %v, %v", grouped, err)
	}
	groupedSnapshot := grouped["task-repo-one"][0]
	if groupedSnapshot.BranchPolicyID != "policy-hotfix" || groupedSnapshot.BranchPolicyName != "Hotfix" || groupedSnapshot.BranchPolicyBaseBranch != "develop" || groupedSnapshot.BranchPolicyBranchTemplate != "hotfix/{title}" || groupedSnapshot.BranchPolicyPullRequestTarget != "main" {
		t.Fatalf("batched task repository snapshot = %+v", groupedSnapshot)
	}
	empty, err := repo.ListTaskRepositoriesByTaskIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("ListTaskRepositoriesByTaskIDs(nil) = %v, %v", empty, err)
	}
	if err := repo.UpdateTaskRepository(ctx, &models.TaskRepository{ID: "missing"}); err == nil {
		t.Fatal("UpdateTaskRepository accepted missing row")
	}
	if err := repo.DeleteTaskRepository(ctx, "link-first"); err != nil {
		t.Fatalf("DeleteTaskRepository: %v", err)
	}
	if err := repo.DeleteTaskRepository(ctx, "link-first"); err == nil {
		t.Fatal("second DeleteTaskRepository returned nil")
	}
	if err := repo.DeleteTaskRepositoriesByTask(ctx, "task-repo-one"); err != nil {
		t.Fatalf("DeleteTaskRepositoriesByTask: %v", err)
	}
	primary, err = repo.GetPrimaryTaskRepository(ctx, "task-repo-one")
	if err != nil || primary != nil {
		t.Fatalf("primary after bulk delete = %+v, %v", primary, err)
	}
}

func TestSessionFileReviewUpsertOrderingAndDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-file-review", "session-file-review", "turn-file-review")
	reviewedAt := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	for _, review := range []*models.SessionFileReview{
		{ID: "review-z", SessionID: "session-file-review", FilePath: "z.go", DiffHash: "old"},
		{ID: "review-a", SessionID: "session-file-review", FilePath: "a.go", Reviewed: true, DiffHash: "a", ReviewedAt: &reviewedAt},
	} {
		if err := repo.UpsertSessionFileReview(ctx, review); err != nil {
			t.Fatalf("UpsertSessionFileReview: %v", err)
		}
	}
	if err := repo.UpsertSessionFileReview(ctx, &models.SessionFileReview{ID: "replacement-id", SessionID: "session-file-review", FilePath: "z.go", Reviewed: true, DiffHash: "new", ReviewedAt: &reviewedAt}); err != nil {
		t.Fatalf("UpsertSessionFileReview(update): %v", err)
	}
	reviews, err := repo.GetSessionFileReviews(ctx, "session-file-review")
	if err != nil || len(reviews) != 2 || reviews[0].FilePath != "a.go" || reviews[1].ID != "review-z" || reviews[1].DiffHash != "new" {
		t.Fatalf("GetSessionFileReviews = %+v, %v", reviews, err)
	}
	if err := repo.DeleteSessionFileReviews(ctx, "session-file-review"); err != nil {
		t.Fatalf("DeleteSessionFileReviews: %v", err)
	}
	reviews, err = repo.GetSessionFileReviews(ctx, "session-file-review")
	if err != nil || len(reviews) != 0 {
		t.Fatalf("reviews after delete = %+v, %v", reviews, err)
	}
}

func workflowIDs(workflows []*models.Workflow) []string {
	ids := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		ids = append(ids, workflow.ID)
	}
	return ids
}
