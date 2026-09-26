package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestSetWorkflowSourceStampsProvenanceAndPublishes(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-src", Name: "Sources"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-src", WorkspaceID: "ws-src", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	bus.ClearEvents()

	if err := svc.SetWorkflowSource(ctx, "wf-src", "github", ".kandev/workflows/review.yaml"); err != nil {
		t.Fatalf("SetWorkflowSource: %v", err)
	}
	stored, err := repo.GetWorkflow(ctx, "wf-src")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if stored.Source != "github" || stored.SourcePath != ".kandev/workflows/review.yaml" {
		t.Fatalf("workflow provenance = %q/%q", stored.Source, stored.SourcePath)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.WorkflowUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.WorkflowUpdated)
	}

	// Re-stamping identical provenance is a no-op with no duplicate event.
	bus.ClearEvents()
	if err := svc.SetWorkflowSource(ctx, "wf-src", "github", ".kandev/workflows/review.yaml"); err != nil {
		t.Fatalf("repeat SetWorkflowSource: %v", err)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("unchanged provenance published %v, want nothing", eventTypes(published))
	}

	if err := svc.SetWorkflowSource(ctx, "wf-missing", "github", "x.yaml"); err == nil {
		t.Fatal("stamping an unknown workflow must fail")
	}
}

func TestReorderWorkflowsIsWorkspaceScoped(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	ctx := context.Background()
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b2", WorkspaceID: "ws-b", Name: "second"}); err != nil {
		t.Fatalf("create second workflow: %v", err)
	}

	if err := svc.ReorderWorkflows(ctxAs("user-a"), "ws-b", []string{"wf-b2", "wf-b"}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("foreign reorder = %v, want ErrWorkspaceNotFound", err)
	}

	if err := svc.ReorderWorkflows(ctxAs("user-b"), "ws-b", []string{"wf-b2", "wf-b"}); err != nil {
		t.Fatalf("owner ReorderWorkflows: %v", err)
	}
	first, err := repo.GetWorkflow(ctx, "wf-b2")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	second, err := repo.GetWorkflow(ctx, "wf-b")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if first.SortOrder != 0 || second.SortOrder != 1 {
		t.Fatalf("sort orders = %d/%d, want 0/1 in the requested order", first.SortOrder, second.SortOrder)
	}

	// A workflow from another workspace cannot be smuggled into the ordering.
	if err := svc.ReorderWorkflows(ctxAs("user-b"), "ws-b", []string{"wf-b", "wf-legacy-missing"}); err == nil {
		t.Fatal("reordering an unknown workflow must fail")
	}
}

func TestStandaloneTaskEventPublishers(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pub", Name: "Publishers"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-pub", WorkspaceID: "ws-pub", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	task := &models.Task{
		ID: "task-pub", WorkspaceID: "ws-pub", WorkflowID: "wf-pub", WorkflowStepID: "step-1",
		Title: "Publish me", State: v1.TaskStateInProgress, Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	bus.ClearEvents()

	svc.PublishTaskQueuePromoted(ctx, task)
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.TaskQueuePromoted {
		t.Fatalf("published %v, want exactly one %s", types, events.TaskQueuePromoted)
	}

	bus.ClearEvents()
	svc.PublishTaskStateChanged(ctx, task, v1.TaskStateCreated)
	published := bus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != events.TaskStateChanged {
		t.Fatalf("published %v, want exactly one %s", eventTypes(published), events.TaskStateChanged)
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data = %T, want a map payload", published[0].Data)
	}
	if data["old_state"] != string(v1.TaskStateCreated) || data["state"] != string(v1.TaskStateInProgress) {
		t.Fatalf("state transition = %v → %v", data["old_state"], data["state"])
	}
}

type recordingTaskStateActivityLogger struct {
	called          bool
	taskID          string
	old             v1.TaskState
	new             v1.TaskState
	publishedBefore int
	eventBus        *MockEventBus
}

func (l *recordingTaskStateActivityLogger) LogTaskStateChange(
	_ context.Context, task *models.Task, oldState v1.TaskState,
) {
	l.called = true
	l.taskID = task.ID
	l.old = oldState
	l.new = task.State
	if l.eventBus != nil {
		l.publishedBefore = len(l.eventBus.GetPublishedEvents())
	}
}

func TestTaskStateActivityIsLoggedBeforeStateEvent(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	activity := &recordingTaskStateActivityLogger{eventBus: eventBus}
	svc.SetTaskStateActivityLogger(activity)

	svc.PublishTaskStateChanged(context.Background(), &models.Task{
		ID: "task-state-activity", WorkspaceID: "ws-1", State: v1.TaskStateCompleted,
	}, v1.TaskStateInProgress)

	if !activity.called || activity.taskID != "task-state-activity" {
		t.Fatalf("activity logger call = %+v, want task-state-activity", activity)
	}
	if activity.old != v1.TaskStateInProgress || activity.new != v1.TaskStateCompleted {
		t.Fatalf("activity transition = %q -> %q", activity.old, activity.new)
	}
	if activity.publishedBefore != 0 {
		t.Fatalf("activity logger observed %d published events, want 0", activity.publishedBefore)
	}
}

func TestPublishAfterTaskEventsRunsCallback(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	// A nil callback is ignored rather than panicking.
	svc.PublishAfterTaskEvents(ctx, "task-pub", events.TaskUpdated, nil)

	// Without a task id the callback runs inline.
	inline := false
	svc.PublishAfterTaskEvents(ctx, "", events.TaskUpdated, func(context.Context) { inline = true })
	if !inline {
		t.Fatal("an id-less publication must run inline")
	}

	queued := make(chan struct{}, 1)
	svc.PublishAfterTaskEvents(ctx, "task-pub", events.TaskUpdated, func(context.Context) { queued <- struct{}{} })
	select {
	case <-queued:
	default:
		t.Fatal("a task-scoped publication must be drained")
	}
}

func TestPublishWorkspaceSourcesAdoptedSkipsBlankSessions(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	bus.ClearEvents()

	svc.PublishWorkspaceSourcesAdopted(ctx, "task-pub", "/tmp/workspace", []string{"sess-1", "", "sess-2"})

	published := bus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published %v, want one event per non-empty session", eventTypes(published))
	}
	seen := map[string]bool{}
	for _, event := range published {
		if event.Type != events.SessionWorkspaceSourcesUpdated {
			t.Fatalf("event type = %q, want %q", event.Type, events.SessionWorkspaceSourcesUpdated)
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data = %T, want a map payload", event.Data)
		}
		if data["task_id"] != "task-pub" || data["workspace_path"] != "/tmp/workspace" {
			t.Fatalf("payload = %v", data)
		}
		seen[data["session_id"].(string)] = true
	}
	if !seen["sess-1"] || !seen["sess-2"] {
		t.Fatalf("session ids = %v, want sess-1 and sess-2", seen)
	}
}
