package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockEventBus captures published events for assertion.
type mockEventBus struct {
	mu     sync.Mutex
	events []publishedEvent
}

func TestUpdateTransitionTaskWithCapacity_QueuesFullLimitedStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "occupant",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step2",
		Title:          "Occupant",
		State:          "TODO",
		Priority:       "medium",
	}); err != nil {
		t.Fatalf("create occupant: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "step2"
	target := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", WIPLimit: 1}

	err = svc.updateTransitionTaskWithCapacity(ctx, task, target)
	if err != nil {
		t.Fatalf("updateTransitionTaskWithCapacity: %v", err)
	}
	stored, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if stored.WorkflowStepID != "step2" || stored.WIPAdmitted || stored.QueuedForStepID != "step2" {
		t.Fatalf("queued task placement: step=%q admitted=%t queue=%q", stored.WorkflowStepID, stored.WIPAdmitted, stored.QueuedForStepID)
	}
}

func TestExecuteStepTransition_FullTargetRunsExitAndDefersEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.SetSessionMetadataKey(ctx, "s1", "plan_mode", true); err != nil {
		t.Fatalf("enable plan mode: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "occupant",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step2",
		Title:          "Occupant",
		State:          "TODO",
		Priority:       "medium",
	}); err != nil {
		t.Fatalf("create occupant: %v", err)
	}

	steps := newMockStepGetter()
	fromStep := &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{
			Type: wfmodels.OnExitDisablePlanMode,
		}}},
	}
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Limited", WIPLimit: 1,
	}
	svc := createTestService(repo, steps, newMockTaskRepo())

	svc.executeStepTransition(ctx, "t1", "s1", fromStep, "step2", false)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != "step2" || task.WIPAdmitted || task.QueuedForStepID != "step2" {
		t.Fatalf("queued task placement: step=%q admitted=%t queue=%q", task.WorkflowStepID, task.WIPAdmitted, task.QueuedForStepID)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if enabled, _ := session.Metadata["plan_mode"].(bool); enabled {
		t.Fatal("queued transition must run source step's on_exit actions")
	}
}

func TestClaimTaskEventMetadataIsOneShot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "queued-lifecycle",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step2",
		Title:          "Queued lifecycle",
		State:          "TODO",
		CreatedAt:      now,
		UpdatedAt:      now,
		Metadata: map[string]interface{}{
			models.MetaKeyQueuePromotionPending: true,
		},
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	task, err := repo.GetTask(ctx, "queued-lifecycle")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if !svc.claimTaskEventMetadata(ctx, task, models.MetaKeyQueuePromotionPending) {
		t.Fatal("first lifecycle event claim was rejected")
	}

	claimedTask, err := repo.GetTask(ctx, "queued-lifecycle")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if svc.claimTaskEventMetadata(ctx, claimedTask, models.MetaKeyQueuePromotionPending) {
		t.Fatal("replayed lifecycle event was claimed twice")
	}
}

func TestQueuedMoveLifecycleTokenRemainsPendingWhenPrerequisitesFail(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "queued-move-retry", "queued-move-session", "source-step")
	task, err := repo.GetTask(ctx, "queued-move-retry")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "destination-step"
	task.WIPAdmitted = false
	task.QueuedForStepID = "destination-step"
	task.Metadata = map[string]interface{}{models.MetaKeyQueuedMoveExitPending: true}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("queue task: %v", err)
	}

	// The source-step lookup fails before the one-shot token is claimed. A
	// replay after the workflow snapshot is available must still be able to run
	// the source on_exit action.
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID:          task.ID,
		SessionID:       "queued-move-session",
		FromStepID:      "source-step",
		ToStepID:        "destination-step",
		WIPAdmitted:     false,
		QueuedForStepID: "destination-step",
	})

	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := stored.Metadata[models.MetaKeyQueuedMoveExitPending]; !ok {
		t.Fatal("queued move lifecycle token was consumed before prerequisites succeeded")
	}
}

