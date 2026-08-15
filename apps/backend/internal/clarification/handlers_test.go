package clarification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	created          [][]Question
	repo             *stubMessageStore
	publishedBundles int
	restoreErr       error
	refuseRestore    bool
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

func (s *stubMessageCreator) CompleteActiveClarificationBundle(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
) ([]*taskmodels.Message, bool, error) {
	msgs := s.repo.messages[pendingID]
	if len(msgs) == 0 {
		return nil, false, nil
	}
	active, err := s.repo.FindActiveClarificationMessagesBySessionID(ctx, msgs[0].TaskSessionID)
	if err != nil {
		return nil, false, err
	}
	activeBundle := false
	for _, message := range active {
		if stringFromMetadata(message.Metadata, "pending_id") == pendingID {
			activeBundle = true
			break
		}
	}
	if !activeBundle {
		return nil, false, nil
	}
	claimedMessages := make([]*taskmodels.Message, 0, len(msgs))
	for _, message := range msgs {
		currentStatus := stringFromMetadata(message.Metadata, "status")
		if currentStatus != "" && currentStatus != "pending" {
			continue
		}
		questionID := stringFromMetadata(message.Metadata, "question_id")
		message.Metadata["status"] = status
		if response, ok := responses[questionID]; ok && response != nil {
			message.Metadata["response"] = response
		}
		s.updates = append(s.updates, struct {
			pendingID  string
			questionID string
			status     string
		}{pendingID, questionID, status})
		claimedMessages = append(claimedMessages, message)
	}
	return claimedMessages, len(claimedMessages) > 0, nil
}

func (s *stubMessageCreator) RestoreActiveClarificationBundle(
	_ context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*taskmodels.Message,
) (bool, error) {
	if s.restoreErr != nil {
		return false, s.restoreErr
	}
	if s.refuseRestore {
		return false, nil
	}
	if len(claimedMessages) == 0 {
		return false, nil
	}
	for _, message := range claimedMessages {
		if stringFromMetadata(message.Metadata, "pending_id") != pendingID {
			return false, nil
		}
		if stringFromMetadata(message.Metadata, "status") != terminalStatus {
			return false, nil
		}
	}
	for _, message := range claimedMessages {
		questionID := stringFromMetadata(message.Metadata, "question_id")
		message.Metadata["status"] = "pending"
		delete(message.Metadata, "response")
		s.updates = append(s.updates, struct {
			pendingID  string
			questionID string
			status     string
		}{pendingID, questionID, "pending"})
	}
	return true, nil
}

func (s *stubMessageCreator) PublishClarificationBundleUpdates(
	context.Context,
	[]*taskmodels.Message,
) {
	s.publishedBundles++
}

func setupTestHandler(t *testing.T, msgs map[string][]*taskmodels.Message) (*Handlers, *stubMessageStore, *stubEventBus, *stubMessageCreator) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: msgs}
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{repo: repo}
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, logger.Default())
	return h, repo, eventBus, messageCreator
}

// TestHttpRespond_RejectedAfterTimeout_NoNewTurn verifies that when the user
// clicks X on an overlay after the agent already moved on (fallback path),
// the handler does NOT publish a ClarificationAnswered event. The user is
// just dismissing a stale overlay; resuming the agent with "User declined
// to answer" is surprising and wastes a turn.
func TestHttpRespond_RejectedAfterTimeout_NoNewTurn(t *testing.T) {
	// Message exists in DB (agent already moved on; canceller detached it).
	msgs := map[string][]*taskmodels.Message{
		"pending-123": {{
			ID:            "m1",
			TaskID:        "t1",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":             "pending",
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
				"status":             "pending",
				"agent_disconnected": true,
				"pending_id":         "pending-456",
				"question_id":        "q1",
				"question":           map[string]interface{}{"id": "q1", "prompt": "orig question"},
			},
		}},
	}
	h, _, eventBus, messageCreator := setupTestHandler(t, msgs)

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
	answerEvents := 0
	for _, ev := range eventBus.events {
		if ev.Type == events.ClarificationAnswered {
			answerEvents++
		}
	}
	if answerEvents != 1 {
		t.Errorf("%s events = %d, want exactly 1", events.ClarificationAnswered, answerEvents)
	}
	if len(messageCreator.updates) != 1 || messageCreator.updates[0].status != "answered" {
		t.Errorf("detached answer updates = %+v, want one answered write", messageCreator.updates)
	}
}

