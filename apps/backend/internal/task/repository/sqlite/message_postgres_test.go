package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresPermissionClaimIgnoresEmptyMetadataRows exercises the real
// PostgreSQL JSONB predicate. It skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresPermissionClaimIgnoresEmptyMetadataRows(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-permission-empty-metadata-pg"
		sessionID = "session-permission-empty-metadata-pg"
		turnID    = "turn-permission-empty-metadata-pg"
	)
	seedPostgresTask(t, repo, taskID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "permission-valid-pg", TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypePermissionRequest,
		Metadata: map[string]any{"request_id": "request-1", "pending_id": "pending-1"},
	}); err != nil {
		t.Fatalf("CreateMessage(valid permission): %v", err)
	}
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content,
			 requests_input, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', '', '', FALSE, 'permission_request', '', ?, ?)
	`), "permission-empty-metadata-pg", sessionID, taskID, turnID, now, now); err != nil {
		t.Fatalf("seed empty-metadata permission: %v", err)
	}

	claimed, err := repo.ClaimPermissionResolution(ctx, models.PermissionResolutionClaimRequest{
		TaskID: taskID, SessionID: sessionID,
		Audit: models.PermissionResolutionAudit{
			ClaimID: "claim-pg", ActorKind: models.PermissionActorPersonalAccessToken,
			Source: models.PermissionSourceExternalMCP, RequestID: "request-1", PendingID: "pending-1",
			OptionID: "allow-once", OptionKind: "allow_once", SelectedAt: now,
		},
	})
	if err != nil || claimed == nil || claimed.Outcome != models.PermissionClaimed || claimed.Message == nil || claimed.Message.ID != "permission-valid-pg" {
		t.Fatalf("claim = %+v, err=%v, want the valid permission row", claimed, err)
	}
}
