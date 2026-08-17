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
	"github.com/kandev/kandev/internal/events/bus"
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

// clarificationMessage builds a durable clarification_request message with
// full question/option metadata, matching what CreateClarificationRequestMessages
// (backendapp/adapters.go) actually persists. Tests need real option_ids in
// the metadata now that N8 validates selected_options against them.
func clarificationMessage(id, pendingID, questionID string, index int, prompt string, optionIDs ...string) *taskmodels.Message {
	options := make([]interface{}, 0, len(optionIDs))
	for _, optID := range optionIDs {
		options = append(options, map[string]interface{}{"option_id": optID, "label": optID})
	}
	return &taskmodels.Message{
		ID:            id,
		TaskID:        "t1",
		TaskSessionID: "s1",
		Metadata: map[string]any{
			"status":         "pending",
			"pending_id":     pendingID,
			"question_id":    questionID,
			"question_index": index,
			"question": map[string]interface{}{
				"id": questionID, "prompt": prompt, "options": options,
			},
		},
	}
}

func setupTestHandler(t *testing.T, msgs map[string][]*taskmodels.Message) (*Handlers, *stubMessageStore, *stubEventBus, *stubMessageCreator) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: msgs}
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	resolver := NewResolver(store, newStubResolutionStore(), repo, messageCreator, &stubAuthorizer{}, eventBus, logger.Default())
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, resolver, logger.Default())
	return h, repo, eventBus, messageCreator
}

// TestHttpRespond_RejectedAfterTimeout_NoNewTurn verifies that when the user
// clicks X on an overlay after the agent already moved on (no live waiter),
// the handler does NOT publish a ClarificationAnswered event. The user is
// just dismissing a stale overlay; resuming the agent with "User declined
// to answer" is surprising and wastes a turn. Resume is always
// not_applicable for a no-waiter rejection (R9).
func TestHttpRespond_RejectedAfterTimeout_NoNewTurn(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-123": {clarificationMessage("m1", "pending-123", "q1", 0, "orig question")},
	}
	h, _, eventBus, messageCreator := setupTestHandler(t, msgs)

	body := RespondBody{
		Rejected:     true,
		RejectReason: "User skipped",
	}
	rec := runRespond(t, h, "pending-123", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	assertClaimedResponse(t, rec, true, taskmodels.ClarificationResolutionStatusRejected, taskmodels.ClarificationResolutionResumeNotApplicable)

	for _, ev := range eventBus.events {
		if ev.Type == events.ClarificationAnswered {
			t.Errorf("expected no %s event; got events: %v", events.ClarificationAnswered, eventBus.events)
		}
	}
	var staleEv *bus.Event
	for _, ev := range eventBus.events {
		if ev.Type == events.ClarificationStaleDismissed {
			staleEv = ev
			break
		}
	}
	if staleEv == nil {
		t.Fatalf("expected %s event for session cleanup; got events: %v",
			events.ClarificationStaleDismissed, eventBus.events)
	}
	data, ok := staleEv.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map event data, got %T", staleEv.Data)
	}
	if got, want := data["session_id"], "s1"; got != want {
		t.Fatalf("session_id: got %v, want %v", got, want)
	}
	if got, want := data["task_id"], "t1"; got != want {
		t.Fatalf("task_id: got %v, want %v", got, want)
	}
	if got, want := data["pending_id"], "pending-123"; got != want {
		t.Fatalf("pending_id: got %v, want %v", got, want)
	}

	if len(messageCreator.updates) != 1 {
		t.Fatalf("expected one message update to clear the durable pending guard, got %d: %+v",
			len(messageCreator.updates), messageCreator.updates)
	}
	if got := messageCreator.updates[0].status; got != "rejected" {
		t.Fatalf("expected rejected status update, got %q", got)
	}
}

