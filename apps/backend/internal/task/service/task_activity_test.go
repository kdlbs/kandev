package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeActivityProvider resolves a per-session foreground activity so the
// task-level aggregate can be exercised with distinct values per session.
type fakeActivityProvider struct {
	byID map[string]v1.ForegroundActivity
}

func (f *fakeActivityProvider) ForegroundActivity(sessionID string) v1.ForegroundActivity {
	return f.byID[sessionID]
}

// blockingActivityEventBus pauses a generating publication until the test
// releases it.
type blockingActivityEventBus struct {
	*MockEventBus
	generatingEntered chan struct{}
	releaseGenerating chan struct{}
}

func (b *blockingActivityEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	data, _ := event.Data.(map[string]interface{})
	activity, _ := data["foreground_activity"].(string)
	if activity == "generating" {
		b.generatingEntered <- struct{}{}
		<-b.releaseGenerating
	}
	return b.MockEventBus.Publish(ctx, subject, event)
}

// failingSessionLister decorates a real SessionRepository but fails the one query
// the task-level aggregate depends on, so the "session set unavailable" path can be
// exercised without disturbing any other repository behavior.
type failingSessionLister struct {
	repository.SessionRepository
}

func (failingSessionLister) ListActiveTaskSessionsByTaskID(context.Context, string) ([]*models.TaskSession, error) {
	return nil, errors.New("boom: sessions unavailable")
}

func createRunningSession(t *testing.T, ctx context.Context, repo interface {
	CreateTaskSession(context.Context, *models.TaskSession) error
}, id, taskID string, state models.TaskSessionState) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: id, TaskID: taskID, State: state, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", id, err)
	}
}

func foregroundActivityField(t *testing.T, data map[string]interface{}) interface{} {
	t.Helper()
	value, ok := data["foreground_activity"]
	if !ok {
		t.Fatalf("foreground_activity missing from payload: %#v", data)
	}
	return value
}

func TestRecordTaskActivity_ZeroValueServiceInitializesDedupCache(t *testing.T) {
	var svc Service

	svc.recordTaskActivity("task-1", v1.ForegroundActivityBackground)

	if got := svc.lastTaskActivity["task-1"]; got != v1.ForegroundActivityBackground {
		t.Fatalf("recorded activity = %q, want %q", got, v1.ForegroundActivityBackground)
	}
}

// TestTaskUpdated_StampsForegroundActivityAggregate covers the task.updated
// payload: it carries the MOST-ACTIVE-WINS aggregate, and emits explicit nil when
// no session is running so a stale background reading is cleared.
func TestTaskUpdated_StampsForegroundActivityAggregate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateRunning)

	provider := &fakeActivityProvider{byID: map[string]v1.ForegroundActivity{"s1": v1.ForegroundActivityBackground}}
	svc.SetForegroundActivityProvider(provider)
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"}

	eventBus.ClearEvents()
	svc.PublishTaskUpdated(ctx, task)
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "background" {
		t.Fatalf("running background: foreground_activity=%#v, want \"background\"", got)
	}

	provider.byID["s1"] = v1.ForegroundActivityGenerating
	eventBus.ClearEvents()
	svc.PublishTaskUpdated(ctx, task)
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "generating" {
		t.Fatalf("running generating: foreground_activity=%#v, want \"generating\"", got)
	}
}

func TestTaskUpdated_BackgroundActivitySurvivesSettledSession(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateWaitingForInput)

	svc.SetForegroundActivityProvider(&fakeActivityProvider{byID: map[string]v1.ForegroundActivity{"s1": v1.ForegroundActivityBackground}})

	eventBus.ClearEvents()
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "background" {
		t.Fatalf("settled background session: foreground_activity=%#v, want background", got)
	}
}

// TestPublishTaskActivityIfChanged_EmitsOnlyOnAggregateChange is the core
// live-propagation-fallback behavior: a generating↔background flip emits
// task.updated only when the MOST-ACTIVE-WINS reading actually changes.
func TestPublishTaskActivityIfChanged_EmitsOnlyOnAggregateChange(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateRunning)
	createRunningSession(t, ctx, repo, "s2", "task-1", models.TaskSessionStateRunning)

	provider := &fakeActivityProvider{byID: map[string]v1.ForegroundActivity{
		"s1": v1.ForegroundActivityGenerating,
		"s2": v1.ForegroundActivityGenerating,
	}}
	svc.SetForegroundActivityProvider(provider)

	// First observation: unseen → generating, emits once.
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "generating" {
		t.Fatalf("first flip: foreground_activity=%#v, want \"generating\"", got)
	}

	// One session flips to background but the other still generates: aggregate
	// stays generating → no emit.
	provider.byID["s1"] = v1.ForegroundActivityBackground
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("aggregate unchanged (still generating) must not emit, got %d events", len(events))
	}

	// The last generating session flips to background: aggregate now background → emit.
	provider.byID["s2"] = v1.ForegroundActivityBackground
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "background" {
		t.Fatalf("flip to background: foreground_activity=%#v, want \"background\"", got)
	}

	// Idempotent: calling again with no change does not re-emit.
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("no change must not re-emit, got %d events", len(events))
	}
}

