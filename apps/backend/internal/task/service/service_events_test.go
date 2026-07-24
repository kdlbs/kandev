package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// taskPublicationBarrierBus blocks one lifecycle publication at the EventBus
// boundary. It deliberately does not hold MockEventBus's mutex while blocked so
// a competing Publish exposes whether Service serializes the task itself.
type taskPublicationBarrierBus struct {
	*MockEventBus
	entered chan struct{}
	release chan struct{}

	mu            sync.Mutex
	blocked       bool
	failNext      bool
	reenter       func()
	contextValue  func(context.Context) any
	contextValues []any
}

type cancellationAwareSessionRepository struct {
	repository.SessionRepository
}

func (r cancellationAwareSessionRepository) ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.SessionRepository.ListActiveTaskSessionsByTaskID(ctx, taskID)
}

func (b *taskPublicationBarrierBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	data, _ := event.Data.(map[string]interface{})
	title, _ := data["title"].(string)

	b.mu.Lock()
	if b.contextValue != nil {
		b.contextValues = append(b.contextValues, b.contextValue(ctx))
	}
	block := title == "ordinary" && !b.blocked
	if block {
		b.blocked = true
	}
	reenter := b.reenter
	b.reenter = nil
	b.mu.Unlock()

	if block {
		b.entered <- struct{}{}
		<-b.release
	}
	if reenter != nil {
		reenter()
	}
	b.mu.Lock()
	fail := b.failNext
	b.failNext = false
	b.mu.Unlock()
	if fail {
		return errors.New("publish failed")
	}
	return b.MockEventBus.Publish(ctx, subject, event)
}

func TestTaskPublication_ActivityRefreshDoesNotOvertakeOrdinaryUpdate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "session-1", "task-1", models.TaskSessionStateRunning)
	provider := &fakeActivityProvider{byID: map[string]v1.ForegroundActivity{
		"session-1": v1.ForegroundActivityBackground,
	}}
	svc.SetForegroundActivityProvider(provider)
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	eventBus.ClearEvents()

	barrier := &taskPublicationBarrierBus{
		MockEventBus: eventBus,
		entered:      make(chan struct{}, 1),
		release:      make(chan struct{}),
	}
	svc.eventBus = barrier

	ordinaryDone := make(chan struct{})
	go func() {
		svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "ordinary"})
		close(ordinaryDone)
	}()
	<-barrier.entered

	provider.byID["session-1"] = v1.ForegroundActivityGenerating
	activityDone := make(chan struct{})
	go func() {
		svc.PublishTaskActivityIfChanged(ctx, "task-1")
		close(activityDone)
	}()

	select {
	case <-activityDone:
	case <-time.After(time.Second):
		t.Fatal("activity refresh did not return after queueing behind the ordinary update")
	}
	if published := eventBus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("activity refresh overtook the blocked ordinary task update: %#v", published)
	}

	close(barrier.release)
	<-ordinaryDone
	<-activityDone
}

