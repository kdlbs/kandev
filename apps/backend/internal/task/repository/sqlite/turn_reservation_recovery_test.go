package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReconcileUnpublishedPromptTurnsRestoresOnlyClaimedMessages(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-reservation-recovery"
	const sessionID = "session-reservation-recovery"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 15, 19, 0, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-clarification", base, nil)
	createRecoveryClarification(
		t, repo, "message-claimed", taskID, sessionID, "turn-clarification", "pending-recovery", base,
	)
	markRecoveryClarificationDeliveryPending(t, repo, "message-claimed")
	createRecoveryClarification(
		t, repo, "message-terminal", taskID, sessionID, "turn-clarification", "pending-recovery", base.Add(time.Second),
	)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-unpublished", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-recovery",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-claimed"},
	})
	mutationTime := base.Add(2*time.Hour + 123*time.Millisecond)
	repo.clockNow = func() time.Time { return mutationTime }
	previousUpdatedAt := setMessageUpdatedAtBeforeMutation(t, repo, "message-claimed", mutationTime)

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	if _, err := repo.GetTurn(ctx, "turn-unpublished"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetTurn(unpublished) error = %v, want sql.ErrNoRows", err)
	}
	claimed, err := repo.GetMessage(ctx, "message-claimed")
	if err != nil {
		t.Fatalf("GetMessage(claimed): %v", err)
	}
	if claimed.Metadata["status"] != "pending" || claimed.Metadata["response"] != nil {
		t.Fatalf("claimed metadata = %#v, want pending without response", claimed.Metadata)
	}
	if !claimed.UpdatedAt.After(previousUpdatedAt) {
		t.Fatalf("restored updated_at = %s, want after %s", claimed.UpdatedAt, previousUpdatedAt)
	}
	if !claimed.UpdatedAt.Equal(mutationTime) {
		t.Fatalf("restored updated_at = %s, want injected mutation time %s", claimed.UpdatedAt, mutationTime)
	}
	terminal, err := repo.GetMessage(ctx, "message-terminal")
	if err != nil {
		t.Fatalf("GetMessage(terminal): %v", err)
	}
	if terminal.Metadata["status"] != "answered" || terminal.Metadata["response"] == nil {
		t.Fatalf("unclaimed terminal metadata = %#v, want unchanged", terminal.Metadata)
	}
}

func TestReconcileUnpublishedPromptTurnsRejectsMalformedClaimMessageIDs(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-malformed-recovery"
	const sessionID = "session-malformed-recovery"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 16, 0, 20, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-clarification", base, nil)
	createRecoveryClarification(
		t, repo, "message-claimed", taskID, sessionID, "turn-clarification", "pending-recovery", base,
	)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-unpublished", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-recovery",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []interface{}{"message-claimed", float64(7)},
	})

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err == nil || reconciled != 0 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 0 and malformed metadata error", reconciled, err)
	}
	if !strings.Contains(err.Error(), "turn-unpublished") || !strings.Contains(err.Error(), sessionID) {
		t.Fatalf("reconciliation error lacks actionable turn/session identity: %v", err)
	}
	if _, err := repo.GetTurn(ctx, "turn-unpublished"); err != nil {
		t.Fatalf("malformed reservation was deleted: %v", err)
	}
	message, err := repo.GetMessage(ctx, "message-claimed")
	if err != nil {
		t.Fatalf("GetMessage(claimed): %v", err)
	}
	if message.Metadata["status"] != "answered" {
		t.Fatalf("claimed message status = %v, want answered", message.Metadata["status"])
	}
}

func TestReconcileUnpublishedPromptTurnsRejectsPartialClarificationRecoveryMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-partial-recovery-metadata"
	const sessionID = "session-partial-recovery-metadata"
	const turnID = "turn-partial-recovery-metadata"
	seedSessionForTurns(t, repo, taskID, sessionID)
	createRecoveryTurn(t, repo, taskID, sessionID, turnID, time.Now().UTC(), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                true,
		models.TurnMetaKeyPromptDispatchAttempted:              true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID: "pending-without-turn-or-messages",
	})

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err == nil || reconciled != 0 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 0 and partial metadata error", reconciled, err)
	}
	turn, getErr := repo.GetTurn(ctx, turnID)
	if getErr != nil {
		t.Fatalf("GetTurn(partial recovery metadata): %v", getErr)
	}
	if pending, _ := turn.Metadata[models.TurnMetaKeyPromptDispatchPending].(bool); !pending {
		t.Fatalf("partial recovery metadata was cleared: %#v", turn.Metadata)
	}
}

