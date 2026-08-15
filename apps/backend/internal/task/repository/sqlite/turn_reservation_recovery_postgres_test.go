package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresTurnAuthorityToleratesStringFlagEncodings(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 22, 0, 0, 0, time.UTC)
	for index, tt := range []struct {
		name          string
		pending       interface{}
		attempted     interface{}
		wantCurrentID string
	}{
		{name: "pending string true", pending: "true", wantCurrentID: "turn-previous"},
		{name: "pending string one", pending: "1", wantCurrentID: "turn-previous"},
		{name: "attempted string true", pending: true, attempted: "true", wantCurrentID: "turn-reserved"},
		{name: "attempted string one", pending: true, attempted: "1", wantCurrentID: "turn-reserved"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			taskID := fmt.Sprintf("task-string-flags-pg-%d", index)
			sessionID := fmt.Sprintf("session-string-flags-pg-%d", index)
			previousID := fmt.Sprintf("turn-previous-pg-%d", index)
			reservedID := fmt.Sprintf("turn-reserved-pg-%d", index)
			seedSessionForTurns(t, repo, taskID, sessionID)
			createRecoveryTurn(t, repo, taskID, sessionID, previousID, base, nil)
			createRecoveryTurn(t, repo, taskID, sessionID, reservedID, base.Add(time.Minute), map[string]interface{}{
				models.TurnMetaKeyPromptDispatchPending:   tt.pending,
				models.TurnMetaKeyPromptDispatchAttempted: tt.attempted,
			})

			active, err := repo.GetActiveTurnBySessionID(ctx, sessionID)
			if err != nil {
				t.Fatalf("GetActiveTurnBySessionID: %v", err)
			}
			wantCurrentID := tt.wantCurrentID
			if wantCurrentID == "turn-previous" {
				wantCurrentID = previousID
			} else {
				wantCurrentID = reservedID
			}
			if active == nil || active.ID != wantCurrentID {
				t.Fatalf("active turn = %#v, want %s", active, wantCurrentID)
			}
		})
	}
}

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

func TestPostgresReconcileAmbiguousEmptyPromptTurn(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-reservation-accepted-pg"
	const sessionID = "session-reservation-accepted-pg"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 15, 21, 30, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-clarification-accepted-pg", base, nil)
	createRecoveryClarification(
		t, repo, "message-accepted-pg", taskID, sessionID,
		"turn-clarification-accepted-pg", "pending-accepted-pg", base,
	)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-reservation-accepted-pg", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchAttempted:               true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-accepted-pg",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification-accepted-pg",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-accepted-pg"},
	})

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	active, err := repo.GetActiveTurnBySessionID(ctx, sessionID)
	if err != nil || active.ID != "turn-reservation-accepted-pg" {
		t.Fatalf("accepted active turn = %#v, %v", active, err)
	}
	message, err := repo.GetMessage(ctx, "message-accepted-pg")
	if err != nil {
		t.Fatalf("GetMessage(accepted): %v", err)
	}
	if message.Metadata["status"] != "answered" {
		t.Fatalf("accepted postgres claim status = %v, want answered", message.Metadata["status"])
	}
}
