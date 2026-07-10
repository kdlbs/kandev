package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type failingTaskRepoRepository struct {
	repository.TaskRepoRepository
	err error
}

type primarySessionInfoRepository struct {
	repository.SessionRepository
	info map[string]*models.TaskSession
}

type taskEventTestRepository interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}

func (r failingTaskRepoRepository) ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
	return nil, r.err
}

func (r primarySessionInfoRepository) GetPrimarySessionInfoByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	return r.info, nil
}

func (r primarySessionInfoRepository) GetSessionCountsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error) {
	return map[string]int{}, nil
}

// TestPublishTaskUpdated_FallbackRepositoryID exercises the DB fallback in
// taskRepositoriesForEvent: orchestrator-originated events load the task via the
// raw repo.GetTask, which does not populate Repositories. The publisher must
// still emit repository_id so the frontend doesn't lose the repo link on
// workflow transitions or state changes.
func TestPublishTaskUpdated_FallbackRepositoryID(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-x", WorkspaceID: "ws-1", Name: "Repo"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		TaskID: "task-1", RepositoryID: "repo-x", BaseBranch: "main",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	eventBus.ClearEvents()

	task := &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	}
	if len(task.Repositories) != 0 {
		t.Fatal("pre-condition: task.Repositories must be nil for this test")
	}
	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	got, ok := data["repository_id"].(string)
	if !ok {
		t.Fatalf("repository_id missing from payload or wrong type: %#v", data["repository_id"])
	}
	if got != "repo-x" {
		t.Fatalf("expected repository_id=repo-x via DB fallback, got %q", got)
	}
	repos, ok := data["repositories"].([]map[string]interface{})
	if !ok {
		t.Fatalf("repositories missing from payload or wrong type: %#v", data["repositories"])
	}
	if len(repos) != 1 || repos[0]["repository_id"] != "repo-x" {
		t.Fatalf("expected repositories payload with repo-x, got %#v", repos)
	}
}

func TestPublishTaskUpdated_EmitsEmptyRepositories(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	createTaskWithoutRepositories(t, ctx, repo)
	eventBus.ClearEvents()

	svc.PublishTaskUpdated(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})

	data := singlePublishedEventData(t, eventBus)
	if _, ok := data["repository_id"]; ok {
		t.Fatalf("repository_id should be absent for tasks with no repositories: %#v", data["repository_id"])
	}
	repos, ok := data["repositories"].([]map[string]interface{})
	if !ok {
		t.Fatalf("repositories missing from payload or wrong type: %#v", data["repositories"])
	}
	if len(repos) != 0 {
		t.Fatalf("expected empty repositories payload, got %#v", repos)
	}
}

func TestPublishTaskUpdated_EmitsNullPrimarySessionFieldsWhenNoPrimaryExists(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	createTaskWithoutRepositories(t, ctx, repo)
	eventBus.ClearEvents()

	svc.PublishTaskUpdated(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})

	data := singlePublishedEventData(t, eventBus)
	if value, ok := data["primary_session_id"]; !ok || value != nil {
		t.Fatalf("primary_session_id = %#v, want explicit nil", value)
	}
	if value, ok := data["primary_session_state"]; !ok || value != nil {
		t.Fatalf("primary_session_state = %#v, want explicit nil", value)
	}
	if value, ok := data["primary_session_pending_action"]; !ok || value != nil {
		t.Fatalf("primary_session_pending_action = %#v, want explicit nil", value)
	}
}

func TestPublishTaskUpdated_EmitsPrimarySessionPendingAction(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	task := &models.Task{
		ID:             "task-1",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "T",
		Priority:       "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "session-1",
		TaskID:    task.ID,
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-1",
		TaskSessionID: "session-1",
		TaskID:        task.ID,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-1",
		TaskSessionID: "session-1",
		TaskID:        task.ID,
		TurnID:        "turn-1",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "question",
		Type:          models.MessageTypeClarificationRequest,
		Metadata:      map[string]interface{}{"status": "pending"},
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	eventBus.ClearEvents()

	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	if value := data["primary_session_pending_action"]; value != "clarification" {
		t.Fatalf("primary_session_pending_action = %#v, want clarification", value)
	}
}

func TestAddTaskSessionEventFields_EmitsNullPrimarySessionStateWhenEmpty(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.sessions = primarySessionInfoRepository{
		info: map[string]*models.TaskSession{
			"task-1": {ID: "session-1", TaskID: "task-1"},
		},
	}
	data := map[string]interface{}{}

	svc.addTaskSessionEventFields(context.Background(), "task-1", data)

	if value := data["primary_session_id"]; value != "session-1" {
		t.Fatalf("primary_session_id = %#v, want session-1", value)
	}
	if value, ok := data["primary_session_state"]; !ok || value != nil {
		t.Fatalf("primary_session_state = %#v, want explicit nil", value)
	}
}

func TestPublishTaskUpdated_OmitsRepositoriesOnLookupError(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	createTaskWithoutRepositories(t, ctx, repo)
	svc.taskRepos = failingTaskRepoRepository{
		TaskRepoRepository: repo,
		err:                errors.New("repository lookup failed"),
	}
	eventBus.ClearEvents()

	svc.PublishTaskUpdated(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})

	data := singlePublishedEventData(t, eventBus)
	if _, ok := data["repository_id"]; ok {
		t.Fatalf("repository_id should be absent when repository lookup fails: %#v", data["repository_id"])
	}
	if _, ok := data["repositories"]; ok {
		t.Fatalf("repositories should be absent when repository lookup fails: %#v", data["repositories"])
	}
}

func createTaskWithoutRepositories(t *testing.T, ctx context.Context, repo taskEventTestRepository) {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	task := &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
}

func singlePublishedEventData(t *testing.T, eventBus *MockEventBus) map[string]interface{} {
	t.Helper()
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(events))
	}
	data, ok := events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event Data wrong type: %T", events[0].Data)
	}
	return data
}
