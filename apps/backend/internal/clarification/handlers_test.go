package clarification

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type stubMessageCreator struct {
	updates []struct {
		pendingID  string
		questionID string
		status     string
	}
	created [][]Question
}

func (s *stubMessageCreator) CreateClarificationRequestMessages(
	_ context.Context, _, _, _ string, questions []Question, _ string,
) ([]string, error) {
	s.created = append(s.created, questions)
	ids := make([]string, len(questions))
	for i := range questions {
		ids[i] = "msg-id"
	}
	return ids, nil
}

func (s *stubMessageCreator) UpdateClarificationMessage(
	_ context.Context, _, pendingID, questionID, status string, _ *Answer,
) error {
	s.updates = append(s.updates, struct {
		pendingID  string
		questionID string
		status     string
	}{pendingID, questionID, status})
	return nil
}

func setupTestHandler(t *testing.T, msgs map[string][]*taskmodels.Message) (*Handlers, *stubMessageStore, *stubEventBus, *stubMessageCreator) {
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
	msgs := map[string][]*taskmodels.Message{
		"pending-123": {{
			ID:            "m1",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":             "expired",
				"agent_disconnected": true,
				"pending_id":         "pending-123",
				"question_id":        "q1",
				"question":           map[string]interface{}{"id": "q1", "prompt": "orig question"},
			},
		}},
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
	msgs := map[string][]*taskmodels.Message{
		"pending-456": {{
			ID:            "m2",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":             "expired",
				"agent_disconnected": true,
				"pending_id":         "pending-456",
				"question_id":        "q1",
				"question":           map[string]interface{}{"id": "q1", "prompt": "orig question"},
			},
		}},
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

// TestHttpRespond_DuplicateQuestionID_Rejected400 covers the dedupe gate:
// a payload that names the same question id twice should be rejected even
// when the cardinality matches the bundle size, otherwise the agent could
// receive a phantom answer for the question that was actually skipped.
func TestHttpRespond_DuplicateQuestionID_Rejected400(t *testing.T) {
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{})
	pendingID := h.store.CreateRequest(&Request{
		SessionID: "s1",
		TaskID:    "t1",
		Questions: []Question{
			{ID: "q1", Prompt: "First?"},
			{ID: "q2", Prompt: "Second?"},
		},
	})
	body := RespondBody{
		Answers: []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"opt1"}},
			{QuestionID: "q1", SelectedOptions: []string{"opt2"}},
		},
	}
	rec := runRespond(t, h, pendingID, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate question_id, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_UnknownQuestionID_Rejected400 ensures that fabricated ids
// are rejected even with the right cardinality.
func TestHttpRespond_UnknownQuestionID_Rejected400(t *testing.T) {
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{})
	pendingID := h.store.CreateRequest(&Request{
		SessionID: "s1",
		TaskID:    "t1",
		Questions: []Question{
			{ID: "q1", Prompt: "First?"},
		},
	})
	body := RespondBody{
		Answers: []Answer{{QuestionID: "qZZZ", SelectedOptions: []string{"opt1"}}},
	}
	rec := runRespond(t, h, pendingID, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown question_id, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_PartialAnswers_Rejected400 confirms that the handler
// refuses a respond payload that does not contain one answer per question
// in the original bundle. All-required gating is enforced here.
func TestHttpRespond_PartialAnswers_Rejected400(t *testing.T) {
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{})

	pendingID := h.store.CreateRequest(&Request{
		SessionID: "s1",
		TaskID:    "t1",
		Questions: []Question{
			{ID: "q1", Prompt: "First?"},
			{ID: "q2", Prompt: "Second?"},
			{ID: "q3", Prompt: "Third?"},
		},
	})

	// Only one answer for a 3-question bundle.
	body := RespondBody{
		Answers: []Answer{{
			QuestionID:      "q1",
			SelectedOptions: []string{"opt1"},
		}},
	}
	rec := runRespond(t, h, pendingID, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_AllAnswers_PrimaryPath_Success verifies that when every
// question is answered the primary path delivers the full response and
// updates each message exactly once.
func TestHttpRespond_AllAnswers_PrimaryPath_Success(t *testing.T) {
	h, _, _, msgCreator := setupTestHandler(t, map[string][]*taskmodels.Message{})

	pendingID := h.store.CreateRequest(&Request{
		SessionID: "s1",
		TaskID:    "t1",
		Questions: []Question{
			{ID: "q1", Prompt: "First?"},
			{ID: "q2", Prompt: "Second?"},
		},
	})

	// Drain the response channel so Respond does not block indefinitely.
	go func() {
		_, _ = h.store.WaitForResponse(context.Background(), pendingID)
	}()

	body := RespondBody{
		Answers: []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"opt1"}},
			{QuestionID: "q2", CustomText: "free-form"},
		},
	}
	rec := runRespond(t, h, pendingID, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	if len(msgCreator.updates) != 2 {
		t.Fatalf("expected 2 message updates (one per question), got %d: %+v",
			len(msgCreator.updates), msgCreator.updates)
	}
	for _, u := range msgCreator.updates {
		if u.status != "answered" {
			t.Errorf("expected status=answered, got %q", u.status)
		}
	}
}

// TestValidateAndNormalizeQuestions_AssignsDefaults checks that question and
// option IDs are auto-generated when omitted, in deterministic q1/q1_opt1 form.
func TestValidateAndNormalizeQuestions_AssignsDefaults(t *testing.T) {
	qs := []Question{
		{Prompt: "First?", Options: []Option{{Label: "A", Description: "a"}, {Label: "B", Description: "b"}}},
		{Prompt: "Second?", Options: []Option{{Label: "X", Description: "x"}, {Label: "Y", Description: "y"}}},
	}
	if err := NormalizeAndValidateQuestions(qs); err != "" {
		t.Fatalf("unexpected validation error: %s", err)
	}
	if qs[0].ID != "q1" || qs[1].ID != "q2" {
		t.Errorf("expected q1/q2, got %q/%q", qs[0].ID, qs[1].ID)
	}
	if qs[0].Options[0].ID != "q1_opt1" || qs[1].Options[1].ID != "q2_opt2" {
		t.Errorf("unexpected option IDs: %+v / %+v", qs[0].Options, qs[1].Options)
	}
}

// TestValidateAndNormalizeQuestions_RejectsInvalid covers the edge cases that
// guard against malformed payloads (no questions, too many, bad option counts).
func TestValidateAndNormalizeQuestions_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		input []Question
	}{
		{"no questions", nil},
		{"too many", []Question{{}, {}, {}, {}, {}}},
		{"missing prompt", []Question{{Options: []Option{{Label: "A", Description: "a"}, {Label: "B", Description: "b"}}}}},
		{"single option", []Question{{Prompt: "?", Options: []Option{{Label: "A", Description: "a"}}}}},
		{"too many options", []Question{{Prompt: "?", Options: []Option{
			{Label: "1", Description: "1"}, {Label: "2", Description: "2"}, {Label: "3", Description: "3"},
			{Label: "4", Description: "4"}, {Label: "5", Description: "5"}, {Label: "6", Description: "6"},
			{Label: "7", Description: "7"},
		}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if msg := NormalizeAndValidateQuestions(tc.input); msg == "" {
				t.Fatalf("expected validation error, got nil for %+v", tc.input)
			}
		})
	}
}

