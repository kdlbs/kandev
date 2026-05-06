package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
)

// agentCallerStubReader returns a configurable AgentInstance for permission tests.
type agentCallerStubReader struct {
	agent *models.AgentInstance
}

func (s *agentCallerStubReader) GetAgentInstance(_ context.Context, _ string) (*models.AgentInstance, error) {
	return s.agent, nil
}

func (s *agentCallerStubReader) ListAgentInstances(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	return nil, nil
}

func (s *agentCallerStubReader) ListAgentInstancesByIDs(_ context.Context, _ []string) ([]*models.AgentInstance, error) {
	return nil, nil
}

// withAgentCaller returns a router middleware that injects an AgentInstance
// into the request context under the "agent_caller" key.
func withAgentCaller(agent *models.AgentInstance) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("agent_caller", agent)
		c.Next()
	}
}

// -- Blockers --

func TestAddTaskBlocker_HappyPath(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "tA", "ws-b", "A", "todo", 2)
	insertTestTask(t, deps.db, "tB", "ws-b", "B", "todo", 2)

	body := `{"blocker_task_id":"tB"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/tA/blockers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}

func TestAddTaskBlocker_RejectsSelf(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "self", "ws-self", "Self", "todo", 2)

	body := `{"blocker_task_id":"self"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/self/blockers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestAddTaskBlocker_RejectsCrossWorkspace(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ta", "ws-1", "A", "todo", 2)
	insertTestTask(t, deps.db, "tb", "ws-2", "B", "todo", 2)

	body := `{"blocker_task_id":"tb"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/ta/blockers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestAddTaskBlocker_RejectsInverseCycle(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()
	insertTestTask(t, deps.db, "x", "ws-c", "X", "todo", 2)
	insertTestTask(t, deps.db, "y", "ws-c", "Y", "todo", 2)

	// y is already blocked by x; adding x→blocked-by-y must fail.
	if err := deps.repo.CreateTaskBlocker(ctx, &models.TaskBlocker{
		TaskID: "y", BlockerTaskID: "x",
	}); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	body := `{"blocker_task_id":"y"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/x/blockers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

// seedBlocker creates a (taskID blocked-by blockerTaskID) row directly
// via the repository, bypassing the service-level cycle check. Used to
// build pre-existing blocker chains in cycle-detection tests.
func seedBlocker(t *testing.T, deps *testDeps, taskID, blockerTaskID string) {
	t.Helper()
	if err := deps.repo.CreateTaskBlocker(context.Background(), &models.TaskBlocker{
		TaskID: taskID, BlockerTaskID: blockerTaskID,
	}); err != nil {
		t.Fatalf("seed blocker %s→%s: %v", taskID, blockerTaskID, err)
	}
}

// postBlocker issues POST /tasks/:id/blockers via the test router and
// returns the recorded response.
func postBlocker(t *testing.T, deps *testDeps, taskID, blockerTaskID string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"blocker_task_id":"` + blockerTaskID + `"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/"+taskID+"/blockers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	return w
}

// TestAddTaskBlocker_RejectsThreeNodeCycle exercises the BFS check.
// Existing rows: A blocks B, B blocks C. Adding C blocks A would close
// a 3-node cycle (C → A → B → C), which the walk must detect.
func TestAddTaskBlocker_RejectsThreeNodeCycle(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "tA", "ws-3", "A", "todo", 2)
	insertTestTask(t, deps.db, "tB", "ws-3", "B", "todo", 2)
	insertTestTask(t, deps.db, "tC", "ws-3", "C", "todo", 2)
	// A is blocked by B; B is blocked by C.
	seedBlocker(t, deps, "tA", "tB")
	seedBlocker(t, deps, "tB", "tC")

	w := postBlocker(t, deps, "tC", "tA")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error string   `json:"error"`
		Cycle []string `json:"cycle"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Cycle) < 3 {
		t.Fatalf("cycle = %v, want at least 3 entries", resp.Cycle)
	}
	if resp.Cycle[0] != resp.Cycle[len(resp.Cycle)-1] {
		t.Fatalf("cycle path must start and end at the same task: %v", resp.Cycle)
	}
	if !strings.Contains(resp.Error, "cycle") {
		t.Errorf("error = %q, want it to mention cycle", resp.Error)
	}
}

// TestAddTaskBlocker_RejectsFourNodeCycle is the same shape with one
// more node in the chain, ensuring the BFS doesn't terminate early.
func TestAddTaskBlocker_RejectsFourNodeCycle(t *testing.T) {
	deps := newTestDeps(t)
	for _, id := range []string{"n1", "n2", "n3", "n4"} {
		insertTestTask(t, deps.db, id, "ws-4", id, "todo", 2)
	}
	// n1 blocked by n2, n2 blocked by n3, n3 blocked by n4.
	seedBlocker(t, deps, "n1", "n2")
	seedBlocker(t, deps, "n2", "n3")
	seedBlocker(t, deps, "n3", "n4")

	// Adding n4 blocked-by n1 would close a 4-node cycle.
	w := postBlocker(t, deps, "n4", "n1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Cycle []string `json:"cycle"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Cycle) < 4 {
		t.Fatalf("cycle = %v, want at least 4 entries", resp.Cycle)
	}
}

// TestAddTaskBlocker_AcceptsDeepChainNoCycle ensures a 5-node linear
// chain that doesn't loop back is accepted (regression guard against
// the BFS over-rejecting).
func TestAddTaskBlocker_AcceptsDeepChainNoCycle(t *testing.T) {
	deps := newTestDeps(t)
	for _, id := range []string{"d1", "d2", "d3", "d4", "d5"} {
		insertTestTask(t, deps.db, id, "ws-d", id, "todo", 2)
	}
	// d1←d2←d3←d4 (linear).
	seedBlocker(t, deps, "d1", "d2")
	seedBlocker(t, deps, "d2", "d3")
	seedBlocker(t, deps, "d3", "d4")

	// d4 blocked-by d5 extends the chain — no cycle.
	w := postBlocker(t, deps, "d4", "d5")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}

// TestAddTaskBlocker_AcceptsParallelChain ensures unrelated parallel
// blocker chains in the same workspace don't trigger false positives.
func TestAddTaskBlocker_AcceptsParallelChain(t *testing.T) {
	deps := newTestDeps(t)
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		insertTestTask(t, deps.db, id, "ws-p", id, "todo", 2)
	}
	// p1←p2 chain already exists.
	seedBlocker(t, deps, "p1", "p2")

	// p3 blocked-by p4 is a separate, parallel edge.
	w := postBlocker(t, deps, "p3", "p4")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}

