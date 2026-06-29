package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeleteWorkspaceCascadeWithNameDeletesWorkspaceChildren(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")

	tasks, workflows, err := repo.DeleteWorkspaceCascadeWithName(ctx, "ws-delete", "Delete Me")
	if err != nil {
		t.Fatalf("DeleteWorkspaceCascadeWithName: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-delete" {
		t.Fatalf("deleted tasks = %#v, want task-delete", tasks)
	}
	if len(workflows) != 1 || workflows[0].ID != "wf-delete" {
		t.Fatalf("deleted workflows = %#v, want wf-delete", workflows)
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatalf("workspace should be deleted")
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err == nil {
		t.Fatalf("workspace task should be deleted")
	}
	workflows, err = repo.ListWorkflows(ctx, "ws-delete", true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workspace workflows should be deleted, got %d", len(workflows))
	}
	assertNoWorkspaceCascadeDependents(t, repo)
}

func TestDeleteWorkspaceCascadeWithNameRejectsMismatchedName(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")

	_, _, err := repo.DeleteWorkspaceCascadeWithName(ctx, "ws-delete", "Wrong")
	if !errors.Is(err, repoerrors.ErrWorkspaceNameMismatch) {
		t.Fatalf("expected ErrWorkspaceNameMismatch, got %v", err)
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should remain: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should remain: %v", err)
	}
}

func TestDeleteWorkspaceCascadeWithNameRollsBackWhenChildDeleteFails(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")
	if _, err := repo.db.Exec(`
		CREATE TRIGGER fail_workspace_task_delete
		BEFORE DELETE ON tasks
		WHEN OLD.workspace_id = 'ws-delete'
		BEGIN
			SELECT RAISE(ABORT, 'task delete blocked');
		END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if _, _, err := repo.DeleteWorkspaceCascadeWithName(ctx, "ws-delete", "Delete Me"); err == nil {
		t.Fatalf("expected child delete failure")
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace should roll back: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should roll back: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should roll back: %v", err)
	}
}

func seedWorkspaceCascadeRows(t *testing.T, repo *Repository, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Delete Me"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID:          "wf-delete",
		WorkspaceID: workspaceID,
		Name:        "Doomed",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    workspaceID,
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:          "repo-delete",
		WorkspaceID: workspaceID,
		Name:        "Repo",
		SourceType:  "local",
		LocalPath:   "/tmp/repo-delete",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateRepositoryScript(ctx, &models.RepositoryScript{
		ID:           "script-delete",
		RepositoryID: "repo-delete",
		Name:         "test",
		Command:      "true",
	}); err != nil {
		t.Fatalf("CreateRepositoryScript: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           "task-repo-delete",
		TaskID:       "task-delete",
		RepositoryID: "repo-delete",
		BaseBranch:   "main",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "env-delete",
		TaskID:        "task-delete",
		RepositoryID:  "repo-delete",
		ExecutorType:  string(models.ExecutorTypeWorktree),
		Status:        models.TaskEnvironmentStatusReady,
		WorktreeID:    "wt-delete",
		WorktreePath:  "/tmp/wt-delete",
		WorkspacePath: "/tmp/wt-delete",
		Repos: []*models.TaskEnvironmentRepo{{
			ID:             "env-repo-delete",
			RepositoryID:   "repo-delete",
			WorktreeID:     "wt-delete",
			WorktreePath:   "/tmp/wt-delete",
			WorktreeBranch: "branch-delete",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-delete",
		TaskID:            "task-delete",
		AgentExecutionID:  "exec-delete",
		TaskEnvironmentID: "env-delete",
		State:             models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "session-wt-delete",
		SessionID:      "session-delete",
		WorktreeID:     "wt-delete",
		RepositoryID:   "repo-delete",
		WorktreePath:   "/tmp/wt-delete",
		WorktreeBranch: "branch-delete",
	}); err != nil {
		t.Fatalf("CreateTaskSessionWorktree: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-delete",
		TaskSessionID: "session-delete",
		TaskID:        "task-delete",
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-delete",
		TaskSessionID: "session-delete",
		TaskID:        "task-delete",
		TurnID:        "turn-delete",
		AuthorType:    models.MessageAuthorUser,
		Content:       "hello",
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID:        "plan-delete",
		TaskID:    "task-delete",
		Title:     "Plan",
		Content:   "one",
		CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		ID:             "plan-revision-delete",
		TaskID:         "task-delete",
		RevisionNumber: 1,
		Title:          "Plan",
		Content:        "one",
		AuthorKind:     "agent",
	}); err != nil {
		t.Fatalf("InsertTaskPlanRevision: %v", err)
	}
}

func assertNoWorkspaceCascadeDependents(t *testing.T, repo *Repository) {
	t.Helper()
	for _, table := range []string{
		"repositories",
		"repository_scripts",
		"task_repositories",
		"task_sessions",
		"task_environments",
		"task_environment_repos",
		"task_session_worktrees",
		"task_session_turns",
		"task_session_messages",
		"task_plans",
		"task_plan_revisions",
	} {
		assertTableRowCount(t, repo, table, 0)
	}
}

func assertTableRowCount(t *testing.T, repo *Repository, table string, want int) {
	t.Helper()
	var got int
	if err := repo.ro.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows = %d, want %d", table, got, want)
	}
}
