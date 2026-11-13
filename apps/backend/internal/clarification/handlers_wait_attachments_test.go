package clarification

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// TestHttpWaitForResponseStripsExecutorPaths keeps the browser wait projection
// path-free even after answer files were materialized for the live agent.
func TestHttpWaitForResponseStripsExecutorPaths(t *testing.T) {
	const pendingID = "pending-wait-attachment-path"
	msgs := map[string][]*taskmodels.Message{
		pendingID: {clarificationMessage("m1", pendingID, "q1", 0, "Which?", "opt1")},
	}
	h, store := setupAuthzHandler(t, msgs, &stubAuthorizer{})
	store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "s1",
		TaskID:    "t1",
		Questions: []Question{{
			ID:      "q1",
			Prompt:  "Which?",
			Options: []Option{{ID: "opt1", Label: "One"}},
		}},
	})
	t.Cleanup(func() { store.CancelRequest(pendingID) })
	if err := store.Respond(pendingID, &Response{
		PendingID: pendingID,
		Answers: []Answer{{
			QuestionID:      "q1",
			SelectedOptions: []string{"opt1"},
			Attachments: []AnswerAttachment{{
				AttachmentID: "att-1", Type: "image", Name: "screen.png",
				MimeType: "image/png", SizeBytes: 3, DeliveryMode: answerAttachmentDeliveryModePath,
				Path: "/executor/.kandev/attachments/session/screen.png",
			}},
		}},
	}); err != nil {
		t.Fatalf("Respond: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := runWait(t, h, pendingID, ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"attachment_id":"att-1"`) {
		t.Fatalf("attachment missing from wait response: %s", body)
	}
	if strings.Contains(body, "/executor/") {
		t.Fatalf("wait response leaked an executor path: %s", body)
	}
}
