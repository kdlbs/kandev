package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
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

func TestDeleteWorkspaceCascadeDeletesWorkspaceChildren(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")

	tasks, workflows, err := repo.DeleteWorkspaceCascade(ctx, "ws-delete")
	if err != nil {
		t.Fatalf("DeleteWorkspaceCascade: %v", err)
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

func TestDeleteWorkspaceCascadeDeletesBootstrappedWorkflowSteps(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	if _, err := workflowrepo.NewWithDB(repo.db, repo.db, nil); err != nil {
		t.Fatalf("initialize workflow repository: %v", err)
	}

	workspace := &models.Workspace{ID: "ws-kanban-delete", Name: "Delete Kanban"}
	workflow, err := repo.CreateWorkspaceWithKanban(ctx, workspace)
	if err != nil {
		t.Fatalf("CreateWorkspaceWithKanban: %v", err)
	}
	var steps int
	if err := repo.db.QueryRow(`
		SELECT COUNT(*)
		FROM workflow_steps
		WHERE workflow_id = ?
	`, workflow.ID).Scan(&steps); err != nil {
		t.Fatalf("count bootstrapped steps: %v", err)
	}
	if steps != 4 {
		t.Fatalf("bootstrapped steps = %d, want 4", steps)
	}

	if _, _, err := repo.DeleteWorkspaceCascade(ctx, workspace.ID); err != nil {
		t.Fatalf("DeleteWorkspaceCascade: %v", err)
	}
	if err := repo.db.QueryRow(`
		SELECT COUNT(*)
		FROM workflow_steps
		WHERE workflow_id = ?
	`, workflow.ID).Scan(&steps); err != nil {
		t.Fatalf("count remaining steps: %v", err)
	}
	if steps != 0 {
		t.Fatalf("remaining workflow steps = %d, want 0", steps)
	}
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

func TestCreateWorkspaceWithKanbanRollsBackAllRowsWhenStepInsertFails(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	if _, err := workflowrepo.NewWithDB(repo.db, repo.db, nil); err != nil {
		t.Fatalf("initialize workflow repository: %v", err)
	}
	if _, err := repo.db.Exec(`
		CREATE TRIGGER fail_kanban_step
		BEFORE INSERT ON workflow_steps
		WHEN NEW.name = 'In Progress'
		BEGIN
			SELECT RAISE(ABORT, 'step insert blocked');
		END
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	stepsBefore := countWorkflowSteps(t, repo)

	_, err := repo.CreateWorkspaceWithKanban(ctx, &models.Workspace{ID: "ws-bootstrap", Name: "Bootstrap"})
	if err == nil {
		t.Fatal("CreateWorkspaceWithKanban succeeded despite step insert failure")
	}
	assertBootstrapRowsAbsent(t, repo, "ws-bootstrap", stepsBefore)
}

func TestCreateWorkspaceWithKanbanRollsBackWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := newRepoForHealTests(t)
	if _, err := workflowrepo.NewWithDB(repo.db, repo.db, nil); err != nil {
		t.Fatalf("initialize workflow repository: %v", err)
	}

	_, err := repo.CreateWorkspaceWithKanban(ctx, &models.Workspace{ID: "ws-cancelled", Name: "Cancelled"})
	if err == nil {
		t.Fatal("CreateWorkspaceWithKanban succeeded with a cancelled context")
	}
	assertBootstrapRowsAbsent(t, repo, "ws-cancelled", countWorkflowSteps(t, repo))
}

func TestCreateWorkspaceWithKanbanUsesSimpleTemplate(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	workflowStore := newWorkflowStore(t, repo)

	workflow, err := repo.CreateWorkspaceWithKanban(ctx, &models.Workspace{ID: "ws-kanban", Name: "Kanban"})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithKanban: %v", err)
	}
	if workflow.WorkflowTemplateID == nil || *workflow.WorkflowTemplateID != kanbanTemplateID {
		t.Fatalf("workflow template = %v, want %q", workflow.WorkflowTemplateID, kanbanTemplateID)
	}
	steps, err := workflowStore.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("Kanban steps = %d, want 4", len(steps))
	}
	template, err := kanbanTemplate()
	if err != nil {
		t.Fatalf("kanbanTemplate: %v", err)
	}
	if len(template.Steps) != 4 {
		t.Fatalf("simple template steps = %d, want 4", len(template.Steps))
	}
	byName := workflowStepsByName(t, steps)
	if len(byName) != len(template.Steps) {
		t.Fatalf("unique Kanban step names = %d, want %d", len(byName), len(template.Steps))
	}
	for _, definition := range template.Steps {
		assertKanbanStep(t, byName[definition.Name], workflow.ID, definition)
	}

	reviewID := byName["Review"].ID
	if got := stepIDFromTurnComplete(t, byName["Backlog"], wfmodels.OnTurnCompleteMoveToStep); got != reviewID {
		t.Fatalf("Backlog move target = %q, want Review ID %q", got, reviewID)
	}
	if got := stepIDFromTurnComplete(t, byName["In Progress"], wfmodels.OnTurnCompleteMoveToStep); got != reviewID {
		t.Fatalf("In Progress move target = %q, want Review ID %q", got, reviewID)
	}
	if got := stepIDFromTurnStart(t, byName["Done"], wfmodels.OnTurnStartMoveToStep); got != byName["In Progress"].ID {
		t.Fatalf("Done move target = %q, want In Progress ID %q", got, byName["In Progress"].ID)
	}
	assertKanbanEventTypes(t, byName)
}

func TestInsertTemplateStepsPreservesOptionalFieldsAndRemapsReferences(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	workflowStore := newWorkflowStore(t, repo)
	workspace := &models.Workspace{ID: "ws-synthetic", Name: "Synthetic"}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	workflow := &models.Workflow{ID: "wf-synthetic", WorkspaceID: workspace.ID, Name: "Synthetic"}
	if err := repo.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	template := &wfmodels.WorkflowTemplate{Steps: []wfmodels.StepDefinition{
		{
			ID:                        "source",
			Name:                      "Source",
			Position:                  4,
			Color:                     "bg-purple-500",
			Prompt:                    "keep every field",
			AllowManualMove:           false,
			IsStartStep:               true,
			ShowInCommandPanel:        false,
			AutoArchiveAfterHours:     72,
			AgentProfileID:            "agent-profile",
			StageType:                 wfmodels.StageTypeReview,
			AutoAdvanceRequiresSignal: true,
			WIPLimit:                  3,
			PullFromStepID:            "target",
			Events: wfmodels.StepEvents{
				OnEnter:        []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
				OnTurnStart:    []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToStep, Config: map[string]any{"step_id": "target"}}},
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "target"}}},
				OnComment:      []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToStep, Config: map[string]any{"step_id": "target"}}},
			},
		},
		{ID: "target", Name: "Target", Position: 5, Color: "bg-green-500"},
	}}
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := repo.insertTemplateSteps(ctx, tx, workflow.ID, template); err != nil {
		t.Fatalf("insertTemplateSteps: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit template steps: %v", err)
	}
	steps, err := workflowStore.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}
	byName := workflowStepsByName(t, steps)
	source := byName["Source"]
	target := byName["Target"]
	if source.ID == "source" || target.ID == "target" || source.ID == target.ID {
		t.Fatalf("generated IDs = source:%q target:%q, want distinct non-template IDs", source.ID, target.ID)
	}
	for _, step := range []*wfmodels.WorkflowStep{source, target} {
		if _, err := uuid.Parse(step.ID); err != nil {
			t.Fatalf("step ID %q is not a UUID: %v", step.ID, err)
		}
	}
	if source.Prompt != "keep every field" || source.Color != "bg-purple-500" || source.Position != 4 ||
		source.AllowManualMove || !source.IsStartStep || source.ShowInCommandPanel ||
		source.AutoArchiveAfterHours != 72 || source.AgentProfileID != "agent-profile" ||
		source.StageType != wfmodels.StageTypeReview || !source.AutoAdvanceRequiresSignal || source.WIPLimit != 3 {
		t.Fatalf("optional fields were not persisted faithfully: %#v", source)
	}
	if source.PullFromStepID != target.ID {
		t.Fatalf("pull source = %q, want remapped target ID %q", source.PullFromStepID, target.ID)
	}
	if got := stepIDFromTurnStart(t, source, wfmodels.OnTurnStartMoveToStep); got != target.ID {
		t.Fatalf("on_turn_start target = %q, want %q", got, target.ID)
	}
	if got := stepIDFromTurnComplete(t, source, wfmodels.OnTurnCompleteMoveToStep); got != target.ID {
		t.Fatalf("on_turn_complete target = %q, want %q", got, target.ID)
	}
	if got := stepIDFromComment(t, source); got != target.ID {
		t.Fatalf("on_comment target = %q, want %q", got, target.ID)
	}
}

func newWorkflowStore(t *testing.T, repo *Repository) *workflowrepo.Repository {
	t.Helper()
	store, err := workflowrepo.NewWithDB(repo.db, repo.db, nil)
	if err != nil {
		t.Fatalf("initialize workflow repository: %v", err)
	}
	return store
}

func workflowStepsByName(t *testing.T, steps []*wfmodels.WorkflowStep) map[string]*wfmodels.WorkflowStep {
	t.Helper()
	byName := make(map[string]*wfmodels.WorkflowStep, len(steps))
	for _, step := range steps {
		byName[step.Name] = step
	}
	return byName
}

func assertKanbanStep(t *testing.T, step *wfmodels.WorkflowStep, workflowID string, definition wfmodels.StepDefinition) {
	t.Helper()
	if step == nil {
		t.Fatal("expected Kanban step is missing")
	}
	if step.WorkflowID != workflowID || step.Position != definition.Position || step.Color != definition.Color ||
		step.Prompt != definition.Prompt || step.AllowManualMove != definition.AllowManualMove ||
		step.IsStartStep != definition.IsStartStep || step.ShowInCommandPanel != definition.ShowInCommandPanel ||
		step.AutoArchiveAfterHours != definition.AutoArchiveAfterHours || step.AgentProfileID != definition.AgentProfileID ||
		step.StageType != wfmodels.StageType(normalizeBootstrapStageType(definition.StageType)) ||
		step.AutoAdvanceRequiresSignal != definition.AutoAdvanceRequiresSignal || step.WIPLimit != definition.WIPLimit ||
		step.PullFromStepID != definition.PullFromStepID {
		t.Fatalf("Kanban step %q was not persisted faithfully: %#v", step.Name, step)
	}
	if _, err := uuid.Parse(step.ID); err != nil {
		t.Fatalf("Kanban step %q ID %q is not a UUID: %v", step.Name, step.ID, err)
	}
}

func assertKanbanEventTypes(t *testing.T, steps map[string]*wfmodels.WorkflowStep) {
	t.Helper()
	backlog := steps["Backlog"]
	inProgress := steps["In Progress"]
	review := steps["Review"]
	if len(backlog.Events.OnTurnStart) != 1 || backlog.Events.OnTurnStart[0].Type != wfmodels.OnTurnStartMoveToNext {
		t.Fatalf("Backlog on_turn_start = %#v, want move_to_next", backlog.Events.OnTurnStart)
	}
	if len(inProgress.Events.OnEnter) != 1 || inProgress.Events.OnEnter[0].Type != wfmodels.OnEnterAutoStartAgent {
		t.Fatalf("In Progress on_enter = %#v, want auto_start_agent", inProgress.Events.OnEnter)
	}
	if len(review.Events.OnTurnStart) != 1 || review.Events.OnTurnStart[0].Type != wfmodels.OnTurnStartMoveToPrevious {
		t.Fatalf("Review on_turn_start = %#v, want move_to_previous", review.Events.OnTurnStart)
	}
}

func stepIDFromTurnStart(t *testing.T, step *wfmodels.WorkflowStep, expected wfmodels.OnTurnStartActionType) string {
	t.Helper()
	if len(step.Events.OnTurnStart) != 1 {
		t.Fatalf("%s on_turn_start actions = %d, want 1", step.Name, len(step.Events.OnTurnStart))
	}
	if step.Events.OnTurnStart[0].Type != expected {
		t.Fatalf("%s on_turn_start type = %q, want %q", step.Name, step.Events.OnTurnStart[0].Type, expected)
	}
	return configStepID(t, step.Name, step.Events.OnTurnStart[0].Config)
}

func stepIDFromTurnComplete(t *testing.T, step *wfmodels.WorkflowStep, expected wfmodels.OnTurnCompleteActionType) string {
	t.Helper()
	if len(step.Events.OnTurnComplete) != 1 {
		t.Fatalf("%s on_turn_complete actions = %d, want 1", step.Name, len(step.Events.OnTurnComplete))
	}
	if step.Events.OnTurnComplete[0].Type != expected {
		t.Fatalf("%s on_turn_complete type = %q, want %q", step.Name, step.Events.OnTurnComplete[0].Type, expected)
	}
	return configStepID(t, step.Name, step.Events.OnTurnComplete[0].Config)
}

func stepIDFromComment(t *testing.T, step *wfmodels.WorkflowStep) string {
	t.Helper()
	if len(step.Events.OnComment) != 1 {
		t.Fatalf("%s on_comment actions = %d, want 1", step.Name, len(step.Events.OnComment))
	}
	if step.Events.OnComment[0].Type != wfmodels.GenericActionMoveToStep {
		t.Fatalf("%s on_comment type = %q, want move_to_step", step.Name, step.Events.OnComment[0].Type)
	}
	return configStepID(t, step.Name, step.Events.OnComment[0].Config)
}

func configStepID(t *testing.T, stepName string, config map[string]interface{}) string {
	t.Helper()
	stepID, ok := config["step_id"].(string)
	if !ok {
		t.Fatalf("%s step_id config = %#v, want string", stepName, config)
	}
	return stepID
}

func assertBootstrapRowsAbsent(t *testing.T, repo *Repository, workspaceID string, stepsBefore int) {
	t.Helper()
	var workspaces, workflows int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE id = ?`, workspaceID).Scan(&workspaces); err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM workflows WHERE workspace_id = ?`, workspaceID).Scan(&workflows); err != nil {
		t.Fatalf("count workflows: %v", err)
	}
	stepsAfter := countWorkflowSteps(t, repo)
	if workspaces != 0 || workflows != 0 || stepsAfter != stepsBefore {
		t.Fatalf("bootstrap rows = workspace:%d workflow:%d total-steps:%d, want workspace/workflow zero and steps unchanged at %d", workspaces, workflows, stepsAfter, stepsBefore)
	}
}

func countWorkflowSteps(t *testing.T, repo *Repository) int {
	t.Helper()
	var steps int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM workflow_steps`).Scan(&steps); err != nil {
		t.Fatalf("count workflow steps: %v", err)
	}
	return steps
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