func TestRemoveTaskBlocker_HappyPath(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()
	insertTestTask(t, deps.db, "p", "ws-r", "P", "todo", 2)
	insertTestTask(t, deps.db, "q", "ws-r", "Q", "todo", 2)
	if err := deps.repo.CreateTaskBlocker(ctx, &models.TaskBlocker{
		TaskID: "p", BlockerTaskID: "q",
	}); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/office/tasks/p/blockers/q", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", w.Code, w.Body.String())
	}
}

// -- Participants (reviewers / approvers) --

// newParticipantTestRouter wires a router with an injected agent_caller
// middleware so handler.agentIDFromCtx returns the configured agent.
func newParticipantTestRouter(t *testing.T, caller *models.AgentInstance) *testDeps {
	t.Helper()
	deps := newTestDeps(t)
	deps.svc = nil // route via h.svc, but we already registered routes; rebuild:

	// Build a fresh router with the middleware in place.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if caller != nil {
		router.Use(withAgentCaller(caller))
	}
	group := router.Group("/api/v1/office")

	// Recreate svc with the existing repo so it shares the in-memory DB.
	// Use the underlying stubAgentReader replaced with caller-aware reader
	// only when caller is non-nil.
	deps.svc = newServiceForParticipantsTest(t, deps, caller)
	dashboard.RegisterRoutes(group, deps.svc, deps.repo, nil, logger.Default())
	deps.router = router
	return deps
}

func newServiceForParticipantsTest(t *testing.T, deps *testDeps, caller *models.AgentInstance) *dashboard.DashboardService {
	t.Helper()
	reader := &agentCallerStubReader{agent: caller}
	svc := dashboard.NewDashboardService(deps.repo, logger.Default(),
		stubActivityForParticipants{}, reader, &stubCostChecker{})
	return svc
}

