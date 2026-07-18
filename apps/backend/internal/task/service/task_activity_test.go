package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
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

func TestTaskUpdated_ForegroundActivityNilWhenNoRunningSession(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "s1", "task-1", models.TaskSessionStateWaitingForInput)

	svc.SetForegroundActivityProvider(&fakeActivityProvider{byID: map[string]v1.ForegroundActivity{"s1": v1.ForegroundActivityBackground}})

	eventBus.ClearEvents()
	svc.PublishTaskUpdated(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	if got := foregroundActivityField(t, singlePublishedEventData(t, eventBus)); got != nil {
		t.Fatalf("no running session: foreground_activity=%#v, want nil", got)
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
