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
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// keyedAuthorizer is a taskAccessAuthorizer double that denies every task_id
// except the ones explicitly allow-listed, and records every task_id it was
// asked to authorize. It lets A8's tests prove which task_id (durable
// message vs in-memory Request) actually reached the authorizer.
type keyedAuthorizer struct {
	allowed map[string]bool
	calls   []string
}

func (a *keyedAuthorizer) AuthorizeTaskAccess(_ context.Context, taskID string) error {
	a.calls = append(a.calls, taskID)
	if a.allowed[taskID] {
		return nil
	}
	return errAuthzDenied
}

var errAuthzDenied = errors.New("denied")

func setupAuthzHandler(t *testing.T, msgs map[string][]*taskmodels.Message, authorizer taskAccessAuthorizer) (*Handlers, *Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: msgs}
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	resolver := NewResolver(store, repo, messageCreator, authorizer, eventBus, eventBus, nil, logger.Default())
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, resolver, logger.Default())
	return h, store
}

func runGet(t *testing.T, h *Handlers, pendingID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clarification/"+pendingID, nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpGetRequest(c)
	return rec
}

func runWait(t *testing.T, h *Handlers, pendingID string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clarification/"+pendingID+"/wait", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: pendingID}}
	h.httpWaitForResponse(c)
	return rec
}

