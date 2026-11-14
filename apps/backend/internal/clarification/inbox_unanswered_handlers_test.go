package clarification

import (
	"context"
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

// fakeUnansweredActionTasks adds the task-write and session-prompt authority
// checks the unanswered writes require on top of the shared inbox task double.
type fakeUnansweredActionTasks struct {
	*fakeInboxTasks
	actionErr   error
	scopeCalls  []string
	promptCalls []string
}

func (f *fakeUnansweredActionTasks) AuthorizeTaskScope(_ context.Context, taskID string, scope authz.Scope) error {
	f.scopeCalls = append(f.scopeCalls, taskID+"|"+string(scope))
	return f.actionErr
}

func (f *fakeUnansweredActionTasks) AuthorizeTaskSessionPromptAccess(_ context.Context, taskID, sessionID string) error {
	f.promptCalls = append(f.promptCalls, taskID+"|"+sessionID)
	return f.actionErr
}

func actionTasks() *fakeUnansweredActionTasks {
	return &fakeUnansweredActionTasks{fakeInboxTasks: &fakeInboxTasks{}}
}

// fakeUnansweredInboxStore adds the unanswered-section surface to the
// shared inbox bundle double.
type fakeUnansweredInboxStore struct {
	fakeInboxBundleStore
	workspace   *taskmodels.Workspace
	unanswered  *taskmodels.ClarificationBundlePage
	listOpts    []taskmodels.ListUnansweredClarificationsOptions
	closeCalls  []taskmodels.CloseUnansweredClarification
	closedIDs   []string
	closeResult []*taskmodels.Message
	closeErr    error
}

func (f *fakeUnansweredInboxStore) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if f.workspace == nil || f.workspace.ID != id {
		return nil, errors.New("workspace not found")
	}
	return f.workspace, nil
}

func (f *fakeUnansweredInboxStore) ListUnansweredClarificationBundles(
	_ context.Context, opts taskmodels.ListUnansweredClarificationsOptions,
) (*taskmodels.ClarificationBundlePage, error) {
	f.listOpts = append(f.listOpts, opts)
	if f.unanswered == nil {
		return &taskmodels.ClarificationBundlePage{}, nil
	}
	return f.unanswered, nil
}

func (f *fakeUnansweredInboxStore) CloseUnansweredClarificationBundle(
	_ context.Context, pendingID string, closeReq taskmodels.CloseUnansweredClarification,
) ([]*taskmodels.Message, error) {
	f.closedIDs = append(f.closedIDs, pendingID)
	f.closeCalls = append(f.closeCalls, closeReq)
	return f.closeResult, f.closeErr
}

// sessionMessageStore answers GetTaskSession from a fixture map.
type sessionMessageStore struct {
	stubMessageStore
	sessions map[string]*taskmodels.TaskSession
}

func (s *sessionMessageStore) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	if session, ok := s.sessions[id]; ok {
		return session, nil
	}
	return nil, errors.New("session not found")
}

type denyAllAuthorizer struct{}

func (denyAllAuthorizer) AuthorizeTaskAccess(context.Context, string) error {
	return errors.New("forbidden")
}

func unansweredFixtureMessages() map[string][]*taskmodels.Message {
	return map[string][]*taskmodels.Message{
		"p1": {{
			ID: "m1", TaskSessionID: "s-old", TaskID: "t1", Content: "Which database?",
			CreatedAt: testInboxNow.Add(-time.Hour),
			Metadata:  map[string]any{"pending_id": "p1", "question_id": "q1", "status": "pending"},
		}},
	}
}

func newUnansweredTestHandler(
	t *testing.T,
	authorizer taskAccessAuthorizer,
	tasks inboxTaskService,
	bundles inboxBundleStore,
	sessions map[string]*taskmodels.TaskSession,
) *Handlers {
	t.Helper()
	gin.SetMode(gin.TestMode)
	msgs := unansweredFixtureMessages()
	switch store := bundles.(type) {
	case *fakeUnansweredInboxStore:
		store.stubMessageStore = stubMessageStore{messages: msgs}
	case *fakeInboxBundleStore:
		store.stubMessageStore = stubMessageStore{messages: msgs}
	}
	store := NewStore(time.Minute)
	repo := &sessionMessageStore{stubMessageStore: stubMessageStore{messages: msgs}, sessions: sessions}
	eventBus := &stubEventBus{}
	creator := &stubMessageCreator{}
	resolver := NewResolver(store, repo, creator, authorizer, eventBus, eventBus, nil, logger.Default())
	h := NewHandlers(store, nil, creator, repo, eventBus, resolver, logger.Default(), tasks, bundles)
	h.now = func() time.Time { return testInboxNow }
	return h
}

