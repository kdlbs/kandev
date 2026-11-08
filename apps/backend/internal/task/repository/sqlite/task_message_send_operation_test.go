package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSendOperationStoreBehavior(t *testing.T) {
	exerciseSendOperationStore(t, newRepoForSessionTests(t))
}

func newSendOperation(id, operationID, senderSessionID string) *models.TaskMessageSendOperation {
	return &models.TaskMessageSendOperation{
		ID: id, OperationID: operationID, SenderTaskID: "sender-task", SenderSessionID: senderSessionID,
		TargetTaskID: "target-task", RequestedPrompt: "hello",
	}
}

// exerciseSendOperationStore pins the dialect-sensitive claim, lookup, settle
// and prune behavior shared by SQLite and PostgreSQL.
func exerciseSendOperationStore(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()

	if err := repo.ClaimSendOperation(ctx, newSendOperation("row-1", "Op-1", "sess-a")); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := repo.ClaimSendOperation(ctx, newSendOperation("row-2", "Op-1", "sess-a")); !errors.Is(err, models.ErrSendOperationNotClaimed) {
		t.Fatalf("duplicate claim err = %v, want ErrSendOperationNotClaimed", err)
	}
	// Identities are compared byte-wise and scoped to the sender session.
	if err := repo.ClaimSendOperation(ctx, newSendOperation("row-3", "op-1", "sess-a")); err != nil {
		t.Fatalf("case-distinct claim: %v", err)
	}
	if err := repo.ClaimSendOperation(ctx, newSendOperation("row-4", "Op-1", "sess-b")); err != nil {
		t.Fatalf("other-session claim: %v", err)
	}
	if _, err := repo.GetSendOperation(ctx, "sender-task", "sess-c", "Op-1"); !errors.Is(err, models.ErrSendOperationNotFound) {
		t.Fatalf("foreign sender read err = %v, want ErrSendOperationNotFound", err)
	}

	updated, err := repo.SettleSendOperation(ctx, "sender-task", "sess-a", "Op-1",
		models.SendOperationStateCommitted, "target-sess", "queued", "", "queue-1", "")
	if err != nil || !updated {
		t.Fatalf("settle = %v, %v; want true, nil", updated, err)
	}
	if updated, err = repo.SettleSendOperation(ctx, "sender-task", "sess-a", "Op-1",
		models.SendOperationStateFailed, "", "", "", "", models.SendOperationFailureOutcomeUnknown); err != nil || updated {
		t.Fatalf("second settle = %v, %v; want false, nil", updated, err)
	}
	for operationID, wantState := range map[string]string{"Op-1": models.SendOperationStateCommitted, "op-1": models.SendOperationStatePending} {
		got, err := repo.GetSendOperation(ctx, "sender-task", "sess-a", operationID)
		if err != nil || got.State != wantState {
			t.Fatalf("%s state = %v (err %v), want %s", operationID, got, err, wantState)
		}
	}

	// Age every row past the stale-pending window: only the pending rows are
	// settled as failed/outcome_unknown, and the committed outcome is kept.
	old := time.Now().UTC().Add(-2 * sendOperationStalePending)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_message_send_operations SET created_at = ?`), old); err != nil {
		t.Fatalf("age rows: %v", err)
	}
	settled, removed, err := repo.PruneSendOperations(ctx, SendOperationPruneBatch)
	if err != nil || settled != 2 || removed != 0 {
		t.Fatalf("prune = %d settled, %d removed, err %v; want 2, 0, nil", settled, removed, err)
	}
	committed, err := repo.GetSendOperation(ctx, "sender-task", "sess-a", "Op-1")
	if err != nil || committed.State != models.SendOperationStateCommitted || committed.QueuedEntryID != "queue-1" {
		t.Fatalf("committed row after prune = %+v (err %v)", committed, err)
	}
	stale, err := repo.GetSendOperation(ctx, "sender-task", "sess-a", "op-1")
	if err != nil || stale.State != models.SendOperationStateFailed || stale.FailureCode != models.SendOperationFailureOutcomeUnknown {
		t.Fatalf("stale row after prune = %+v (err %v)", stale, err)
	}
}