func TestQueuePromotionTokenRemainsPendingWhenTargetLookupFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "promotion-retry", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "missing-target",
		Title: "Promotion retry", State: "TODO", WIPAdmitted: true,
		Metadata: map[string]interface{}{models.MetaKeyQueuePromotionPending: true},
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "promotion-retry"})

	stored, err := repo.GetTask(ctx, "promotion-retry")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := stored.Metadata[models.MetaKeyQueuePromotionPending]; !ok {
		t.Fatal("queue promotion token was consumed before target lookup succeeded")
	}
}

func TestQueuePromotionWithoutSessionCompletesDestinationEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "promotion-no-session", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "destination-step",
		Title: "Promotion without session", State: v1.TaskStateTODO, WIPAdmitted: true,
		Metadata: map[string]interface{}{models.MetaKeyQueuePromotionPending: true},
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination", Position: 0,
	}
	steps.steps["next-step"] = &wfmodels.WorkflowStep{
		ID: "next-step", WorkflowID: "wf1", Name: "Next", Position: 1,
	}
	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "promotion-no-session"})

	stored, err := repo.GetTask(ctx, "promotion-no-session")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyQueuePromotionPending]; pending {
		t.Fatal("queue promotion token remained after destination entry completed without a session")
	}
}

func TestQueuedMoveWithoutSessionCompletesSourceExitBarrier(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "queued-no-session", WorkspaceID: "ws1", WorkflowID: "wf1",
		WorkflowStepID: "destination-step", Title: "Queued", State: v1.TaskStateTODO,
		WIPAdmitted: false, QueuedForStepID: "destination-step",
		Metadata: map[string]interface{}{
			models.MetaKeyQueuedMoveExitPending: map[string]interface{}{"from_step_id": "source-step"},
		},
	}); err != nil {
		t.Fatalf("create queued task: %v", err)
	}
	steps := newMockStepGetter()
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{ID: "destination-step", WorkflowID: "wf1", Name: "Destination"}
	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID: "queued-no-session", FromStepID: "source-step", ToStepID: "destination-step",
		WIPAdmitted: false, QueuedForStepID: "destination-step",
	})

	stored, err := repo.GetTask(ctx, "queued-no-session")
	if err != nil {
		t.Fatalf("reload queued task: %v", err)
	}
	if queuedMoveExitPending(stored) || !queuedMoveExitCompleted(stored) {
		t.Fatalf("source-exit barrier metadata = %#v, want completed", stored.Metadata)
	}
}

func TestQueuedMovePromotionWaitsForSourceExitCompletion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "queued-barrier", "queued-barrier-session", "source-step")
	if err := repo.SetSessionMetadataKey(ctx, "queued-barrier-session", "plan_mode", true); err != nil {
		t.Fatalf("seed source plan mode: %v", err)
	}
	task, err := repo.GetTask(ctx, "queued-barrier")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = "destination-step"
	task.WIPAdmitted = false
	task.QueuedForStepID = "destination-step"
	task.Metadata = map[string]interface{}{
		models.MetaKeyQueuedMoveExitPending: map[string]interface{}{"from_step_id": "source-step"},
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("queue task: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["source-step"] = &wfmodels.WorkflowStep{
		ID: "source-step", WorkflowID: "wf1", Name: "Source",
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}}},
	}
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type:   wfmodels.OnEnterSetSessionMode,
			Config: map[string]interface{}{"mode": "destination"},
		}}},
	}
	svc := createTestService(repo, steps, newMockTaskRepo())
	exitStarted := make(chan struct{})
	releaseExit := make(chan struct{})
	exitCompleted := make(chan struct{})
	entryCompleted := make(chan struct{})
	svc.onQueuedMoveExitStart = func() {
		close(exitStarted)
		<-releaseExit
	}
	svc.onQueuedMoveExitComplete = func() { close(exitCompleted) }
	svc.onTaskQueuePromotionEntryComplete = func() { close(entryCompleted) }

	svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID: "queued-barrier", SessionID: "queued-barrier-session",
		FromStepID: "source-step", ToStepID: "destination-step",
		WIPAdmitted: false, QueuedForStepID: "destination-step",
	})
	select {
	case <-exitStarted:
	case <-time.After(time.Second):
		t.Fatal("source exit did not start")
	}

	promoted, err := repo.GetTask(ctx, "queued-barrier")
	if err != nil {
		t.Fatalf("reload queued task: %v", err)
	}
	promoted.WIPAdmitted = true
	promoted.QueuedForStepID = ""
	promoted.Metadata[models.MetaKeyQueuePromotionPending] = true
	if err := repo.UpdateTask(ctx, promoted); err != nil {
		t.Fatalf("persist promoted task while exit is blocked: %v", err)
	}
	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: promoted.ID})
	select {
	case <-entryCompleted:
		t.Fatal("destination entry started before source exit completed")
	default:
	}

	close(releaseExit)
	select {
	case <-exitCompleted:
	case <-time.After(time.Second):
		t.Fatal("source exit did not complete")
	}
	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: promoted.ID})
	select {
	case <-entryCompleted:
	case <-time.After(time.Second):
		t.Fatal("destination entry did not run after source exit completed")
	}

	session, err := repo.GetTaskSession(ctx, "queued-barrier-session")
	if err != nil {
		t.Fatalf("reload barrier session: %v", err)
	}
	if session.Metadata[models.SessionMetaKeySessionMode] != "destination" {
		t.Fatalf("session mode = %v, want destination", session.Metadata[models.SessionMetaKeySessionMode])
	}
}

