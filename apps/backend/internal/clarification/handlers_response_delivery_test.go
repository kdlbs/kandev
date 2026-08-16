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