func TestReconcileUnpublishedPromptTurnsContinuesAfterMalformedReservation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 0, 25, 0, 0, time.UTC)
	seedSessionForTurns(t, repo, "task-malformed-mixed", "session-malformed-mixed")
	createRecoveryTurn(
		t, repo, "task-malformed-mixed", "session-malformed-mixed", "turn-malformed-mixed", base,
		map[string]interface{}{
			models.TurnMetaKeyPromptDispatchPending:                 true,
			models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []interface{}{"message", float64(7)},
		},
	)
	seedSessionForTurns(t, repo, "task-healthy-mixed", "session-healthy-mixed")
	createRecoveryTurn(
		t, repo, "task-healthy-mixed", "session-healthy-mixed", "turn-healthy-mixed", base.Add(time.Minute),
		map[string]interface{}{models.TurnMetaKeyPromptDispatchPending: true},
	)

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err == nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1 and malformed metadata error", reconciled, err)
	}
	if _, err := repo.GetTurn(ctx, "turn-malformed-mixed"); err != nil {
		t.Fatalf("malformed reservation changed: %v", err)
	}
	if _, err := repo.GetTurn(ctx, "turn-healthy-mixed"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("healthy reservation error = %v, want recovered deletion", err)
	}
}

func TestReconcileUnpublishedPromptTurnsRunsDeliveryRecoveryAfterReservationError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 20, 40, 0, 0, time.UTC)
	seedSessionForTurns(t, repo, "task-malformed-delivery", "session-malformed-delivery")
	createRecoveryTurn(
		t, repo, "task-malformed-delivery", "session-malformed-delivery",
		"turn-malformed-delivery", base,
		map[string]interface{}{
			models.TurnMetaKeyPromptDispatchPending:                 true,
			models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []interface{}{"message", float64(7)},
		},
	)

	seedPendingActionSession(t, repo, "task-independent-delivery", "session-independent-delivery")
	createPendingActionTurn(
		t, repo, "task-independent-delivery", "session-independent-delivery",
		"turn-independent-delivery", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-independent-delivery", "task-independent-delivery",
		"session-independent-delivery", "turn-independent-delivery",
		"pending-independent-delivery", "q1", base,
	)
	_, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-independent-delivery",
		clarificationStatusAnswered,
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1", "custom_text": "continue"}},
	)
	if err != nil || !claimed {
		t.Fatalf("claim independent delivery = %v, %v; want true, nil", claimed, err)
	}

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err == nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1 and reservation error", reconciled, err)
	}
	if !strings.Contains(err.Error(), "turn-malformed-delivery") {
		t.Fatalf("reservation error missing turn identity: %v", err)
	}
	message, err := repo.GetMessage(ctx, "message-independent-delivery")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != clarificationStatusPending || message.Metadata["response"] != nil {
		t.Fatalf("independent delivery metadata = %#v, want recovered pending", message.Metadata)
	}
}

