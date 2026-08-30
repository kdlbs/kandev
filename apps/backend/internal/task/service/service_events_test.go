package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
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

func TestTaskPublication_PreservesSameSecondActivityOrdering(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	messageAt := time.Date(2026, 8, 18, 10, 20, 0, 100_000_000, time.UTC)
	taskUpdatedAt := messageAt.Add(800 * time.Millisecond)

	svc.publishTaskEventNow(context.Background(), events.TaskUpdated, &models.Task{
		ID:          "task-precision",
		WorkspaceID: "ws-1",
		CreatedAt:   messageAt.Add(-time.Minute),
		UpdatedAt:   taskUpdatedAt,
	}, nil, nil, nil, nil)

	published := eventBus.GetPublishedEvents()
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", published[0].Data)
	}
	updatedAt, ok := data["updated_at"].(string)
	if !ok {
		t.Fatalf("updated_at type = %T, want string", data["updated_at"])
	}
	parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t.Fatalf("parse updated_at %q: %v", updatedAt, err)
	}
	if !parsed.Equal(taskUpdatedAt) {
		t.Fatalf("published updated_at = %s, want %s", parsed, taskUpdatedAt)
	}
	if !parsed.After(messageAt) {
		t.Fatalf("task mutation at %s was not ordered after same-second message at %s", parsed, messageAt)
	}
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
	// The queue's tombstone deliberately survives an idle drain: it stays in
	// the map, marked deleted, so a later stale publication for this task ID
	// is dropped instead of silently recreating an un-tombstoned queue.
	svc.taskPublicationMu.Lock()
	queue, ok := svc.taskPublications["task-1"]
	svc.taskPublicationMu.Unlock()
	if !ok {
		t.Fatal("deleted task's publication queue should remain as a tombstone")
	}
	if !queue.deleted || queue.draining || len(queue.pending) != 0 {
		t.Fatalf("tombstoned queue = %#v, want deleted=true, idle, empty", queue)
	}
	if got := len(eventBus.GetPublishedEvents()); got != 1 {
		t.Fatalf("deleted publication count = %d, want 1", got)
	}
}

// TestTaskPublication_StaleUpdateAfterDeletionIsDropped is the maintainer's
// exact repro: task.updated -> task.deleted -> a stale task.updated. Before
// the tombstone, drainTaskPublications deleted the queue's map entry once
// idle, so the stale update recreated a fresh, un-tombstoned queue and
// published normally — resurrecting a task the operator had just deleted on
// the frontend, which upserts whatever it receives last.
func TestTaskPublication_StaleUpdateAfterDeletionIsDropped(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "update"})
	svc.PublishTaskDeleted(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "stale-after-delete"})

	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published %d events, want 2 (update, deleted); stale post-deletion update must be dropped: %#v", len(published), published)
	}
	for index, want := range []string{events.TaskUpdated, events.TaskDeleted} {
		if published[index].Type != want {
			t.Fatalf("event %d subject = %q, want %q", index, published[index].Type, want)
		}
	}
}

// TestTaskPublication_PendingEntriesQueuedBeforeDeletionAreDropped covers the
// second repro leg: publications already queued (but not yet drained) behind
// a blocked in-flight publication are moot once a task.deleted for the same
// task lands — only the deletion publication itself should survive.
func TestTaskPublication_PendingEntriesQueuedBeforeDeletionAreDropped(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	barrier := &taskPublicationBarrierBus{
		MockEventBus: eventBus,
		entered:      make(chan struct{}, 1),
		release:      make(chan struct{}),
	}
	svc.eventBus = barrier

	blockedDone := make(chan struct{})
	go func() {
		svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "ordinary"})
		close(blockedDone)
	}()
	<-barrier.entered

	// These two updates queue behind the blocked "ordinary" publication.
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "queued-1"})
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "queued-2"})
	svc.PublishTaskDeleted(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})

	close(barrier.release)
	<-blockedDone

	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published %d events, want 2 (ordinary, deleted); pending updates queued before deletion must be dropped: %#v", len(published), published)
	}
	firstData, _ := published[0].Data.(map[string]interface{})
	if firstData["title"] != "ordinary" {
		t.Fatalf("event 0 title = %#v, want %q", firstData["title"], "ordinary")
	}
	if published[1].Type != events.TaskDeleted {
		t.Fatalf("event 1 subject = %q, want %q", published[1].Type, events.TaskDeleted)
	}
}