func TestTaskPublication_QueuedActivityOutlivesCallerCancellation(t *testing.T) {
	svc, eventBus, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) repository.SessionRepository {
		return cancellationAwareSessionRepository{SessionRepository: repo}
	})
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "session-1", "task-1", models.TaskSessionStateRunning)
	if err := svc.SetPrimarySession(ctx, "session-1"); err != nil {
		t.Fatalf("SetPrimarySession: %v", err)
	}
	provider := &fakeActivityProvider{byID: map[string]v1.ForegroundActivity{
		"session-1": v1.ForegroundActivityBackground,
	}}
	svc.SetForegroundActivityProvider(provider)

	barrier := &taskPublicationBarrierBus{
		MockEventBus: eventBus,
		entered:      make(chan struct{}, 1),
		release:      make(chan struct{}),
	}
	svc.eventBus = barrier

	ordinaryDone := make(chan struct{})
	go func() {
		svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "ordinary"})
		close(ordinaryDone)
	}()
	<-barrier.entered

	provider.byID["session-1"] = v1.ForegroundActivityGenerating
	type publicationContextKey struct{}
	activityCtx, cancelActivity := context.WithCancel(context.WithValue(context.Background(), publicationContextKey{}, "retained"))
	barrier.contextValue = func(ctx context.Context) any { return ctx.Value(publicationContextKey{}) }
	activityDone := make(chan struct{})
	go func() {
		svc.PublishTaskActivityIfChanged(activityCtx, "task-1")
		close(activityDone)
	}()
	select {
	case <-activityDone:
	case <-time.After(time.Second):
		t.Fatal("activity refresh did not return after queueing")
	}
	cancelActivity()
	close(barrier.release)
	<-ordinaryDone

	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published %d events, want ordinary update followed by queued activity update", len(published))
	}
	for index, event := range published {
		data, _ := event.Data.(map[string]interface{})
		if got := data["primary_session_id"]; got != "session-1" {
			t.Fatalf("event %d primary_session_id = %#v, want session-1", index, got)
		}
		if got := data["session_count"]; got != 1 {
			t.Fatalf("event %d session_count = %#v, want 1", index, got)
		}
	}
	activityData, _ := published[1].Data.(map[string]interface{})
	if got := activityData["foreground_activity"]; got != "generating" {
		t.Fatalf("queued activity foreground_activity = %#v, want generating", got)
	}
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if got := barrier.contextValues[0]; got != "retained" {
		t.Fatalf("queued activity context value = %#v, want retained", got)
	}
}

func TestTaskPublication_ReentrantSameTaskPublishesAfterOuterEvent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	barrier := &taskPublicationBarrierBus{MockEventBus: eventBus}
	barrier.reenter = func() {
		svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "inner"})
	}
	svc.eventBus = barrier

	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "outer"})

	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published %d events, want 2", len(published))
	}
	for index, want := range []string{"outer", "inner"} {
		data, _ := published[index].Data.(map[string]interface{})
		if got := data["title"]; got != want {
			t.Fatalf("event %d title = %#v, want %q", index, got, want)
		}
	}
}

func TestTaskPublication_DifferentTasksDrainIndependently(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "second"}); err != nil {
		t.Fatalf("CreateTask(task-2): %v", err)
	}
	barrier := &taskPublicationBarrierBus{
		MockEventBus: eventBus,
		entered:      make(chan struct{}, 1),
		release:      make(chan struct{}),
	}
	svc.eventBus = barrier

	firstDone := make(chan struct{})
	go func() {
		svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "ordinary"})
		close(firstDone)
	}()
	<-barrier.entered

	secondDone := make(chan struct{})
	go func() {
		svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "second"})
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("task-2 publication waited for blocked task-1 publication")
	}
	if published := eventBus.GetPublishedEvents(); len(published) != 1 {
		t.Fatalf("independent task publication count = %d, want 1", len(published))
	}

	close(barrier.release)
	<-firstDone
}

func TestTaskPublication_FailedActivityRefreshRetriesSameAggregate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "session-1", "task-1", models.TaskSessionStateRunning)
	svc.SetForegroundActivityProvider(&fakeActivityProvider{byID: map[string]v1.ForegroundActivity{
		"session-1": v1.ForegroundActivityBackground,
	}})
	svc.eventBus = &taskPublicationBarrierBus{MockEventBus: eventBus, failNext: true}

	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if _, seen := svc.lastTaskActivity["task-1"]; seen {
		t.Fatal("failed activity publication advanced the dedup baseline")
	}
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if got := len(eventBus.GetPublishedEvents()); got != 1 {
		t.Fatalf("same aggregate retry published %d events, want 1", got)
	}
}

