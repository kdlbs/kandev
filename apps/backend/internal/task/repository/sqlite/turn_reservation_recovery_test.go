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

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	turn, err := repo.GetTurn(ctx, "turn-message-backed")
	if err != nil {
		t.Fatalf("GetTurn(message-backed): %v", err)
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