func TestHttpRespond_DetachedAnswerPublishesOnlyForWinningClaim(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-race": {{
			ID: "message-race", TaskID: "task-race", TaskSessionID: "session-race",
			Metadata: map[string]any{
				"status": "pending", "pending_id": "pending-race", "question_id": "q1",
				"question": map[string]any{"id": "q1", "prompt": "Continue?"},
			},
		}},
	}
	h, _, eventBus, _ := setupTestHandler(t, msgs)
	body := RespondBody{Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"yes"}}}}

	first := runRespond(t, h, "pending-race", body)
	second := runRespond(t, h, "pending-race", body)
	if first.Code != http.StatusOK || second.Code != http.StatusConflict {
		t.Fatalf("response statuses = (%d, %d), want (200, 409)", first.Code, second.Code)
	}
	answeredEvents := 0
	for _, event := range eventBus.events {
		if event.Type == events.ClarificationAnswered {
			answeredEvents++
		}
	}
	if answeredEvents != 1 {
		t.Fatalf("ClarificationAnswered events = %d, want 1", answeredEvents)
	}
}

func TestHttpRespond_DetachedPublishFailureRestoresRetryableBundle(t *testing.T) {
	msg := &taskmodels.Message{
		ID: "message-retry", TaskID: "task-retry", TaskSessionID: "session-retry",
		Metadata: map[string]any{
			"status": "pending", "pending_id": "pending-retry", "question_id": "q1",
			"question": map[string]any{"id": "q1", "prompt": "Continue?"},
		},
	}
	h, _, eventBus, messageCreator := setupTestHandler(t, map[string][]*taskmodels.Message{
		"pending-retry": {msg},
	})
	eventBus.publishErr = errors.New("event bus unavailable")
	body := RespondBody{Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"yes"}}}}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	first := runRespondWithContext(t, h, "pending-retry", body, cancelledCtx)
	if first.Code != http.StatusInternalServerError {
		t.Fatalf("first response status = %d, want 500; body=%s", first.Code, first.Body.String())
	}
	if got := msg.Metadata["status"]; got != "pending" {
		t.Fatalf("status after failed publication = %v, want pending for retry", got)
	}
	if messageCreator.publishedBundles != 0 {
		t.Fatalf("published terminal bundles after failed resume = %d, want 0", messageCreator.publishedBundles)
	}

	eventBus.publishErr = nil
	second := runRespond(t, h, "pending-retry", body)
	if second.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200; body=%s", second.Code, second.Body.String())
	}
	if got := msg.Metadata["status"]; got != "answered" {
		t.Fatalf("status after retry = %v, want answered", got)
	}
	if messageCreator.publishedBundles != 1 {
		t.Fatalf("published terminal bundles after retry = %d, want 1", messageCreator.publishedBundles)
	}
	if len(eventBus.events) != 1 || eventBus.events[0].Type != events.ClarificationAnswered {
		t.Fatalf("successful resume events = %+v, want exactly one", eventBus.events)
	}
	for index, contextErr := range eventBus.contextErrs {
		if contextErr != nil {
			t.Fatalf("event publish %d used cancelled context: %v", index, contextErr)
		}
	}
}

func TestHttpRespond_DetachedPublishFailureDoesNotPromiseRetryWhenRestoreFails(t *testing.T) {
	for _, tt := range []struct {
		name          string
		restoreErr    error
		refuseRestore bool
	}{
		{name: "restore error", restoreErr: errors.New("database unavailable")},
		{name: "claim no longer restorable", refuseRestore: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			msg := &taskmodels.Message{
				ID: "message-recovery", TaskID: "task-recovery", TaskSessionID: "session-recovery",
				Metadata: map[string]any{
					"status": "pending", "pending_id": "pending-recovery", "question_id": "q1",
					"question": map[string]any{"id": "q1", "prompt": "Continue?"},
				},
			}
			h, _, eventBus, messageCreator := setupTestHandler(t, map[string][]*taskmodels.Message{
				"pending-recovery": {msg},
			})
			eventBus.publishErr = errors.New("event bus unavailable")
			messageCreator.restoreErr = tt.restoreErr
			messageCreator.refuseRestore = tt.refuseRestore

			rec := runRespond(t, h, "pending-recovery", RespondBody{
				Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"yes"}}},
			})
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("response status = %d, want 500; body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "can be retried") {
				t.Fatalf("failed restore advertised an unsafe retry: %s", rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "recover pending clarification state") {
				t.Fatalf("failed restore response = %s, want recovery failure", rec.Body.String())
			}
		})
	}
}

func TestHttpRespond_LiveSupersededBundleReturnsConflict(t *testing.T) {
	old := &taskmodels.Message{
		ID: "old-message", TaskID: "task-live", TaskSessionID: "session-live",
		Metadata: map[string]any{
			"status": "pending", "pending_id": "pending-live", "question_id": "q1",
			"question": map[string]any{"id": "q1", "prompt": "Old question?"},
		},
	}
	h, repo, eventBus, messageCreator := setupTestHandler(t, map[string][]*taskmodels.Message{
		"pending-live": {old},
	})
	pendingID, _ := h.store.CreateRequest(&Request{
		PendingID: "pending-live", SessionID: "session-live", TaskID: "task-live",
		Questions: []Question{{ID: "q1", Prompt: "Old question?"}},
	})
	repo.activeBySession = map[string][]*taskmodels.Message{
		"session-live": {{
			ID: "new-message", TaskID: "task-live", TaskSessionID: "session-live",
			Metadata: map[string]any{"status": "pending", "pending_id": "pending-new"},
		}},
	}

	rec := runRespond(t, h, pendingID, RespondBody{
		Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"yes"}}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	if len(messageCreator.updates) != 0 || len(eventBus.events) != 0 {
		t.Fatalf("stale live response produced writes=%+v events=%+v", messageCreator.updates, eventBus.events)
	}
}

