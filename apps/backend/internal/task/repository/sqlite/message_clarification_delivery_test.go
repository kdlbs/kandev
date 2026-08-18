package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 16, 20, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-delivery-intent", "session-delivery-intent")
	createPendingActionTurn(
		t, repo, "task-delivery-intent", "session-delivery-intent", "turn-delivery-intent", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-delivery-intent", "task-delivery-intent", "session-delivery-intent",
		"turn-delivery-intent", "pending-delivery-intent", "q1", base,
	)

	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-delivery-intent",
		clarificationStatusAnswered,
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1", "custom_text": "continue"}},
	)
	if err != nil || !claimed {
		t.Fatalf("CompleteActiveClarificationBundle = claimed %v, %v; want true, nil", claimed, err)
	}
	if got := claimedMessages[0].Metadata[clarificationResponseDeliveryPendingKey]; got != true {
		t.Fatalf("delivery intent = %v, want true", got)
	}

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	message, err := repo.GetMessage(ctx, "message-delivery-intent")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != clarificationStatusPending || message.Metadata["response"] != nil {
		t.Fatalf("recovered metadata = %#v, want pending without response", message.Metadata)
	}
	if marker := message.Metadata[clarificationResponseDeliveryPendingKey]; marker != nil {
		t.Fatalf("recovered delivery marker = %v, want absent", marker)
	}
}

func TestFinalizeClarificationResponseDeliveryPreventsRecovery(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 16, 25, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-delivery-finalize", "session-delivery-finalize")
	createPendingActionTurn(
		t, repo, "task-delivery-finalize", "session-delivery-finalize", "turn-delivery-finalize", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-delivery-finalize", "task-delivery-finalize", "session-delivery-finalize",
		"turn-delivery-finalize", "pending-delivery-finalize", "q1", base,
	)

	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-delivery-finalize",
		clarificationStatusAnswered,
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1", "custom_text": "continue"}},
	)
	if err != nil || !claimed {
		t.Fatalf("CompleteActiveClarificationBundle = claimed %v, %v; want true, nil", claimed, err)
	}
	finalizedMessages, finalized, err := repo.FinalizeClarificationResponseDelivery(
		ctx,
		"pending-delivery-finalize",
		clarificationStatusAnswered,
		claimedMessages,
	)
	if err != nil || !finalized {
		t.Fatalf("FinalizeClarificationResponseDelivery = finalized %v, %v; want true, nil", finalized, err)
	}
	if marker := finalizedMessages[0].Metadata[clarificationResponseDeliveryPendingKey]; marker != nil {
		t.Fatalf("finalized delivery marker = %v, want absent", marker)
	}
	_, restored, err := repo.RestoreActiveClarificationBundle(
		ctx,
		"pending-delivery-finalize",
		clarificationStatusAnswered,
		claimedMessages,
	)
	if err != nil || restored {
		t.Fatalf("RestoreActiveClarificationBundle after finalize = restored %v, %v; want false, nil", restored, err)
	}

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 0 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 0, nil", reconciled, err)
	}
	message, err := repo.GetMessage(ctx, "message-delivery-finalize")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != clarificationStatusAnswered || message.Metadata["response"] == nil {
		t.Fatalf("finalized metadata = %#v, want answered with response", message.Metadata)
	}
}

func TestClarificationDeliveryRecoveryRestoresDetachedBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 17, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-detached-recovery", "session-detached-recovery")
	createPendingActionTurn(
		t, repo, "task-detached-recovery", "session-detached-recovery",
		"turn-detached-recovery", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-detached-recovery", "task-detached-recovery",
		"session-detached-recovery", "turn-detached-recovery",
		"pending-detached-recovery", "q1", base,
	)
	setClarificationMessageMetadata(t, repo, "message-detached-recovery", func(metadata map[string]interface{}) {
		metadata["agent_disconnected"] = true
	})
	_, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-detached-recovery",
		clarificationStatusRejected,
		nil,
	)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v; want true, nil", claimed, err)
	}

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	message, err := repo.GetMessage(ctx, "message-detached-recovery")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != clarificationStatusPending {
		t.Fatalf("recovered status = %v, want pending", message.Metadata["status"])
	}
	if disconnected, _ := message.Metadata["agent_disconnected"].(bool); !disconnected {
		t.Fatalf("agent_disconnected = %v, want true after restore", message.Metadata["agent_disconnected"])
	}
	if _, ok := message.Metadata[clarificationResponseDeliveryPendingKey]; ok {
		t.Fatal("delivery marker present after restore")
	}
}

func TestClarificationDeliveryRecoveryDoesNotReactivateSupersededTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 16, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-delivery-superseded", "session-delivery-superseded")
	createPendingActionTurn(
		t, repo, "task-delivery-superseded", "session-delivery-superseded", "turn-delivery-old", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-delivery-superseded", "task-delivery-superseded",
		"session-delivery-superseded", "turn-delivery-old", "pending-delivery-superseded", "q1", base,
	)
	_, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-delivery-superseded",
		clarificationStatusAnswered,
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1", "custom_text": "continue"}},
	)
	if err != nil || !claimed {
		t.Fatalf("CompleteActiveClarificationBundle = claimed %v, %v; want true, nil", claimed, err)
	}
	createPendingActionTurn(
		t, repo, "task-delivery-superseded", "session-delivery-superseded", "turn-delivery-new",
		base.Add(time.Minute), base.Add(time.Minute),
	)

	reconciled, err := repo.ReconcileUnpublishedPromptTurns(ctx)
	if err != nil || reconciled != 1 {
		t.Fatalf("ReconcileUnpublishedPromptTurns = %d, %v; want 1, nil", reconciled, err)
	}
	message, err := repo.GetMessage(ctx, "message-delivery-superseded")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != clarificationStatusAnswered || message.Metadata["response"] == nil {
		t.Fatalf("superseded delivery metadata = %#v, want terminal answer", message.Metadata)
	}
	if marker := message.Metadata[clarificationResponseDeliveryPendingKey]; marker != nil {
		t.Fatalf("superseded delivery marker = %v, want absent", marker)
	}
}