// TestHttpGetRequest_UnknownPendingID_404 proves A5: a pending_id with no
// durable messages is not found on the read endpoint too.
func TestHttpGetRequest_UnknownPendingID_404(t *testing.T) {
	h, _ := setupAuthzHandler(t, map[string][]*taskmodels.Message{}, &stubAuthorizer{})
	rec := runGet(t, h, "does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpGetRequest_AuthorizationDenied_404 proves A3/A7: a caller denied
// access to a bundle that DOES exist (both durably and in-memory) gets the
// same 404 as an unknown pending_id — never a 403, never a body leaking
// question text.
func TestHttpGetRequest_AuthorizationDenied_404(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-authz-1": {clarificationMessage("m1", "pending-authz-1", "q1", 0, "secret question", "opt1")},
	}
	h, store := setupAuthzHandler(t, msgs, &stubAuthorizer{err: errAuthzDenied})
	store.pending["pending-authz-1"] = &PendingClarification{
		Request:  &Request{PendingID: "pending-authz-1", SessionID: "s1", TaskID: "t1"},
		done:     make(chan struct{}),
		CancelCh: make(chan struct{}),
	}

	rec := runGet(t, h, "pending-authz-1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a denied caller, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret question") {
		t.Fatalf("denied response must not leak question text: %s", rec.Body.String())
	}
}

// TestHttpWaitForResponse_AuthorizationDenied_404 proves A7 on the wait
// endpoint specifically: authorization runs before store.WaitForResponse, so
// a denied caller gets 404 rather than blocking for a 504 timeout.
func TestHttpWaitForResponse_AuthorizationDenied_404(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-authz-2": {clarificationMessage("m1", "pending-authz-2", "q1", 0, "First?", "opt1")},
	}
	h, _ := setupAuthzHandler(t, msgs, &stubAuthorizer{err: errAuthzDenied})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := runWait(t, h, "pending-authz-2", ctx)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a denied caller (not a 504 timeout), got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpWaitForResponse_UnknownPendingID_404 proves A5a: a pending_id with
// no durable messages is 404 on GET /:id/wait, where the pre-ResolveBundle
// handler fell through to a 504 timeout.
func TestHttpWaitForResponse_UnknownPendingID_404(t *testing.T) {
	h, _ := setupAuthzHandler(t, map[string][]*taskmodels.Message{}, &stubAuthorizer{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := runWait(t, h, "does-not-exist", ctx)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpWaitForResponse_A5a_ForeignAndFabricatedPendingIDsReturnSameStatus
// proves A5a directly: GET /:id/wait returns the SAME status for a foreign
// bundle (exists durably, caller denied) and for a fabricated pending_id (no
// durable messages at all). Asserting only "both are 404" via two independent
// hardcoded literals — as TestHttpWaitForResponse_AuthorizationDenied_404 and
// TestHttpWaitForResponse_UnknownPendingID_404 do above — would keep passing
// even if a future change moved just one of the two cases to a different
// status; it would not, on its own, prove the two remain indistinguishable to
// an unauthorized caller. A3 exists precisely so a status-code oracle can't
// tell a caller which pending_ids exist, so this test compares the two
// responses to each other rather than each to a hardcoded literal.
func TestHttpWaitForResponse_A5a_ForeignAndFabricatedPendingIDsReturnSameStatus(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-a5a-foreign": {clarificationMessage("m1", "pending-a5a-foreign", "q1", 0, "First?", "opt1")},
	}
	h, _ := setupAuthzHandler(t, msgs, &stubAuthorizer{err: errAuthzDenied})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	foreignRec := runWait(t, h, "pending-a5a-foreign", ctx)
	fabricatedRec := runWait(t, h, "pending-a5a-fabricated-does-not-exist", ctx)

	if foreignRec.Code != fabricatedRec.Code {
		t.Fatalf("foreign bundle status = %d, fabricated pending_id status = %d; A5a requires them to be identical so an unauthorized caller cannot distinguish the two",
			foreignRec.Code, fabricatedRec.Code)
	}
	if foreignRec.Code != http.StatusNotFound {
		t.Fatalf("both responses = %d, want 404", foreignRec.Code)
	}
}

// TestHttpGetRequest_A8_AuthorizesDurableTaskID_NotInMemory proves A8: the
// task_id used for authorization always comes from the bundle's durable
// messages (M5), never the in-memory Request.TaskID — even when the two
// disagree. Here the authorizer permits only the durable task_id; the
// in-memory entry carries a different, unauthorized task_id.
func TestHttpGetRequest_A8_AuthorizesDurableTaskID_NotInMemory(t *testing.T) {
	msg := clarificationMessage("m1", "pending-authz-3", "q1", 0, "First?", "opt1")
	msg.TaskID = "task-durable"
	msgs := map[string][]*taskmodels.Message{"pending-authz-3": {msg}}

	authorizer := &keyedAuthorizer{allowed: map[string]bool{"task-durable": true}}
	h, store := setupAuthzHandler(t, msgs, authorizer)
	store.pending["pending-authz-3"] = &PendingClarification{
		Request:  &Request{PendingID: "pending-authz-3", SessionID: "s1", TaskID: "task-memory"},
		done:     make(chan struct{}),
		CancelCh: make(chan struct{}),
	}

	rec := runGet(t, h, "pending-authz-3")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (durable task_id is authorized), got %d (body=%s)", rec.Code, rec.Body.String())
	}
	for _, called := range authorizer.calls {
		if called == "task-memory" {
			t.Fatalf("authorizer must never be called with the in-memory Request.TaskID; calls=%v", authorizer.calls)
		}
	}
}

// TestHttpGetRequest_A8_StaleInMemoryTaskIDCannotAuthorize proves the
// converse of A8: an authorizer that only allows the stale in-memory
// Request.TaskID does NOT let the caller through, because authorization is
// keyed on the durable task_id instead.
func TestHttpGetRequest_A8_StaleInMemoryTaskIDCannotAuthorize(t *testing.T) {
	msg := clarificationMessage("m1", "pending-authz-4", "q1", 0, "First?", "opt1")
	msg.TaskID = "task-durable"
	msgs := map[string][]*taskmodels.Message{"pending-authz-4": {msg}}

	authorizer := &keyedAuthorizer{allowed: map[string]bool{"task-memory": true}}
	h, store := setupAuthzHandler(t, msgs, authorizer)
	store.pending["pending-authz-4"] = &PendingClarification{
		Request:  &Request{PendingID: "pending-authz-4", SessionID: "s1", TaskID: "task-memory"},
		done:     make(chan struct{}),
		CancelCh: make(chan struct{}),
	}

	rec := runGet(t, h, "pending-authz-4")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (only the stale in-memory task_id is authorized), got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_AuthorizationDenied_404 and
// TestHttpCancelRequest_AuthorizationDenied_404 prove A2/A3 hold on the two
// write endpoints too, sharing ResolveBundle's own authorization step.
func TestHttpRespond_AuthorizationDenied_404(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-authz-5": {clarificationMessage("m1", "pending-authz-5", "q1", 0, "First?", "opt1")},
	}
	h, _ := setupAuthzHandler(t, msgs, &stubAuthorizer{err: errAuthzDenied})
	rec := runRespond(t, h, "pending-authz-5", RespondBody{Rejected: true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a denied caller, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestHttpCancelRequest_AuthorizationDenied_404(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-authz-6": {clarificationMessage("m1", "pending-authz-6", "q1", 0, "First?", "opt1")},
	}
	h, _ := setupAuthzHandler(t, msgs, &stubAuthorizer{err: errAuthzDenied})
	rec := runCancel(t, h, "pending-authz-6")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a denied caller, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpRespond_L16Bundle_RejectionReturns400NotUpstream500 proves L16 end
// to end at the REST boundary: a rejection against a bundle with a missing
// question_id must be rejected pre-claim with a named 400, never reach
// CompleteActiveClarificationBundle, and never surface upstream's 500 (R4a).
// stubMessageCreator's zero-valued CompleteActiveClarificationBundle would
// return completeClaimed=false with no error if it were ever called, so a
// bug that let this outcome reach the claim would show up as an unexpected
// 200/409 here rather than accidentally passing.
func TestHttpRespond_L16Bundle_RejectionReturns400NotUpstream500(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-l16": {clarificationMessage("m1", "pending-l16", "", 0, "First?", "opt1")},
	}
	h, _ := setupAuthzHandler(t, msgs, &stubAuthorizer{})

	rec := runRespond(t, h, "pending-l16", RespondBody{Rejected: true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a rejection against an L16 bundle, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpGetRequest_UnscopedCaller_Authorized proves the auth-disabled
// compatibility path: a permissive authorizer (the production stand-in for
// no identity / a synthetic identity, per CLAUDE.md's per-user scoping
// notes) reaches every bundle exactly as it did before this feature existed.
func TestHttpGetRequest_UnscopedCaller_Authorized(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"pending-authz-7": {clarificationMessage("m1", "pending-authz-7", "q1", 0, "First?", "opt1")},
	}
	h, store := setupAuthzHandler(t, msgs, &stubAuthorizer{})
	store.pending["pending-authz-7"] = &PendingClarification{
		Request:  &Request{PendingID: "pending-authz-7", SessionID: "s1", TaskID: "t1"},
		done:     make(chan struct{}),
		CancelCh: make(chan struct{}),
	}

	rec := runGet(t, h, "pending-authz-7")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an unscoped/permissive caller, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestHttpGetRequest_RepositoryFailure_500NotSilent404 and its siblings below
// prove the fix for the P2 finding that AuthorizeBundleAccess's error path
// collapsed EVERY error to the same silent 404 as an unknown/foreign
// pending_id -- including a genuine repository failure (e.g. a database
// outage), which was previously indistinguishable from "no such bundle" and
// left no log record for an operator to find. A repository failure now
// reports 500 and does not leak the raw repository error text into the
// response body.
func TestHttpGetRequest_RepositoryFailure_500NotSilent404(t *testing.T) {
	repo := &stubMessageStore{findErr: errors.New("db is on fire")}
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	resolver := NewResolver(store, repo, messageCreator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, resolver, logger.Default())

	rec := runGet(t, h, "pending-repo-failure")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for a repository failure, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "db is on fire") {
		t.Fatalf("response must not leak the raw repository error text: %s", rec.Body.String())
	}
}

func TestHttpWaitForResponse_RepositoryFailure_500NotSilent404(t *testing.T) {
	repo := &stubMessageStore{findErr: errors.New("db is on fire")}
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	resolver := NewResolver(store, repo, messageCreator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, resolver, logger.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := runWait(t, h, "pending-repo-failure", ctx)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for a repository failure, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "db is on fire") {
		t.Fatalf("response must not leak the raw repository error text: %s", rec.Body.String())
	}
}

func TestHttpCancelRequest_RepositoryFailure_500NotSilent404(t *testing.T) {
	repo := &stubMessageStore{findErr: errors.New("db is on fire")}
	gin.SetMode(gin.TestMode)
	store := NewStore(time.Minute)
	eventBus := &stubEventBus{}
	messageCreator := &stubMessageCreator{}
	resolver := NewResolver(store, repo, messageCreator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	h := NewHandlers(store, nil, messageCreator, repo, eventBus, resolver, logger.Default())

	rec := runCancel(t, h, "pending-repo-failure")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for a repository failure, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "db is on fire") {
		t.Fatalf("response must not leak the raw repository error text: %s", rec.Body.String())
	}
}