func TestHttpRespond_FallbackInactiveBundleReturnsConflictWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		rejected   bool
		superseded bool
	}{
		{name: "superseded answer", status: "pending", superseded: true},
		{name: "superseded rejection", status: "pending", rejected: true, superseded: true},
		{name: "terminal answer", status: "answered"},
		{name: "terminal rejection", status: "rejected", rejected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := &taskmodels.Message{
				ID: "old-message", TaskID: "t1", TaskSessionID: "s1",
				Metadata: map[string]any{
					"status": tt.status, "agent_disconnected": true,
					"pending_id": "pending-old", "question_id": "q1",
					"question": map[string]any{"id": "q1", "prompt": "old question"},
				},
			}
			h, repo, eventBus, messageCreator := setupTestHandler(
				t,
				map[string][]*taskmodels.Message{"pending-old": {old}},
			)
			repo.activeBySession = map[string][]*taskmodels.Message{"s1": {}}
			if tt.superseded {
				repo.activeBySession["s1"] = []*taskmodels.Message{{
					ID: "new-message", TaskID: "t1", TaskSessionID: "s1",
					Metadata: map[string]any{"status": "pending", "pending_id": "pending-new"},
				}}
			}
			body := RespondBody{Rejected: tt.rejected, RejectReason: "skip"}
			if !tt.rejected {
				body.Answers = []Answer{{QuestionID: "q1", SelectedOptions: []string{"one"}}}
			}

			rec := runRespond(t, h, "pending-old", body)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
			}
			if len(messageCreator.updates) != 0 {
				t.Fatalf("inactive response wrote messages: %+v", messageCreator.updates)
			}
			if len(eventBus.events) != 0 {
				t.Fatalf("inactive response published events: %+v", eventBus.events)
			}
		})
	}
}

func TestHttpRespond_FallbackAuthorityErrorHasNoSideEffects(t *testing.T) {
	msg := &taskmodels.Message{
		ID: "message", TaskID: "t1", TaskSessionID: "s1",
		Metadata: map[string]any{
			"status": "pending", "pending_id": "pending", "question_id": "q1",
			"question": map[string]any{"id": "q1", "prompt": "question"},
		},
	}
	h, repo, eventBus, messageCreator := setupTestHandler(
		t,
		map[string][]*taskmodels.Message{"pending": {msg}},
	)
	repo.activeErr = errors.New("read failed")
	rec := runRespond(t, h, "pending", RespondBody{
		Answers: []Answer{{QuestionID: "q1", SelectedOptions: []string{"one"}}},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if len(messageCreator.updates) != 0 || len(eventBus.events) != 0 {
		t.Fatalf("authority error produced writes=%+v events=%+v", messageCreator.updates, eventBus.events)
	}
}

// TestHttpRespond_DuplicateQuestionID_Rejected400 covers the dedupe gate:
// a payload that names the same question id twice should be rejected even
// when the cardinality matches the bundle size, otherwise the agent could
// receive a phantom answer for the question that was actually skipped.
func TestHttpRespond_DuplicateQuestionID_Rejected400(t *testing.T) {
	h, _, _, _ := setupTestHandler(t, map[string][]*taskmodels.Message{})
	pendingID, _ := h.store.CreateRequest(&Request{
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
	pendingID, _ := h.store.CreateRequest(&Request{
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

	pendingID, _ := h.store.CreateRequest(&Request{
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
	h, repo, _, msgCreator := setupTestHandler(t, map[string][]*taskmodels.Message{})

	pendingID, _ := h.store.CreateRequest(&Request{
		SessionID: "s1",
		TaskID:    "t1",
		Questions: []Question{
			{ID: "q1", Prompt: "First?"},
			{ID: "q2", Prompt: "Second?"},
		},
	})
	repo.messages[pendingID] = []*taskmodels.Message{
		{
			ID: "message-q1", TaskID: "t1", TaskSessionID: "s1",
			Metadata: map[string]any{
				"status": "pending", "pending_id": pendingID, "question_id": "q1",
			},
		},
		{
			ID: "message-q2", TaskID: "t1", TaskSessionID: "s1",
			Metadata: map[string]any{
				"status": "pending", "pending_id": pendingID, "question_id": "q2",
			},
		},
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
	return runRespondWithContext(t, h, pendingID, body, context.Background())
}

func runRespondWithContext(
	t *testing.T,
	h *Handlers,
	pendingID string,
	body RespondBody,
	ctx context.Context,
) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/clarification/"+pendingID+"/respond",
		bytes.NewReader(payload),
	).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpRespond(c)
	return rec
}
