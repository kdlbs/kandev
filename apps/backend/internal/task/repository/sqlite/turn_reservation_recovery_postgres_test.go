package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresReconcileUnpublishedPromptTurns pins restart recovery on the
// second repository dialect. It skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresReconcileUnpublishedPromptTurns(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-reservation-recovery-pg"
	const sessionID = "session-reservation-recovery-pg"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 15, 21, 0, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-clarification-pg", base, nil)
	createRecoveryClarification(
		t, repo, "message-claimed-pg", taskID, sessionID,
		"turn-clarification-pg", "pending-recovery-pg", base,
	)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-unpublished-pg", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-recovery-pg",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification-pg",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-claimed-pg"},
	})

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	if _, err := repo.GetTurn(ctx, "turn-unpublished-pg"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetTurn(unpublished) error = %v, want sql.ErrNoRows", err)
	}
	message, err := repo.GetMessage(ctx, "message-claimed-pg")
	if err != nil {
		t.Fatalf("GetMessage(claimed): %v", err)
	}
	if message.Metadata["status"] != "pending" || message.Metadata["response"] != nil {
		t.Fatalf("recovered postgres metadata = %#v", message.Metadata)
	}
}