// TestHttpRespond_AnsweredAfterTimeout_PublishesEvent confirms that an
// affirmative answer (option selected or custom text) with no live waiter
// still publishes the clarification.answered event so the orchestrator
// resumes the agent with a new turn — the user chose to continue.
func TestHttpRespond_AnsweredAfterTimeout_PublishesEvent(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-456": {clarificationMessage("m2", "pending-456", "q1", 0, "orig question", "opt1")},
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
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	assertClaimedResponse(t, rec, true, taskmodels.ClarificationResolutionStatusAnswered, taskmodels.ClarificationResolutionResumePublished)

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
	msgs := map[string][]*taskmodels.Message{
		"pending-1": {
			clarificationMessage("m1", "pending-1", "q1", 0, "First?", "opt1", "opt2"),
			clarificationMessage("m2", "pending-1", "q2", 1, "Second?", "opt1", "opt2"),
		},
	}
	h, _, _, _ := setupTestHandler(t, msgs)
	body := RespondBody{
		Answers: []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"opt1"}},
			{QuestionID: "q1", SelectedOptions: []string{"opt2"}},
		},
	}
	rec := runRespond(t, h, "pending-1", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate question_id, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_UnknownQuestionID_Rejected400 ensures that fabricated ids
// are rejected even with the right cardinality.
func TestHttpRespond_UnknownQuestionID_Rejected400(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-2": {clarificationMessage("m1", "pending-2", "q1", 0, "First?", "opt1")},
	}
	h, _, _, _ := setupTestHandler(t, msgs)
	body := RespondBody{
		Answers: []Answer{{QuestionID: "qZZZ", SelectedOptions: []string{"opt1"}}},
	}
	rec := runRespond(t, h, "pending-2", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown question_id, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_PartialAnswers_Rejected400 confirms that the handler
// refuses a respond payload that does not contain one answer per question
// in the original bundle. All-required gating is enforced here.
func TestHttpRespond_PartialAnswers_Rejected400(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-3": {
			clarificationMessage("m1", "pending-3", "q1", 0, "First?", "opt1"),
			clarificationMessage("m2", "pending-3", "q2", 1, "Second?", "opt1"),
			clarificationMessage("m3", "pending-3", "q3", 2, "Third?", "opt1"),
		},
	}
	h, _, _, _ := setupTestHandler(t, msgs)

	// Only one answer for a 3-question bundle.
	body := RespondBody{
		Answers: []Answer{{
			QuestionID:      "q1",
			SelectedOptions: []string{"opt1"},
		}},
	}
	rec := runRespond(t, h, "pending-3", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_UnknownOptionID_Rejected400 proves N8 (A4 item 5): an
// option id that does not belong to the question is now rejected, where the
// pre-ResolveBundle handler accepted anything.
func TestHttpRespond_UnknownOptionID_Rejected400(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-4": {clarificationMessage("m1", "pending-4", "q1", 0, "First?", "opt1", "opt2")},
	}
	h, _, _, _ := setupTestHandler(t, msgs)
	body := RespondBody{
		Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"not-an-option"}}},
	}
	rec := runRespond(t, h, "pending-4", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown option_id, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_AllAnswers_PrimaryPath_Success verifies that when every
// question is answered and a live waiter exists, the primary path delivers
// the full response, updates each message exactly once, and the response
// envelope reports claimed:true, status:answered, resume:published (R7, R10).
func TestHttpRespond_AllAnswers_PrimaryPath_Success(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-5": {
			clarificationMessage("m1", "pending-5", "q1", 0, "First?", "opt1"),
			clarificationMessage("m2", "pending-5", "q2", 1, "Second?"),
		},
	}
	h, _, _, msgCreator := setupTestHandler(t, msgs)

	pendingID := "pending-5"
	h.store.pending[pendingID] = &PendingClarification{
		Request:  &Request{PendingID: pendingID, SessionID: "s1"},
		done:     make(chan struct{}),
		CancelCh: make(chan struct{}),
	}

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
	assertClaimedResponse(t, rec, true, taskmodels.ClarificationResolutionStatusAnswered, taskmodels.ClarificationResolutionResumePublished)

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

// TestHttpRespond_DuplicateSubmit_SecondCallReportsClaimedFalse proves R2/R11:
// a second POST /respond for an already-resolved bundle is a 200 with
// claimed:false and the winner's own status/response, not the 409 the old
// handler returned.
func TestHttpRespond_DuplicateSubmit_SecondCallReportsClaimedFalse(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-6": {clarificationMessage("m1", "pending-6", "q1", 0, "First?", "opt1")},
	}
	h, _, _, _ := setupTestHandler(t, msgs)

	body := RespondBody{Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"opt1"}}}}
	first := runRespond(t, h, "pending-6", body)
	if first.Code != http.StatusOK {
		t.Fatalf("expected first submit to succeed with 200, got %d", first.Code)
	}

	second := runRespond(t, h, "pending-6", RespondBody{Rejected: true})
	if second.Code != http.StatusOK {
		t.Fatalf("expected duplicate submit to be 200 (not 409), got %d (body=%s)", second.Code, second.Body.String())
	}
	assertClaimedResponse(t, second, false, taskmodels.ClarificationResolutionStatusAnswered, taskmodels.ClarificationResolutionResumePublished)
}

// TestHttpRespond_UnknownPendingID_404 proves A5: a pending_id with no
// durable messages is not found, not a silent 200.
func TestHttpRespond_UnknownPendingID_404(t *testing.T) {
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{})
	rec := runRespond(t, h, "does-not-exist", RespondBody{Rejected: true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpCancelRequest_NoInMemoryEntry_StillSucceeds proves X2: cancelling
// a bundle whose in-memory entry is already gone (agent's turn completed)
// still succeeds via the durable claim, rather than 404ing as it did before
// ResolveBundle.
func TestHttpCancelRequest_NoInMemoryEntry_StillSucceeds(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-7": {clarificationMessage("m1", "pending-7", "q1", 0, "First?", "opt1")},
	}
	h, _, _, _ := setupTestHandler(t, msgs)

	rec := runCancel(t, h, "pending-7")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	assertClaimedResponse(t, rec, true, taskmodels.ClarificationResolutionStatusCancelled, taskmodels.ClarificationResolutionResumeNotApplicable)
}

// TestHttpCancelRequest_UnknownPendingID_404 proves cancel shares A5 with
// the other three endpoints.
func TestHttpCancelRequest_UnknownPendingID_404(t *testing.T) {
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{})
	rec := runCancel(t, h, "does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", rec.Code, rec.Body.String())
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

// assertClaimedResponse decodes a respond/cancel 200 envelope and checks the
// R10 fields ResolveBundle wiring is responsible for.
func assertClaimedResponse(t *testing.T, rec *httptest.ResponseRecorder, wantClaimed bool, wantStatus, wantResume string) {
	t.Helper()
	var body struct {
		Success bool   `json:"success"`
		Claimed bool   `json:"claimed"`
		Status  string `json:"status"`
		Resume  string `json:"resume"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, rec.Body.String())
	}
	if !body.Success {
		t.Fatalf("expected success:true, got %+v", body)
	}
	if body.Claimed != wantClaimed {
		t.Errorf("claimed: got %v, want %v", body.Claimed, wantClaimed)
	}
	if body.Status != wantStatus {
		t.Errorf("status: got %q, want %q", body.Status, wantStatus)
	}
	if body.Resume != wantResume {
		t.Errorf("resume: got %q, want %q", body.Resume, wantResume)
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

func runCancel(t *testing.T, h *Handlers, pendingID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clarification/"+pendingID+"/cancel", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpCancelRequest(c)
	return rec
}
