package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The workflow-step surface reaches any workflow, workspace or step by ID.
// Scoping it means the guard has to run *before* the repository, so these
// tests need to see the statements the request actually issued, not just its
// status code: a guard that rejects after reading the row still read the row,
// and a guard deleted from one path still answers 404 on the sibling paths.
//
// stepQueryLog is that instrument. A wrapper driver records every statement
// naming workflow_steps, so a test can assert "denied, and the table was never
// touched" and can tell a real delete from a rejected one.

const countingDriverName = "sqlite3-workflow-scope"

// stepQueries is process-global because a database/sql driver is registered by
// name, not per pool. The tests in this package run sequentially (none call
// t.Parallel), and each scoped harness resets the log at setup.
var stepQueries = &stepQueryLog{}

func init() {
	sql.Register(countingDriverName, &countingDriver{inner: &sqlite3.SQLiteDriver{}, log: stepQueries})
}

type stepQueryLog struct {
	mu         sync.Mutex
	statements []string
}

func (l *stepQueryLog) record(query string) {
	if !strings.Contains(query, "workflow_steps") {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statements = append(l.statements, query)
}

func (l *stepQueryLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statements = nil
}

func (l *stepQueryLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.statements...)
}

// writes returns the statements that would have changed the table.
func (l *stepQueryLog) writes() []string {
	var out []string
	for _, stmt := range l.all() {
		switch strings.ToUpper(strings.Fields(strings.TrimSpace(stmt))[0]) {
		case "INSERT", "UPDATE", "DELETE":
			out = append(out, stmt)
		}
	}
	return out
}

type countingDriver struct {
	inner driver.Driver
	log   *stepQueryLog
}

func (d *countingDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &countingConn{Conn: conn, log: d.log}, nil
}

type countingConn struct {
	driver.Conn
	log *stepQueryLog
}