// TestTaskPublication_TaskCreatedAfterTombstoneClearsAndPublishes covers
// theoretical ID reuse: a task.created enqueue for a tombstoned task ID must
// clear the tombstone and publish normally again.
func TestTaskPublication_TaskCreatedAfterTombstoneClearsAndPublishes(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	svc.PublishTaskDeleted(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	eventBus.ClearEvents()

	svc.enqueueTaskPublication(ctx, "task-1", events.TaskCreated, func(publicationCtx context.Context) {
		svc.publishTaskEventNow(publicationCtx, events.TaskCreated, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"}, nil, nil, nil, nil)
	})

	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != events.TaskCreated {
		t.Fatalf("task.created after tombstone did not publish: %#v", published)
	}

	// A subsequent update now publishes normally: the tombstone is cleared.
	// (The queue itself drains back out of the map like any non-tombstoned
	// queue once idle — the tombstone's effect is that task.created was
	// accepted at all, and that this and later publications are not dropped.)
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "after-recreate"})
	published = eventBus.GetPublishedEvents()
	if len(published) != 2 || published[1].Type != events.TaskUpdated {
		t.Fatalf("update after task.created recreate did not publish: %#v", published)
	}
}

func TestMessageEventChangesPendingActionForAuthorityMutations(t *testing.T) {
	tests := []struct {
		name        string
		eventType   string
		messageType models.MessageType
		want        bool
	}{
		{name: "ordinary add can establish turn authority", eventType: events.MessageAdded, messageType: models.MessageTypeMessage, want: true},
		{name: "ordinary delete can remove turn authority", eventType: events.MessageDeleted, messageType: models.MessageTypeAgentPlan, want: true},
		{name: "ordinary update preserves turn authority", eventType: events.MessageUpdated, messageType: models.MessageTypeMessage, want: false},
		{name: "clarification add changes pending action", eventType: events.MessageAdded, messageType: models.MessageTypeClarificationRequest, want: true},
		{name: "clarification update changes pending action", eventType: events.MessageUpdated, messageType: models.MessageTypeClarificationRequest, want: true},
		{name: "permission delete changes pending action", eventType: events.MessageDeleted, messageType: models.MessageTypePermissionRequest, want: true},
		{name: "unrelated event", eventType: events.TurnStarted, messageType: models.MessageTypeClarificationRequest, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messageEventChangesPendingAction(tt.eventType, &models.Message{Type: tt.messageType}); got != tt.want {
				t.Fatalf("messageEventChangesPendingAction(%s, %s) = %t, want %t", tt.eventType, tt.messageType, got, tt.want)
			}
		})
	}
}

func TestTaskPublication_TaskUpdatedIncludesNullArchivedAtForActiveTasks(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	svc.PublishTaskUpdated(ctx, &models.Task{
		ID:             "task-1",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
	})

	published := eventBus.GetPublishedEvents()
	if len(published) != 1 {
		t.Fatalf("published events = %d, want 1", len(published))
	}
	data, _ := published[0].Data.(map[string]interface{})
	archivedAt, ok := data["archived_at"]
	if !ok {
		t.Fatal("task.updated payload omitted archived_at")
	}
	if archivedAt != nil {
		t.Fatalf("active task archived_at = %#v, want nil", archivedAt)
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

func TestPublishTaskUpdated_EmitsAutopilot(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.PublishTaskUpdated(context.Background(), &models.Task{
		ID: "task-autopilot", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Autopilot: true,
	})

	data := singlePublishedEventData(t, eventBus)
	if got, ok := data["autopilot"].(bool); !ok || !got {
		t.Fatalf("autopilot payload = %#v, want true", data["autopilot"])
	}
}

func TestPublishTaskUpdatedRedactsDeferredLaunchAttribution(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	task := &models.Task{
		ID: "task-private-attribution", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				models.DeferredLaunchUserIDKey:          "user-private",
				models.DeferredLaunchRecordRecentUseKey: true,
			},
		},
	}

	svc.PublishTaskUpdated(context.Background(), task)

	data := singlePublishedEventData(t, eventBus)
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("event metadata = %#v, want a map", data["metadata"])
	}
	launch, ok := metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok {
		t.Fatalf("event deferred launch metadata = %#v, want a map", metadata[models.MetaKeyDeferredLaunch])
	}
	if _, exists := launch[models.DeferredLaunchUserIDKey]; exists {
		t.Fatalf("event exposed deferred launch user_id = %v", launch[models.DeferredLaunchUserIDKey])
	}
	if _, exists := launch[models.DeferredLaunchRecordRecentUseKey]; exists {
		t.Fatalf("event exposed deferred launch recency marker = %v", launch[models.DeferredLaunchRecordRecentUseKey])
	}
	if got := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})[models.DeferredLaunchUserIDKey]; got != "user-private" {
		t.Fatalf("redacting the event mutated the source task user_id = %v", got)
	}
}