func TestReconcileUnpublishedPromptTurnsAcceptsMessageBackedReservation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-reservation-output"
	const sessionID = "session-reservation-output"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 15, 20, 0, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-message-backed", base, map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-output",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-clarification"},
	})
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-output", TaskID: taskID, TaskSessionID: sessionID,
		TurnID: "turn-message-backed", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeMessage, Content: "accepted output", CreatedAt: base.Add(time.Second),
	}); err != nil {
		t.Fatalf("CreateMessage(output): %v", err)
	}
	mutationTime := base.Add(2*time.Hour + 456*time.Millisecond)
	repo.clockNow = func() time.Time { return mutationTime }
	previousUpdatedAt := setTurnUpdatedAtBeforeMutation(t, repo, "turn-message-backed", mutationTime)

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	turn, err := repo.GetTurn(ctx, "turn-message-backed")
	if err != nil {
		t.Fatalf("GetTurn(message-backed): %v", err)
	}
	if !turn.UpdatedAt.After(previousUpdatedAt) {
		t.Fatalf("published updated_at = %s, want after %s", turn.UpdatedAt, previousUpdatedAt)
	}
	if !turn.UpdatedAt.Equal(mutationTime) {
		t.Fatalf("published updated_at = %s, want injected mutation time %s", turn.UpdatedAt, mutationTime)
	}
	for _, key := range []string{
		models.TurnMetaKeyPromptDispatchPending,
		models.TurnMetaKeyPromptDispatchClarificationPendingID,
		models.TurnMetaKeyPromptDispatchClarificationTurnID,
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs,
	} {
		if _, exists := turn.Metadata[key]; exists {
			t.Fatalf("message-backed turn retained %q: %#v", key, turn.Metadata)
		}
	}
	if pending, _ := turn.Metadata[models.TurnMetaKeyPromptDispatchStartEventPending].(bool); !pending {
		t.Fatalf("message-backed turn metadata = %#v, want durable start-event marker", turn.Metadata)
	}
	pendingEvents, err := repo.ListTurnsPendingStartEvent(ctx)
	if err != nil {
		t.Fatalf("ListTurnsPendingStartEvent: %v", err)
	}
	if len(pendingEvents) != 1 || pendingEvents[0].ID != turn.ID {
		t.Fatalf("turns pending start event = %#v, want %s", pendingEvents, turn.ID)
	}
}

func TestReconcileUnpublishedPromptTurnsPreservesAmbiguousEmptyReservation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-reservation-accepted"
	const sessionID = "session-reservation-accepted"
	seedSessionForTurns(t, repo, taskID, sessionID)
	base := time.Date(2026, time.August, 15, 20, 30, 0, 0, time.UTC)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-clarification", base, nil)
	createRecoveryClarification(
		t, repo, "message-accepted-claim", taskID, sessionID, "turn-clarification",
		"pending-accepted", base,
	)
	markRecoveryClarificationDeliveryPending(t, repo, "message-accepted-claim")
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-accepted-reservation", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchAttempted:               true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-accepted",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-accepted-claim"},
	})

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	turn, err := repo.GetTurn(ctx, "turn-accepted-reservation")
	if err != nil {
		t.Fatalf("GetTurn(accepted reservation): %v", err)
	}
	for _, key := range []string{
		models.TurnMetaKeyPromptDispatchPending,
		models.TurnMetaKeyPromptDispatchAttempted,
		models.TurnMetaKeyPromptDispatchClarificationPendingID,
		models.TurnMetaKeyPromptDispatchClarificationTurnID,
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs,
	} {
		if _, exists := turn.Metadata[key]; exists {
			t.Fatalf("accepted reservation retained %q: %#v", key, turn.Metadata)
		}
	}
	if pending, _ := turn.Metadata[models.TurnMetaKeyPromptDispatchStartEventPending].(bool); !pending {
		t.Fatalf("accepted reservation metadata = %#v, want durable start-event marker", turn.Metadata)
	}
	claimed, err := repo.GetMessage(ctx, "message-accepted-claim")
	if err != nil {
		t.Fatalf("GetMessage(accepted claim): %v", err)
	}
	if claimed.Metadata["status"] != "answered" {
		t.Fatalf("accepted claim status = %v, want answered", claimed.Metadata["status"])
	}
}

func TestReconcileUnpublishedPromptTurnsRejectsPartialResponseDeliveryIntent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-partial-delivery"
	const sessionID = "session-partial-delivery"
	const pendingID = "pending-partial-delivery"
	base := time.Date(2026, time.August, 16, 20, 20, 0, 0, time.UTC)
	seedSessionForTurns(t, repo, taskID, sessionID)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-clarification", base, nil)
	for index, messageID := range []string{"message-a", "message-b"} {
		createRecoveryClarification(
			t, repo, messageID, taskID, sessionID, "turn-clarification", pendingID,
			base.Add(time.Duration(index)*time.Second),
		)
	}
	markRecoveryClarificationDeliveryPending(t, repo, "message-a")
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-unpublished", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchAttempted:               true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  pendingID,
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-a", "message-b"},
	})

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err == nil || reconciled != 0 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 0 and partial intent error", reconciled, err)
	}
	if !strings.Contains(err.Error(), "expected 2 messages, found 1") {
		t.Fatalf("partial response delivery error = %v", err)
	}
	if _, err := repo.GetTurn(ctx, "turn-unpublished"); err != nil {
		t.Fatalf("partial response delivery reservation changed: %v", err)
	}
}