func runUnansweredWrite(pendingID, action, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	target := "/api/v1/clarification-inbox/unanswered/" + pendingID + "/" + action
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "pendingID", Value: pendingID}}
	handler(c)
	c.Writer.WriteHeaderNow()
	return rec
}

func TestHttpListInboxUnansweredSettingOffReturnsDisabled(t *testing.T) {
	store := &fakeUnansweredInboxStore{workspace: &taskmodels.Workspace{ID: "w1", ClarificationUnansweredRetentionDays: 14}}
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, store, nil)
	rec := runInboxGet(h, "/api/v1/clarification-inbox/unanswered?workspace_id=w1", h.httpListInboxUnanswered)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp inboxUnansweredListResponse
	decodeInboxBody(t, rec, &resp)
	if resp.Enabled || resp.Count != 0 || len(resp.Bundles) != 0 || resp.RetentionDays != 14 {
		t.Fatalf("response = %+v, want disabled and empty", resp)
	}
	if len(store.listOpts) != 0 {
		t.Fatalf("setting off listed unanswered bundles %d times", len(store.listOpts))
	}
}

func TestHttpListInboxUnansweredListsWorkspaceBundlesWithSendToTask(t *testing.T) {
	store := &fakeUnansweredInboxStore{
		workspace: &taskmodels.Workspace{ID: "w1", ClarificationUnansweredInboxEnabled: true, ClarificationUnansweredRetentionDays: 30},
		unanswered: &taskmodels.ClarificationBundlePage{Bundles: []taskmodels.ClarificationBundleSummary{{
			PendingID: "p1", TaskID: "t1", SessionID: "s-old", CreatedAt: testInboxNow.Add(-time.Hour),
		}}},
	}
	tasks := &fakeInboxTasks{
		tasks: map[string]*taskmodels.Task{"t1": {ID: "t1", Title: "Task one"}},
		sessions: map[string]*taskmodels.TaskSession{
			"s-old":     {ID: "s-old", TaskID: "t1", State: taskmodels.TaskSessionStateCompleted},
			"s-primary": {ID: "s-primary", TaskID: "t1", State: taskmodels.TaskSessionStateRunning, IsPrimary: true},
		},
	}
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, tasks, store, nil)
	rec := runInboxGet(h, "/api/v1/clarification-inbox/unanswered?workspace_id=w1", h.httpListInboxUnanswered)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp inboxUnansweredListResponse
	decodeInboxBody(t, rec, &resp)
	if !resp.Enabled || resp.Count != 1 || resp.RetentionDays != 30 {
		t.Fatalf("response = %+v", resp)
	}
	row := resp.Bundles[0]
	if row.PendingID != "p1" || row.TaskTitle != "Task one" || !row.OriginalSessionTerminal ||
		row.SendToTaskSessionID == nil || *row.SendToTaskSessionID != "s-primary" {
		t.Fatalf("row = %+v", row)
	}
	opts := store.listOpts[0]
	if len(opts.WorkspaceIDs) != 1 || opts.WorkspaceIDs[0] != "w1" || opts.AnyWorkspace ||
		opts.Sidecar == nil || opts.Sidecar.UserID == "" || !opts.Now.Equal(testInboxNow) {
		t.Fatalf("list options = %+v", opts)
	}
}

func TestHttpListInboxUnansweredLiveOriginalOffersNoSendToTask(t *testing.T) {
	store := &fakeUnansweredInboxStore{
		workspace: &taskmodels.Workspace{ID: "w1", ClarificationUnansweredInboxEnabled: true},
		unanswered: &taskmodels.ClarificationBundlePage{Bundles: []taskmodels.ClarificationBundleSummary{{
			PendingID: "p1", TaskID: "t1", SessionID: "s-old", CreatedAt: testInboxNow.Add(-time.Hour),
		}}},
	}
	tasks := &fakeInboxTasks{sessions: map[string]*taskmodels.TaskSession{
		"s-old":     {ID: "s-old", TaskID: "t1", State: taskmodels.TaskSessionStateWaitingForInput},
		"s-primary": {ID: "s-primary", TaskID: "t1", State: taskmodels.TaskSessionStateRunning, IsPrimary: true},
	}}
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, tasks, store, nil)
	rec := runInboxGet(h, "/api/v1/clarification-inbox/unanswered?workspace_id=w1", h.httpListInboxUnanswered)
	var resp inboxUnansweredListResponse
	decodeInboxBody(t, rec, &resp)
	if len(resp.Bundles) != 1 || resp.Bundles[0].OriginalSessionTerminal || resp.Bundles[0].SendToTaskSessionID != nil {
		t.Fatalf("rows = %+v, want the original session as the only target", resp.Bundles)
	}
}

