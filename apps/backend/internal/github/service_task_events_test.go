package github

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestListActivePRWatches_ExcludesArchivedTasks(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t-active", false)
	seedTask(t, store, "t-archived", true)

	mustCreateWatch(t, store, "s-active", "t-active")
	mustCreateWatch(t, store, "s-archived", "t-archived")

	watches, err := svc.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("expected 1 watch (active only), got %d", len(watches))
	}
	if watches[0].TaskID != "t-active" {
		t.Errorf("expected watch for t-active, got %q", watches[0].TaskID)
	}
}

func TestListActivePRWatches_ExcludesOrphanedWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Orphan: watch exists but no matching task row (task was hard-deleted).
	mustCreateWatch(t, store, "s-orphan", "t-gone")

	watches, err := svc.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 0 {
		t.Fatalf("expected orphaned watch to be excluded, got %d watches", len(watches))
	}
}

func TestHandleTaskUpdated_ArchiveDeletesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Simulate task-service publishing task.updated with archived_at set.
	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := svc.handleTaskUpdated(ctx, event); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted after archive event, got %+v", got)
	}
}

func TestHandleTaskUpdated_NonArchiveUpdateLeavesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Regular edit: no archived_at in payload.
	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id": "t1",
		"title":   "Edited title",
	})
	if err := svc.handleTaskUpdated(ctx, event); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got == nil {
		t.Error("expected watch to persist after non-archive update")
	}
}

func TestHandleTaskDeleted_DeletesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{
		"task_id": "t1",
	})
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted after delete event, got %+v", got)
	}
}

func TestHandleTaskDeletedRevokesCredentialLeases(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	broker := revocationTestBroker("workspace-1", "t1", "s1")
	svc.SetCredentialBroker(broker)

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{
		"task_id": "t1",
	})
	if err := svc.handleTaskDeleted(context.Background(), event); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}
	if got := len(broker.leases); got != 0 {
		t.Fatalf("lease records = %d, want 0", got)
	}
}

func TestHandleTaskEvents_MalformedPayloadIsNoop(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Wrong payload type — should be ignored, not crash.
	bad := bus.NewEvent(events.TaskUpdated, "task-service", "not-a-map")
	if err := svc.handleTaskUpdated(ctx, bad); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	// Missing task_id — should be ignored.
	empty := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{})
	if err := svc.handleTaskDeleted(ctx, empty); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got == nil {
		t.Error("expected watch to persist when payload is malformed")
	}
}

func TestHandleWorkspaceDeletedRemovesOnlyOwnedConnectionSecrets(t *testing.T) {
	svc, secrets := newWorkspaceConnectionService(t, "octocat")
	secrets.values[WorkspacePATSecretKey("ws-1")] = "pat"
	secrets.values[UserAccessTokenSecretKey("ws-1", "user-1")] = "access"
	secrets.values[UserRefreshTokenSecretKey("ws-1", "user-1")] = "refresh"
	secrets.values[WorkspacePATSecretKey("ws-2")] = "other"

	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := svc.handleWorkspaceDeleted(context.Background(), event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	for _, id := range []string{
		WorkspacePATSecretKey("ws-1"),
		UserAccessTokenSecretKey("ws-1", "user-1"),
		UserRefreshTokenSecretKey("ws-1", "user-1"),
	} {
		if _, ok := secrets.values[id]; ok {
			t.Fatalf("workspace-owned secret %q was not deleted", id)
		}
	}
	if got := secrets.values[WorkspacePATSecretKey("ws-2")]; got != "other" {
		t.Fatalf("unrelated workspace secret changed: %q", got)
	}
}

func TestHandleWorkspaceDeletedRevokesCredentialLeasesWithoutSecretStore(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	broker := revocationTestBroker("workspace-1", "t1", "s1")
	svc.SetCredentialBroker(broker)
	svc.connectionSecrets = nil

	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "workspace-1"})
	if err := svc.handleWorkspaceDeleted(context.Background(), event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	if got := len(broker.leases); got != 0 {
		t.Fatalf("lease records = %d, want 0", got)
	}
}

func TestSubscribeTaskEventsTerminalSessionRevokesCredentialLease(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	broker := revocationTestBroker("workspace-1", "t1", "s1")
	svc.SetCredentialBroker(broker)
	svc.subscribeTaskEvents()
	t.Cleanup(svc.unsubscribeTaskEvents)

	event := bus.NewEvent(events.TaskSessionStateChanged, "orchestrator", map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"new_state":  string(taskmodels.TaskSessionStateCompleted),
	})
	if err := svc.eventBus.Publish(context.Background(), events.TaskSessionStateChanged, event); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := len(broker.leases); got != 0 {
		t.Fatalf("lease records = %d, want 0", got)
	}
}

func TestSubscribeTaskEvents_EndToEnd(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	svc.subscribeTaskEvents()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := svc.eventBus.Publish(ctx, events.TaskUpdated, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// MemoryEventBus delivers synchronously, so the watch should already be gone.
	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted by subscribed handler, got %+v", got)
	}
}

// mustCreateWatch is a test helper for creating a PR watch with minimal boilerplate.
func mustCreateWatch(t *testing.T, store *Store, sessionID, taskID string) {
	t.Helper()
	w := &PRWatch{
		SessionID: sessionID,
		TaskID:    taskID,
		Owner:     "owner",
		Repo:      "repo",
		PRNumber:  0,
		Branch:    "main",
	}
	if err := store.CreatePRWatch(context.Background(), w); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
}

func revocationTestBroker(workspaceID, taskID, sessionID string) *CredentialBroker {
	hash := sha256.Sum256([]byte(workspaceID + taskID + sessionID))
	return &CredentialBroker{
		leases: map[[sha256.Size]byte]credentialLeaseRecord{
			hash: {
				WorkspaceID: workspaceID,
				TaskID:      taskID,
				SessionID:   sessionID,
				ExpiresAt:   time.Now().Add(time.Hour),
			},
		},
		now: time.Now,
	}
}
