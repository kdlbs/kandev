package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/statussummary"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestProtectedTaskLifecycleWorkspaceProjectionAndTaskDetails(t *testing.T) {
	const (
		taskID      = "protected-task"
		workspaceID = "protected-workspace"
		creatorID   = "automation-creator"
	)
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceReaders: func(_ context.Context, gotWorkspaceID string) ([]string, error) {
			require.Equal(t, workspaceID, gotWorkspaceID)
			return []string{creatorID, "workspace-reader"}, nil
		},
		Subscriptions: SubscriptionAccessPolicy{
			Task: func(ctx context.Context, gotTaskID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if gotTaskID != taskID || identity.UserID != creatorID {
					return errors.New("task access denied")
				}
				return nil
			},
		},
	})
	creator := newAuthorizedNotificationClient(hub, "creator", creatorID)
	reader := newAuthorizedNotificationClient(hub, "reader", "workspace-reader")
	hub.SubscribeToTask(creator, taskID)
	hub.SubscribeToTask(reader, taskID)
	broadcaster := protectedTaskNotificationBroadcaster(hub, taskID)

	for _, action := range []string{ws.ActionTaskCreated, ws.ActionTaskUpdated, ws.ActionTaskStateChanged} {
		data := map[string]interface{}{
			"task_id": taskID, "workspace_id": workspaceID,
			"title": "private title", "description": "private description",
			"primary_session_id": "private-primary-session", "session_id": "private-session",
			"state": "IN_PROGRESS", "old_state": "CREATED", "new_state": "IN_PROGRESS",
			"updated_at": "2026-11-27T12:00:00Z", "metadata": map[string]interface{}{"secret": "private metadata"},
			"agent_summary": "private summary", "error_message": "private error",
		}
		require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(action, "test", data), action))

		workspaceFrame := nextClientFrame(t, reader)
		require.Equal(t, action, workspaceFrame.Action)
		require.Equal(t, map[string]interface{}{
			"task_id": taskID, "workspace_id": workspaceID, "state": "IN_PROGRESS",
			"old_state": "CREATED", "new_state": "IN_PROGRESS", "updated_at": "2026-11-27T12:00:00Z",
		}, notificationPayload(t, workspaceFrame))

		creatorWorkspaceFrame := nextClientFrame(t, creator)
		require.Equal(t, map[string]interface{}{
			"task_id": taskID, "workspace_id": workspaceID, "state": "IN_PROGRESS",
			"old_state": "CREATED", "new_state": "IN_PROGRESS", "updated_at": "2026-11-27T12:00:00Z",
		}, notificationPayload(t, creatorWorkspaceFrame))
		if action != ws.ActionTaskCreated {
			hub.SubscribeToTask(reader, taskID)
		}
		detailFrame := nextClientFrame(t, creator)
		require.Equal(t, action, detailFrame.Action)
		require.Equal(t, data, notificationPayload(t, detailFrame))
		assertNoMoreFrames(t, reader)
		assertNoMoreFrames(t, creator)
	}
}

func TestProtectedSessionStateUsesWorkspaceProjectionAndAuthorizedSessionDelivery(t *testing.T) {
	const (
		taskID      = "protected-session-task"
		workspaceID = "protected-session-workspace"
		sessionID   = "protected-session"
		creatorID   = "automation-creator"
	)
	hub := newTestHub(t)
	sessionChecks := 0
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceReaders: func(_ context.Context, gotWorkspaceID string) ([]string, error) {
			require.Equal(t, workspaceID, gotWorkspaceID)
			return []string{creatorID, "workspace-reader"}, nil
		},
		Subscriptions: SubscriptionAccessPolicy{
			Session: func(ctx context.Context, gotSessionID string) error {
				sessionChecks++
				identity, _ := authn.IdentityFromContext(ctx)
				if gotSessionID != sessionID || identity.UserID != creatorID {
					return errors.New("session access denied")
				}
				return nil
			},
		},
	})
	creator := newAuthorizedNotificationClient(hub, "creator", creatorID)
	reader := newAuthorizedNotificationClient(hub, "reader", "workspace-reader")
	hub.SubscribeToSession(creator, sessionID)
	hub.SubscribeToSession(reader, sessionID)
	broadcaster := protectedTaskNotificationBroadcaster(hub, taskID)
	data := map[string]interface{}{
		"task_id": taskID, "workspace_id": workspaceID, "session_id": sessionID,
		"old_state": "STARTING", "new_state": "RUNNING", "updated_at": "2026-11-27T12:00:00Z",
		"title": "private title", "description": "private description",
		"primary_session_id": "another-private-session", "metadata": map[string]interface{}{"credential": "private metadata"},
		"session_metadata": map[string]interface{}{"credential_error": "private failure"},
		"agent_summary":    "private summary", "error_message": "private error",
	}

	require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", data), ws.ActionSessionStateChanged))

	workspaceFrame := nextClientFrame(t, reader)
	require.Equal(t, ws.ActionSessionStateChanged, workspaceFrame.Action)
	require.Equal(t, map[string]interface{}{
		"task_id": taskID, "workspace_id": workspaceID,
		"old_state": "STARTING", "new_state": "RUNNING", "updated_at": "2026-11-27T12:00:00Z",
	}, notificationPayload(t, workspaceFrame))
	creatorWorkspaceFrame := nextClientFrame(t, creator)
	require.Equal(t, map[string]interface{}{
		"task_id": taskID, "workspace_id": workspaceID,
		"old_state": "STARTING", "new_state": "RUNNING", "updated_at": "2026-11-27T12:00:00Z",
	}, notificationPayload(t, creatorWorkspaceFrame))
	creatorSessionFrame := nextClientFrame(t, creator)
	require.Equal(t, ws.ActionSessionStateChanged, creatorSessionFrame.Action)
	require.Equal(t, map[string]interface{}{
		"task_id": taskID, "session_id": sessionID,
		"old_state": "STARTING", "new_state": "RUNNING", "updated_at": "2026-11-27T12:00:00Z",
	}, notificationPayload(t, creatorSessionFrame))
	assertNoMoreFrames(t, reader)
	require.Equal(t, 2, sessionChecks, "session delivery must revalidate each subscriber through the existing session policy")
	require.False(t, reader.sessionSubscriptions[sessionID], "unauthorized session subscriber is evicted")
}

