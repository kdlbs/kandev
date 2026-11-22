package dashboard_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// hasStatusChangeActivity reports whether the office_activity_log already
// contains a task_status_changed row for taskID.
func hasStatusChangeActivity(t *testing.T, deps *testDeps, taskID, wsID string) bool {
	t.Helper()
	entries, err := deps.repo.ListActivityEntriesByTarget(context.Background(), wsID, taskID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "task_status_changed" {
			return true
		}
	}
	return false
}

// TestUpdateTaskStatus_LogsActivitySynchronously pins that UpdateTaskStatus
// writes the task_status_changed activity row itself, in-process, rather
// than depending on an event-bus subscriber to do it. This DashboardService
// (built by newTestDeps) has no event bus wired at all, so before the fix
// (the write lived solely in office/service.Service.handleTaskStatusChanged,
// an OfficeTaskStatusChanged subscriber) this would never produce a row: the
// event is never published without a bus, and nothing bridges to the
// subscriber outside DashboardService anyway. If this activity write ever
// moves back behind an event-bus round-trip, this test catches it.
func TestUpdateTaskStatus_LogsActivitySynchronously(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "sync-task", "ws-sync", "Sync Task", "todo", 0)

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "sync-task",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	if !hasStatusChangeActivity(t, deps, "sync-task", "ws-sync") {
		t.Fatal("expected task_status_changed activity row immediately after UpdateTaskStatus returns")
	}
}

// LogTaskStateChange is the task-service seam used by workflow moves. It must
// write the same durable projection as the Office status endpoint.
func TestLogTaskStateChange_LogsOfficeTask(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "workflow-task", "ws-workflow", "Workflow Task", "in_progress", 0)
	if _, err := deps.db.Exec(`INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-workflow', 'wf-office')`); err != nil {
		t.Fatalf("create Office workspace: %v", err)
	}
	if _, err := deps.db.Exec(`UPDATE tasks SET workflow_id = 'wf-office' WHERE id = 'workflow-task'`); err != nil {
		t.Fatalf("attach task to Office workflow: %v", err)
	}

	deps.svc.LogTaskStateChange(context.Background(), &taskmodels.Task{
		ID: "workflow-task", WorkspaceID: "ws-workflow", State: v1.TaskStateCompleted,
	}, v1.TaskStateInProgress)

	if !hasStatusChangeActivity(t, deps, "workflow-task", "ws-workflow") {
		t.Fatal("expected task_status_changed activity row for a workflow state transition")
	}
}

// orderingEventBus is a minimal bus.EventBus fake that, on Publish of
// OfficeTaskStatusChanged, snapshots whether the activity row is already
// persisted. Every other EventBus method is a harmless no-op; this fix
// only touches the Publish ordering relative to the activity write.
type orderingEventBus struct {
	checkRow                 func() bool
	activityPresentAtPublish bool
	statePublished           bool
	activityAtStatePublish   bool
	published                bool
}

func (b *orderingEventBus) Publish(_ context.Context, subject string, _ *bus.Event) error {
	switch subject {
	case events.TaskStateChanged:
		b.statePublished = true
		b.activityAtStatePublish = b.checkRow()
	case events.OfficeTaskStatusChanged:
		b.published = true
		b.activityPresentAtPublish = b.checkRow()
	}
	return nil
}

func (b *orderingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *orderingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *orderingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (b *orderingEventBus) Close()            {}
func (b *orderingEventBus) IsConnected() bool { return true }

// TestUpdateTaskStatus_LogsActivityBeforePublishingEvent reproduces the P1
// race from PR fixup round 1: under a NATS-backed bus, the activity-log
// write and the WS broadcast that triggers the frontend's task-detail
// refetch were two independent async subscribers to the same
// OfficeTaskStatusChanged event, with no ordering guarantee between them —
// a browser GET could land before the activity row existed and see stale
// Started/Completed values with nothing left to trigger a corrective
// refetch. The fix sequences the activity write before the publish call
// inside UpdateTaskStatus itself, so the row is guaranteed durable before
// the event (and therefore any broadcast subscriber) can ever fire,
// independent of the bus implementation. This test proves that ordering
// directly: by the time Publish(OfficeTaskStatusChanged) is observed, the
// activity row must already be queryable.
func TestUpdateTaskStatus_LogsActivityBeforePublishingEvent(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "race-task", "ws-race", "Race Task", "todo", 0)

	fake := &orderingEventBus{}
	fake.checkRow = func() bool {
		return hasStatusChangeActivity(t, deps, "race-task", "ws-race")
	}
	deps.svc.SetEventBus(fake)
	deps.svc.SetTaskStateChangePublisher(statusStateChangePublisher{eventBus: fake})

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "race-task",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	if !fake.published {
		t.Fatal("expected OfficeTaskStatusChanged to be published")
	}
	if !fake.activityPresentAtPublish {
		t.Fatal("activity row was not yet persisted when the status-changed event was published: " +
			"the WS broadcast (and any refetch it triggers) could race ahead of the activity write")
	}
	if !fake.statePublished || !fake.activityAtStatePublish {
		t.Fatal("canonical task.state_changed was not published after the activity row")
	}
}

