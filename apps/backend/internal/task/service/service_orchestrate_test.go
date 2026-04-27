package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// dbStepResolver resolves the start step from the DB for testing.
type dbStepResolver struct {
	repo *sqliterepo.Repository
}

func (r *dbStepResolver) ResolveStartStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1 LIMIT 1`,
		workflowID).Scan(&stepID)
	if err == sql.ErrNoRows {
		return r.ResolveFirstStep(ctx, workflowID)
	}
	return stepID, err
}

func (r *dbStepResolver) ResolveFirstStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position LIMIT 1`,
		workflowID).Scan(&stepID)
	return stepID, err
}

func setupOrchestrateTest(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Create the workflow_steps table (normally created by workflow repository)
	_, err := repo.DB().Exec(`
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT DEFAULT '',
			prompt TEXT DEFAULT '',
			events TEXT DEFAULT '{}',
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			agent_profile_id TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		)`)
	if err != nil {
		t.Fatalf("create workflow_steps: %v", err)
	}

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_, err = repo.EnsureOrchestrateWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("EnsureOrchestrateWorkflow: %v", err)
	}
	svc.SetStartStepResolver(&dbStepResolver{repo: repo})
	return svc, repo
}

func TestCreateTask_Orchestrate_WithProjectID(t *testing.T) {
	svc, repo := setupOrchestrateTest(t)
	ctx := context.Background()

	ws, _ := repo.GetWorkspace(ctx, "ws-1")
	orchWfID := ws.OrchestrateWorkflowID

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Orchestrate Task",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if task.WorkflowID != orchWfID {
		t.Errorf("workflow_id: got %s, want %s", task.WorkflowID, orchWfID)
	}
	if task.Identifier == "" {
		t.Error("expected identifier to be assigned")
	}
	if !strings.HasPrefix(task.Identifier, "KAN-") {
		t.Errorf("identifier: got %s, want KAN-* prefix", task.Identifier)
	}
	if task.ProjectID != "proj-1" {
		t.Errorf("project_id: got %s, want proj-1", task.ProjectID)
	}
	if task.Origin != models.TaskOriginManual {
		t.Errorf("origin: got %s, want manual", task.Origin)
	}
	if task.WorkflowStepID == "" {
		t.Error("expected workflow_step_id to be resolved")
	}
}

func TestCreateTask_Orchestrate_AgentCreated(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:             "ws-1",
		Title:                   "Agent Task",
		Origin:                  models.TaskOriginAgentCreated,
		AssigneeAgentInstanceID: "agent-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if task.Identifier == "" {
		t.Error("expected identifier for agent-created task")
	}
	if task.Origin != models.TaskOriginAgentCreated {
		t.Errorf("origin: got %s, want agent_created", task.Origin)
	}
	if task.AssigneeAgentInstanceID != "agent-1" {
		t.Errorf("assignee: got %s, want agent-1", task.AssigneeAgentInstanceID)
	}
}

func TestCreateTask_Kanban_StillWorks(t *testing.T) {
	svc, repo := setupOrchestrateTest(t)
	ctx := context.Background()

	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-kanban", WorkspaceID: "ws-1", Name: "Dev"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-kanban",
		Title:       "Kanban Task",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if task.Identifier != "" {
		t.Errorf("kanban task should not have identifier, got %s", task.Identifier)
	}
	if task.WorkflowID != "wf-kanban" {
		t.Errorf("workflow_id: got %s, want wf-kanban", task.WorkflowID)
	}
}

func TestCreateTask_Kanban_RequiresWorkflow(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	ctx := context.Background()

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "No Workflow Task",
	})
	if err == nil {
		t.Error("expected error for non-ephemeral task without workflow")
	}
}

