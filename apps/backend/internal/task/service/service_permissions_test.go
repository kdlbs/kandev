package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestService_ExpirePendingPermissionsForSession_MarksOnlyPending guards the
// turn-complete sweep added for issue #717: when an agent finishes its turn,
// any permission_request messages still in pending status must transition to
// expired so the UI stops showing approval cards. Already-resolved messages
// (approved/rejected/expired) must not be re-touched.
func TestService_ExpirePendingPermissionsForSession_MarksOnlyPending(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-perm-1")

	mustCreatePermMsg(t, repo, sessionID, turnID, "msg-pending-1", "pend-1", map[string]any{
		"pending_id": "pend-1",
	})
	mustCreatePermMsg(t, repo, sessionID, turnID, "msg-pending-2", "pend-2", map[string]any{
		"pending_id": "pend-2",
		"status":     "pending",
	})
	mustCreatePermMsg(t, repo, sessionID, turnID, "msg-approved", "pend-3", map[string]any{
		"pending_id": "pend-3",
		"status":     "approved",
	})
	mustCreatePermMsg(t, repo, sessionID, turnID, "msg-rejected", "pend-4", map[string]any{
		"pending_id": "pend-4",
		"status":     "rejected",
	})
	mustCreatePermMsg(t, repo, sessionID, turnID, "msg-already-expired", "pend-5", map[string]any{
		"pending_id": "pend-5",
		"status":     "expired",
	})

	count, err := svc.ExpirePendingPermissionsForSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ExpirePendingPermissionsForSession: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 pending messages expired, got %d", count)
	}

	cases := []struct {
		id   string
		want string
	}{
		{"msg-pending-1", "expired"},
		{"msg-pending-2", "expired"},
		{"msg-approved", "approved"},
		{"msg-rejected", "rejected"},
		{"msg-already-expired", "expired"},
	}
	for _, c := range cases {
		got, err := repo.GetMessage(ctx, c.id)
		if err != nil {
			t.Fatalf("GetMessage(%s): %v", c.id, err)
		}
		status, _ := got.Metadata["status"].(string)
		if status != c.want {
			t.Errorf("%s: status = %q, want %q", c.id, status, c.want)
		}
	}
}

// TestService_ExpirePendingPermissionsForSession_NoPending is a no-op
// fast-path: when nothing is pending, the sweep must return zero and not
// fail.
func TestService_ExpirePendingPermissionsForSession_NoPending(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	count, err := svc.ExpirePendingPermissionsForSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ExpirePendingPermissionsForSession: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 with no permission messages, got %d", count)
	}
}

func mustCreatePermMsg(t *testing.T, repo interface {
	CreateMessage(ctx context.Context, m *models.Message) error
}, sessionID, turnID, id, _ string, metadata map[string]any) {
	t.Helper()
	msg := &models.Message{
		ID:            id,
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		AuthorID:      "agent-123",
		Content:       "Permission required",
		Type:          models.MessageTypePermissionRequest,
		Metadata:      metadata,
	}
	if err := repo.CreateMessage(context.Background(), msg); err != nil {
		t.Fatalf("seed permission message %s: %v", id, err)
	}
}
