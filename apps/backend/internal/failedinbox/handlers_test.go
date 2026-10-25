package failedinbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeWorkspaceAuthorizer struct {
	err error
}

func (f *fakeWorkspaceAuthorizer) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error {
	return f.err
}

type fakeFailedTaskStore struct {
	page *taskmodels.FailedInboxPage
	err  error
	// lastOpts records the options of the most recent call, for assertions
	// on what the handler passed through.
	lastOpts taskmodels.ListFailedInboxOptions
}

func (f *fakeFailedTaskStore) ListFailedInboxTasks(_ context.Context, opts taskmodels.ListFailedInboxOptions) (*taskmodels.FailedInboxPage, error) {
	f.lastOpts = opts
	if f.err != nil {
		return nil, f.err
	}
	if f.page != nil {
		return f.page, nil
	}
	return &taskmodels.FailedInboxPage{}, nil
}

func newFailedInboxTestHandler(authorizer workspaceAuthorizer, store failedTaskStore) *Handlers {
	gin.SetMode(gin.TestMode)
	return NewHandlers(authorizer, store, logger.Default())
}

func runFailedInboxGet(h *Handlers, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	h.httpListFailedInbox(c)
	return rec
}

func decodeFailedInboxBody(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
}

func TestHttpListFailedInbox_MissingWorkspaceIDIs400(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHttpListFailedInbox_NonNumericLimitIs400(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1&limit=abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHttpListFailedInbox_NonPositiveLimitIs400(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1&limit=0")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHttpListFailedInbox_AbsentLimitDefaultsTo50(t *testing.T) {
	store := &fakeFailedTaskStore{}
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, store)
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.lastOpts.Limit != 50 {
		t.Errorf("limit = %d, want 50", store.lastOpts.Limit)
	}
}

func TestHttpListFailedInbox_LimitAbove200Clamps(t *testing.T) {
	store := &fakeFailedTaskStore{}
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, store)
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1&limit=999")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.lastOpts.Limit != 200 {
		t.Errorf("limit = %d, want clamped 200", store.lastOpts.Limit)
	}
}

func TestHttpListFailedInbox_UnreachableWorkspaceIs404NonDisclosing(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{err: repoerrors.ErrWorkspaceNotFound}, &fakeFailedTaskStore{})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws-outside")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHttpListFailedInbox_ForbiddenMapsTo403(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{err: service.ErrForbidden}, &fakeFailedTaskStore{})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestHttpListFailedInbox_StoreErrorIs500(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{err: errors.New("boom")})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestHttpListFailedInbox_EmptyPageReturnsEmptyListNotAbsent(t *testing.T) {
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{page: &taskmodels.FailedInboxPage{}})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1")
	var body struct {
		Rows      []json.RawMessage `json:"rows"`
		Count     int               `json:"count"`
		Truncated bool              `json:"truncated"`
	}
	decodeFailedInboxBody(t, rec, &body)
	if body.Rows == nil {
		t.Error("expected an empty array, got absent/null rows")
	}
	if body.Count != 0 || body.Truncated {
		t.Errorf("expected count=0 truncated=false, got count=%d truncated=%v", body.Count, body.Truncated)
	}
}

func TestHttpListFailedInbox_CountIsRenderedRowSetSize(t *testing.T) {
	instant := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	page := &taskmodels.FailedInboxPage{
		Rows: []taskmodels.FailedInboxTaskRow{
			{TaskID: "t1", Title: "Task one", WorkspaceID: "ws1", Origin: "manual", FailureInstant: &instant, Reason: "boom"},
			{TaskID: "t2", Title: "Task two", WorkspaceID: "ws1", Origin: "", FailureInstant: nil, Reason: ""},
		},
		HasMore: true,
	}
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{page: page})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1")

	var body struct {
		Rows []struct {
			TaskID         string  `json:"task_id"`
			Title          string  `json:"title"`
			WorkspaceID    string  `json:"workspace_id"`
			Origin         string  `json:"origin"`
			FailureInstant *string `json:"failure_instant"`
			Reason         string  `json:"reason"`
		} `json:"rows"`
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	decodeFailedInboxBody(t, rec, &body)
	if body.Count != 2 {
		t.Errorf("count = %d, want 2 (the rendered row set's own size)", body.Count)
	}
	if !body.Truncated {
		t.Error("expected truncated=true")
	}
	if body.Rows[0].TaskID != "t1" || body.Rows[0].FailureInstant == nil || *body.Rows[0].FailureInstant != "2026-09-14T01:00:00Z" {
		t.Errorf("row 0 = %+v", body.Rows[0])
	}
	if body.Rows[1].FailureInstant != nil {
		t.Errorf("row 1 failure_instant should be omitted (nil), got %v", *body.Rows[1].FailureInstant)
	}
	if body.Rows[1].Origin != "" || body.Rows[1].Reason != "" {
		t.Errorf("row 1 = %+v, want empty-string origin and reason rather than absent", body.Rows[1])
	}
}

func TestHttpListFailedInbox_ReasonSanitizedAndTruncated(t *testing.T) {
	longReason := ""
	for i := 0; i < 600; i++ {
		longReason += "a"
	}
	page := &taskmodels.FailedInboxPage{
		Rows: []taskmodels.FailedInboxTaskRow{
			{TaskID: "t1", Title: "Task one", WorkspaceID: "ws1", Reason: longReason},
		},
	}
	h := newFailedInboxTestHandler(&fakeWorkspaceAuthorizer{}, &fakeFailedTaskStore{page: page})
	rec := runFailedInboxGet(h, "/api/v1/failed-inbox?workspace_id=ws1")

	var body struct {
		Rows []struct {
			Reason string `json:"reason"`
		} `json:"rows"`
	}
	decodeFailedInboxBody(t, rec, &body)
	if got := utf8.RuneCountInString(body.Rows[0].Reason); got != 512 {
		t.Errorf("expected the reason truncated to 512 code points, got length %d", got)
	}
}