func TestIdentifier_SequentialPerWorkspace(t *testing.T) {
	svc, repo := setupOrchestrateTest(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})
	_, _ = repo.EnsureOrchestrateWorkflow(ctx, "ws-2")

	// Create 3 tasks in ws-1
	for i := 0; i < 3; i++ {
		_, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-1",
			Title:       "Task",
			ProjectID:   "proj-1",
		})
		if err != nil {
			t.Fatalf("create task %d ws-1: %v", i, err)
		}
	}

	// ws-2 starts from 1
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-2",
		Title:       "Task",
		ProjectID:   "proj-2",
	})
	if err != nil {
		t.Fatalf("create task ws-2: %v", err)
	}
	if task.Identifier != "KAN-1" {
		t.Errorf("ws-2 first task: got %s, want KAN-1", task.Identifier)
	}

	// ws-1 should be at KAN-4
	task4, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Task 4",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create task 4 ws-1: %v", err)
	}
	if task4.Identifier != "KAN-4" {
		t.Errorf("ws-1 fourth task: got %s, want KAN-4", task4.Identifier)
	}
}

func TestTaskTree_FlatListWithParentID(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	ctx := context.Background()

	parent, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Child 1",
		ProjectID:   "proj-1",
		ParentID:    parent.ID,
	})
	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Child 2",
		ProjectID:   "proj-1",
		ParentID:    parent.ID,
	})

	tasks, err := svc.ListTaskTree(ctx, "ws-1", models.TaskTreeFilters{})
	if err != nil {
		t.Fatalf("ListTaskTree: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	childCount := 0
	for _, task := range tasks {
		if task.ParentID == parent.ID {
			childCount++
		}
	}
	if childCount != 2 {
		t.Errorf("expected 2 children, got %d", childCount)
	}
}

func TestTaskTree_FilterByProject(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	ctx := context.Background()

	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Proj1",
		ProjectID:   "proj-1",
	})
	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Proj2",
		ProjectID:   "proj-2",
	})

	tasks, err := svc.ListTaskTree(ctx, "ws-1", models.TaskTreeFilters{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ListTaskTree: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for proj-1, got %d", len(tasks))
	}
}

func TestListTasksByAssignee(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	ctx := context.Background()

	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:             "ws-1",
		Title:                   "Agent1 Task",
		ProjectID:               "proj-1",
		AssigneeAgentInstanceID: "agent-1",
	})
	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:             "ws-1",
		Title:                   "Agent2 Task",
		ProjectID:               "proj-1",
		AssigneeAgentInstanceID: "agent-2",
	})

	tasks, err := svc.ListTasksByAssignee(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListTasksByAssignee: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for agent-1, got %d", len(tasks))
	}
}

// mockBlockerRepo implements BlockerRepository for testing.
type mockBlockerRepo struct {
	blockers []*orchmodels.TaskBlocker
}

func (m *mockBlockerRepo) CreateTaskBlocker(_ context.Context, b *orchmodels.TaskBlocker) error {
	m.blockers = append(m.blockers, b)
	return nil
}

func (m *mockBlockerRepo) ListTaskBlockers(_ context.Context, taskID string) ([]*orchmodels.TaskBlocker, error) {
	var result []*orchmodels.TaskBlocker
	for _, b := range m.blockers {
		if b.TaskID == taskID {
			result = append(result, b)
		}
	}
	return result, nil
}

func (m *mockBlockerRepo) DeleteTaskBlocker(_ context.Context, taskID, blockerTaskID string) error {
	for i, b := range m.blockers {
		if b.TaskID == taskID && b.BlockerTaskID == blockerTaskID {
			m.blockers = append(m.blockers[:i], m.blockers[i+1:]...)
			return nil
		}
	}
	return nil
}

func TestBlocker_CRUD(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	if err := svc.AddBlocker(ctx, "task-1", "task-2"); err != nil {
		t.Fatalf("AddBlocker: %v", err)
	}

	ids, err := svc.GetBlockers(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(ids) != 1 || ids[0] != "task-2" {
		t.Errorf("expected [task-2], got %v", ids)
	}

	if err := svc.RemoveBlocker(ctx, "task-1", "task-2"); err != nil {
		t.Fatalf("RemoveBlocker: %v", err)
	}
	ids, _ = svc.GetBlockers(ctx, "task-1")
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
}

func TestBlocker_CircularDetection(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	_ = svc.AddBlocker(ctx, "task-A", "task-B")
	_ = svc.AddBlocker(ctx, "task-B", "task-C")

	err := svc.AddBlocker(ctx, "task-C", "task-A")
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("expected 'circular dependency' in error, got: %v", err)
	}
}

