package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPendingActionSnapshotDoesNotConsumeWorkspaceEvent(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-pending")
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	eventBus := bus.NewMemoryEventBus(svc.logger)
	svc.eventBus = eventBus
	t.Cleanup(eventBus.Close)
	compactEvents := make(chan *bus.Event, 1)
	subscription, err := eventBus.Subscribe(events.SessionPendingActionChanged, func(_ context.Context, event *bus.Event) error {
		compactEvents <- event
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to compact pending-action events: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	svc.SetPendingActionProjectionEpoch(7)
	if _, _, err := svc.GetPendingActionSnapshotProjectionsForSessions(ctx, []string{sessionID}); err != nil {
		t.Fatalf("read initial empty session snapshot: %v", err)
	}

	message := &models.Message{
		ID:            "message-pending",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "Choose",
		Type:          models.MessageTypeClarificationRequest,
		Metadata:      map[string]interface{}{"pending_id": "pending-1", "status": "pending"},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create pending message: %v", err)
	}

	actions, _, err := svc.GetPendingActionSnapshotProjectionsForSessions(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("read full session snapshot after message mutation: %v", err)
	}
	if actions[sessionID] != models.TaskPendingActionClarification {
		t.Fatalf("snapshot action = %q, want clarification", actions[sessionID])
	}
	if err := svc.publishMessageEvent(ctx, events.MessageAdded, message); err != nil {
		t.Fatalf("publish message and compact pending-action events: %v", err)
	}

	select {
	case event := <-compactEvents:
		data, ok := event.Data.(map[string]interface{})
		if !ok || data["pending_action"] != string(models.TaskPendingActionClarification) {
			t.Fatalf("workspace event data = %#v, want clarification", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("full-session snapshot consumed the workspace pending-action event")
	}
}

func TestPendingActionEventIsPublishedBeforeAnySnapshotRead(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-pending")
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	eventBus := bus.NewMemoryEventBus(svc.logger)
	svc.eventBus = eventBus
	t.Cleanup(eventBus.Close)
	compactEvents := make(chan *bus.Event, 1)
	subscription, err := eventBus.Subscribe(events.SessionPendingActionChanged, func(_ context.Context, event *bus.Event) error {
		compactEvents <- event
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to compact pending-action events: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	svc.SetPendingActionProjectionEpoch(7)
	message := &models.Message{
		ID:            "message-pending-cold-start",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "Choose",
		Type:          models.MessageTypeClarificationRequest,
		Metadata:      map[string]interface{}{"pending_id": "pending-cold-start", "status": "pending"},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create pending message: %v", err)
	}
	if err := svc.publishMessageEvent(ctx, events.MessageAdded, message); err != nil {
		t.Fatalf("publish message and compact pending-action events: %v", err)
	}

	select {
	case event := <-compactEvents:
		data, ok := event.Data.(map[string]interface{})
		if !ok || data["pending_action"] != string(models.TaskPendingActionClarification) {
			t.Fatalf("workspace event data = %#v, want clarification", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("cold-start message event did not publish a workspace pending-action event")
	}
}
