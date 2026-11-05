package backendapp

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	runsmodels "github.com/kandev/kandev/internal/runs/models"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

type fakeOfficeCommentWindowReader struct {
	comments []*officemodels.TaskComment
	total    int
}

func (f *fakeOfficeCommentWindowReader) ListTaskCommentsWindow(
	context.Context, string, int,
) ([]*officemodels.TaskComment, int, error) {
	return f.comments, f.total, nil
}

func TestOfficeCommentReaderAdapterMapsPersistenceRows(t *testing.T) {
	createdAt := time.Date(2026, 1, 1, 2, 3, 4, 0, time.UTC)
	adapter := officeCommentReaderAdapter{reader: &fakeOfficeCommentWindowReader{
		comments: []*officemodels.TaskComment{{
			ID: "comment-1", TaskID: "task-1", AuthorType: "agent", AuthorID: "agent-1",
			Body: "done", Source: "run", ReplyChannelID: "private", CreatedAt: createdAt,
		}},
		total: 4,
	}}

	rows, total, err := adapter.ListTaskCommentsWindow(context.Background(), "task-1", 2)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ID != "comment-1" || rows[0].Body != "done" || !rows[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("row = %+v, want mapped comment", rows[0])
	}
}

type adapterStartStepResolver struct {
	repo *tasksqlite.Repository
}

func (r *adapterStartStepResolver) ResolveStartStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1 LIMIT 1`,
		workflowID,
	).Scan(&stepID)
	if err == sql.ErrNoRows {
		return r.ResolveFirstStep(ctx, workflowID)
	}
	return stepID, err
}

func (r *adapterStartStepResolver) ResolveAutoStartStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? AND events LIKE '%auto_start_agent%' ORDER BY position LIMIT 1`,
		workflowID,
	).Scan(&stepID)
	if err == sql.ErrNoRows {
		return r.ResolveStartStep(ctx, workflowID)
	}
	return stepID, err
}

