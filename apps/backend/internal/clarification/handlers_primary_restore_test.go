package clarification

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type failingRespondStore struct {
	*Store
	err error
}

func (s *failingRespondStore) Respond(string, *Response) error {
	return s.err
}

func TestHttpRespond_PrimaryDeliveryFailureReportsRestoreOutcome(t *testing.T) {
	for _, test := range []struct {
		name          string
		refuseRestore bool
		wantBody      string
	}{
		{name: "restored", wantBody: "response can be retried"},
		{name: "restore refused", refuseRestore: true, wantBody: "recover pending clarification state"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(time.Minute)
			pendingID, _ := store.CreateRequest(&Request{
				PendingID: "pending-primary-restore",
				TaskID:    "task-primary-restore",
				SessionID: "session-primary-restore",
				Questions: []Question{{ID: "q1", Prompt: "Continue?"}},
			})
			message := &taskmodels.Message{
				ID: "message-primary-restore", TaskID: "task-primary-restore",
				TaskSessionID: "session-primary-restore",
				Metadata: map[string]any{
					"status": "pending", "pending_id": pendingID, "question_id": "q1",
					"question": map[string]any{"id": "q1", "prompt": "Continue?"},
				},
			}
			h, _, _, messageCreator := setupTestHandler(t, map[string][]*taskmodels.Message{
				pendingID: {message},
			})
			h.store = &failingRespondStore{Store: store, err: errors.New("delivery failed")}
			messageCreator.refuseRestore = test.refuseRestore

			recorder := runRespond(t, h, pendingID, RespondBody{
				Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"yes"}}},
			})
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("response status = %d, want 500", recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), test.wantBody) {
				t.Fatalf("response body = %q, want %q", recorder.Body.String(), test.wantBody)
			}
		})
	}
}
