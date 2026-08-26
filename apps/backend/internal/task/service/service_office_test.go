package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
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

func (r *dbStepResolver) ResolveAutoStartStep(ctx context.Context, workflowID string) (string, error) {
	var stepID string
	err := r.repo.DB().QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? AND events LIKE '%auto_start_agent%' ORDER BY position LIMIT 1`,
		workflowID).Scan(&stepID)
	if err == sql.ErrNoRows {
		return r.ResolveStartStep(ctx, workflowID)
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

func setupOfficeTest(t *testing.T) (*Service, *sqliterepo.Repository) {
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
	_, err = repo.EnsureOfficeWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	svc.SetStartStepResolver(&dbStepResolver{repo: repo})
	return svc, repo
}

func TestCreateTask_Office_WithProjectID(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	ws, _ := repo.GetWorkspace(ctx, "ws-1")
	orchWfID := ws.OfficeWorkflowID

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Office Task",
		ProjectID:   "proj-1",
	})
	task := taskResult.Task
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

func TestCreateTask_Office_AgentCreated(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		Title:                  "Agent Task",
		Origin:                 models.TaskOriginAgentCreated,
		AssigneeAgentProfileID: "agent-1",
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if task.Identifier == "" {
		t.Error("expected identifier for agent-created task")
	}
	if task.Origin != models.TaskOriginAgentCreated {
		t.Errorf("origin: got %s, want agent_created", task.Origin)
	}
	if task.AssigneeAgentProfileID != "agent-1" {
		t.Errorf("assignee: got %s, want agent-1", task.AssigneeAgentProfileID)
	}
}

func TestCreateTask_Kanban_StillWorks(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-kanban", WorkspaceID: "ws-1", Name: "Dev"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-kanban",
		Title:       "Kanban Task",
	})
	task := taskResult.Task
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
	svc, _ := setupOfficeTest(t)
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
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})
	_, _ = repo.EnsureOfficeWorkflow(ctx, "ws-2")

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
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-2",
		Title:       "Task",
		ProjectID:   "proj-2",
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("create task ws-2: %v", err)
	}
	if task.Identifier != "KAN-1" {
		t.Errorf("ws-2 first task: got %s, want KAN-1", task.Identifier)
	}

	// ws-1 should be at KAN-4
	task4Result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Task 4",
		ProjectID:   "proj-1",
	})
	task4 := task4Result.Task
	if err != nil {
		t.Fatalf("create task 4 ws-1: %v", err)
	}
	if task4.Identifier != "KAN-4" {
		t.Errorf("ws-1 fourth task: got %s, want KAN-4", task4.Identifier)
	}
}

func TestTaskTree_FlatListWithParentID(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	parentResult, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
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
	svc, _ := setupOfficeTest(t)
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
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		Title:                  "Agent1 Task",
		ProjectID:              "proj-1",
		AssigneeAgentProfileID: "agent-1",
	})
	_, _ = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		Title:                  "Agent2 Task",
		ProjectID:              "proj-1",
		AssigneeAgentProfileID: "agent-2",
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

func (m *mockBlockerRepo) ListTasksBlockedBy(_ context.Context, blockerTaskID string) ([]string, error) {
	var ids []string
	for _, b := range m.blockers {
		if b.BlockerTaskID == blockerTaskID {
			ids = append(ids, b.TaskID)
		}
	}
	return ids, nil
}

func (m *mockBlockerRepo) ListBlockersForTasks(_ context.Context, taskIDs []string) (map[string][]string, error) {
	want := make(map[string]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		want[id] = struct{}{}
	}
	out := map[string][]string{}
	for _, b := range m.blockers {
		if _, ok := want[b.TaskID]; ok {
			out[b.TaskID] = append(out[b.TaskID], b.BlockerTaskID)
		}
	}
	return out, nil
}

func (m *mockBlockerRepo) ListDependentsForTasks(_ context.Context, blockerTaskIDs []string) (map[string][]string, error) {
	want := make(map[string]struct{}, len(blockerTaskIDs))
	for _, id := range blockerTaskIDs {
		want[id] = struct{}{}
	}
	out := map[string][]string{}
	for _, b := range m.blockers {
		if _, ok := want[b.BlockerTaskID]; ok {
			out[b.BlockerTaskID] = append(out[b.BlockerTaskID], b.TaskID)
		}
	}
	return out, nil
}

func TestBlocker_CRUD(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	// Both ends must exist: the single edge validator resolves each task so it
	// can enforce the same-workspace rule and refuse an edge to a task that is
	// not there (which would leave the dependent blocked forever).
	one := mustSeedTask(t, svc, "Task 1")
	two := mustSeedTask(t, svc, "Task 2")

	if err := svc.AddBlocker(ctx, one.ID, two.ID); err != nil {
		t.Fatalf("AddBlocker: %v", err)
	}

	ids, err := svc.GetBlockers(ctx, one.ID)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(ids) != 1 || ids[0] != two.ID {
		t.Errorf("expected [%s], got %v", two.ID, ids)
	}

	if err := svc.RemoveBlocker(ctx, one.ID, two.ID); err != nil {
		t.Fatalf("RemoveBlocker: %v", err)
	}
	ids, _ = svc.GetBlockers(ctx, one.ID)
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
}

// mustSeedTask creates a task in the office test workspace so dependency-edge
// tests have real IDs on both ends of an edge.
func mustSeedTask(t *testing.T, svc *Service, title string) *models.Task {
	t.Helper()
	result, err := svc.CreateTask(context.Background(), &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: title, ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("seed task %q: %v", title, err)
	}
	return result.Task
}

func TestGetBlocking_ReverseLookup(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()
	t1 := mustSeedTask(t, svc, "Task 1").ID
	t2 := mustSeedTask(t, svc, "Task 2").ID
	t3 := mustSeedTask(t, svc, "Task 3").ID
	t4 := mustSeedTask(t, svc, "Task 4").ID

	// No blocker edges at all.
	ids, err := svc.GetBlocking(ctx, t1)
	if err != nil {
		t.Fatalf("GetBlocking: %v", err)
	}
	if ids == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(ids) != 0 {
		t.Errorf("expected no blocked tasks, got %v", ids)
	}

	// One blocked task: task-2 is blocked by task-1.
	if err := svc.AddBlocker(ctx, t2, t1); err != nil {
		t.Fatalf("AddBlocker: %v", err)
	}
	ids, err = svc.GetBlocking(ctx, t1)
	if err != nil {
		t.Fatalf("GetBlocking: %v", err)
	}
	if len(ids) != 1 || ids[0] != t2 {
		t.Errorf("expected [task-2], got %v", ids)
	}

	// The reverse direction must not be confused with the forward one:
	// task-2 blocks nothing, and task-1 is the blocker of task-2.
	if ids, err = svc.GetBlocking(ctx, t2); err != nil || len(ids) != 0 {
		t.Errorf("GetBlocking(task-2) = %v, %v; want empty", ids, err)
	}
	if ids, err = svc.GetBlockers(ctx, t2); err != nil || len(ids) != 1 || ids[0] != t1 {
		t.Errorf("GetBlockers(task-2) = %v, %v; want [task-1]", ids, err)
	}

	// Several blocked tasks.
	for _, blocked := range []string{t3, t4} {
		if err := svc.AddBlocker(ctx, blocked, t1); err != nil {
			t.Fatalf("AddBlocker(%s): %v", blocked, err)
		}
	}
	ids, err = svc.GetBlocking(ctx, t1)
	if err != nil {
		t.Fatalf("GetBlocking: %v", err)
	}
	sort.Strings(ids)
	want := []string{t2, t3, t4}
	sort.Strings(want)
	if !slices.Equal(ids, want) {
		t.Errorf("expected %v, got %v", want, ids)
	}

	// Removing an edge removes it from the reverse lookup too.
	if err := svc.RemoveBlocker(ctx, t3, t1); err != nil {
		t.Fatalf("RemoveBlocker: %v", err)
	}
	ids, _ = svc.GetBlocking(ctx, t1)
	sort.Strings(ids)
	wantAfter := []string{t2, t4}
	sort.Strings(wantAfter)
	if !slices.Equal(ids, wantAfter) {
		t.Errorf("expected %v after removal, got %v", wantAfter, ids)
	}
}

func TestGetBlocking_NoRepositoryConfigured(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	if _, err := svc.GetBlocking(ctx, "task-1"); err == nil {
		t.Fatal("expected error when blocker repository is not configured")
	}
}

// Archiving a task does not drop it from the reverse lookup: GetBlocking
// mirrors GetBlockers and reports the raw blocker edges. Callers that only
// want active tasks filter themselves.
func TestGetBlocking_IncludesArchivedTasks(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	blockerResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Blocker", ProjectID: "proj-1",
	})
	blocker := blockerResult.Task
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	blockedResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Blocked", ProjectID: "proj-1",
		BlockedBy: []string{blocker.ID},
	})
	blocked := blockedResult.Task
	if err != nil {
		t.Fatalf("create blocked task: %v", err)
	}

	ids, err := svc.GetBlocking(ctx, blocker.ID)
	if err != nil {
		t.Fatalf("GetBlocking: %v", err)
	}
	if len(ids) != 1 || ids[0] != blocked.ID {
		t.Fatalf("expected [%s], got %v", blocked.ID, ids)
	}

	if err := repo.ArchiveTask(ctx, blocked.ID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	ids, err = svc.GetBlocking(ctx, blocker.ID)
	if err != nil {
		t.Fatalf("GetBlocking after archive: %v", err)
	}
	if len(ids) != 1 || ids[0] != blocked.ID {
		t.Errorf("expected archived task %s to still be reported, got %v", blocked.ID, ids)
	}
}

func TestBlocker_CircularDetection(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()
	a := mustSeedTask(t, svc, "Task A").ID
	b := mustSeedTask(t, svc, "Task B").ID
	c := mustSeedTask(t, svc, "Task C").ID

	if err := svc.AddBlocker(ctx, a, b); err != nil {
		t.Fatalf("AddBlocker(a,b): %v", err)
	}
	if err := svc.AddBlocker(ctx, b, c); err != nil {
		t.Fatalf("AddBlocker(b,c): %v", err)
	}

	err := svc.AddBlocker(ctx, c, a)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	// The error must carry the traversal path so the UI can render
	// "A → B → C → A" rather than a bare "there is a cycle".
	var cycle *CycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("expected *CycleError, got %T: %v", err, err)
	}
	if len(cycle.Path) < 3 {
		t.Errorf("expected a cycle path naming the loop, got %v", cycle.Path)
	}
	if cycle.Path[0] != c || cycle.Path[len(cycle.Path)-1] != c {
		t.Errorf("cycle path should start and end at the new dependent %s, got %v", c, cycle.Path)
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
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	// Create two blocker tasks first.
	blocker1Result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Blocker 1", ProjectID: "proj-1",
	})
	blocker1 := blocker1Result.Task
	if err != nil {
		t.Fatalf("create blocker1: %v", err)
	}
	blocker2Result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Blocker 2", ProjectID: "proj-1",
	})
	blocker2 := blocker2Result.Task
	if err != nil {
		t.Fatalf("create blocker2: %v", err)
	}

	// Create a task blocked by both.
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Blocked Task",
		ProjectID:   "proj-1",
		BlockedBy:   []string{blocker1.ID, blocker2.ID},
	})
	task := taskResult.Task
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
	svc, _ := setupOfficeTest(t)
	svc.SetBlockerRepository(&mockBlockerRepo{})
	ctx := context.Background()

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "No Blockers",
		ProjectID:   "proj-1",
		BlockedBy:   []string{},
	})
	task := taskResult.Task
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

func TestCreateTask_OfficeFields_Roundtrip(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		Title:                  "Full Office Task",
		ProjectID:              "proj-1",
		AssigneeAgentProfileID: "agent-1",
		Origin:                 models.TaskOriginRoutine,
		Labels:                 `["bug","urgent"]`,
	})
	task := taskResult.Task
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
	if got.AssigneeAgentProfileID != "agent-1" {
		t.Errorf("assignee: got %s, want agent-1", got.AssigneeAgentProfileID)
	}
	if got.Origin != models.TaskOriginRoutine {
		t.Errorf("origin: got %s, want routine", got.Origin)
	}
	if got.Labels != `["bug","urgent"]` {
		t.Errorf("labels: got %s", got.Labels)
	}
	if got.Identifier == "" {
		t.Error("expected identifier")
	}
}