func TestProtectedTaskSnapshotDropsWhenTaskAuthorizationIsUnavailable(t *testing.T) {
	const taskID = "protected-snapshot-task"
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{Enforced: func() bool { return true }})
	reader := newAuthorizedNotificationClient(hub, "reader", "workspace-reader")
	hub.SubscribeToTask(reader, taskID)
	broadcaster := protectedTaskNotificationBroadcaster(hub, taskID)
	for _, event := range []struct {
		action string
		data   map[string]interface{}
	}{
		{ws.ActionTaskPlanCommentsChanged, map[string]interface{}{"task_id": taskID, "comment": "private snapshot"}},
		{ws.ActionTaskStatusSummaryUpdated, map[string]interface{}{"task_id": taskID, "status_summary": map[string]interface{}{"error": "private summary"}}},
	} {
		message, err := ws.NewNotification(event.action, event.data)
		require.NoError(t, err)
		require.NoError(t, broadcaster.routeBroadcastContext(context.Background(), event.action, event.data, "", "", message))
	}
	assertNoMoreFrames(t, reader)
	select {
	case broadcast := <-hub.broadcast:
		t.Fatalf("protected snapshot used global broadcast: %s", broadcast.Action)
	default:
	}
}

func TestProtectedTaskLifecycleFailsClosedWhenProtectionLookupFails(t *testing.T) {
	const (
		taskID      = "protected-task-unavailable"
		workspaceID = "protected-workspace-unavailable"
		creatorID   = "automation-creator"
	)
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced:         func() bool { return true },
		WorkspaceReaders: func(context.Context, string) ([]string, error) { return []string{creatorID}, nil },
		Subscriptions: SubscriptionAccessPolicy{
			Task: func(context.Context, string) error { return nil },
		},
	})
	creator := newAuthorizedNotificationClient(hub, "creator", creatorID)
	hub.SubscribeToTask(creator, taskID)
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
	broadcaster.SetProtectedTaskLookup(func(context.Context, string) (bool, error) {
		return false, errors.New("authority store unavailable")
	})
	data := map[string]interface{}{
		"task_id": taskID, "workspace_id": workspaceID,
		"title": "private title", "description": "private description",
		"state": "IN_PROGRESS", "updated_at": "2026-11-27T12:00:00Z",
		"metadata": map[string]interface{}{"secret": "private metadata"},
	}

	require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(events.TaskUpdated, "test", data), ws.ActionTaskUpdated))

	frame := nextClientFrame(t, creator)
	require.Equal(t, map[string]interface{}{
		"task_id": taskID, "workspace_id": workspaceID,
		"state": "IN_PROGRESS", "updated_at": "2026-11-27T12:00:00Z",
	}, notificationPayload(t, frame))
	assertNoMoreFrames(t, creator)
}