func TestHttpListInboxUnansweredWorkspaceAuthorization(t *testing.T) {
	store := &fakeUnansweredInboxStore{workspace: &taskmodels.Workspace{ID: "w1", ClarificationUnansweredInboxEnabled: true}}
	tasks := &fakeInboxTasks{authzErr: repoerrors.ErrWorkspaceNotFound}
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, tasks, store, nil)
	rec := runInboxGet(h, "/api/v1/clarification-inbox/unanswered?workspace_id=w1", h.httpListInboxUnanswered)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if len(store.listOpts) != 0 {
		t.Fatal("unauthorized caller reached the unanswered read")
	}
	missing := runInboxGet(h, "/api/v1/clarification-inbox/unanswered", h.httpListInboxUnanswered)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d, want 400", missing.Code)
	}
}

func TestHttpCloseInboxUnanswered(t *testing.T) {
	store := &fakeUnansweredInboxStore{closeResult: []*taskmodels.Message{{ID: "m1"}}}
	tasks := actionTasks()
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, tasks, store, nil)
	rec := runUnansweredWrite("p1", "close", "", h.httpCloseInboxUnanswered)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(store.closeCalls) != 1 || store.closedIDs[0] != "p1" ||
		store.closeCalls[0].Reason != taskmodels.ClarificationClosedByUser || !store.closeCalls[0].At.Equal(testInboxNow) ||
		store.closeCalls[0].SourceSessionID != "s-old" {
		t.Fatalf("close calls = %+v", store.closeCalls)
	}
	if len(tasks.scopeCalls) != 1 || tasks.scopeCalls[0] != "t1|"+string(authz.ScopeTaskWrite) || len(tasks.promptCalls) != 0 {
		t.Fatalf("close authorized with scope calls %v prompt calls %v, want task write on t1", tasks.scopeCalls, tasks.promptCalls)
	}

	store.closeResult = nil
	conflict := runUnansweredWrite("p1", "close", "", h.httpCloseInboxUnanswered)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), unansweredConflictCode) {
		t.Fatalf("not-unanswered status = %d body %s, want 409 %s", conflict.Code, conflict.Body.String(), unansweredConflictCode)
	}
}

func TestHttpUnansweredWritesRequireBundleAccess(t *testing.T) {
	store := &fakeUnansweredInboxStore{closeResult: []*taskmodels.Message{{ID: "m1"}}}
	h := newUnansweredTestHandler(t, denyAllAuthorizer{}, actionTasks(), store, nil)
	for _, write := range []struct {
		action, body string
		handler      gin.HandlerFunc
	}{
		{"close", "", h.httpCloseInboxUnanswered},
		{"late-answer", `{"message_id":"late-1"}`, h.httpLateAnswerInboxUnanswered},
	} {
		rec := runUnansweredWrite("p1", write.action, write.body, write.handler)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", write.action, rec.Code)
		}
	}
	if len(store.closeCalls) != 0 {
		t.Fatalf("unauthorized writes reached the store: %+v", store.closeCalls)
	}
	unknown := runUnansweredWrite("missing", "close", "", h.httpCloseInboxUnanswered)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown pending id status = %d, want 404", unknown.Code)
	}
}

func TestHttpLateAnswerInboxUnansweredDefaultsToOriginalSession(t *testing.T) {
	store := &fakeUnansweredInboxStore{closeResult: []*taskmodels.Message{{ID: "m1"}}}
	tasks := actionTasks()
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, tasks, store, nil)
	if rec := runUnansweredWrite("p1", "late-answer", `{}`, h.httpLateAnswerInboxUnanswered); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing message_id status = %d, want 400", rec.Code)
	}
	rec := runUnansweredWrite("p1", "late-answer", `{"message_id":"late-1"}`, h.httpLateAnswerInboxUnanswered)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := store.closeCalls[0]
	if got.Reason != taskmodels.ClarificationClosedAnsweredLate || got.MessageID != "late-1" ||
		got.SessionID != "s-old" || got.SourceSessionID != "s-old" {
		t.Fatalf("late-answer close = %+v", got)
	}
	if len(tasks.promptCalls) != 1 || tasks.promptCalls[0] != "t1|s-old" || len(tasks.scopeCalls) != 0 {
		t.Fatalf("late answer authorized with prompt calls %v scope calls %v, want prompt access to s-old", tasks.promptCalls, tasks.scopeCalls)
	}
}

