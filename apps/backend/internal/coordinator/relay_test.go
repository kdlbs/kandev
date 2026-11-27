package coordinator

import (
	"context"
	"errors"
	"expvar"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type fakeRelayReader struct {
	page        *taskmodels.ClarificationBundlePage
	bundleErr   error
	bundleOpts  taskmodels.ListClarificationBundlesOptions
	bundleCalls int
	messages    map[string][]*taskmodels.Message
	interacts   []*taskmodels.Message
	interactErr error
	filter      taskmodels.PendingInteractionFilter
}

func (f *fakeRelayReader) ListUnresolvedClarificationBundles(_ context.Context, opts taskmodels.ListClarificationBundlesOptions) (*taskmodels.ClarificationBundlePage, error) {
	f.bundleCalls++
	f.bundleOpts = opts
	if f.bundleErr != nil {
		return nil, f.bundleErr
	}
	if f.page == nil {
		return &taskmodels.ClarificationBundlePage{}, nil
	}
	return f.page, nil
}

func (f *fakeRelayReader) FindMessagesByPendingIDs(_ context.Context, _ []string) (map[string][]*taskmodels.Message, error) {
	return f.messages, nil
}

func (f *fakeRelayReader) ListPendingInteractions(_ context.Context, filter taskmodels.PendingInteractionFilter) ([]*taskmodels.Message, error) {
	f.filter = filter
	return f.interacts, f.interactErr
}

type fakeRelayTasks struct {
	tasks   map[string]*taskmodels.Task
	primary map[string]string
	err     error
}

func (f *fakeRelayTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	if task, ok := f.tasks[id]; ok {
		return task, nil
	}
	return nil, repoerrors.ErrTaskNotFound
}

func (f *fakeRelayTasks) GetPrimarySessionIDsForTasks(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if s, ok := f.primary[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

type relayFixture struct {
	h      *Handlers
	svc    *Service
	c      *Coordinator
	az     *fakeWorkspaceAuthorizer
	reader *fakeRelayReader
	tasks  *fakeRelayTasks
}

func newRelayFixture(t *testing.T, phase3 bool) *relayFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	az := &fakeWorkspaceAuthorizer{}
	svc := NewService(store, newValidatorForTest(nil, nil), az, newTestLogger(t), WithPhase2(true), WithPhase3(phase3))
	reader := &fakeRelayReader{}
	tasks := &fakeRelayTasks{
		tasks:   map[string]*taskmodels.Task{"t1": {ID: "t1", WorkspaceID: "ws-1"}, "tx": {ID: "tx", WorkspaceID: "ws-2"}},
		primary: map[string]string{"t1": "s1", "tx": "sx"},
	}
	svc.SetRelayDeps(reader, tasks)
	c := newTestCoordinator(t, store, "ws-1")
	return &relayFixture{h: NewHandlers(svc, newTestLogger(t)), svc: svc, c: c, az: az, reader: reader, tasks: tasks}
}

func relayParams(cid, taskID string) gin.Params {
	return gin.Params{{Key: "id", Value: "ws-1"}, {Key: "cid", Value: cid}, {Key: "taskId", Value: taskID}}
}

func permissionMsg(meta map[string]any) *taskmodels.Message {
	return &taskmodels.Message{ID: "pm1", TaskSessionID: "s1", TaskID: "t1", Type: taskmodels.MessageTypePermissionRequest, Content: "Run ls", Metadata: meta}
}

func fullPermissionMeta() map[string]any {
	return map[string]any{
		"request_id": "r1", "pending_id": "p1",
		"options": []any{map[string]any{"option_id": "allow", "name": "Allow", "kind": "allow_once"}},
	}
}

func (f *relayFixture) get(t *testing.T, taskID string) (int, RelayResult) {
	t.Helper()
	rec := runHandler(f.h.httpGetRelay, http.MethodGet, "/x", "", relayParams(f.c.ID, taskID))
	var body RelayResult
	if rec.Code == http.StatusOK {
		decodeBody(t, rec, &body)
	}
	return rec.Code, body
}

func TestRelay_NotFoundWhenPhase3NotEffective(t *testing.T) {
	f := newRelayFixture(t, false)
	if code, _ := f.get(t, "t1"); code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if f.reader.bundleCalls != 0 {
		t.Fatal("phase 3 off must not read")
	}
}

func TestRelay_ForeignCoordinatorAndTaskAre404AndUseReadScope(t *testing.T) {
	f := newRelayFixture(t, true)
	other := newTestCoordinator(t, f.svc.store, "ws-2")
	rec := runHandler(f.h.httpGetRelay, http.MethodGet, "/x", "", relayParams(other.ID, "t1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign coordinator status = %d", rec.Code)
	}
	if code, _ := f.get(t, "tx"); code != http.StatusNotFound {
		t.Fatalf("foreign task status = %d", code)
	}
	if code, _ := f.get(t, "missing"); code != http.StatusNotFound {
		t.Fatalf("missing task status = %d", code)
	}
	if got := f.az.scopes[len(f.az.scopes)-1]; got != authz.ScopeWorkspaceRead {
		t.Fatalf("scope = %v, want workspace.read", got)
	}
}

func TestRelay_NoPrimarySessionReturnsNulls(t *testing.T) {
	f := newRelayFixture(t, true)
	delete(f.tasks.primary, "t1")
	code, body := f.get(t, "t1")
	if code != http.StatusOK || body.SessionID != "" || body.Clarification != nil || body.Permission != nil {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	if f.reader.bundleCalls != 0 {
		t.Fatal("no session must not read bundles")
	}
}

func TestRelay_BundleReadIsSessionScopedUnscopedLimitOne(t *testing.T) {
	f := newRelayFixture(t, true)
	f.reader.page = &taskmodels.ClarificationBundlePage{Bundles: []taskmodels.ClarificationBundleSummary{{PendingID: "pb1", SessionID: "s1", TaskID: "t1"}}}
	f.reader.messages = map[string][]*taskmodels.Message{"pb1": {{ID: "m1", Metadata: map[string]any{"question_id": "q", "context": "why"}}}}
	code, body := f.get(t, "t1")
	if code != http.StatusOK || body.Clarification == nil {
		t.Fatalf("status=%d body=%+v", code, body)
	}
	o := f.reader.bundleOpts
	if !o.Unscoped || o.SessionID != "s1" || o.Sidecar != nil || o.Limit != 1 {
		t.Fatalf("opts = %+v", o)
	}
	if body.Clarification.PendingID != "pb1" || body.Clarification.Context != "why" || len(body.Clarification.Messages) != 1 {
		t.Fatalf("clarification = %+v", body.Clarification)
	}
	if body.TaskID != "t1" || body.SessionID != "s1" {
		t.Fatalf("ids = %q %q", body.TaskID, body.SessionID)
	}
}

func TestRelay_BundleWithNoMessagesIsNull(t *testing.T) {
	f := newRelayFixture(t, true)
	f.reader.page = &taskmodels.ClarificationBundlePage{Bundles: []taskmodels.ClarificationBundleSummary{{PendingID: "pb1"}}}
	_, body := f.get(t, "t1")
	if body.Clarification != nil {
		t.Fatalf("clarification = %+v, want nil", body.Clarification)
	}
}

func TestRelay_PermissionReturnedOnlyWhenAnswerable(t *testing.T) {
	f := newRelayFixture(t, true)
	f.reader.interacts = []*taskmodels.Message{permissionMsg(fullPermissionMeta())}
	_, body := f.get(t, "t1")
	if body.Permission == nil || body.Permission.Message == nil || body.Permission.Message.Content != "Run ls" {
		t.Fatalf("permission = %+v", body.Permission)
	}
	if got := f.reader.filter; len(got.SessionIDs) != 1 || got.SessionIDs[0] != "s1" || len(got.Kinds) != 1 || got.Kinds[0] != "permission" {
		t.Fatalf("filter = %+v", got)
	}
	for name, mutate := range map[string]func(map[string]any){
		"no request_id":   func(m map[string]any) { delete(m, "request_id") },
		"empty request":   func(m map[string]any) { m["request_id"] = "" },
		"no pending_id":   func(m map[string]any) { delete(m, "pending_id") },
		"empty options":   func(m map[string]any) { m["options"] = []any{} },
		"missing options": func(m map[string]any) { delete(m, "options") },
	} {
		meta := fullPermissionMeta()
		mutate(meta)
		f.reader.interacts = []*taskmodels.Message{permissionMsg(meta)}
		if _, got := f.get(t, "t1"); got.Permission != nil {
			t.Errorf("%s: permission = %+v, want nil", name, got.Permission)
		}
	}
}

func TestRelay_PermissionTakesNewestRow(t *testing.T) {
	f := newRelayFixture(t, true)
	older := permissionMsg(fullPermissionMeta())
	older.ID = "old"
	newer := permissionMsg(fullPermissionMeta())
	newer.ID = "new"
	f.reader.interacts = []*taskmodels.Message{older, newer}
	if _, body := f.get(t, "t1"); body.Permission == nil || body.Permission.Message.ID != "new" {
		t.Fatalf("permission = %+v, want the newest row", body.Permission)
	}
}

func TestRelay_ReadErrorIs500AndCounted(t *testing.T) {
	f := newRelayFixture(t, true)
	counter := expvar.Get("coordinator_relay_read_failed_total").(*expvar.Int)
	before := counter.Value()
	f.reader.bundleErr = errors.New("boom")
	if code, _ := f.get(t, "t1"); code != http.StatusInternalServerError {
		t.Fatalf("bundle error status = %d", code)
	}
	f.reader.bundleErr = nil
	f.reader.interactErr = errors.New("boom")
	if code, _ := f.get(t, "t1"); code != http.StatusInternalServerError {
		t.Fatalf("permission error status = %d", code)
	}
	if got := counter.Value() - before; got != 2 {
		t.Fatalf("counter delta = %d, want 2", got)
	}
	f.reader.interactErr = nil
	if code, _ := f.get(t, "missing"); code != http.StatusNotFound || counter.Value()-before != 2 {
		t.Fatal("a 404 must not be counted")
	}
}
