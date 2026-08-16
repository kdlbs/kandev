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
			var wantCurrentID string
			switch tt.wantCurrentID {
			case "turn-previous":
				wantCurrentID = previousID
			case "turn-reserved":
				wantCurrentID = reservedID
			default:
				t.Fatalf("unknown expected turn %q", tt.wantCurrentID)
			}
			if active == nil || active.ID != wantCurrentID {
				t.Fatalf("active turn = %#v, want %s", active, wantCurrentID)
			}
		})
	}
}

func TestPostgresListTurnsHidesEmptyReservationUntilMessageEvidence(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-turn-list-pg"
	const sessionID = "session-turn-list-pg"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 16, 9, 0, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-accepted-pg", base, nil)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-reserved-pg", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:   true,
		models.TurnMetaKeyPromptDispatchAttempted: true,
	})

	listed, err := repo.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListTurnsBySession before output: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "turn-accepted-pg" {
		t.Fatalf("listed turns before output = %#v, want only accepted predecessor", listed)
	}

	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-reserved-output-pg", TaskSessionID: sessionID, TaskID: taskID,
		TurnID: "turn-reserved-pg", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeMessage, Content: "accepted output", CreatedAt: base.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	listed, err = repo.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListTurnsBySession after output: %v", err)
	}
	if len(listed) != 2 || listed[1].ID != "turn-reserved-pg" {
		t.Fatalf("listed turns after output = %#v, want reservation restored", listed)
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

func TestPostgresActiveTurnMetadataUpdateUsesSessionTurnLock(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 5)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-metadata-lock-pg"
	const sessionID = "session-metadata-lock-pg"
	const turnID = "turn-metadata-lock-pg"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 16, 13, 45, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, turnID, base, map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending: true,
	})

	blocker, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin session turn lock blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback() })
	if err := lockSessionTurnWrites(ctx, blocker, repo.db.DriverName(), sessionID); err != nil {
		t.Fatalf("hold session turn lock: %v", err)
	}

	type updateResult struct {
		updated bool
		err     error
	}
	updateDone := make(chan updateResult, 1)
	go func() {
		updated, _, updateErr := repo.UpdateActiveTurnMetadata(
			ctx,
			sessionID,
			turnID,
			map[string]interface{}{
				models.TurnMetaKeyPromptDispatchPending:   true,
				models.TurnMetaKeyPromptDispatchAttempted: true,
			},
		)
		updateDone <- updateResult{updated: updated, err: updateErr}
	}()
	waitForPostgresLockWait(t, ctx, repo, "pg_advisory_xact_lock")
	select {
	case result := <-updateDone:
		t.Fatalf("metadata update bypassed session turn lock: %+v", result)
	default:
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release session turn lock: %v", err)
	}
	select {
	case result := <-updateDone:
		if result.err != nil || !result.updated {
			t.Fatalf("metadata update after lock release = %+v, want updated", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for metadata update")
	}
}