// TestBuildAnswerSummary_SingleQuestion preserves the original "User selected:"
// text shape so existing prompts in the orchestrator stay readable.
func TestBuildAnswerSummary_SingleQuestion(t *testing.T) {
	got := buildAnswerSummary(
		[]Question{{ID: "q1", Prompt: "Which?"}},
		[]Answer{{QuestionID: "q1", SelectedOptions: []string{"opt1"}}},
		false, "",
	)
	if got != "User selected: [opt1]" {
		t.Errorf("unexpected single-q summary: %q", got)
	}
}

// TestBuildAnswerSummary_MultiQuestion produces an A1/A2 layout so the
// orchestrator resume prompt clearly maps each answer to its question.
func TestBuildAnswerSummary_MultiQuestion(t *testing.T) {
	got := buildAnswerSummary(
		[]Question{
			{ID: "q1", Prompt: "First?"},
			{ID: "q2", Prompt: "Second?"},
		},
		[]Answer{
			{QuestionID: "q1", SelectedOptions: []string{"opt1"}},
			{QuestionID: "q2", CustomText: "free"},
		},
		false, "",
	)
	if got == "" || !strings.Contains(got, "A1:") || !strings.Contains(got, "A2:") {
		t.Errorf("expected multi-line summary with A1/A2, got %q", got)
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