type failOnceLifecycleRepo struct {
	sessionExecutorStore
	failGetTaskSession  bool
	failureObserved     chan struct{}
	restorationObserved chan struct{}
}

func (r *failOnceLifecycleRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if r.failGetTaskSession {
		r.failGetTaskSession = false
		close(r.failureObserved)
		return nil, errors.New("destination entry failed once")
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, id)
}

func (r *failOnceLifecycleRepo) SetTaskMetadataKey(ctx context.Context, taskID, key string, value interface{}) error {
	err := r.sessionExecutorStore.SetTaskMetadataKey(ctx, taskID, key, value)
	if err == nil && key == models.MetaKeyQueuePromotionPending && r.restorationObserved != nil {
		close(r.restorationObserved)
	}
	return err
}

func (r *failOnceLifecycleRepo) ListTasksWithMetadataKey(ctx context.Context, key string) ([]*models.Task, error) {
	return r.sessionExecutorStore.(interface {
		ListTasksWithMetadataKey(context.Context, string) ([]*models.Task, error)
	}).ListTasksWithMetadataKey(ctx, key)
}

func TestReconcileTaskLifecycleTokensRetriesDestinationEntryOnce(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "promotion-recovery", "promotion-recovery-session", "destination-step")
	if err := baseRepo.SetTaskMetadataKey(ctx, "promotion-recovery", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	steps := newMockStepGetter()
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{ID: "destination-step", WorkflowID: "wf1", Name: "Destination"}
	svc := createTestService(baseRepo, steps, newMockTaskRepo())
	recoveryRepo := &failOnceLifecycleRepo{
		sessionExecutorStore: baseRepo,
		failGetTaskSession:   true,
		failureObserved:      make(chan struct{}),
		restorationObserved:  make(chan struct{}),
	}
	svc.repo = recoveryRepo
	var entryCalls atomic.Int32
	entryCompleted := make(chan struct{}, 2)
	svc.onTaskQueuePromotionEntryComplete = func() {
		entryCalls.Add(1)
		entryCompleted <- struct{}{}
	}

	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "promotion-recovery"})
	select {
	case <-recoveryRepo.failureObserved:
	case <-time.After(time.Second):
		t.Fatal("initial destination-entry failure did not occur")
	}
	select {
	case <-recoveryRepo.restorationObserved:
	case <-time.After(time.Second):
		t.Fatal("failed destination entry did not restore its lifecycle token")
	}
	svc.reconcileTaskLifecycleTokens(ctx)
	select {
	case <-entryCompleted:
	case <-time.After(2 * time.Second):
		t.Fatal("startup lifecycle recovery did not retry destination entry")
	}
	if got := entryCalls.Load(); got != 1 {
		t.Fatalf("destination entry calls = %d, want exactly 1 successful retry", got)
	}
	stored, err := baseRepo.GetTask(ctx, "promotion-recovery")
	if err != nil {
		t.Fatalf("reload recovered task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyQueuePromotionPending]; pending {
		t.Fatal("queue promotion token remained after recovery")
	}
}

