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

func TestPostgresDeliveryRecoverySkipsBundleOwnedByFailedReservation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 21, 20, 0, 0, time.UTC)
	seedSessionForTurns(t, repo, "task-owned-pg", "session-owned-pg")
	createRecoveryTurn(t, repo, "task-owned-pg", "session-owned-pg", "turn-owned-pg", base, nil)
	for index, messageID := range []string{"message-owned-a-pg", "message-owned-b-pg"} {
		createRecoveryClarification(
			t, repo, messageID, "task-owned-pg", "session-owned-pg", "turn-owned-pg",
			"pending-owned-pg", base.Add(time.Duration(index)*time.Second),
		)
	}
	markRecoveryClarificationDeliveryPending(t, repo, "message-owned-a-pg")
	createRecoveryTurn(
		t, repo, "task-owned-pg", "session-owned-pg", "turn-reserved-owned-pg", base.Add(time.Minute),
		map[string]interface{}{
			models.TurnMetaKeyPromptDispatchPending:                 true,
			models.TurnMetaKeyPromptDispatchAttempted:               true,
			models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-owned-pg",
			models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-owned-pg",
			models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-owned-a-pg", "message-owned-b-pg"},
		},
	)

	seedPendingActionSession(t, repo, "task-independent-pg", "session-independent-pg")
	createPendingActionTurn(
		t, repo, "task-independent-pg", "session-independent-pg", "turn-independent-pg", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-independent-pg", "task-independent-pg", "session-independent-pg",
		"turn-independent-pg", "pending-independent-pg", "q1", base,
	)
	_, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-independent-pg",
		clarificationStatusAnswered,
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("claim independent delivery = %v, %v; want true, nil", claimed, err)
	}

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err == nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1 and reservation error", reconciled, err)
	}
	owned, err := repo.GetMessage(ctx, "message-owned-a-pg")
	if err != nil {
		t.Fatalf("GetMessage(owned): %v", err)
	}
	if owned.Metadata[clarificationResponseDeliveryPendingKey] != true {
		t.Fatalf("owned delivery marker changed: %#v", owned.Metadata)
	}
	independent, err := repo.GetMessage(ctx, "message-independent-pg")
	if err != nil {
		t.Fatalf("GetMessage(independent): %v", err)
	}
	if independent.Metadata["status"] != clarificationStatusPending {
		t.Fatalf("independent delivery status = %v, want pending", independent.Metadata["status"])
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
	if pending, _ := active.Metadata[models.TurnMetaKeyPromptDispatchStartEventPending].(bool); !pending {
		t.Fatalf("accepted active turn metadata = %#v, want durable start-event marker", active.Metadata)
	}
	pendingEvents, err := repo.ListTurnsPendingStartEvent(ctx)
	if err != nil {
		t.Fatalf("ListTurnsPendingStartEvent: %v", err)
	}
	if len(pendingEvents) != 1 || pendingEvents[0].ID != active.ID {
		t.Fatalf("turns pending start event = %#v, want %s", pendingEvents, active.ID)
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
		updated, _, _, updateErr := repo.UpdateActiveTurnMetadata(
			ctx,
			sessionID,
			turnID,
			map[string]interface{}{
				models.TurnMetaKeyPromptDispatchPending:   true,
				models.TurnMetaKeyPromptDispatchAttempted: true,
			},
			nil,
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

func TestPostgresUpdateTurnUsesSessionTurnLock(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 5)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-full-metadata-lock-pg"
	const sessionID = "session-full-metadata-lock-pg"
	const turnID = "turn-full-metadata-lock-pg"
	seedSessionForTurns(t, repo, taskID, sessionID)
	createRecoveryTurn(t, repo, taskID, sessionID, turnID, time.Now().UTC(), nil)
	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	turn.Metadata = map[string]interface{}{"prompt_usage": true}

	blocker, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin session turn lock blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback() })
	if err := lockSessionTurnWrites(ctx, blocker, repo.db.DriverName(), sessionID); err != nil {
		t.Fatalf("hold session turn lock: %v", err)
	}

	updateDone := make(chan error, 1)
	go func() { updateDone <- repo.UpdateTurn(ctx, turn) }()
	waitForPostgresLockWait(t, ctx, repo, "pg_advisory_xact_lock")
	select {
	case updateErr := <-updateDone:
		t.Fatalf("full metadata update bypassed session turn lock: %v", updateErr)
	default:
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release session turn lock: %v", err)
	}
	select {
	case updateErr := <-updateDone:
		if updateErr != nil {
			t.Fatalf("full metadata update after lock release: %v", updateErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for full metadata update")
	}
}