// stubActivityForParticipants is a no-op LogActivity used by the
// participants tests so we don't need to wire the real activity log.
type stubActivityForParticipants struct{}

func (stubActivityForParticipants) LogActivity(
	_ context.Context,
	_, _, _, _, _, _, _ string,
) {
}

func (stubActivityForParticipants) LogActivityWithRun(
	_ context.Context,
	_, _, _, _, _, _, _, _, _ string,
) {
}

func TestAddTaskReviewer_HappyPath_NoCaller(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "rev1", "ws-rev", "R", "todo", 2)

	body := `{"agent_profile_id":"agent-1"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/rev1/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}

func TestAddTaskApprover_HappyPath_NoCaller(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "app1", "ws-app", "A", "todo", 2)

	body := `{"agent_profile_id":"agent-2"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/app1/approvers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}

func TestRemoveTaskReviewer_NonExistent_Idempotent(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "rmv1", "ws-rmv", "R", "todo", 2)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/office/tasks/rmv1/reviewers/never-added", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", w.Code, w.Body.String())
	}
}

func TestListTaskReviewers_AfterAdd(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "lst1", "ws-lst", "L", "todo", 2)
	addReviewer(t, deps.router, "lst1", "agent-x")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/tasks/lst1/reviewers", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		AgentProfileIDs []string `json:"agent_profile_ids"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.AgentProfileIDs) != 1 || resp.AgentProfileIDs[0] != "agent-x" {
		t.Fatalf("ids = %v, want [agent-x]", resp.AgentProfileIDs)
	}
}

func TestListTaskApprovers_AfterAdd(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "lst2", "ws-lst", "L2", "todo", 2)
	addApprover(t, deps.router, "lst2", "agent-y")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/tasks/lst2/approvers", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		AgentProfileIDs []string `json:"agent_profile_ids"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.AgentProfileIDs) != 1 || resp.AgentProfileIDs[0] != "agent-y" {
		t.Fatalf("ids = %v, want [agent-y]", resp.AgentProfileIDs)
	}
}

func TestGetTask_SurfacesParticipants(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "dt1", "ws-dt", "DT", "todo", 2)
	addReviewer(t, deps.router, "dt1", "rev-1")
	addApprover(t, deps.router, "dt1", "app-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/dt1", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp dashboard.TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Task.Reviewers) != 1 || resp.Task.Reviewers[0] != "rev-1" {
		t.Errorf("reviewers = %v, want [rev-1]", resp.Task.Reviewers)
	}
	if len(resp.Task.Approvers) != 1 || resp.Task.Approvers[0] != "app-1" {
		t.Errorf("approvers = %v, want [app-1]", resp.Task.Approvers)
	}
}

func TestAddTaskReviewer_Forbidden_WhenAgentLacksApprove(t *testing.T) {
	caller := &models.AgentInstance{
		ID:          "caller-1",
		Role:        models.AgentRoleWorker,
		Permissions: `{"can_approve":false}`,
	}
	deps := newParticipantTestRouter(t, caller)
	insertTestTask(t, deps.db, "fb1", "ws-fb", "F", "todo", 2)

	body := `{"agent_profile_id":"agent-z"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/fb1/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
}

func TestRemoveTaskApprover_Forbidden_WhenAgentLacksApprove(t *testing.T) {
	caller := &models.AgentInstance{
		ID:          "caller-2",
		Role:        models.AgentRoleWorker,
		Permissions: `{"can_approve":false}`,
	}
	deps := newParticipantTestRouter(t, caller)
	insertTestTask(t, deps.db, "fb2", "ws-fb2", "F2", "todo", 2)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/office/tasks/fb2/approvers/agent-z", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
}

// -- helpers --

func addReviewer(t *testing.T, router *gin.Engine, taskID, agentID string) {
	t.Helper()
	body := `{"agent_profile_id":"` + agentID + `"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/"+taskID+"/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("seed reviewer: status = %d: %s", w.Code, w.Body.String())
	}
}

func addApprover(t *testing.T, router *gin.Engine, taskID, agentID string) {
	t.Helper()
	body := `{"agent_profile_id":"` + agentID + `"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/"+taskID+"/approvers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("seed approver: status = %d: %s", w.Code, w.Body.String())
	}
}
