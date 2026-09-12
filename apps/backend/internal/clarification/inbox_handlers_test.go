package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

var testInboxNow = time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)

// fakeInboxTasks is an inboxTaskService double: configurable workspace
// authorization plus task/session lookups for the task_title/session_state
// enrichment.
type fakeInboxTasks struct {
	authzErr error
	tasks    map[string]*taskmodels.Task
	sessions map[string]*taskmodels.TaskSession
}

func (f *fakeInboxTasks) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error {
	return f.authzErr
}

func (f *fakeInboxTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if t, ok := f.tasks[id]; ok {
		return t, nil
	}
	return nil, errors.New("task not found")
}

func (f *fakeInboxTasks) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	if s, ok := f.sessions[id]; ok {
		return s, nil
	}
	return nil, errors.New("session not found")
}

type sidecarUpsertCall struct {
	userID, pendingID string
	state             taskmodels.ClarificationSidecarState
	snoozeUntil       *time.Time
}

// fakeInboxBundleStore is an inboxBundleStore double. It embeds
// stubMessageStore (canceller_test.go) for FindMessagesByPendingID so the
// same message fixtures back both the resolver's authorization lookup and
// the inbox handlers' own message hydration.
type fakeInboxBundleStore struct {
	stubMessageStore

	page    *taskmodels.ClarificationBundlePage
	listErr error

	lastOpts taskmodels.ListClarificationBundlesOptions

	upsertErr error
	deleteErr error
	upserted  []sidecarUpsertCall
	deleted   []string

	summary    taskmodels.ClarificationInboxHiddenSummary
	summaryErr error

	states    map[string]taskmodels.ClarificationInboxHiddenBundle
	statesErr error
}

func (f *fakeInboxBundleStore) ListUnresolvedClarificationBundles(
	_ context.Context, opts taskmodels.ListClarificationBundlesOptions,
) (*taskmodels.ClarificationBundlePage, error) {
	f.lastOpts = opts
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.page == nil {
		return &taskmodels.ClarificationBundlePage{}, nil
	}
	return f.page, nil
}

func (f *fakeInboxBundleStore) UpsertClarificationInboxSidecar(
	_ context.Context, userID, pendingID string, state taskmodels.ClarificationSidecarState, snoozeUntil *time.Time, _ time.Time,
) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = append(f.upserted, sidecarUpsertCall{userID, pendingID, state, snoozeUntil})
	return nil
}

func (f *fakeInboxBundleStore) DeleteClarificationInboxSidecar(_ context.Context, _, pendingID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, pendingID)
	return nil
}

func (f *fakeInboxBundleStore) CountHiddenClarificationBundles(
	context.Context, taskmodels.ListClarificationBundlesOptions,
) (taskmodels.ClarificationInboxHiddenSummary, error) {
	if f.summaryErr != nil {
		return taskmodels.ClarificationInboxHiddenSummary{}, f.summaryErr
	}
	return f.summary, nil
}

func (f *fakeInboxBundleStore) GetClarificationInboxSidecarStates(
	context.Context, string, []string,
) (map[string]taskmodels.ClarificationInboxHiddenBundle, error) {
	if f.statesErr != nil {
		return nil, f.statesErr
	}
	if f.states == nil {
		return map[string]taskmodels.ClarificationInboxHiddenBundle{}, nil
	}
	return f.states, nil
}

// alwaysAllowAuthorizer is a taskAccessAuthorizer double that authorizes
// every task_id, for tests that only care about the inbox layer's own
// validation/response behavior.
type alwaysAllowAuthorizer struct{}

func (alwaysAllowAuthorizer) AuthorizeTaskAccess(context.Context, string) error { return nil }

func newInboxTestHandler(
	t *testing.T,
	msgs map[string][]*taskmodels.Message,
	authorizer taskAccessAuthorizer,
	tasks *fakeInboxTasks,
	bundles *fakeInboxBundleStore,
) *Handlers {
	t.Helper()
	gin.SetMode(gin.TestMode)
	bundles.stubMessageStore = stubMessageStore{messages: msgs}
	store := NewStore(time.Minute)
	resolverRepo := &stubMessageStore{messages: msgs}
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	resolver := NewResolver(store, resolverRepo, messageCreator, authorizer, eventBus, eventBus, nil, logger.Default())
	h := NewHandlers(store, nil, messageCreator, resolverRepo, eventBus, resolver, logger.Default(), tasks, bundles)
	h.now = func() time.Time { return testInboxNow }
	return h
}