func TestReconcileUnpublishedPromptTurnsPreservesToleratedAttemptMarkers(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value interface{}
	}{
		{name: "boolean", value: true},
		{name: "string_true", value: metadataTrueString},
		{name: "string_one", value: "1"},
		{name: "number_one", value: float64(1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID := "task-attempt-marker-" + tt.name
			sessionID := "session-attempt-marker-" + tt.name
			turnID := "turn-attempt-marker-" + tt.name
			seedSessionForTurns(t, repo, taskID, sessionID)
			createRecoveryTurn(t, repo, taskID, sessionID, turnID, time.Now().UTC(), map[string]interface{}{
				models.TurnMetaKeyPromptDispatchPending:   true,
				models.TurnMetaKeyPromptDispatchAttempted: tt.value,
			})

			reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
			if err != nil || reconciled != 1 {
				t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
			}
			turn, err := repo.GetTurn(ctx, turnID)
			if err != nil {
				t.Fatalf("attempted reservation was deleted: %v", err)
			}
			if _, exists := turn.Metadata[models.TurnMetaKeyPromptDispatchAttempted]; exists {
				t.Fatalf("attempt marker was not cleared: %#v", turn.Metadata)
			}
		})
	}
}

func TestDeleteTurnIfUnreferencedRemovesDefinitivelyRejectedAttempt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-reservation-rollback"
	const sessionID = "session-reservation-rollback"
	seedSessionForTurns(t, repo, taskID, sessionID)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-accepted-rollback", time.Now().UTC(), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:   true,
		models.TurnMetaKeyPromptDispatchAttempted: true,
	})

	deleted, err := repo.DeleteTurnIfUnreferenced(ctx, sessionID, "turn-accepted-rollback")
	if err != nil || !deleted {
		t.Fatalf("DeleteTurnIfUnreferenced = %v, %v; want true, nil", deleted, err)
	}
	if _, err := repo.GetTurn(ctx, "turn-accepted-rollback"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("definitively rejected attempt remains: %v", err)
	}
}

func setTurnUpdatedAtBeforeMutation(
	t *testing.T,
	repo *Repository,
	turnID string,
	mutationTime time.Time,
) time.Time {
	t.Helper()
	updatedAt := mutationTime.Add(-time.Nanosecond)
	if _, err := repo.db.Exec(
		repo.db.Rebind(`UPDATE task_session_turns SET updated_at = ? WHERE id = ?`),
		updatedAt,
		turnID,
	); err != nil {
		t.Fatalf("seed turn updated_at: %v", err)
	}
	return updatedAt
}

func createRecoveryTurn(
	t *testing.T,
	repo *Repository,
	taskID, sessionID, turnID string,
	startedAt time.Time,
	metadata map[string]interface{},
) {
	t.Helper()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: startedAt, CreatedAt: startedAt, Metadata: metadata,
	}); err != nil {
		t.Fatalf("CreateTurn(%s): %v", turnID, err)
	}
}

func createRecoveryClarification(
	t *testing.T,
	repo *Repository,
	messageID, taskID, sessionID, turnID, pendingID string,
	createdAt time.Time,
) {
	t.Helper()
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest,
		Content: "Continue?", CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			"pending_id": pendingID, "question_id": messageID, "status": "answered",
			"response": map[string]interface{}{"custom_text": "yes"},
		},
	}); err != nil {
		t.Fatalf("CreateMessage(%s): %v", messageID, err)
	}
}

func markRecoveryClarificationDeliveryPending(t *testing.T, repo *Repository, messageID string) {
	t.Helper()
	message, err := repo.GetMessage(context.Background(), messageID)
	if err != nil {
		t.Fatalf("GetMessage(%s): %v", messageID, err)
	}
	message.Metadata[clarificationResponseDeliveryPendingKey] = true
	if err := repo.UpdateMessage(context.Background(), message); err != nil {
		t.Fatalf("mark response delivery pending for %s: %v", messageID, err)
	}
}
