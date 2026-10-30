package clarification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification/protocol"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeInboxAsyncReader struct {
	questions     []*taskmodels.AsyncQuestion
	waiting       map[string]bool
	countOverride *int
}

func (f *fakeInboxAsyncReader) ListInboxAsyncQuestions(context.Context, string, string, int, string, string) (*taskmodels.AsyncQuestionPage, error) {
	return &taskmodels.AsyncQuestionPage{Questions: f.questions}, nil
}

func (f *fakeInboxAsyncReader) HasParkedWait(_ context.Context, sessionID string) bool {
	return f.waiting[sessionID]
}

func (f *fakeInboxAsyncReader) CountInboxAsyncQuestions(context.Context, string, string) (int, error) {
	if f.countOverride != nil {
		return *f.countOverride, nil
	}
	return len(f.questions), nil
}

func (f *fakeInboxAsyncReader) ListHiddenInboxAsyncQuestions(_ context.Context, _ string, _ string, _ int, _ time.Time) ([]*taskmodels.AsyncQuestion, int, *time.Time, error) {
	return f.questions, len(f.questions), nil, nil
}

func (f *fakeInboxAsyncReader) AsyncQuestionWorkspace(_ context.Context, questionID string) (string, bool, error) {
	for _, question := range f.questions {
		if question.ID == questionID {
			return question.WorkspaceID, true, nil
		}
	}
	return "", false, nil
}

func asyncInboxQuestion() *taskmodels.AsyncQuestion {
	return &taskmodels.AsyncQuestion{
		ID:          "async-q1",
		TaskID:      "t1",
		WorkspaceID: "w1",
		SessionID:   "s1",
		Recipient:   taskmodels.AsyncQuestionRecipientUser,
		Status:      taskmodels.AsyncQuestionStatusPending,
		Context:     "API integration",
		CreatedAt:   testInboxNow,
		UpdatedAt:   testInboxNow,
		Questions: []protocol.Question{{
			ID: "q1", Title: "Database choice", Prompt: "Which database?",
			Options: []protocol.Option{{ID: "a", Label: "Postgres"}, {ID: "b", Label: "SQLite"}},
		}},
	}
}

func newAsyncInboxTestHandler(t *testing.T, question *taskmodels.AsyncQuestion, waiting bool) *Handlers {
	t.Helper()
	tasks := &fakeInboxTasks{
		tasks: map[string]*taskmodels.Task{
			"t1": {ID: "t1", Title: "Checkout integration"},
		},
		sessions: map[string]*taskmodels.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateRunning},
		},
	}
	bundles := &fakeInboxBundleStore{}
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, tasks, bundles)
	h.SetAsyncQuestionReader(&fakeInboxAsyncReader{
		questions: []*taskmodels.AsyncQuestion{question},
		waiting:   map[string]bool{"s1": waiting},
	})
	return h
}