func TestProtectedTaskSummaryAndReviewEventsUseTaskAuthorization(t *testing.T) {
	const (
		workspaceID = "protected-projection-workspace"
		taskID      = "protected-projection-task"
		creatorID   = "automation-creator"
	)
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceReaders: func(context.Context, string) ([]string, error) {
			return []string{creatorID, "workspace-reader"}, nil
		},
		Subscriptions: SubscriptionAccessPolicy{
			Task: func(ctx context.Context, gotTaskID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if gotTaskID != taskID || identity.UserID != creatorID {
					return errors.New("task access denied")
				}
				return nil
			},
		},
	})
	creator := newAuthorizedNotificationClient(hub, "creator", creatorID)
	reader := newAuthorizedNotificationClient(hub, "reader", "workspace-reader")
	broadcaster := protectedTaskNotificationBroadcaster(hub, taskID)
	summary := statussummary.SummaryUpdated{TaskID: taskID, WorkspaceID: workspaceID}
	review := map[string]interface{}{
		"task_id": taskID, "run_id": "review-run", "findings": []interface{}{
			map[string]interface{}{"summary": "private finding", "error": "private error"},
		},
	}
	for _, event := range []struct {
		subject string
		action  string
		data    interface{}
	}{
		{events.TaskStatusSummaryUpdated, ws.ActionTaskStatusSummaryUpdated, summary},
		{events.TaskReviewFindingsPublished, ws.ActionTaskReviewFindingsPublished, review},
	} {
		hub.SubscribeToTask(creator, taskID)
		hub.SubscribeToTask(reader, taskID)
		require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(event.subject, "test", event.data), event.action))
		creatorFrame := nextClientFrame(t, creator)
		require.Equal(t, event.action, creatorFrame.Action)
		expectedMessage, err := ws.NewNotification(event.action, event.data)
		require.NoError(t, err)
		require.Equal(t, notificationPayload(t, *expectedMessage), notificationPayload(t, creatorFrame))
		assertNoMoreFrames(t, reader)
		assertNoMoreFrames(t, creator)
	}

	for _, event := range []struct {
		subject string
		action  string
		data    interface{}
	}{
		{
			subject: events.TaskStatusSummaryUpdated,
			action:  ws.ActionTaskStatusSummaryUpdated,
			data:    map[string]interface{}{"task_id": taskID, "status_summary": map[string]interface{}{"error": "private summary"}},
		},
		{
			subject: events.TaskReviewFindingsPublished,
			action:  ws.ActionTaskReviewFindingsPublished,
			data:    map[string]interface{}{"task_id": taskID, "findings": []interface{}{map[string]interface{}{"error": "private review"}}},
		},
	} {
		broadcaster.SetProtectedTaskLookup(func(context.Context, string) (bool, error) {
			return false, errors.New("authority store unavailable")
		})
		require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(event.subject, "test", event.data), event.action))
		assertNoMoreFrames(t, creator)
		assertNoMoreFrames(t, reader)
	}

	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceReaders: func(context.Context, string) ([]string, error) {
			return []string{creatorID, "workspace-reader"}, nil
		},
	})
	broadcaster.SetProtectedTaskLookup(func(context.Context, string) (bool, error) { return true, nil })
	for _, event := range []struct {
		subject string
		action  string
		data    interface{}
	}{
		{
			subject: events.TaskStatusSummaryUpdated,
			action:  ws.ActionTaskStatusSummaryUpdated,
			data:    map[string]interface{}{"task_id": taskID, "status_summary": map[string]interface{}{"error": "private summary"}},
		},
		{
			subject: events.TaskReviewFindingsPublished,
			action:  ws.ActionTaskReviewFindingsPublished,
			data:    map[string]interface{}{"task_id": taskID, "findings": []interface{}{map[string]interface{}{"error": "private review"}}},
		},
	} {
		require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(event.subject, "test", event.data), event.action))
		assertNoMoreFrames(t, creator)
		assertNoMoreFrames(t, reader)
	}
	select {
	case message := <-hub.broadcast:
		t.Fatalf("protected projection used global broadcast: %s", message.Action)
	default:
	}

	broadcaster.SetProtectedTaskLookup(func(context.Context, string) (bool, error) { return false, nil })
	for _, event := range []struct {
		subject string
		action  string
		data    interface{}
	}{
		{
			subject: events.TaskStatusSummaryUpdated,
			action:  ws.ActionTaskStatusSummaryUpdated,
			data:    statussummary.SummaryUpdated{TaskID: "ordinary-summary-task", WorkspaceID: workspaceID},
		},
		{
			subject: events.TaskReviewFindingsPublished,
			action:  ws.ActionTaskReviewFindingsPublished,
			data:    map[string]interface{}{"task_id": "ordinary-review-task", "workspace_id": workspaceID, "findings": []interface{}{}},
		},
	} {
		require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent(event.subject, "test", event.data), event.action))
		for _, client := range []*Client{creator, reader} {
			frame := nextClientFrame(t, client)
			require.Equal(t, event.action, frame.Action)
		}
	}
}

func protectedTaskNotificationBroadcaster(hub *Hub, taskID string) *TaskEventBroadcaster {
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
	broadcaster.SetProtectedTaskLookup(func(_ context.Context, gotTaskID string) (bool, error) {
		return gotTaskID == taskID, nil
	})
	return broadcaster
}

func newAuthorizedNotificationClient(hub *Hub, id, userID string) *Client {
	client := newTestClient(id)
	client.hub = hub
	client.identity = authn.Identity{UserID: userID, Role: authn.RoleMember}
	registerTestClient(hub, client)
	return client
}

func notificationPayload(t *testing.T, message ws.Message) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(message.Payload, &payload))
	return payload
}
