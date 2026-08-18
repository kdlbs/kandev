package clarification

import (
	"context"
	"errors"
	"net/http"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestHttpRespond_LiveWaiterRejectsUnconfirmedDelivery(t *testing.T) {
	h, repo, _, messageCreator := setupTestHandler(t, map[string][]*taskmodels.Message{})
	pendingID, _ := h.store.CreateRequest(&Request{
		PendingID: "pending-finalize-failure",
		SessionID: "session-finalize-failure",
		TaskID:    "task-finalize-failure",
		Questions: []Question{{ID: "q1", Prompt: "Continue?"}},
	})
	repo.messages[pendingID] = []*taskmodels.Message{{
		ID: "message-finalize-failure", TaskID: "task-finalize-failure",
		TaskSessionID: "session-finalize-failure",
		Metadata: map[string]any{
			"status": "pending", "pending_id": pendingID, "question_id": "q1",
		},
	}}
	messageCreator.finalizeErr = errors.New("database unavailable")
	waitDone := startTestClarificationWaiter(t, h, pendingID)

	recorder := runRespond(t, h, pendingID, RespondBody{
		Answers: []Answer{{QuestionID: "q1", CustomText: "continue"}},
	})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("response status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	if err := <-waitDone; err == nil {
		t.Fatal("live waiter returned a response whose delivery was not durably confirmed")
	}
	if status := repo.messages[pendingID][0].Metadata["status"]; status != "pending" {
		t.Fatalf("restored status = %v, want pending", status)
	}
	if marker := repo.messages[pendingID][0].Metadata["response_delivery_pending"]; marker != nil {
		t.Fatalf("restored delivery marker = %v, want absent", marker)
	}
}

func TestFinalizeClarificationResponseDeliveryKeepsClaimSnapshotImmutable(t *testing.T) {
	pendingID := "pending-immutable-claim"
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{
		pendingID: {
			{
				ID:            "message-immutable-claim",
				TaskID:        "task-immutable-claim",
				TaskSessionID: "session-immutable-claim",
				Metadata: map[string]any{
					"status":                    "answered",
					"pending_id":                pendingID,
					"question_id":               "q1",
					"response_delivery_pending": true,
				},
			},
		},
	})
	claimedMessage := &taskmodels.Message{
		ID:            "message-immutable-claim",
		TaskID:        "task-immutable-claim",
		TaskSessionID: "session-immutable-claim",
		Metadata: map[string]any{
			"status":                    "answered",
			"pending_id":                pendingID,
			"question_id":               "q1",
			"response_delivery_pending": true,
		},
	}
	claim := &clarificationResponseClaim{
		terminalStatus: "answered",
		messages:       []*taskmodels.Message{claimedMessage},
	}

	finalized, ok := h.finalizeClarificationResponseDelivery(context.Background(), pendingID, claim)
	if !ok {
		t.Fatal("delivery was not finalized")
	}
	if claim.messages[0] != claimedMessage {
		t.Fatal("finalization replaced the immutable claim message")
	}
	if marker := claim.messages[0].Metadata["response_delivery_pending"]; marker != true {
		t.Fatalf("claim delivery marker = %v, want true", marker)
	}
	if len(finalized) != 1 || finalized[0] == claimedMessage {
		t.Fatalf("finalized messages = %#v, want a separate committed snapshot", finalized)
	}
	if marker := finalized[0].Metadata["response_delivery_pending"]; marker != nil {
		t.Fatalf("finalized delivery marker = %v, want absent", marker)
	}
}

func startTestClarificationWaiter(t *testing.T, h *Handlers, pendingID string) <-chan error {
	t.Helper()
	store, ok := h.store.(*Store)
	if !ok {
		t.Fatalf("handler store = %T, want *Store", h.store)
	}
	entered := make(chan struct{}, 1)
	store.SetOnWaitEntered(func(string) { entered <- struct{}{} })
	waitDone := make(chan error, 1)
	go func() {
		_, err := store.WaitForResponse(context.Background(), pendingID)
		waitDone <- err
	}()
	<-entered
	store.SetOnWaitEntered(nil)
	return waitDone
}