// TestPublishTaskUpdated_EmitsAutoStartFailed regression-tests the WS gap
// found in Review round 2: setTaskAutoStartFailedMarker /
// clearTaskAutoStartFailedMarker both call PublishTaskUpdated expecting open
// clients to see the flip, but publishTaskEventNow hand-builds the payload
// map and never carried auto_start_failed, so the publishes were dead code
// and the badge only ever appeared after a reload (REST snapshot).
func TestPublishTaskUpdated_EmitsAutoStartFailed(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.PublishTaskUpdated(context.Background(), &models.Task{
		ID: "task-auto-start-failed", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Metadata: map[string]interface{}{models.MetaKeyAutoStartFailed: true},
	})

	data := singlePublishedEventData(t, eventBus)
	if got, ok := data["auto_start_failed"].(bool); !ok || !got {
		t.Fatalf("auto_start_failed payload = %#v, want true", data["auto_start_failed"])
	}
}

// TestPublishTaskUpdated_EmitsAutoStartFailedFalseWhenCleared proves the
// clear path also propagates: a task with no MetaKeyAutoStartFailed key
// publishes an explicit `false`, not an omitted field, so
// preserveOmittedField on the frontend does not pin the stale `true` from a
// previous failure.
func TestPublishTaskUpdated_EmitsAutoStartFailedFalseWhenCleared(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.PublishTaskUpdated(context.Background(), &models.Task{
		ID: "task-auto-start-cleared", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})

	data := singlePublishedEventData(t, eventBus)
	got, ok := data["auto_start_failed"].(bool)
	if !ok {
		t.Fatalf("auto_start_failed payload missing or wrong type: %#v", data["auto_start_failed"])
	}
	if got {
		t.Fatalf("auto_start_failed payload = true, want false for a task without the marker")
	}
}

func TestPublishWorkspaceSourcesAdoptedPublishesSessionScopedPayload(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	eventBus.ClearEvents()

	svc.PublishWorkspaceSourcesAdopted(context.Background(), "task-1", "/workspaces/task-1", []string{"session-1"})

	data := singlePublishedEventData(t, eventBus)
	if data["task_id"] != "task-1" || data["session_id"] != "session-1" || data["workspace_path"] != "/workspaces/task-1" {
		t.Fatalf("unexpected source-adoption payload: %#v", data)
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

func TestPublishTaskUpdated_EmitsWorkspaceFolders(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	createTaskWithoutRepositories(t, ctx, repo)
	if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{TaskID: "task-1", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{
		LocalPath: "/canonical/docs", DisplayName: "docs",
	}}}}); err != nil {
		t.Fatalf("create workspace folder: %v", err)
	}
	svc.workspaceFolders = repo
	eventBus.ClearEvents()

	svc.PublishTaskUpdated(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})

	data := singlePublishedEventData(t, eventBus)
	folders, ok := data["workspace_folders"].([]map[string]interface{})
	if !ok {
		t.Fatalf("workspace_folders missing from payload or wrong type: %#v", data["workspace_folders"])
	}
	if len(folders) != 1 || folders[0]["local_path"] != "/canonical/docs" {
		t.Fatalf("workspace_folders = %#v, want canonical docs folder", folders)
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

// panicOnceEventBus panics on its first Publish call (simulating a
// synchronous subscriber panic reaching the EventBus boundary) and behaves
// normally afterward.
type panicOnceEventBus struct {
	*MockEventBus
	mu       sync.Mutex
	panicked bool
}

func (b *panicOnceEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	shouldPanic := !b.panicked
	b.panicked = true
	b.mu.Unlock()
	if shouldPanic {
		panic("boom: synchronous subscriber panic")
	}
	return b.MockEventBus.Publish(ctx, subject, event)
}

// TestTaskPublication_QueueRecoversDrainingAfterSubscriberPanic guards
// against the publication queue getting stuck "draining" forever: a panic
// from a synchronous EventBus subscriber (recovered by the caller, matching
// how a panic recovered higher up the stack would behave in production —
// MemoryEventBus.Publish itself has no recover) must not leave later
// publications for the same task silently un-delivered.
func TestTaskPublication_QueueRecoversDrainingAfterSubscriberPanic(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	panicBus := &panicOnceEventBus{MockEventBus: eventBus}
	svc.eventBus = panicBus

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected the first publication's subscriber panic to propagate")
			}
		}()
		svc.PublishTaskUpdated(ctx, &models.Task{
			ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "first",
		})
	}()

	// Draining is released synchronously by the recover in
	// drainTaskPublications before the panic re-propagates above, so this
	// second publication for the same task must drain immediately rather
	// than being silently swallowed by a queue stuck "draining".
	svc.PublishTaskUpdated(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "second",
	})

	published := eventBus.GetPublishedEvents()
	if len(published) != 1 {
		t.Fatalf("expected exactly 1 delivered event after panic recovery, got %d", len(published))
	}
	data, _ := published[0].Data.(map[string]interface{})
	if data["title"] != "second" {
		t.Fatalf("delivered event title = %#v, want %q", data["title"], "second")
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