func runInboxGet(h *Handlers, target string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	handler(c)
	return rec
}

func runInboxSidecarWrite(method, pendingID, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, "/api/v1/clarification-inbox/sidecar/"+pendingID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, "/api/v1/clarification-inbox/sidecar/"+pendingID, nil)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "pendingID", Value: pendingID}}
	handler(c)
	// c.Status()/c.JSON() only set the writer's buffered status; gin's engine
	// normally flushes it via WriteHeaderNow() at the end of ServeHTTP. These
	// tests call the handler directly, bypassing that, so a bare c.Status(204)
	// (no body) would otherwise leave the recorder at its default 200.
	c.Writer.WriteHeaderNow()
	return rec
}

func decodeInboxBody(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
}

func TestHttpListInbox_MissingWorkspaceID_BadRequest(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox", h.httpListInbox)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_NonNumericLimit_BadRequest(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1&limit=abc", h.httpListInbox)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_ZeroLimit_RejectedNotClamped(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1&limit=0", h.httpListInbox)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_LimitAboveCap_ClampedTo200(t *testing.T) {
	bundles := &fakeInboxBundleStore{}
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1&limit=99999", h.httpListInbox)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if bundles.lastOpts.Limit != 200 {
		t.Fatalf("limit passed to query = %d, want clamped 200", bundles.lastOpts.Limit)
	}
}