func TestTaskPublication_IdleAndDeletedTaskStateAreCleanedUp(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	svc.recordTaskActivity("task-1", v1.ForegroundActivityBackground)

	svc.PublishTaskDeleted(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	if _, seen := svc.lastTaskActivity["task-1"]; seen {
		t.Fatal("task deletion did not clear activity baseline")
	}
	svc.taskPublicationMu.Lock()
	defer svc.taskPublicationMu.Unlock()
	if len(svc.taskPublications) != 0 {
		t.Fatalf("idle publication dispatchers = %#v, want none", svc.taskPublications)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 1 {
		t.Fatalf("deleted publication count = %d, want 1", got)
	}
}

type failingTaskRepoRepository struct {
	repository.TaskRepoRepository
	err error
}

type failingMessageRepository struct {
	repository.MessageRepository
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

func (r failingMessageRepository) GetPendingActionsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error) {
	return nil, r.err
}

func (r primarySessionInfoRepository) GetPrimarySessionInfoByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	return r.info, nil
}

func (r primarySessionInfoRepository) GetSessionCountsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error) {
	return map[string]int{}, nil
}

func (r primarySessionInfoRepository) ListActiveTaskSessionsByTaskID(_ context.Context, taskID string) ([]*models.TaskSession, error) {
	info := r.info[taskID]
	if info == nil {
		return nil, nil
	}
	return []*models.TaskSession{info}, nil
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

func TestPublishTaskUpdated_EmitsTaskPendingPermissionFromSecondarySession(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	requireTaskEventFixture(t, ctx, repo)
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T", Priority: "medium"}
	for _, session := range []*models.TaskSession{
		{ID: "primary", TaskID: task.ID, State: models.TaskSessionStateRunning, IsPrimary: true, StartedAt: now, UpdatedAt: now},
		{ID: "secondary", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now},
		{ID: "stale-starting", TaskID: task.ID, State: models.TaskSessionStateStarting, StartedAt: now, UpdatedAt: now},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
	}
	for _, turn := range []*models.Turn{
		{ID: "primary-turn", TaskSessionID: "primary", TaskID: task.ID},
		{ID: "secondary-turn", TaskSessionID: "secondary", TaskID: task.ID},
		{ID: "stale-turn", TaskSessionID: "stale-starting", TaskID: task.ID},
	} {
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("CreateTurn(%s): %v", turn.ID, err)
		}
	}
	for _, message := range []*models.Message{
		{ID: "primary-clarification", TaskSessionID: "primary", TaskID: task.ID, TurnID: "primary-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest, Metadata: map[string]interface{}{"status": "pending"}, CreatedAt: now},
		{ID: "secondary-permission", TaskSessionID: "secondary", TaskID: task.ID, TurnID: "secondary-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypePermissionRequest, Metadata: map[string]interface{}{"status": "pending"}, CreatedAt: now},
		{ID: "stale-clarification", TaskSessionID: "stale-starting", TaskID: task.ID, TurnID: "stale-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest, Metadata: map[string]interface{}{"status": "pending"}, CreatedAt: now},
	} {
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatalf("CreateMessage(%s): %v", message.ID, err)
		}
	}
	eventBus.ClearEvents()

	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	if value := data["task_pending_action"]; value != "permission" {
		t.Fatalf("task_pending_action = %#v, want permission", value)
	}
}

func requireTaskEventFixture(t *testing.T, ctx context.Context, repo taskEventTestRepository) {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T", Priority: "medium"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
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

func TestAddTaskSessionEventFields_OmitsPendingActionOnLookupErrorForWaitingSession(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.sessions = primarySessionInfoRepository{
		info: map[string]*models.TaskSession{
			"task-1": {
				ID:     "session-1",
				TaskID: "task-1",
				State:  models.TaskSessionStateWaitingForInput,
			},
		},
	}
	svc.messages = failingMessageRepository{err: errors.New("pending lookup failed")}
	data := map[string]interface{}{}

	svc.addTaskSessionEventFields(context.Background(), "task-1", data)

	if value := data["primary_session_id"]; value != "session-1" {
		t.Fatalf("primary_session_id = %#v, want session-1", value)
	}
	if value := data["primary_session_state"]; value != string(models.TaskSessionStateWaitingForInput) {
		t.Fatalf("primary_session_state = %#v, want WAITING_FOR_INPUT", value)
	}
	if value, ok := data["primary_session_pending_action"]; ok {
		t.Fatalf("primary_session_pending_action should be omitted on lookup error, got %#v", value)
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