func (c *countingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.log.record(query)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (c *countingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.log.record(query)
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *countingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.log.record(query)
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
}

func (c *countingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

// scopedOwnership is the task-domain seam the workflow service authorizes
// through. It mirrors internal/task/service's rule exactly: no identity, or a
// synthetic one, means unscoped; a workspace with no owner stays visible to
// everyone; a denial is reported as not-found so existence does not leak.
//
// internal/task/service's own tests cover the real implementation
// (TestAuthorizeWorkflowAccess); this fake exists so the handler tests can
// assert *when* the workflow service calls out, not just the outcome.
type scopedOwnership struct {
	workflowWorkspace map[string]string
	taskWorkspace     map[string]string
	workspaceOwner    map[string]string

	mu             sync.Mutex
	workflowCalls  []string
	workspaceCalls []string
	taskCalls      []string
}

func (o *scopedOwnership) authorizeWorkflow(ctx context.Context, workflowID string) error {
	o.mu.Lock()
	o.workflowCalls = append(o.workflowCalls, workflowID)
	o.mu.Unlock()
	userID, scoped := scopeOf(ctx)
	if !scoped {
		return nil
	}
	workspaceID, ok := o.workflowWorkspace[workflowID]
	if !ok {
		// The task repository reports a missing workflow as a formatted error,
		// not a sentinel (internal/task/repository/sqlite/workflow.go).
		return fmt.Errorf("workflow not found: %s", workflowID)
	}
	return o.visible(userID, workspaceID)
}

func (o *scopedOwnership) authorizeWorkspace(ctx context.Context, workspaceID string) error {
	o.mu.Lock()
	o.workspaceCalls = append(o.workspaceCalls, workspaceID)
	o.mu.Unlock()
	userID, scoped := scopeOf(ctx)
	if !scoped {
		return nil
	}
	return o.visible(userID, workspaceID)
}

func (o *scopedOwnership) authorizeTask(ctx context.Context, taskID string) error {
	o.mu.Lock()
	o.taskCalls = append(o.taskCalls, taskID)
	o.mu.Unlock()
	userID, scoped := scopeOf(ctx)
	if !scoped {
		return nil
	}
	workspaceID, ok := o.taskWorkspace[taskID]
	if !ok {
		// The task service reports a foreign or missing task with the task
		// sentinel, not the workspace one.
		return repoerrors.ErrTaskNotFound
	}
	if err := o.visible(userID, workspaceID); err != nil {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

func (o *scopedOwnership) visible(userID, workspaceID string) error {
	owner, ok := o.workspaceOwner[workspaceID]
	if !ok {
		return repoerrors.ErrWorkspaceNotFound
	}
	if owner != "" && owner != userID {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

func (o *scopedOwnership) calls() (workflows, workspaces, tasks []string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.workflowCalls...),
		append([]string(nil), o.workspaceCalls...),
		append([]string(nil), o.taskCalls...)
}

func (o *scopedOwnership) resetCalls() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.workflowCalls, o.workspaceCalls, o.taskCalls = nil, nil, nil
}

func scopeOf(ctx context.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return "", false
	}
	return identity.UserID, true
}

// scopedHarness is the workflow harness with the ownership checkers wired, two
// workspaces owned by different users, and a step in each.
type scopedHarness struct {
	*workflowHarness
	owner    *scopedOwnership
	provider *fakeWorkflowProvider
	queries  *stepQueryLog
	stepA    string
	stepB    string
	stepB2   string
}

const (
	userA = "user-a"
	userB = "user-b"
)

func setupScopedRouter(t *testing.T, logOverride ...*logger.Logger) *scopedHarness {
	t.Helper()
	h := setupStepRouterWithDriver(t, countingDriverName, logOverride...)
	if _, err := h.db.Exec(
		`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		 VALUES ('wf-a','ws-a','A Flow',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
		        ('wf-b','ws-b','B Flow',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
		        ('wf-b2','ws-b','B Second Flow',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed workflows: %v", err)
	}
	// Seed the steps before the checkers are wired: an identity-less request is
	// unscoped either way, but this keeps the seed out of the call log.
	stepA := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "wf-a", "name": "A Backlog", "position": 0,
	})
	stepB := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "wf-b", "name": "B Backlog", "position": 0,
	})
	// A second workflow for the same owner, so "not yours" and "not this
	// workflow" can be told apart.
	stepB2 := createStepViaHTTP(t, h.router, map[string]interface{}{
		"workflow_id": "wf-b2", "name": "B Second Backlog", "position": 0,
	})

	provider := &fakeWorkflowProvider{workflows: []*taskmodels.Workflow{
		{ID: "wf-a", WorkspaceID: "ws-a", Name: "A Flow"},
		{ID: "wf-b", WorkspaceID: "ws-b", Name: "B Flow"},
	}}
	h.service.SetWorkflowProvider(provider)

	owner := &scopedOwnership{
		workflowWorkspace: map[string]string{"wf-a": "ws-a", "wf-b": "ws-b", "wf-b2": "ws-b"},
		taskWorkspace:     map[string]string{"task-a": "ws-a", "task-b": "ws-b"},
		workspaceOwner:    map[string]string{"ws-a": userA, "ws-b": userB},
	}
	h.service.SetWorkflowAccessChecker(owner.authorizeWorkflow)
	h.service.SetWorkspaceAccessChecker(owner.authorizeWorkspace)
	h.service.SetTaskAccessChecker(owner.authorizeTask)

	stepQueries.reset()
	return &scopedHarness{
		workflowHarness: h, owner: owner, provider: provider, queries: stepQueries,
		stepA: stepA.ID, stepB: stepB.ID, stepB2: stepB2.ID,
	}
}

// asUser attaches a real (non-synthetic) identity, the way the auth middleware
// does for an authenticated request.
func asUser(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

// asSyntheticUser attaches the implicit identity injected when authentication
// is disabled.
func asSyntheticUser() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userA, Synthetic: true})
}

// doAs issues an HTTP request carrying ctx's identity.
func doAs(t *testing.T, h *scopedHarness, ctx context.Context, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	return doRawAs(t, h, ctx, method, path, string(payload))
}

// doRawAs issues a request with a raw (possibly non-JSON) body under ctx's identity.
func doRawAs(t *testing.T, h *scopedHarness, ctx context.Context, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

// dispatchAs routes one WS message through the registered handlers under ctx's
// identity, the way the gateway dispatches an authenticated client's action.
func dispatchAs(t *testing.T, h *scopedHarness, ctx context.Context, action string, payload any) *ws.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: action, Payload: raw}
	resp, err := h.dispatcher.Dispatch(ctx, msg)
	if err != nil {
		t.Fatalf("dispatch %s: %v", action, err)
	}
	if resp == nil {
		t.Fatalf("dispatch %s returned no message", action)
	}
	return resp
}

// requireStepUntouched asserts the workflow_steps table was never read or
// written while serving the request under test.
func requireStepTableUntouched(t *testing.T, h *scopedHarness) {
	t.Helper()
	if stmts := h.queries.all(); len(stmts) != 0 {
		t.Fatalf("guard let %d workflow_steps statement(s) through: %v", len(stmts), stmts)
	}
}

// requireNoStepWrites asserts nothing mutated the table. Step-keyed routes
// resolve the step's owning workflow through the repository on purpose, so
// only writes can be forbidden there.
func requireNoStepWrites(t *testing.T, h *scopedHarness) {
	t.Helper()
	if stmts := h.queries.writes(); len(stmts) != 0 {
		t.Fatalf("denied request wrote to workflow_steps: %v", stmts)
	}
}

// stepNames returns the persisted step names for a workflow, so a test can
// prove a rejected mutation left the rows alone.
func stepNames(t *testing.T, h *scopedHarness, workflowID string) []string {
	t.Helper()
	steps, err := h.repo.ListStepsByWorkflow(context.Background(), workflowID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		names = append(names, step.Name)
	}
	return names
}

// decodeJSON decodes a successful response body into out.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, want, rec.Body.String())
	}
}

func requireNotFound(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	requireStatus(t, rec, http.StatusNotFound)
}
