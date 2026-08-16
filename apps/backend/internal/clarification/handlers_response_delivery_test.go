package clarification

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestHttpRespond_PrimaryDeliveryFinalizationFailureIsNonRetryable(t *testing.T) {
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

	recorder := runRespond(t, h, pendingID, RespondBody{
		Answers: []Answer{{QuestionID: "q1", CustomText: "continue"}},
	})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("response status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "can be retried") {
		t.Fatalf("delivered response advertised retry: %s", recorder.Body.String())
	}
	if marker := repo.messages[pendingID][0].Metadata["response_delivery_pending"]; marker != true {
		t.Fatalf("delivery marker = %v, want recoverable true", marker)
	}
}