func TestUpdateTaskStatus_LogsOneActivityForOneStateChange(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "single-activity", "ws-single", "Single activity", "todo", 0)

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "single-activity",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	entries, err := deps.repo.ListActivityEntriesByTarget(context.Background(), "ws-single", "single-activity", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.Action == "task_status_changed" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("task_status_changed entries = %d, want 1", count)
	}
}

type cancelAfterCommitTaskStateWriter struct {
	shared.TaskStateWriter
	cancel context.CancelFunc
}

func (w cancelAfterCommitTaskStateWriter) CommitTaskState(
	ctx context.Context, taskID string, write taskmodels.TaskStateWrite,
) (taskmodels.TaskStateChange, error) {
	change, err := w.TaskStateWriter.CommitTaskState(ctx, taskID, write)
	w.cancel()
	return change, err
}

type fixedRunResolver string

type statusActivityContextKey struct{}

func (r fixedRunResolver) ResolveRunForTask(ctx context.Context, _ string) string {
	if ctx.Err() != nil || ctx.Value(statusActivityContextKey{}) != "run-context" {
		return ""
	}
	return string(r)
}

func TestUpdateTaskStatusActivitySurvivesCancellationAfterCommit(t *testing.T) {
	baseCtx := context.WithValue(context.Background(), statusActivityContextKey{}, "run-context")
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()
	log := logger.Default()
	databaseConn, err := internaldb.OpenSQLite(filepath.Join(t.TempDir(), "status-activity.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(databaseConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, closeTaskRepo, err := taskrepo.Provide(database, database, log)
	if err != nil {
		t.Fatalf("provide task repository: %v", err)
	}
	t.Cleanup(func() { _ = closeTaskRepo() })
	officeRepo, err := officesqlite.NewWithDB(database, database, log)
	if err != nil {
		t.Fatalf("provide Office repository: %v", err)
	}
	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-cancel", Name: "Cancel workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: "cancel-task", WorkspaceID: "ws-cancel", Title: "Cancel task", State: v1.TaskStateTODO,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces: taskRepo, Tasks: taskRepo, TaskRepos: taskRepo, Workflows: taskRepo,
		Messages: taskRepo, Turns: taskRepo, Sessions: taskRepo, GitSnapshots: taskRepo,
		RepoEntities: taskRepo, RepositorySets: taskRepo, Executors: taskRepo,
		Environments: taskRepo, TaskEnvironments: taskRepo, RecoveryOperations: taskRepo,
		Reviews: taskRepo, ResourceCleanups: taskRepo, StatusSummaries: taskRepo,
		WorkspaceFolders: taskRepo,
	}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	activity := shared.NewActivityLogger(officeRepo, log)
	service := dashboard.NewDashboardService(officeRepo, log, activity, nil, nil)
	service.SetTaskStateWriter(cancelAfterCommitTaskStateWriter{TaskStateWriter: taskRepo, cancel: cancel})
	service.SetTaskStateChangePublisher(taskSvc)
	service.SetTaskLifecyclePublisher(taskSvc)
	service.SetRunResolver(fixedRunResolver("run-attributed"))

	observed := make(chan []*officemodels.ActivityEntry, 1)
	subscription, err := eventBus.Subscribe(events.TaskStateChanged, func(ctx context.Context, _ *bus.Event) error {
		entries, err := officeRepo.ListActivityEntriesByTarget(context.Background(), "ws-cancel", "cancel-task", 50)
		if err != nil {
			return err
		}
		observed <- entries
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to state events: %v", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()

	if err := service.UpdateTaskStatus(ctx, dashboard.TaskStatusUpdateRequest{
		TaskID: "cancel-task", NewStatus: "in_progress", ActorAgentID: "agent-actor",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("state writer did not cancel the caller after committing")
	}
	var entries []*officemodels.ActivityEntry
	select {
	case entries = <-observed:
	case <-time.After(2 * time.Second):
		t.Fatal("canonical task.state_changed event was not published")
	}
	statusEntries := make([]*officemodels.ActivityEntry, 0, 1)
	for _, entry := range entries {
		if entry.Action == "task_status_changed" {
			statusEntries = append(statusEntries, entry)
		}
	}
	if len(statusEntries) != 1 {
		t.Fatalf("activity rows visible at canonical event = %+v, want exactly one", statusEntries)
	}
	entry := statusEntries[0]
	if entry.ActorType != "agent" || entry.ActorID != "agent-actor" || entry.RunID != "run-attributed" {
		t.Fatalf("activity attribution = actor %s/%s run %q, want agent/agent-actor run-attributed", entry.ActorType, entry.ActorID, entry.RunID)
	}
}