func TestHttpListInbox_UndecodableCursor_BadRequest(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1&cursor=not-valid-base64!!", h.httpListInbox)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_WorkspaceNotFound_404(t *testing.T) {
	tasks := &fakeInboxTasks{authzErr: repoerrors.ErrWorkspaceNotFound}
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, tasks, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_Forbidden_403(t *testing.T) {
	tasks := &fakeInboxTasks{authzErr: service.ErrForbidden}
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, tasks, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_HiddenCountQueryFails_500NotDefaulted(t *testing.T) {
	// A failing second bounded query fails the whole read rather than
	// defaulting hidden_count/next_snooze_expiry.
	bundleCreatedAt := testInboxNow.Add(-time.Hour)
	msgs := map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskSessionID: "s1", TaskID: "t1", Metadata: map[string]any{"pending_id": "p1"}}},
	}
	bundles := &fakeInboxBundleStore{
		page: &taskmodels.ClarificationBundlePage{
			Bundles: []taskmodels.ClarificationBundleSummary{
				{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: bundleCreatedAt},
			},
		},
		summaryErr: errors.New("boom"),
	}
	h := newInboxTestHandler(t, msgs, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_MessagesFetchFails_500(t *testing.T) {
	bundleCreatedAt := testInboxNow.Add(-time.Hour)
	bundles := &fakeInboxBundleStore{
		page: &taskmodels.ClarificationBundlePage{
			Bundles: []taskmodels.ClarificationBundleSummary{
				{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: bundleCreatedAt},
			},
		},
	}
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)
	bundles.findErr = errors.New("db down")
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInbox_SuccessShape_RewritesQuestionIndexAndTrimsContext(t *testing.T) {
	bundleCreatedAt := testInboxNow.Add(-time.Hour)
	// Messages are seeded out of order (index 5 before index 3) and with a
	// whitespace-only context on the first message, to exercise the
	// server-side question_index rank rewrite and the context trim rule
	// together.
	msgs := map[string][]*taskmodels.Message{
		"p1": {
			{
				ID: "m-b", TaskSessionID: "s1", TaskID: "t1", CreatedAt: bundleCreatedAt,
				Metadata: map[string]any{"pending_id": "p1", "question_id": "q2", "question_index": 5, "context": "   "},
			},
			{
				ID: "m-a", TaskSessionID: "s1", TaskID: "t1", CreatedAt: bundleCreatedAt,
				Metadata: map[string]any{"pending_id": "p1", "question_id": "q1", "question_index": 3},
			},
		},
	}
	bundles := &fakeInboxBundleStore{
		page: &taskmodels.ClarificationBundlePage{
			HasMore: true,
			Bundles: []taskmodels.ClarificationBundleSummary{
				{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: bundleCreatedAt},
			},
		},
		summary: taskmodels.ClarificationInboxHiddenSummary{HiddenCount: 2},
	}
	tasks := &fakeInboxTasks{
		tasks:    map[string]*taskmodels.Task{"t1": {ID: "t1", Title: "Fix the thing"}},
		sessions: map[string]*taskmodels.TaskSession{"s1": {ID: "s1", State: "WAITING_FOR_INPUT"}},
	}
	h := newInboxTestHandler(t, msgs, alwaysAllowAuthorizer{}, tasks, bundles)

	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp inboxListResponse
	decodeInboxBody(t, rec, &resp)

	if resp.Count != 1 || len(resp.Bundles) != 1 {
		t.Fatalf("expected 1 bundle, got count=%d bundles=%d", resp.Count, len(resp.Bundles))
	}
	b := resp.Bundles[0]
	if b.TaskTitle != "Fix the thing" || b.SessionState != "WAITING_FOR_INPUT" {
		t.Fatalf("enrichment = %+v, want title/state populated", b)
	}
	if b.Context != "" {
		t.Fatalf("context = %q, want \"\" for a whitespace-only first-message context", b.Context)
	}
	if resp.HiddenCount != 2 {
		t.Fatalf("hidden_count = %d, want 2", resp.HiddenCount)
	}
	if resp.NextSnoozeExpiry != nil {
		t.Fatalf("next_snooze_expiry = %v, want nil", resp.NextSnoozeExpiry)
	}
	if resp.NextCursor == "" {
		t.Fatalf("expected next_cursor to be set when HasMore is true")
	}
	if len(b.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(b.Messages))
	}
	// m-a (question_index 3) sorts before m-b (question_index 5); the server
	// rewrites the emitted question_index to the 0-based rank within that
	// order.
	if b.Messages[0].ID != "m-a" || b.Messages[1].ID != "m-b" {
		t.Fatalf("message order = [%s, %s], want [m-a, m-b]", b.Messages[0].ID, b.Messages[1].ID)
	}
	if got := b.Messages[0].Metadata["question_index"]; got != float64(0) {
		t.Fatalf("first message's rewritten question_index = %v, want 0", got)
	}
	if got := b.Messages[1].Metadata["question_index"]; got != float64(1) {
		t.Fatalf("second message's rewritten question_index = %v, want 1", got)
	}
	// The original message's metadata must not have been mutated in place.
	if msgs["p1"][0].Metadata["question_index"] != 5 || msgs["p1"][1].Metadata["question_index"] != 3 {
		t.Fatalf("source message metadata was mutated: %+v", msgs["p1"])
	}
}

func TestHttpListInbox_EmptyResult_NeverNullBundles(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxGet(h, "/api/v1/clarification-inbox?workspace_id=w1", h.httpListInbox)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !jsonHasEmptyArrayField(t, rec.Body.Bytes(), "bundles") {
		t.Fatalf("expected bundles to serialize as [], got %s", rec.Body.String())
	}
}

func jsonHasEmptyArrayField(t *testing.T, body []byte, field string) bool {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return string(raw[field]) == "[]"
}