func TestPublishTaskActivityIfChanged_OrdersConcurrentAggregatePublications(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateRunning)
	createRunningSession(t, ctx, repo, "s2", "task-1", models.TaskSessionStateRunning)

	provider := &fakeActivityProvider{byID: map[string]v1.ForegroundActivity{
		"s1": v1.ForegroundActivityBackground,
		"s2": v1.ForegroundActivityBackground,
	}}
	svc.SetForegroundActivityProvider(provider)
	svc.PublishTaskActivityIfChanged(ctx, "task-1") // Establish the background baseline.
	eventBus.ClearEvents()

	blockingBus := &blockingActivityEventBus{
		MockEventBus:      eventBus,
		generatingEntered: make(chan struct{}, 1),
		releaseGenerating: make(chan struct{}),
	}
	svc.eventBus = blockingBus

	provider.byID["s1"] = v1.ForegroundActivityGenerating
	firstDone := make(chan struct{})
	go func() {
		svc.PublishTaskActivityIfChanged(ctx, "task-1")
		close(firstDone)
	}()
	<-blockingBus.generatingEntered

	provider.byID["s1"] = v1.ForegroundActivityBackground
	secondDone := make(chan struct{})
	go func() {
		svc.PublishTaskActivityIfChanged(ctx, "task-1")
		close(secondDone)
	}()

	// The second caller queues behind the blocked first one and returns. The
	// timeout is only a deadlock guard; channels establish the ordering.
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("newer activity publication did not queue behind the older publication")
	}
	if published := eventBus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("newer activity publication overtook the blocked older publication: %#v", published)
	}

	close(blockingBus.releaseGenerating)
	<-firstDone
	<-secondDone

	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published %d task activity events, want 2", len(published))
	}
	firstData, _ := published[0].Data.(map[string]interface{})
	if got := foregroundActivityField(t, firstData); got != "generating" {
		t.Fatalf("first activity = %#v, want generating", got)
	}
	secondData, _ := published[1].Data.(map[string]interface{})
	if got := foregroundActivityField(t, secondData); got != "background" {
		t.Fatalf("second activity = %#v, want background", got)
	}
}

func TestPublishTaskActivityIfChanged_BackgroundToEmptyEmitsExplicitNull(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateWaitingForInput)
	provider := &fakeActivityProvider{byID: map[string]v1.ForegroundActivity{
		"s1": v1.ForegroundActivityBackground,
	}}
	svc.SetForegroundActivityProvider(provider)

	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "background" {
		t.Fatalf("baseline foreground_activity=%#v, want background", got)
	}

	provider.byID["s1"] = v1.ForegroundActivityGenerating
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	data := singlePublishedEventData(t, eventBus)
	value, present := data["foreground_activity"]
	if !present || value != nil {
		t.Fatalf("background->empty must emit explicit foreground_activity null, got %#v", data)
	}
}

func assertForegroundActivityAbsent(t *testing.T, data map[string]interface{}) {
	t.Helper()
	if v, ok := data["foreground_activity"]; ok {
		t.Fatalf("foreground_activity must be omitted when the session set is unavailable, got %#v", v)
	}
}

// TestTaskUpdated_OmitsForegroundActivityWhenSessionSetUnavailable locks the safe
// fallback (§spec:live-propagation-fallback): when the session set can't be loaded
// the aggregate is UNKNOWN, so the event omits the field entirely rather than
// stamping an explicit nil that would clear the client to a coarse "done".
func TestTaskUpdated_OmitsForegroundActivityWhenSessionSetUnavailable(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateRunning)
	svc.SetForegroundActivityProvider(&fakeActivityProvider{byID: map[string]v1.ForegroundActivity{"s1": v1.ForegroundActivityBackground}})
	svc.sessions = failingSessionLister{svc.sessions}

	eventBus.ClearEvents()
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	assertForegroundActivityAbsent(t, singlePublishedEventData(t, eventBus))
}

// TestForegroundActivity_UnavailablePreservesLastKnown proves the unknown path does
// not corrupt the dedup baseline: a failed load neither emits a spurious clear nor
// overwrites the last-known aggregate, so once the session set is readable again the
// unchanged reading does not re-emit.
func TestForegroundActivity_UnavailablePreservesLastKnown(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateRunning)
	svc.SetForegroundActivityProvider(&fakeActivityProvider{byID: map[string]v1.ForegroundActivity{"s1": v1.ForegroundActivityBackground}})

	// Establish a known "background" baseline.
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != "background" {
		t.Fatalf("baseline: foreground_activity=%#v, want \"background\"", got)
	}

	// Session set becomes unavailable: no spurious emit.
	realSessions := svc.sessions
	svc.sessions = failingSessionLister{realSessions}
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("unavailable session set must not emit, got %d events", len(events))
	}

	// Readable again with the same reading: baseline was preserved, so no re-emit.
	svc.sessions = realSessions
	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("preserved baseline must not re-emit unchanged reading, got %d events", len(events))
	}
}

func TestPublishTaskActivityIfChanged_NoProviderIsNoOp(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	eventBus.ClearEvents()
	svc.PublishTaskActivityIfChanged(ctx, "task-1")
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("no provider wired must not emit, got %d events", len(events))
	}
}