type publishedEvent struct {
	Subject string
	Event   *bus.Event
}

func (m *mockEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, publishedEvent{Subject: subject, Event: event})
	return nil
}

func (m *mockEventBus) Subscribe(_ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) QueueSubscribe(_, _ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) Request(_ context.Context, _ string, _ *bus.Event, _ time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (m *mockEventBus) Close()            {}
func (m *mockEventBus) IsConnected() bool { return true }

func (m *mockEventBus) published() []publishedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]publishedEvent, len(m.events))
	copy(out, m.events)
	return out
}

func TestPublishSessionWaitingEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("includes agent_profile_id and metadata", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Update session with agent profile and metadata.
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		session.AgentProfileID = "profile-auggie"
		_ = repo.UpdateTaskSession(ctx, session)
		_ = repo.UpdateSessionMetadata(ctx, session.ID, map[string]any{"plan_mode": true})
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("failed to update session: %v", err)
		}

		eb := &mockEventBus{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.eventBus = eb

		svc.publishSessionWaitingEvent(ctx, "t1", "s1", "step1")

		published := eb.published()
		if len(published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(published))
		}
		if published[0].Subject != events.TaskSessionStateChanged {
			t.Errorf("expected subject %q, got %q", events.TaskSessionStateChanged, published[0].Subject)
		}

		data, ok := published[0].Event.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected event data to be map[string]any, got %T", published[0].Event.Data)
		}
		if data["task_id"] != "t1" {
			t.Errorf("expected task_id %q, got %q", "t1", data["task_id"])
		}
		if data["session_id"] != "s1" {
			t.Errorf("expected session_id %q, got %q", "s1", data["session_id"])
		}
		if data["new_state"] != string(models.TaskSessionStateWaitingForInput) {
			t.Errorf("expected new_state %q, got %q", models.TaskSessionStateWaitingForInput, data["new_state"])
		}
		session, err = repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("GetTaskSession: %v", err)
		}
		if data["updated_at"] != session.UpdatedAt.UTC().Format(time.RFC3339Nano) {
			t.Errorf("expected updated_at %q, got %q", session.UpdatedAt.UTC().Format(time.RFC3339Nano), data["updated_at"])
		}
		if data["agent_profile_id"] != "profile-auggie" {
			t.Errorf("expected agent_profile_id %q, got %v", "profile-auggie", data["agent_profile_id"])
		}
		if data["session_metadata"] == nil {
			t.Error("expected session_metadata to be set")
		}
	})

	t.Run("omits agent_profile_id when empty", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		eb := &mockEventBus{}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.eventBus = eb

		svc.publishSessionWaitingEvent(ctx, "t1", "s1", "step1")

		published := eb.published()
		if len(published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(published))
		}

		data := published[0].Event.Data.(map[string]any)
		if _, exists := data["agent_profile_id"]; exists {
			t.Errorf("expected agent_profile_id to be absent, got %v", data["agent_profile_id"])
		}
		if _, exists := data["session_metadata"]; exists {
			t.Errorf("expected session_metadata to be absent, got %v", data["session_metadata"])
		}
	})

	t.Run("no-op when eventBus is nil", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// eventBus is nil by default

		// Should not panic.
		svc.publishSessionWaitingEvent(ctx, "t1", "s1", "step1")
	})
}

func TestPublishSessionCreatedEventIncludesUpdatedAt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	eb := &mockEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	svc.publishSessionCreatedEvent(ctx, "t1", "s1", "step1")

	published := eb.published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(published))
	}
	data, ok := published[0].Event.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected event data to be map[string]any, got %T", published[0].Event.Data)
	}
	if data["new_state"] != string(models.TaskSessionStateCreated) {
		t.Errorf("expected new_state %q, got %q", models.TaskSessionStateCreated, data["new_state"])
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if data["updated_at"] != session.UpdatedAt.UTC().Format(time.RFC3339Nano) {
		t.Errorf("expected updated_at %q, got %q", session.UpdatedAt.UTC().Format(time.RFC3339Nano), data["updated_at"])
	}
}