func TestHttpUpsertInboxSidecar_InvalidState_BadRequest(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxSidecarWrite(http.MethodPut, "p1", `{"state":"archived"}`, h.httpUpsertInboxSidecar)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpUpsertInboxSidecar_InvalidSnoozeDuration_BadRequest(t *testing.T) {
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxSidecarWrite(http.MethodPut, "p1", `{"state":"snoozed","snooze_duration":"2h"}`, h.httpUpsertInboxSidecar)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpUpsertInboxSidecar_UnknownPendingID_404(t *testing.T) {
	h := newInboxTestHandler(t, map[string][]*taskmodels.Message{}, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxSidecarWrite(http.MethodPut, "unknown", `{"state":"dismissed"}`, h.httpUpsertInboxSidecar)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpUpsertInboxSidecar_Dismiss_204AndUpserts(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskSessionID: "s1", TaskID: "t1", Metadata: map[string]any{"pending_id": "p1"}}},
	}
	bundles := &fakeInboxBundleStore{}
	h := newInboxTestHandler(t, msgs, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)
	rec := runInboxSidecarWrite(http.MethodPut, "p1", `{"state":"dismissed"}`, h.httpUpsertInboxSidecar)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(bundles.upserted) != 1 || bundles.upserted[0].pendingID != "p1" {
		t.Fatalf("upserted = %+v", bundles.upserted)
	}
	if bundles.upserted[0].state != taskmodels.ClarificationSidecarDismissed {
		t.Fatalf("state = %v, want dismissed", bundles.upserted[0].state)
	}
	if bundles.upserted[0].snoozeUntil != nil {
		t.Fatalf("snoozeUntil = %v, want nil for a dismiss", bundles.upserted[0].snoozeUntil)
	}
}

func TestHttpUpsertInboxSidecar_SnoozeDefaultsTo4h(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskSessionID: "s1", TaskID: "t1", Metadata: map[string]any{"pending_id": "p1"}}},
	}
	bundles := &fakeInboxBundleStore{}
	h := newInboxTestHandler(t, msgs, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)
	rec := runInboxSidecarWrite(http.MethodPut, "p1", `{"state":"snoozed"}`, h.httpUpsertInboxSidecar)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	want := testInboxNow.Add(4 * time.Hour)
	got := bundles.upserted[0].snoozeUntil
	if got == nil || !got.Equal(want) {
		t.Fatalf("snoozeUntil = %v, want default 4h from %v = %v", got, testInboxNow, want)
	}
}

func TestHttpDeleteInboxSidecar_UnknownPendingID_404(t *testing.T) {
	h := newInboxTestHandler(t, map[string][]*taskmodels.Message{}, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
	rec := runInboxSidecarWrite(http.MethodDelete, "unknown", "", h.httpDeleteInboxSidecar)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpDeleteInboxSidecar_KnownBundle_204AndDeletes(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskSessionID: "s1", TaskID: "t1", Metadata: map[string]any{"pending_id": "p1"}}},
	}
	bundles := &fakeInboxBundleStore{}
	h := newInboxTestHandler(t, msgs, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)
	rec := runInboxSidecarWrite(http.MethodDelete, "p1", "", h.httpDeleteInboxSidecar)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(bundles.deleted) != 1 || bundles.deleted[0] != "p1" {
		t.Fatalf("deleted = %+v", bundles.deleted)
	}
}

func TestHttpListInboxHidden_ReportsStateSnoozeUntilAndDistinctTotal(t *testing.T) {
	bundleCreatedAt := testInboxNow.Add(-time.Hour)
	snoozeUntil := testInboxNow.Add(2 * time.Hour)
	msgs := map[string][]*taskmodels.Message{
		"p1": {{ID: "m1", TaskSessionID: "s1", TaskID: "t1", CreatedAt: bundleCreatedAt, Metadata: map[string]any{"pending_id": "p1"}}},
	}
	bundles := &fakeInboxBundleStore{
		page: &taskmodels.ClarificationBundlePage{
			Bundles: []taskmodels.ClarificationBundleSummary{
				{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: bundleCreatedAt},
			},
		},
		// total (workspace-wide) can exceed this page's count (AC .37).
		summary: taskmodels.ClarificationInboxHiddenSummary{HiddenCount: 5},
		states: map[string]taskmodels.ClarificationInboxHiddenBundle{
			"p1": {PendingID: "p1", State: taskmodels.ClarificationSidecarSnoozed, SnoozeUntil: &snoozeUntil},
		},
	}
	h := newInboxTestHandler(t, msgs, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, bundles)

	rec := runInboxGet(h, "/api/v1/clarification-inbox/hidden?workspace_id=w1", h.httpListInboxHidden)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp inboxHiddenListResponse
	decodeInboxBody(t, rec, &resp)
	if resp.Count != 1 {
		t.Fatalf("count = %d, want 1", resp.Count)
	}
	if resp.Total != 5 {
		t.Fatalf("total = %d, want 5 (workspace-wide, distinct from page count)", resp.Total)
	}
	if len(resp.Bundles) != 1 || resp.Bundles[0].State != "snoozed" || resp.Bundles[0].SnoozeUntil == nil {
		t.Fatalf("hidden bundle = %+v", resp.Bundles)
	}
}
