package sqlite

import (
	"context"
	"database/sql"
	"errors"
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
	createRecoveryClarification(
		t, repo, "message-terminal", taskID, sessionID, "turn-clarification", "pending-recovery", base.Add(time.Second),
	)
	createRecoveryTurn(t, repo, taskID, sessionID, "turn-unpublished", base.Add(time.Minute), map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:                 true,
		models.TurnMetaKeyPromptDispatchClarificationPendingID:  "pending-recovery",
		models.TurnMetaKeyPromptDispatchClarificationTurnID:     "turn-clarification",
		models.TurnMetaKeyPromptDispatchClarificationMessageIDs: []string{"message-claimed"},
	})
	previousUpdatedAt := setMessageUpdatedAtJustAfterSecond(t, repo, "message-claimed")

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
	terminal, err := repo.GetMessage(ctx, "message-terminal")
	if err != nil {
		t.Fatalf("GetMessage(terminal): %v", err)
	}
	if terminal.Metadata["status"] != "answered" || terminal.Metadata["response"] == nil {
		t.Fatalf("unclaimed terminal metadata = %#v, want unchanged", terminal.Metadata)
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
	previousUpdatedAt := setTurnUpdatedAtJustAfterSecond(t, repo, "turn-message-backed")

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
	claimed, err := repo.GetMessage(ctx, "message-accepted-claim")
	if err != nil {
		t.Fatalf("GetMessage(accepted claim): %v", err)
	}
	if claimed.Metadata["status"] != "answered" {
		t.Fatalf("accepted claim status = %v, want answered", claimed.Metadata["status"])
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

func setTurnUpdatedAtJustAfterSecond(t *testing.T, repo *Repository, turnID string) time.Time {
	t.Helper()
	for time.Now().UTC().Nanosecond() >= int((100 * time.Millisecond).Nanoseconds()) {
		time.Sleep(5 * time.Millisecond)
	}
	updatedAt := time.Now().UTC().Truncate(time.Second).Add(time.Nanosecond)
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
