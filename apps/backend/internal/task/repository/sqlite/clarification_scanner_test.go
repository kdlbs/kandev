package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedClarificationBundle seeds a session/turn plus a pending multi-question
// clarification bundle (two clarification_request messages sharing one
// pending_id), the fixture every clarification repository path reads.
func seedClarificationBundle(t *testing.T, repo *Repository, taskID, sessionID, turnID, pendingID string, base time.Time) {
	t.Helper()
	seedPendingActionSession(t, repo, taskID, sessionID)
	createPendingActionTurn(t, repo, taskID, sessionID, turnID, base, base)
	createClarificationBundleMessage(t, repo, taskID+"-m1", taskID, sessionID, turnID, pendingID, "q1", base)
	createClarificationBundleMessage(t, repo, taskID+"-m2", taskID, sessionID, turnID, pendingID, "q2", base.Add(time.Second))
}

// TestClarificationScannerRegression exercises every legacy 12-column
// scanMessageRows / inline-scanner caller in the clarification delivery and
// response paths after the enriched 13-column scanner was introduced: they
// must still scan and return full messages.
func TestClarificationScannerRegression(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 17, 9, 30, 0, 0, time.UTC)

	// Active-lookup and multi-row pending lookup (scanMessageRows).
	seedClarificationBundle(t, repo, "task-clar-scan", "session-clar-scan", "turn-clar-scan", "pending-scan", base)
	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-clar-scan")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active clarifications = %d, want 2", len(active))
	}
	if active[0].Type != models.MessageTypeClarificationRequest || active[0].Metadata["pending_id"] != "pending-scan" {
		t.Errorf("active row lost type/metadata: %+v", active[0])
	}

	byPending, err := repo.FindMessagesByPendingID(ctx, "pending-scan")
	if err != nil {
		t.Fatalf("FindMessagesByPendingID: %v", err)
	}
	if len(byPending) != 2 {
		t.Fatalf("pending bundle = %d rows, want 2", len(byPending))
	}
	if byPending[0].Metadata["pending_id"] != "pending-scan" {
		t.Errorf("pending bundle row lost metadata: %+v", byPending[0].Metadata)
	}

	// Single-row inline 12-column scans.
	single, err := repo.GetMessageByPendingID(ctx, "session-clar-scan", "pending-scan")
	if err != nil {
		t.Fatalf("GetMessageByPendingID: %v", err)
	}
	if single.Metadata["question_id"] != "q1" {
		t.Errorf("GetMessageByPendingID metadata = %+v, want q1", single.Metadata)
	}
	one, err := repo.FindMessageByPendingID(ctx, "pending-scan")
	if err != nil {
		t.Fatalf("FindMessageByPendingID: %v", err)
	}
	if one.ID != "task-clar-scan-m2" { // newest of the bundle
		t.Errorf("FindMessageByPendingID = %s, want newest bundle row", one.ID)
	}
	perQuestion, err := repo.FindMessageByPendingIDAndQuestion(ctx, "session-clar-scan", "pending-scan", "q1")
	if err != nil {
		t.Fatalf("FindMessageByPendingIDAndQuestion: %v", err)
	}
	if perQuestion.ID != "task-clar-scan-m1" {
		t.Errorf("FindMessageByPendingIDAndQuestion = %s, want q1 row", perQuestion.ID)
	}

	// Detach (RETURNING scanMessageRows).
	detached, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-clar-scan")
	if err != nil {
		t.Fatalf("DetachActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(detached) != 2 {
		t.Fatalf("detached rows = %d, want 2", len(detached))
	}
	if detached[0].TaskSessionID != "session-clar-scan" || detached[0].Metadata["agent_disconnected"] != true {
		t.Errorf("detached row lost fields: %+v", detached[0])
	}

	// Expire (RETURNING scanMessageRows) on a fresh bundle.
	seedClarificationBundle(t, repo, "task-clar-expire", "session-clar-expire", "turn-clar-expire", "pending-expire", base.Add(time.Hour))
	expired, err := repo.ExpireActiveClarificationBundle(ctx, "session-clar-expire", "pending-expire")
	if err != nil {
		t.Fatalf("ExpireActiveClarificationBundle: %v", err)
	}
	if len(expired) != 2 {
		t.Fatalf("expired rows = %d, want 2", len(expired))
	}
	if expired[0].Metadata["status"] != "expired" {
		t.Errorf("expired metadata status = %v, want expired", expired[0].Metadata["status"])
	}

	// Complete → Restore → Complete → Finalize: the claim/restorable/delivery
	// bundle loads all scan through the legacy 12-column scanner.
	seedClarificationBundle(t, repo, "task-clar-claim", "session-clar-claim", "turn-clar-claim", "pending-claim", base.Add(2*time.Hour))
	responses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "custom_text": "yes"},
		"q2": map[string]interface{}{"question_id": "q2", "custom_text": "also yes"},
	}
	claimed, claimedOK, err := repo.CompleteActiveClarificationBundle(ctx, "pending-claim", clarificationStatusAnswered, responses)
	if err != nil || !claimedOK {
		t.Fatalf("CompleteActiveClarificationBundle = claimed %v, %v; want true, nil", claimedOK, err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed rows = %d, want 2", len(claimed))
	}
	if marker := claimed[0].Metadata[clarificationResponseDeliveryPendingKey]; marker != true {
		t.Errorf("claimed row delivery marker = %v, want true", marker)
	}

	restored, restoredOK, err := repo.RestoreActiveClarificationBundle(ctx, "pending-claim", clarificationStatusAnswered, claimed)
	if err != nil || !restoredOK {
		t.Fatalf("RestoreActiveClarificationBundle = restored %v, %v; want true, nil", restoredOK, err)
	}
	if len(restored) != 2 {
		t.Fatalf("restored rows = %d, want 2", len(restored))
	}
	if restored[0].Metadata["pending_id"] != "pending-claim" || restored[0].Metadata["status"] != "pending" {
		t.Errorf("restored row lost fields: %+v", restored[0])
	}

	claimed2, claimedOK2, err := repo.CompleteActiveClarificationBundle(ctx, "pending-claim", clarificationStatusAnswered, responses)
	if err != nil || !claimedOK2 {
		t.Fatalf("second CompleteActiveClarificationBundle = claimed %v, %v; want true, nil", claimedOK2, err)
	}
	finalized, finalizedOK, err := repo.FinalizeClarificationResponseDelivery(ctx, "pending-claim", clarificationStatusAnswered, claimed2)
	if err != nil || !finalizedOK {
		t.Fatalf("FinalizeClarificationResponseDelivery = finalized %v, %v; want true, nil", finalizedOK, err)
	}
	if len(finalized) != 2 {
		t.Fatalf("finalized rows = %d, want 2", len(finalized))
	}
	if marker := finalized[0].Metadata[clarificationResponseDeliveryPendingKey]; marker != nil {
		t.Errorf("finalized row still carries delivery marker: %v", marker)
	}
}
