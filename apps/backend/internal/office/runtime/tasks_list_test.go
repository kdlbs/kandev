package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// fakeTaskFilteredLister is a TaskFilteredLister fake that records the
// workspace and options it was called with, and returns a canned result or
// error.
type fakeTaskFilteredLister struct {
	calls        int
	workspaceIDs []string
	opts         []sqlite.ListTasksOptions
	result       *sqlite.ListTasksFilteredResult
	err          error
}

func (f *fakeTaskFilteredLister) ListTasksFiltered(
	_ context.Context, workspaceID string, opts sqlite.ListTasksOptions,
) (*sqlite.ListTasksFilteredResult, error) {
	f.calls++
	f.workspaceIDs = append(f.workspaceIDs, workspaceID)
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &sqlite.ListTasksFilteredResult{}, nil
}

// boardReadHarness wires a runtime handler around a taskless run (empty
// task_id claim) and an injected TaskFilteredLister, mirroring
// newRuntimeHandlerHarnessWithProjectManager but for the board-read route,
// which needs a taskless token and a lister dependency neither existing
// harness constructs.
type boardReadHarness struct {
	router    *gin.Engine
	token     string
	runEvents *recordingRunEvents
}

// newBoardReadHarness mints a taskless token (empty task_id) claiming
// claimWorkspaceID. The agent row itself always lives in a fixed real
// workspace, independent of claimWorkspaceID, so an empty-claim test case
// can still resolve the agent by ID.
func newBoardReadHarness(t *testing.T, caps Capabilities, claimWorkspaceID string, lister TaskFilteredLister) *boardReadHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	agentSvc := agents.NewAgentService(repo, logger.Default(), nil)
	agentSvc.SetAuth(agents.NewAgentAuth("board-read-test-key"))
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-agent-home",
		Name:        "Coordinator",
		Role:        models.AgentRoleCEO,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	capabilityJSON, err := MarshalCapabilities(caps)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	token, err := agentSvc.MintRuntimeJWT("agent-1", "", claimWorkspaceID, "run-1", "sess-1", capabilityJSON)
	if err != nil {
		t.Fatalf("mint runtime token: %v", err)
	}
	runEvents := &recordingRunEvents{}
	router := gin.New()
	RegisterRoutes(router.Group(""), NewHandler(
		agentSvc,
		NewActions(ActionDependencies{Agents: agentSvc}),
		nil,
		runEvents,
		lister,
	))
	return &boardReadHarness{router: router, token: token, runEvents: runEvents}
}

func (h *boardReadHarness) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, req)
	return resp
}

func TestListTasks_TasklessRunWithCapabilitySucceeds(t *testing.T) {
	lister := &fakeTaskFilteredLister{result: &sqlite.ListTasksFilteredResult{
		Tasks: []*sqlite.TaskRow{
			{ID: "task-a", WorkspaceID: "ws-1", Title: "Alpha", Status: "TODO", Priority: "medium"},
		},
		NextCursor: "cursor-1",
		NextID:     "task-a",
	}}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if lister.calls != 1 || lister.workspaceIDs[0] != "ws-1" {
		t.Fatalf("lister calls/workspaces = %d/%v, want 1/[ws-1]", lister.calls, lister.workspaceIDs)
	}
	var decoded struct {
		Tasks      []TaskListItem `json:"tasks"`
		NextCursor string         `json:"next_cursor"`
		NextID     string         `json:"next_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(decoded.Tasks) != 1 || decoded.Tasks[0].ID != "task-a" {
		t.Fatalf("tasks = %+v", decoded.Tasks)
	}
	if decoded.NextCursor != "cursor-1" || decoded.NextID != "task-a" {
		t.Fatalf("cursor = %q/%q", decoded.NextCursor, decoded.NextID)
	}
}

func TestListTasks_WithoutCapabilityDenied(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if lister.calls != 0 {
		t.Fatalf("lister should not be called when capability is denied: %d", lister.calls)
	}
}

func TestListTasks_EmptyWorkspaceClaimRefusedBeforeQuery(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "", lister)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if lister.calls != 0 {
		t.Fatalf("lister should not be called with an empty workspace claim: %d", lister.calls)
	}
}

func TestListTasks_InvalidParamRefusedBeforeQuery(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks?sort=bogus")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if lister.calls != 0 {
		t.Fatalf("lister should not be called with an invalid parameter: %d", lister.calls)
	}
}

func TestListTasks_RepeatableStatusFilterReachesLister(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks?status=TODO&status=IN_PROGRESS")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if lister.calls != 1 {
		t.Fatalf("lister calls = %d, want 1", lister.calls)
	}
	got := lister.opts[0].Status
	if len(got) != 2 || got[0] != "TODO" || got[1] != "IN_PROGRESS" {
		t.Fatalf("status opts = %v, want [TODO IN_PROGRESS]", got)
	}
}

func TestListTasks_ListerErrorSurfacesAsInternalError(t *testing.T) {
	lister := &fakeTaskFilteredLister{err: errors.New("db unavailable")}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusInternalServerError, resp.Body.String())
	}
}

func TestListTasks_NoListerConfiguredFailsClosed(t *testing.T) {
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", nil)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusInternalServerError, resp.Body.String())
	}
}

func TestListTasks_AppendsActionRunEventOnSuccess(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	assertActionRunEvent(t, h.runEvents, "list_tasks", "workspace", "ws-1")
}

func TestListTasks_RepeatedIdenticalCallsAreNotDeduplicated(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{CanListTasks: true}, "ws-1", lister)

	for i := 0; i < 2; i++ {
		resp := h.get(t, "/runtime/tasks")
		if resp.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want %d; body=%s", i, resp.Code, http.StatusOK, resp.Body.String())
		}
	}
	if lister.calls != 2 {
		t.Fatalf("lister calls = %d, want 2", lister.calls)
	}
	if len(h.runEvents.events) != 2 {
		t.Fatalf("run events = %d, want 2 (no dedup)", len(h.runEvents.events))
	}
	for _, event := range h.runEvents.events {
		if event.eventType != "runtime.action" {
			t.Fatalf("event = %#v, want eventType runtime.action", event)
		}
	}
}

func TestListTasks_DeniedCapabilityAppendsDeniedRunEvent(t *testing.T) {
	lister := &fakeTaskFilteredLister{}
	h := newBoardReadHarness(t, Capabilities{}, "ws-1", lister)

	resp := h.get(t, "/runtime/tasks")
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	assertDeniedRunEvent(t, h.runEvents, "list_tasks", "workspace", "ws-1")
}