func TestAsyncQuestionsInboxMixedPage(t *testing.T) {
	h := newAsyncInboxTestHandler(t, asyncInboxQuestion(), false)
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != 200 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response inboxListResponse
	decodeInboxBody(t, rec, &response)
	if response.Count != 1 || response.AsyncCount != 1 {
		t.Fatalf("count = %d async = %d, want 1/1", response.Count, response.AsyncCount)
	}
	if len(response.AsyncQuestions) != 1 {
		t.Fatalf("async questions = %d", len(response.AsyncQuestions))
	}
	view := response.AsyncQuestions[0]
	if view.QuestionID != "async-q1" || view.TaskTitle != "Checkout integration" {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.Waiting {
		t.Fatal("a working author must not be labelled waiting")
	}
}

func TestAsyncQuestionsInboxBootHiddenCount(t *testing.T) {
	h := newAsyncInboxTestHandler(t, asyncInboxQuestion(), true)
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	var response inboxListResponse
	decodeInboxBody(t, rec, &response)
	if !response.AsyncQuestions[0].Waiting {
		t.Fatal("a parked explicit wait must label the row as waiting")
	}

	// Hiding the async request removes it from the actionable list and counts
	// it as hidden under its namespaced sidecar identity.
	h.inboxBundles.(*fakeInboxBundleStore).states = map[string]taskmodels.ClarificationInboxHiddenBundle{
		"async:async-q1": {
			PendingID: "async:async-q1",
			State:     taskmodels.ClarificationSidecarDismissed,
		},
	}
	h.inboxBundles.(*fakeInboxBundleStore).summary = taskmodels.ClarificationInboxHiddenSummary{HiddenCount: 1}
	zero := 0
	h.inboxAsync.(*fakeInboxAsyncReader).countOverride = &zero
	rec = runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	decodeInboxBody(t, rec, &response)
	if response.Count != 0 || len(response.AsyncQuestions) != 0 {
		t.Fatalf("dismissed async request must leave the actionable list: %+v", response)
	}
	if response.HiddenCount != 2 {
		t.Fatalf("hidden count = %d, want the combined 1 blocking + 1 async", response.HiddenCount)
	}

	rec = runInboxGet(h, "/api/v1/clarification-inbox/hidden?workspace_id=w1", h.httpListInboxHidden)
	var hidden inboxHiddenListResponse
	decodeInboxBody(t, rec, &hidden)
	if len(hidden.AsyncQuestions) != 1 || hidden.AsyncQuestions[0].State != "dismissed" {
		t.Fatalf("hidden async enumeration = %+v", hidden.AsyncQuestions)
	}
	if hidden.Total != 2 {
		t.Fatalf("combined hidden total = %d, want 2", hidden.Total)
	}
}

func TestAsyncQuestionsInboxHiddenPageKeepsOneBound(t *testing.T) {
	createdAt := testInboxNow.Add(-time.Hour)
	tasks := &fakeInboxTasks{
		tasks:    map[string]*taskmodels.Task{"t1": {ID: "t1", Title: "Checkout integration"}},
		sessions: map[string]*taskmodels.TaskSession{"s1": {ID: "s1", TaskID: "t1", State: taskmodels.TaskSessionStateRunning}},
	}
	bundles := &fakeInboxBundleStore{
		page: &taskmodels.ClarificationBundlePage{
			Bundles: []taskmodels.ClarificationBundleSummary{
				{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: createdAt},
			},
		},
		summary: taskmodels.ClarificationInboxHiddenSummary{HiddenCount: 1},
	}
	messages := map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskSessionID: "s1", TaskID: "t1", CreatedAt: createdAt, Metadata: map[string]any{"pending_id": "p1"}}},
	}
	h := newInboxTestHandler(t, messages, alwaysAllowAuthorizer{}, tasks, bundles)
	h.SetAsyncQuestionReader(&fakeInboxAsyncReader{questions: []*taskmodels.AsyncQuestion{asyncInboxQuestion()}})

	rec := runInboxGet(h, "/api/v1/clarification-inbox/hidden?workspace_id=w1&limit=1", h.httpListInboxHidden)
	var response inboxHiddenListResponse
	decodeInboxBody(t, rec, &response)
	if len(response.Bundles)+len(response.AsyncQuestions) != 1 {
		t.Fatalf("merged hidden page = %d blocking + %d async, want one bounded row",
			len(response.Bundles), len(response.AsyncQuestions))
	}
	if response.Count != 1 {
		t.Fatalf("bounded page count = %d, want 1", response.Count)
	}
	if response.Total != 2 {
		t.Fatalf("combined hidden total = %d, want 2", response.Total)
	}
}

