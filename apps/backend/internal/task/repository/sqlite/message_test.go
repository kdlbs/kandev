package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// insertMsgWithType inserts a message row with a configurable type column,
// so tests can mix tool_call and plain message rows in the same session.
func insertMsgWithType(t *testing.T, repo *Repository, id, sessionID, turnID, msgType string, ts time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, '', ?, 'agent', '', '', 0, ?, '{}', ?)
	`), id, sessionID, turnID, msgType, ts)
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func TestListMessagesByTurnID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-T", "sess-T", "turn-1")
	seedForMsgTest(t, repo, "task-T2", "sess-T", "turn-2")

	// Two messages on turn-1 (out of insertion order to check created_at sort)
	// and one on turn-2 in the same session.
	insertMsgWithType(t, repo, "m-b", "sess-T", "turn-1", "message", now.Add(2*time.Second))
	insertMsgWithType(t, repo, "m-a", "sess-T", "turn-1", "tool_call", now)
	insertMsgWithType(t, repo, "m-other", "sess-T", "turn-2", "message", now.Add(time.Second))

	got, err := repo.ListMessagesByTurnID(ctx, "turn-1")
	if err != nil {
		t.Fatalf("ListMessagesByTurnID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages for turn-1, got %d", len(got))
	}
	if got[0].ID != "m-a" || got[1].ID != "m-b" {
		t.Errorf("expected [m-a, m-b] ordered by created_at, got [%s, %s]", got[0].ID, got[1].ID)
	}
	for _, m := range got {
		if m.TurnID != "turn-1" {
			t.Errorf("message %s has turn_id %q, want turn-1", m.ID, m.TurnID)
		}
	}

	empty, err := repo.ListMessagesByTurnID(ctx, "turn-missing")
	if err != nil {
		t.Fatalf("ListMessagesByTurnID(missing): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected no messages for unknown turn, got %d", len(empty))
	}
}

func TestUpdateMessageBumpsUpdatedAt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-U", "sess-U", "turn-U")

	created := time.Now().UTC().Add(-time.Hour)
	msg := &models.Message{
		ID:            "m-u",
		TaskSessionID: "sess-U",
		TurnID:        "turn-U",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "hello",
		Type:          models.MessageTypeMessage,
		CreatedAt:     created,
	}
	if err := repo.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	// On insert, updated_at defaults to created_at.
	got, err := repo.GetMessage(ctx, "m-u")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Errorf("after create, updated_at = %v, want == created_at %v", got.UpdatedAt, got.CreatedAt)
	}

	// Update advances updated_at past created_at.
	msg.Content = "hello world"
	if err := repo.UpdateMessage(ctx, msg); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	got, err = repo.GetMessage(ctx, "m-u")
	if err != nil {
		t.Fatalf("GetMessage after update: %v", err)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("after update, updated_at = %v, want after created_at %v", got.UpdatedAt, got.CreatedAt)
	}
}

func TestCountToolCallMessagesBySession_Empty(t *testing.T) {
	repo := newRepoForSessionTests(t)
	got, err := repo.CountToolCallMessagesBySession(context.Background(), nil)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}

func TestCountToolCallMessagesBySession_Single(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-A", "sess-A", "turn-A")
	insertMsgWithType(t, repo, "m1", "sess-A", "turn-A", "tool_call", now)
	insertMsgWithType(t, repo, "m2", "sess-A", "turn-A", "tool_call", now.Add(time.Second))
	insertMsgWithType(t, repo, "m3", "sess-A", "turn-A", "message", now.Add(2*time.Second))

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"sess-A"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["sess-A"] != 2 {
		t.Errorf("sess-A count = %d, want 2", got["sess-A"])
	}
}

func TestCountToolCallMessagesBySession_Multi(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-1", "s1", "turn-1")
	seedForMsgTest(t, repo, "task-2", "s2", "turn-2")
	seedForMsgTest(t, repo, "task-3", "s3", "turn-3")
	insertMsgWithType(t, repo, "m-s1-a", "s1", "turn-1", "tool_call", now)
	insertMsgWithType(t, repo, "m-s2-a", "s2", "turn-2", "tool_call", now)
	insertMsgWithType(t, repo, "m-s2-b", "s2", "turn-2", "tool_call", now.Add(time.Second))
	insertMsgWithType(t, repo, "m-s2-c", "s2", "turn-2", "tool_call", now.Add(2*time.Second))
	// s3 has only a plain message — must be omitted from the result map.
	insertMsgWithType(t, repo, "m-s3-a", "s3", "turn-3", "message", now)

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"s1", "s2", "s3"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["s1"] != 1 {
		t.Errorf("s1 count = %d, want 1", got["s1"])
	}
	if got["s2"] != 3 {
		t.Errorf("s2 count = %d, want 3", got["s2"])
	}
	if _, ok := got["s3"]; ok {
		t.Errorf("s3 should be omitted (zero tool_call rows), got %d", got["s3"])
	}
}

func createPendingActionMessage(
	t *testing.T,
	repo *Repository,
	id string,
	taskID string,
	sessionID string,
	turnID string,
	msgType models.MessageType,
	status string,
	createdAt time.Time,
) {
	t.Helper()
	metadata := map[string]interface{}{}
	if status != "<missing>" {
		metadata["status"] = status
	}
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID:            id,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       id,
		Type:          msgType,
		Metadata:      metadata,
		CreatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("CreateMessage(%s): %v", id, err)
	}
}

func TestGetPendingActionsBySessionIDs(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedForMsgTest(t, repo, "task-clar", "sess-clar", "turn-clar")
	createPendingActionMessage(t, repo, "perm-clar", "task-clar", "sess-clar", "turn-clar", models.MessageTypePermissionRequest, "<missing>", now)
	createPendingActionMessage(t, repo, "clar-clar", "task-clar", "sess-clar", "turn-clar", models.MessageTypeClarificationRequest, "pending", now.Add(time.Second))

	seedForMsgTest(t, repo, "task-resolved", "sess-resolved", "turn-resolved")
	createPendingActionMessage(t, repo, "perm-old", "task-resolved", "sess-resolved", "turn-resolved", models.MessageTypePermissionRequest, "pending", now)
	createPendingActionMessage(t, repo, "perm-new", "task-resolved", "sess-resolved", "turn-resolved", models.MessageTypePermissionRequest, "approved", now.Add(time.Second))

	seedForMsgTest(t, repo, "task-perm", "sess-perm", "turn-perm")
	createPendingActionMessage(t, repo, "perm-pending", "task-perm", "sess-perm", "turn-perm", models.MessageTypePermissionRequest, "pending", now)

	seedForMsgTest(t, repo, "task-stale", "sess-stale", "turn-stale")
	createPendingActionMessage(t, repo, "perm-stale", "task-stale", "sess-stale", "turn-stale", models.MessageTypePermissionRequest, "pending", now)
	seedForMsgTest(t, repo, "task-stale", "sess-stale", "turn-current")
	createPendingActionMessage(t, repo, "message-current", "task-stale", "sess-stale", "turn-current", models.MessageTypeMessage, "<missing>", now.Add(time.Second))

	got, err := repo.GetPendingActionsBySessionIDs(ctx, []string{
		"sess-clar",
		"sess-resolved",
		"sess-perm",
		"sess-stale",
		"sess-missing",
	})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if got["sess-clar"] != models.TaskPendingActionClarification {
		t.Fatalf("sess-clar action = %q, want clarification", got["sess-clar"])
	}
	if _, ok := got["sess-resolved"]; ok {
		t.Fatalf("sess-resolved should not have a pending action: %#v", got["sess-resolved"])
	}
	if got["sess-perm"] != models.TaskPendingActionPermission {
		t.Fatalf("sess-perm action = %q, want permission", got["sess-perm"])
	}
	if _, ok := got["sess-stale"]; ok {
		t.Fatalf("sess-stale should not inherit a previous turn permission: %#v", got["sess-stale"])
	}
	if _, ok := got["sess-missing"]; ok {
		t.Fatalf("sess-missing should not have a pending action: %#v", got["sess-missing"])
	}
}
