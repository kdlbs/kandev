package clarification

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type stubMessageCreator struct {
	updates []struct {
		pendingID string
		status    string
	}
}

func (s *stubMessageCreator) CreateClarificationRequestMessage(
	_ context.Context, _, _, _ string, _ Question, _ string,
) (string, error) {
	return "msg-id", nil
}

func (s *stubMessageCreator) UpdateClarificationMessage(
	_ context.Context, _, pendingID, status string, _ *Answer,
) error {
	s.updates = append(s.updates, struct {
		pendingID string
		status    string
	}{pendingID, status})
	return nil
}

func setupTestHandler(t *testing.T, msgs map[string]*taskmodels.Message) (*Handlers, *stubMessageStore, *stubEventBus, *stubMessageCreator) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: msgs}
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, logger.Default())
	return h, repo, eventBus, messageCreator
}

// TestHttpRespond_RejectedAfterTimeout_NoNewTurn verifies that when the user
// clicks X on an overlay after the agent already moved on (fallback path),
// the handler does NOT publish a ClarificationAnswered event. The user is
// just dismissing a stale overlay; resuming the agent with "User declined
// to answer" is surprising and wastes a turn.
func TestHttpRespond_RejectedAfterTimeout_NoNewTurn(t *testing.T) {
	// Message exists in DB (agent already moved on; canceller ran).
	msgs := map[string]*taskmodels.Message{
		"pending-123": {
			ID:            "m1",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":             "expired",
				"agent_disconnected": true,
				"question":           map[string]interface{}{"prompt": "orig question"},
			},
		},
	}
	h, _, eventBus, messageCreator := setupTestHandler(t, msgs)

	body := RespondBody{
		Rejected:     true,
		RejectReason: "User skipped",
	}
	rec := runRespond(t, h, "pending-123", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	for _, ev := range eventBus.events {
		if ev.Type == events.ClarificationAnswered {
			t.Errorf("expected no %s event; got events: %v", events.ClarificationAnswered, eventBus.events)
		}
	}

	// The message is already "expired" (set by the canceller). The no-op path
	// must NOT re-update the status — otherwise a stale X click would overwrite
	// "expired" with "rejected" and the history entry would look wrong.
	if len(messageCreator.updates) != 0 {
		t.Errorf("expected no message updates in rejected no-op path, got %d: %+v",
			len(messageCreator.updates), messageCreator.updates)
	}
}

// TestHttpRespond_AnsweredAfterTimeout_PublishesEvent confirms that an
// affirmative answer (option selected or custom text) still goes through
// the fallback path and publishes the event so the orchestrator resumes
// the agent — the user chose to continue, so a new turn is expected.
func TestHttpRespond_AnsweredAfterTimeout_PublishesEvent(t *testing.T) {
	msgs := map[string]*taskmodels.Message{
		"pending-456": {
			ID:            "m2",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":             "expired",
				"agent_disconnected": true,
				"question":           map[string]interface{}{"prompt": "orig question"},
			},
		},
	}
	h, _, eventBus, _ := setupTestHandler(t, msgs)

	body := RespondBody{
		Answers: []Answer{{
			QuestionID:      "q1",
			SelectedOptions: []string{"opt1"},
		}},
	}
	rec := runRespond(t, h, "pending-456", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var found bool
	for _, ev := range eventBus.events {
		if ev.Type == events.ClarificationAnswered {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s event to be published", events.ClarificationAnswered)
	}
}

func runRespond(t *testing.T, h *Handlers, pendingID string, body RespondBody) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clarification/"+pendingID+"/respond", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpRespond(c)
	return rec
}