func TestAsyncQuestionsInboxAsyncReadFailureIsNotAnEmptySuccess(t *testing.T) {
	h := newAsyncInboxTestHandler(t, asyncInboxQuestion(), false)
	h.SetAsyncQuestionReader(failingAsyncReader{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code == 200 {
		t.Fatalf("a failed async read must not return an empty success: %s", rec.Body.String())
	}
}

type failingAsyncReader struct{}

func (failingAsyncReader) ListInboxAsyncQuestions(context.Context, string, string, int, string, string) (*taskmodels.AsyncQuestionPage, error) {
	return nil, context.DeadlineExceeded
}

func (failingAsyncReader) HasParkedWait(context.Context, string) bool { return false }

func (failingAsyncReader) CountInboxAsyncQuestions(context.Context, string, string) (int, error) {
	return 0, context.DeadlineExceeded
}

func (failingAsyncReader) ListHiddenInboxAsyncQuestions(context.Context, string, string, int, time.Time) ([]*taskmodels.AsyncQuestion, int, *time.Time, error) {
	return nil, 0, nil, context.DeadlineExceeded
}

func (failingAsyncReader) AsyncQuestionWorkspace(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func TestAsyncQuestionsInboxSidecarRoutesAuthorizeAsyncIdentity(t *testing.T) {
	h := newAsyncInboxTestHandler(t, asyncInboxQuestion(), false)
	rec := runInboxSidecarWrite("PUT", "async:async-q1", `{"state":"dismissed"}`, h.httpUpsertInboxSidecar)
	if rec.Code != 204 {
		t.Fatalf("async dismiss status = %d body = %s", rec.Code, rec.Body.String())
	}
	bundles := h.inboxBundles.(*fakeInboxBundleStore)
	if len(bundles.upserted) != 1 || bundles.upserted[0].pendingID != "async:async-q1" {
		t.Fatalf("upserted = %+v", bundles.upserted)
	}
	rec = runInboxSidecarWrite("DELETE", "async:async-q1", "", h.httpDeleteInboxSidecar)
	if rec.Code != 204 {
		t.Fatalf("async restore status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(bundles.deleted) != 1 || bundles.deleted[0] != "async:async-q1" {
		t.Fatalf("deleted = %+v", bundles.deleted)
	}

	rec = runInboxSidecarWrite("PUT", "async:missing", `{"state":"dismissed"}`, h.httpUpsertInboxSidecar)
	if rec.Code != 404 {
		t.Fatalf("unknown async identity status = %d, want 404", rec.Code)
	}
}

func TestAsyncQuestionsInboxSidecarRejectsForeignWorkspace(t *testing.T) {
	h := newAsyncInboxTestHandler(t, asyncInboxQuestion(), false)
	h.inboxTasks.(*fakeInboxTasks).authzErr = errors.New("forbidden")
	rec := runInboxSidecarWrite("PUT", "async:async-q1", `{"state":"dismissed"}`, h.httpUpsertInboxSidecar)
	if rec.Code != 404 {
		t.Fatalf("foreign workspace status = %d, want non-disclosing 404", rec.Code)
	}
	if len(h.inboxBundles.(*fakeInboxBundleStore).upserted) != 0 {
		t.Fatal("a foreign workspace must not write a sidecar")
	}
}

func TestAsyncQuestionsInboxMergedPageBound(t *testing.T) {
	h := newAsyncInboxTestHandler(t, asyncInboxQuestion(), false)
	bundles := h.inboxBundles.(*fakeInboxBundleStore)
	bundles.page = &taskmodels.ClarificationBundlePage{
		Bundles: []taskmodels.ClarificationBundleSummary{
			{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: testInboxNow.Add(-2 * time.Minute)},
			{PendingID: "p2", TaskID: "t1", SessionID: "s1", CreatedAt: testInboxNow.Add(-time.Minute)},
		},
		HasMore: true,
	}
	second := asyncInboxQuestion()
	second.ID = "async-q2"
	second.CreatedAt = testInboxNow
	h.inboxAsync.(*fakeInboxAsyncReader).questions = []*taskmodels.AsyncQuestion{asyncInboxQuestion(), second}
	// The fake bundle hydration has no messages; build views directly by
	// inserting minimal messages through the existing fake message store.
	bundles.messages = map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskID: "t1", TaskSessionID: "s1", Content: "one"}},
		"p2": {{ID: "m2", TaskID: "t1", TaskSessionID: "s1", Content: "two"}},
	}
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1&limit=2", h.httpListInbox)
	var response inboxListResponse
	decodeInboxBody(t, rec, &response)
	if len(response.Bundles)+len(response.AsyncQuestions) != 2 {
		t.Fatalf("merged page = %d bundles + %d async, want 2 total", len(response.Bundles), len(response.AsyncQuestions))
	}
	if response.NextCursor == "" {
		t.Fatal("a truncated merged page must return a cursor")
	}
}
