package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskRepositoryBranchPolicyMigrationPreservesLegacyRows(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-task-repository-policy-migration")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-task-repository-policy-migration", WorkspaceID: "workspace-task-repository-policy-migration", Name: "Workflow",
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-task-repository-policy-migration", WorkspaceID: "workspace-task-repository-policy-migration",
		WorkflowID: "workflow-task-repository-policy-migration", Title: "Legacy task",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-task-repository-policy-migration", WorkspaceID: "workspace-task-repository-policy-migration", Name: "Legacy repository",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-policy-migration", TaskID: "task-task-repository-policy-migration",
		RepositoryID: "repo-task-repository-policy-migration", BaseBranch: "main", Position: 0,
	}); err != nil {
		t.Fatalf("create legacy task repository: %v", err)
	}

	for _, column := range []string{
		"branch_policy_pull_request_target",
		"branch_policy_branch_template",
		"branch_policy_base_branch",
		"branch_policy_name",
		"branch_policy_id",
	} {
		if _, err := repo.db.Exec(`ALTER TABLE task_repositories DROP COLUMN ` + column); err != nil {
			t.Fatalf("drop legacy column %s: %v", column, err)
		}
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy task repository schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay legacy task repository migration: %v", err)
	}

	got, err := repo.GetTaskRepository(ctx, "task-repository-policy-migration")
	if err != nil {
		t.Fatalf("get migrated task repository: %v", err)
	}
	if got.BaseBranch != "main" {
		t.Fatalf("migrated base branch = %q, want main", got.BaseBranch)
	}
	if got.BranchPolicyID != "" || got.BranchPolicyName != "" || got.BranchPolicyBaseBranch != "" || got.BranchPolicyBranchTemplate != "" || got.BranchPolicyPullRequestTarget != "" {
		t.Fatalf("migrated policy snapshot = %+v, want empty fields", got)
	}
}