func TestHttpLateAnswerInboxUnansweredSendToTaskTargets(t *testing.T) {
	cases := []struct {
		name         string
		originalOver bool
		target       string
		wantStatus   int
	}{
		{name: "live original rejects another session", originalOver: false, target: "s-primary", wantStatus: http.StatusBadRequest},
		{name: "ended original admits same-task primary", originalOver: true, target: "s-primary", wantStatus: http.StatusNoContent},
		{name: "foreign task target rejected", originalOver: true, target: "s-foreign", wantStatus: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			originalState := taskmodels.TaskSessionStateWaitingForInput
			if tc.originalOver {
				originalState = taskmodels.TaskSessionStateFailed
			}
			sessions := map[string]*taskmodels.TaskSession{
				"s-old":     {ID: "s-old", TaskID: "t1", State: originalState},
				"s-primary": {ID: "s-primary", TaskID: "t1", State: taskmodels.TaskSessionStateRunning, IsPrimary: true},
				"s-foreign": {ID: "s-foreign", TaskID: "t2", State: taskmodels.TaskSessionStateRunning},
			}
			store := &fakeUnansweredInboxStore{closeResult: []*taskmodels.Message{{ID: "m1"}}}
			h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, actionTasks(), store, sessions)
			rec := runUnansweredWrite("p1", "late-answer", `{"message_id":"late-1","session_id":"`+tc.target+`"}`, h.httpLateAnswerInboxUnanswered)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusNoContent {
				if len(store.closeCalls) != 0 {
					t.Fatalf("rejected target reached the store: %+v", store.closeCalls)
				}
				return
			}
			if store.closeCalls[0].SessionID != tc.target || store.closeCalls[0].SourceSessionID != "s-old" {
				t.Fatalf("recorded session = %q source %q, want %q from s-old",
					store.closeCalls[0].SessionID, store.closeCalls[0].SourceSessionID, tc.target)
			}
		})
	}
}

func TestUnansweredSectionDisabledWithoutStoreSupport(t *testing.T) {
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{}, nil)
	rec := runInboxGet(h, "/api/v1/clarification-inbox/unanswered?workspace_id=w1", h.httpListInboxUnanswered)
	var resp inboxUnansweredListResponse
	decodeInboxBody(t, rec, &resp)
	if rec.Code != http.StatusOK || resp.Enabled {
		t.Fatalf("status %d response %+v, want disabled", rec.Code, resp)
	}
	if write := runUnansweredWrite("p1", "close", "", h.httpCloseInboxUnanswered); write.Code != http.StatusConflict {
		t.Fatalf("close status = %d, want 409", write.Code)
	}
}

func TestHttpUnansweredWritesRequireActionAuthority(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "forbidden", err: service.ErrForbidden, wantStatus: http.StatusForbidden},
		{name: "task gone", err: repoerrors.ErrTaskNotFound, wantStatus: http.StatusNotFound},
		{name: "lookup failure", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeUnansweredInboxStore{closeResult: []*taskmodels.Message{{ID: "m1"}}}
			tasks := actionTasks()
			tasks.actionErr = tc.err
			h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, tasks, store, nil)
			for _, write := range []struct {
				action, body string
				handler      gin.HandlerFunc
			}{
				{"close", "", h.httpCloseInboxUnanswered},
				{"late-answer", `{"message_id":"late-1"}`, h.httpLateAnswerInboxUnanswered},
			} {
				rec := runUnansweredWrite("p1", write.action, write.body, write.handler)
				if rec.Code != tc.wantStatus {
					t.Fatalf("%s status = %d, want %d: %s", write.action, rec.Code, tc.wantStatus, rec.Body.String())
				}
			}
			if len(store.closeCalls) != 0 {
				t.Fatalf("writes without action authority reached the store: %+v", store.closeCalls)
			}
		})
	}
}

func TestHttpUnansweredWritesUnavailableWithoutActionAuthorizer(t *testing.T) {
	store := &fakeUnansweredInboxStore{closeResult: []*taskmodels.Message{{ID: "m1"}}}
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, store, nil)
	if rec := runUnansweredWrite("p1", "close", "", h.httpCloseInboxUnanswered); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("close status = %d, want 503", rec.Code)
	}
	if len(store.closeCalls) != 0 {
		t.Fatalf("write without an action authorizer reached the store: %+v", store.closeCalls)
	}
}

func TestHttpLateAnswerInboxUnansweredUnconfirmedAdmissionConflicts(t *testing.T) {
	store := &fakeUnansweredInboxStore{closeErr: taskmodels.ErrUnansweredAdmissionUnknown}
	h := newUnansweredTestHandler(t, alwaysAllowAuthorizer{}, actionTasks(), store, nil)
	rec := runUnansweredWrite("p1", "late-answer", `{"message_id":"late-1"}`, h.httpLateAnswerInboxUnanswered)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "late_answer_not_admitted") {
		t.Fatalf("status = %d body %s, want 409 late_answer_not_admitted", rec.Code, rec.Body.String())
	}
}