func TestBlocker_SelfReference(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	err := svc.AddBlocker(ctx, "task-1", "task-1")
	if err == nil {
		t.Fatal("expected self-reference error")
	}
}

func TestCreateTask_WithBlockedBy(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	// Create two blocker tasks first.
	blocker1, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Blocker 1", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("create blocker1: %v", err)
	}
	blocker2, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Blocker 2", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("create blocker2: %v", err)
	}

	// Create a task blocked by both.
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Blocked Task",
		ProjectID:   "proj-1",
		BlockedBy:   []string{blocker1.ID, blocker2.ID},
	})
	if err != nil {
		t.Fatalf("create blocked task: %v", err)
	}

	blockers, err := svc.GetBlockers(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(blockers) != 2 {
		t.Errorf("expected 2 blockers, got %d", len(blockers))
	}
}

func TestCreateTask_WithBlockedBy_Empty(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "No Blockers",
		ProjectID:   "proj-1",
		BlockedBy:   []string{},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	blockers, _ := svc.GetBlockers(ctx, task.ID)
	if len(blockers) != 0 {
		t.Errorf("expected 0 blockers, got %d", len(blockers))
	}
	_ = task
}

// mockCommentRepo implements CommentRepository for testing.
type mockCommentRepo struct {
	comments []*orchmodels.TaskComment
}

func (m *mockCommentRepo) CreateTaskComment(_ context.Context, c *orchmodels.TaskComment) error {
	m.comments = append(m.comments, c)
	return nil
}

func (m *mockCommentRepo) ListTaskComments(_ context.Context, taskID string) ([]*orchmodels.TaskComment, error) {
	var result []*orchmodels.TaskComment
	for _, c := range m.comments {
		if c.TaskID == taskID {
			result = append(result, c)
		}
	}
	return result, nil
}

func TestComment_CRUD(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetCommentRepository(&mockCommentRepo{})
	ctx := context.Background()

	_ = svc.CreateComment(ctx, &orchmodels.TaskComment{
		TaskID: "task-1", AuthorType: "user", AuthorID: "u-1", Body: "Hello", Source: "user",
	})
	_ = svc.CreateComment(ctx, &orchmodels.TaskComment{
		TaskID: "task-1", AuthorType: "agent", AuthorID: "a-1", Body: "Working", Source: "agent",
	})

	comments, err := svc.ListComments(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}

	comments, _ = svc.ListComments(ctx, "task-nonexistent")
	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}

func TestCreateTask_OrchestrateFields_Roundtrip(t *testing.T) {
	svc, _ := setupOrchestrateTest(t)
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:             "ws-1",
		Title:                   "Full Orchestrate Task",
		ProjectID:               "proj-1",
		AssigneeAgentInstanceID: "agent-1",
		Origin:                  models.TaskOriginRoutine,
		RequiresApproval:        true,
		ExecutionPolicy:         `{"stages":[]}`,
		Labels:                  `["bug","urgent"]`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if got.ProjectID != "proj-1" {
		t.Errorf("project_id: got %s, want proj-1", got.ProjectID)
	}
	if got.AssigneeAgentInstanceID != "agent-1" {
		t.Errorf("assignee: got %s, want agent-1", got.AssigneeAgentInstanceID)
	}
	if got.Origin != models.TaskOriginRoutine {
		t.Errorf("origin: got %s, want routine", got.Origin)
	}
	if !got.RequiresApproval {
		t.Error("requires_approval: expected true")
	}
	if got.ExecutionPolicy != `{"stages":[]}` {
		t.Errorf("execution_policy: got %s", got.ExecutionPolicy)
	}
	if got.Labels != `["bug","urgent"]` {
		t.Errorf("labels: got %s", got.Labels)
	}
	if got.Identifier == "" {
		t.Error("expected identifier")
	}
}
