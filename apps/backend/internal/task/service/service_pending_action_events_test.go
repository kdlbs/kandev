package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPublishMessageEventPublishesCompactPendingActionReplacement(t *testing.T) {
	const compactEventType = "session.pending_action_changed"
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-pending")
	message := &models.Message{
		ID:            "message-pending",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "private question body",
		Type:          models.MessageTypeClarificationRequest,
		Metadata:      map[string]interface{}{"pending_id": "pending-1", "status": "pending"},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.publishMessageEvent(ctx, events.MessageAdded, message); err != nil {
		t.Fatalf("publish message event: %v", err)
	}

	var compact *bus.Event
	for _, published := range eventBus.GetPublishedEvents() {
		if published.Type == compactEventType {
			compact = published
			break
		}
	}
	if compact == nil {
		t.Fatal("publishMessageEvent did not publish a compact pending-action event")
	}
	data, ok := compact.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("compact event data type = %T, want map[string]interface{}", compact.Data)
	}
	if len(data) != 5 {
		t.Fatalf("compact event fields = %#v, want exactly five bounded fields", data)
	}
	if data["workspace_id"] != "ws-1" || data["task_id"] != "task-123" || data["session_id"] != sessionID {
		t.Fatalf("compact event identity = %#v", data)
	}
	if data["pending_action"] != "clarification" {
		t.Fatalf("compact pending_action = %#v, want clarification", data["pending_action"])
	}
	revision, ok := data["pending_action_revision"].(models.PendingActionRevision)
	if !ok || revision.Epoch == "" || revision.Sequence == 0 {
		t.Fatalf("compact pending_action_revision = %#v", data["pending_action_revision"])
	}
	if _, exists := data["content"]; exists {
		t.Fatal("compact pending-action event must not contain message content")
	}

	message.Metadata["status"] = "answered"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("clear pending message: %v", err)
	}
	eventBus.ClearEvents()
	if err := svc.publishMessageEvent(ctx, events.MessageUpdated, message); err != nil {
		t.Fatalf("publish clear event: %v", err)
	}
	var cleared *bus.Event
	for _, published := range eventBus.GetPublishedEvents() {
		if published.Type == compactEventType {
			cleared = published
			break
		}
	}
	if cleared == nil {
		t.Fatal("clear transition did not publish a compact pending-action event")
	}
	clearedData := cleared.Data.(map[string]interface{})
	if clearedData["pending_action"] != nil {
		t.Fatalf("cleared pending_action = %#v, want nil", clearedData["pending_action"])
	}
	clearedRevision := clearedData["pending_action_revision"].(models.PendingActionRevision)
	if clearedRevision.Epoch != revision.Epoch || clearedRevision.Sequence <= revision.Sequence {
		t.Fatalf("cleared revision = %#v, want newer than %#v", clearedRevision, revision)
	}
}

func TestPublishMessageEventSuppressesUnchangedCompactPendingAction(t *testing.T) {
	const compactEventType = "session.pending_action_changed"
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-pending")
	pending := &models.Message{
		ID:            "message-pending",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "Choose",
		Type:          models.MessageTypeClarificationRequest,
		Metadata:      map[string]interface{}{"pending_id": "pending-1", "status": "pending"},
	}
	if err := repo.CreateMessage(ctx, pending); err != nil {
		t.Fatalf("create pending message: %v", err)
	}
	if err := svc.publishMessageEvent(ctx, events.MessageAdded, pending); err != nil {
		t.Fatalf("publish pending message event: %v", err)
	}

	ordinary := &models.Message{
		ID:            "message-ordinary",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "More detail",
		Type:          models.MessageTypeMessage,
	}
	if err := repo.CreateMessage(ctx, ordinary); err != nil {
		t.Fatalf("create ordinary message: %v", err)
	}
	eventBus.ClearEvents()
	if err := svc.publishMessageEvent(ctx, events.MessageAdded, ordinary); err != nil {
		t.Fatalf("publish ordinary message event: %v", err)
	}

	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == compactEventType {
			t.Fatal("unchanged pending action published a duplicate compact event")
		}
	}
}

func singlePublishedEventDataOfType(t *testing.T, eventBus *MockEventBus, eventType string) map[string]interface{} {
	t.Helper()
	var matching []*bus.Event
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == eventType {
			matching = append(matching, event)
		}
	}
	if len(matching) != 1 {
		t.Fatalf("expected 1 %s event, got %d", eventType, len(matching))
	}
	data, ok := matching[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", matching[0].Data)
	}
	return data
}