func (r *adapterStartStepResolver) ResolveFirstStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position LIMIT 1`,
		workflowID,
	).Scan(&stepID)
	return stepID, err
}

func TestTaskCreatorAdapterPersistsOriginByCreationPath(t *testing.T) {
	adapter, taskSvc, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()

	agentTaskID, err := adapter.CreateOfficeTaskAsAgent(
		ctx, "ws-1", "project-1", "agent-worker", "Agent task", "Created at runtime", nil,
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent: %v", err)
	}
	agentTask, err := taskSvc.GetTask(ctx, agentTaskID)
	if err != nil {
		t.Fatalf("GetTask(agent): %v", err)
	}
	if agentTask.Origin != models.TaskOriginAgentCreated {
		t.Errorf("agent task origin = %q, want %q", agentTask.Origin, models.TaskOriginAgentCreated)
	}
	if agentTask.ProjectID != "project-1" {
		t.Errorf("agent task project_id = %q, want project-1", agentTask.ProjectID)
	}
	if agentTask.AssigneeAgentProfileID != "agent-worker" {
		t.Errorf("agent task assignee_agent_profile_id = %q, want agent-worker", agentTask.AssigneeAgentProfileID)
	}

	onboardingTaskID, err := adapter.CreateOfficeTask(
		ctx, "ws-1", "project-setup", "agent-ceo", "Onboarding task", "Created during setup",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTask: %v", err)
	}
	onboardingTask, err := taskSvc.GetTask(ctx, onboardingTaskID)
	if err != nil {
		t.Fatalf("GetTask(onboarding): %v", err)
	}
	if onboardingTask.Origin != models.TaskOriginOnboarding {
		t.Errorf("onboarding task origin = %q, want %q", onboardingTask.Origin, models.TaskOriginOnboarding)
	}
}

func TestOfficeWorkspaceCreatorDoesNotBootstrapKanbanWorkflow(t *testing.T) {
	_, taskSvc, _ := newOfficeTaskAdapterHarness(t)
	adapter := &taskWorkspaceCreatorAdapter{taskSvc: taskSvc}
	ctx := context.Background()

	if err := adapter.CreateWorkspace(ctx, "Office workspace", "Created by Office onboarding"); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	workspaceID, err := adapter.FindWorkspaceIDByName(ctx, "Office workspace")
	if err != nil {
		t.Fatalf("FindWorkspaceIDByName: %v", err)
	}
	workflows, err := taskSvc.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("Office workspace workflows = %d, want 0 before Office onboarding materializes them", len(workflows))
	}
}

func TestCreateWorkspaceKanbanBootstrapCreatesUsableSteps(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name:                    "Kanban workspace",
		BootstrapKanbanWorkflow: true,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspace.ID, false)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("visible workflows = %d, want 1", len(workflows))
	}
	workflow := workflows[0]
	if workflow.Name != "Kanban" || workflow.WorkflowTemplateID == nil || *workflow.WorkflowTemplateID != "simple" {
		t.Fatalf("workflow = %+v, want visible Kanban workflow from simple template", workflow)
	}
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("Kanban workflow has no usable steps")
	}
}

// TestCreateOfficeTaskInWorkflowCarriesAssigneeIntoLaunchMetadata covers
// WO-36's routine-launch-inputs gap: a materialized heavy routine run lands
// on the Routine workflow's start step, which pins no agent (routine.yml).
// The kanban auto-start path's only fallback for the agent to launch with is
// task.Metadata[MetaKeyAgentProfileID] — AssigneeAgentProfileID alone (a
// separate DB column) never reaches it. Without this, the routine's task
// gets created but the orchestrator has no agent profile to start.
//
// It also covers WO-36 round 2: handleTaskCreated only evaluates a step's
// on_enter for a freshly created task when the task carries the positive
// MetaKeyAutoStartOnCreate opt-in (see event_handlers_workflow.go). This
// method is that opt-in's only production setter, so both cases here —
// with and without an assignee — must always stamp it true.
func TestCreateOfficeTaskInWorkflowCarriesAssigneeIntoLaunchMetadata(t *testing.T) {
	adapter, taskSvc, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()

	workflows, err := taskSvc.ListWorkflows(ctx, "ws-1", true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: %v (len=%d)", err, len(workflows))
	}
	workflowID := workflows[0].ID

	taskID, err := adapter.CreateOfficeTaskInWorkflow(
		ctx, "ws-1", "", "routine-assignee", workflowID, "Routine run", "Materialized run", "routine-1",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskInWorkflow: %v", err)
	}
	task, err := taskSvc.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got, _ := task.Metadata[models.MetaKeyAgentProfileID].(string); got != "routine-assignee" {
		t.Errorf("task.Metadata[MetaKeyAgentProfileID] = %q, want routine-assignee", got)
	}
	if !models.HasAutoStartOnCreateIntent(task.Metadata) {
		t.Errorf("task.Metadata[MetaKeyAutoStartOnCreate] not set, want true")
	}
	assertRoutineFireCarrier(t, task.Metadata, "routine-1")

	noAssigneeTaskID, err := adapter.CreateOfficeTaskInWorkflow(
		ctx, "ws-1", "", "", workflowID, "Unassigned run", "Materialized run", "routine-2",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskInWorkflow (no assignee): %v", err)
	}
	noAssigneeTask, err := taskSvc.GetTask(ctx, noAssigneeTaskID)
	if err != nil {
		t.Fatalf("GetTask (no assignee): %v", err)
	}
	if _, ok := noAssigneeTask.Metadata[models.MetaKeyAgentProfileID]; ok {
		t.Errorf("task.Metadata[MetaKeyAgentProfileID] set for an unassigned routine, want absent")
	}
	if !models.HasAutoStartOnCreateIntent(noAssigneeTask.Metadata) {
		t.Errorf("task.Metadata[MetaKeyAutoStartOnCreate] not set for an unassigned routine, want true")
	}
	assertRoutineFireCarrier(t, noAssigneeTask.Metadata, "routine-2")
}

func TestCreateOfficeRoutineTaskPersistsContextAndRequiredAssignmentsBeforeStart(t *testing.T) {
	adapter, taskSvc, officeRepo := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wireOfficeProjectRepositorySources(taskSvc, officeRepo)
	encodedRepositories, err := officemodels.EncodeRepositories([]string{"https://github.com/example/research-repo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := officeRepo.CreateProject(ctx, &officemodels.Project{
		ID: "project-routine", WorkspaceID: "ws-1", Name: "Research",
		Status: officemodels.ProjectStatusActive, Repositories: encodedRepositories,
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	workflows, err := taskSvc.ListWorkflows(ctx, "ws-1", true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: %v", err)
	}
	blocker, err := taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflows[0].ID, Title: "Prerequisite",
	})
	if err != nil {
		t.Fatalf("create blocker task: %v", err)
	}
	id, err := adapter.CreateOfficeTaskInWorkflowWithContext(ctx, "ws-1", "routine-assignee", workflows[0].ID, "Discovery", "Record evidence", "routine-research", officeroutines.RoutineTaskOptions{
		ProjectID: "project-routine", BlockedBy: []string{blocker.Task.ID}, RequiredSubtaskCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := taskSvc.GetTask(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := task.Metadata[models.MetaKeyRequiredSubtaskCount].(float64); got != 3 {
		t.Fatalf("required assignments = %v, want 3", task.Metadata[models.MetaKeyRequiredSubtaskCount])
	}
	if task.ProjectID != "project-routine" {
		t.Fatalf("project_id = %q, want project-routine", task.ProjectID)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("project repositories = %#v, want persisted project source", task.Repositories)
	}
	blockers, err := officeRepo.ListBlockersForTasks(ctx, []string{id})
	if err != nil {
		t.Fatalf("list blockers: %v", err)
	}
	if len(blockers[id]) != 1 || blockers[id][0] != blocker.Task.ID {
		t.Fatalf("blockers = %#v, want %s", blockers[id], blocker.Task.ID)
	}
	if !models.HasAutoStartOnCreateIntent(task.Metadata) {
		t.Fatal("routine start intent lost")
	}
}

func TestCreateOfficeRoutineTaskRejectsCrossWorkspaceContext(t *testing.T) {
	adapter, taskSvc, officeRepo, taskRepo := newOfficeTaskAdapterHarnessWithTaskRepo(t)
	ctx := context.Background()
	wireOfficeProjectRepositorySources(taskSvc, officeRepo)
	if err := taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-foreign", Name: "Foreign workspace"}); err != nil {
		t.Fatalf("create foreign workspace: %v", err)
	}
	foreignWorkflow, err := taskRepo.EnsureOfficeWorkflow(ctx, "ws-foreign")
	if err != nil {
		t.Fatalf("ensure foreign workflow: %v", err)
	}
	if err := officeRepo.CreateProject(ctx, &officemodels.Project{
		ID: "project-foreign", WorkspaceID: "ws-foreign", Name: "Foreign project",
		Status: officemodels.ProjectStatusActive,
	}); err != nil {
		t.Fatalf("create foreign project: %v", err)
	}
	foreignBlocker, err := taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: "ws-foreign", WorkflowID: foreignWorkflow, Title: "Foreign task",
	})
	if err != nil {
		t.Fatalf("create foreign blocker: %v", err)
	}
	workflows, err := taskSvc.ListWorkflows(ctx, "ws-1", true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: %v", err)
	}
	for _, options := range []officeroutines.RoutineTaskOptions{
		{ProjectID: "project-foreign"},
		{BlockedBy: []string{foreignBlocker.Task.ID}},
	} {
		_, err := adapter.CreateOfficeTaskInWorkflowWithContext(ctx, "ws-1", "routine-assignee", workflows[0].ID, "Discovery", "Record evidence", "routine-research", options)
		if err == nil {
			t.Fatalf("CreateOfficeTaskInWorkflowWithContext accepted foreign context: %+v", options)
		}
	}
}

// assertRoutineFireCarrier pins AC-OFFICE-RUN-CAUSATION-001.14/.24: a
// routine-fire task carries the full task-boundary causation carrier with
// the AC.24 root values for the lineage triple (a routine fire has no
// creating run), a system actor, human_rooted=false, and the firing
// routine's id as routine attribution.
func assertRoutineFireCarrier(t *testing.T, metadata map[string]interface{}, wantRoutineID string) {
	t.Helper()
	if got, _ := metadata[models.MetaKeyOfficeCarrierCausationID].(string); got != "" {
		t.Errorf("carrier causation_id = %q, want empty (root)", got)
	}
	if got, _ := metadata[models.MetaKeyOfficeCarrierCausationDepth].(int); got != 0 {
		t.Errorf("carrier causation_depth = %v, want 0 (root)", metadata[models.MetaKeyOfficeCarrierCausationDepth])
	}
	if got, _ := metadata[models.MetaKeyOfficeCarrierCreatingRunID].(string); got != "" {
		t.Errorf("carrier creating_run_id = %q, want empty (root)", got)
	}
	if got, _ := metadata[models.MetaKeyOfficeCarrierHumanRooted].(bool); got {
		t.Error("carrier human_rooted = true, want false")
	}
	if got, _ := metadata[models.MetaKeyOfficeCarrierRoutineID].(string); got != wantRoutineID {
		t.Errorf("carrier routine_id = %q, want %q", got, wantRoutineID)
	}
	if got, _ := metadata[models.MetaKeyOfficeCarrierActorKind].(string); got != string(runsmodels.ActorKindSystem) {
		t.Errorf("carrier actor_kind = %q, want %q", got, runsmodels.ActorKindSystem)
	}
	if got, _ := metadata[models.MetaKeyOfficeCarrierActorID].(string); got != "" {
		t.Errorf("carrier actor_id = %q, want empty", got)
	}
}

// TestTaskCreatorAdapterCreateOfficeSubtaskPassesMetadataThrough covers the
// AC-OFFICE-RUN-CAUSATION-001.5/.18 carrier-writing fix: the adapter must
// forward CreateOfficeSubtask's metadata parameter into ChildTaskSpec so the
// task-boundary causation carrier a caller resolved actually lands on the
// created child task's persisted metadata, exactly as CreateOfficeTaskAsAgent
// already does for a root task.
func TestTaskCreatorAdapterCreateOfficeSubtaskPassesMetadataThrough(t *testing.T) {
	adapter, taskSvc, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()

	parentTaskID, err := adapter.CreateOfficeTaskAsAgent(
		ctx, "ws-1", "project-1", "agent-worker", "Parent task", "Parent", nil,
	)
	if err != nil {
		t.Fatalf("create parent task: %v", err)
	}

	carrier := map[string]interface{}{
		models.MetaKeyOfficeCarrierCausationID:    "causation-1",
		models.MetaKeyOfficeCarrierCausationDepth: 1,
		models.MetaKeyOfficeCarrierCreatingRunID:  "run-1",
		models.MetaKeyOfficeCarrierHumanRooted:    false,
		models.MetaKeyOfficeCarrierRoutineID:      "",
		models.MetaKeyOfficeCarrierActorKind:      string(runsmodels.ActorKindAgent),
		models.MetaKeyOfficeCarrierActorID:        "agent-worker",
	}
	// Metadata round-trips through a JSON column: numbers come back as
	// float64 regardless of the concrete numeric type written.
	wantMetadata := map[string]interface{}{}
	for k, v := range carrier {
		wantMetadata[k] = v
	}
	wantMetadata[models.MetaKeyOfficeCarrierCausationDepth] = float64(1)

	childTaskID, err := adapter.CreateOfficeSubtask(
		ctx, parentTaskID, "agent-assignee", "Child task", "A subtask", carrier,
	)
	if err != nil {
		t.Fatalf("CreateOfficeSubtask: %v", err)
	}
	if childTaskID == "" {
		t.Fatal("expected non-empty child task ID")
	}

	childTask, err := taskSvc.GetTask(ctx, childTaskID)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	if childTask.ParentID != parentTaskID {
		t.Errorf("child task parent_id = %q, want %q", childTask.ParentID, parentTaskID)
	}
	for key, want := range wantMetadata {
		if got := childTask.Metadata[key]; got != want {
			t.Errorf("child task metadata[%q] = %v, want %v", key, got, want)
		}
	}
}

func newOfficeTaskAdapterHarness(t *testing.T) (*taskCreatorAdapter, *taskservice.Service, *officesqlite.Repository) {
	adapter, taskSvc, officeRepo, _ := newOfficeTaskAdapterHarnessWithTaskRepo(t)
	return adapter, taskSvc, officeRepo
}

func newOfficeTaskAdapterHarnessWithTaskRepo(t *testing.T) (*taskCreatorAdapter, *taskservice.Service, *officesqlite.Repository, *tasksqlite.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "office-adapter.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if _, err := worktree.NewSQLiteStore(database, database); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("office migrations: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
		ResourceCleanups: repo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
	taskSvc.SetBlockerRepository(officeRepo)
	taskSvc.SetProjectRepositorySourceReader(emptyOfficeProjectSourceReader{})
	// Mirrors production wiring (registerRoutes in helpers.go): CreateChildTask
	// requires a WorkspacePolicyAttacher to attach the child's workspace-group
	// membership before returning.
	taskSvc.SetWorkspacePolicyAttacher(taskservice.NewHandoffService(repo, repo, nil, officeRepo, officeRepo, nil))

	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := repo.EnsureOfficeWorkflow(ctx, "ws-1"); err != nil {
		t.Fatalf("ensure office workflow: %v", err)
	}
	taskSvc.SetStartStepResolver(&adapterStartStepResolver{repo: repo})
	return &taskCreatorAdapter{taskSvc: taskSvc}, taskSvc, officeRepo, repo
}
