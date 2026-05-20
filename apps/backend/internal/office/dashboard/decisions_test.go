package dashboard_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// sqlxExecutor is the subset of *sqlx.DB used by helpers in this
// file. Defined here so the helpers don't import sqlx directly.
type sqlxExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// stubApprovalQueuer captures runs queued via the approval-flow
// reactivity hook so tests can assert on them without wiring the real
// scheduler.
type stubApprovalQueuer struct {
	runs []dashboard.ApprovalRun
}

func (q *stubApprovalQueuer) QueueApprovalRuns(
	_ context.Context, w []dashboard.ApprovalRun,
) error {
	q.runs = append(q.runs, w...)
	return nil
}

// TestApproveTask_SecondDecisionSupersedes verifies the workflow
// store's supersede semantics survive end-to-end through the dashboard
// service: a second decision by the same (task, decider, role) leaves
// only one active row in the listing (ADR 0005 Wave E).
func TestApproveTask_SecondDecisionSupersedes(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ss1", "ws-d", "S", "in_review", 2)
	mustAddParticipant(t, deps, "ss1", "agent-1", models.ParticipantRoleApprover)

	first, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-1", "ss1", "first")
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}
	second, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeAgent, "agent-1", "ss1", "fix it")
	if err != nil {
		t.Fatalf("second decision: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected distinct decision ids, got %s twice", first.ID)
	}
	rows, err := deps.svc.ListTaskDecisions(context.Background(), "ss1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("active decisions = %d, want 1 (first should be superseded)", len(rows))
	}
	if rows[0].ID != second.ID {
		t.Errorf("active = %s, want %s", rows[0].ID, second.ID)
	}
	if rows[0].Decision != models.DecisionChangesRequested {
		t.Errorf("active.Decision = %s, want %s",
			rows[0].Decision, models.DecisionChangesRequested)
	}
}

// TestApproveTask_HappyPath records an approved decision when the
// caller is in the participants list.
func TestApproveTask_HappyPath(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ap1", "ws-d", "A", "in_progress", 2)
	mustAddParticipant(t, deps, "ap1", "agent-1", models.ParticipantRoleApprover)

	d, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-1", "ap1", "lgtm")
	if err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	if d == nil || d.Decision != models.DecisionApproved || d.Role != models.ParticipantRoleApprover {
		t.Fatalf("decision = %+v", d)
	}
	rows, _ := deps.svc.ListTaskDecisions(context.Background(), "ap1")
	if len(rows) != 1 {
		t.Fatalf("decisions = %d, want 1", len(rows))
	}
}

// TestRequestTaskChanges_RequiresComment rejects empty comments.
func TestRequestTaskChanges_RequiresComment(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "rc1", "ws-d", "R", "in_review", 2)
	mustAddParticipant(t, deps, "rc1", "agent-2", models.ParticipantRoleReviewer)

	_, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeAgent, "agent-2", "rc1", "")
	if err == nil {
		t.Fatal("expected error for empty comment")
	}
}

// TestApproveTask_ForbiddenWhenNotParticipant returns ErrForbidden
// for an agent caller that isn't in reviewers/approvers.
func TestApproveTask_ForbiddenWhenNotParticipant(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "fb1", "ws-d", "F", "in_review", 2)

	_, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-zzz", "fb1", "")
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

// TestApproveTask_PrefersApproverRole when caller holds both roles
// the recorded decision is keyed approver.
func TestApproveTask_PrefersApproverRole(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "both1", "ws-d", "B", "in_review", 2)
	mustAddParticipant(t, deps, "both1", "agent-X", models.ParticipantRoleReviewer)
	mustAddParticipant(t, deps, "both1", "agent-X", models.ParticipantRoleApprover)

	d, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-X", "both1", "")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if d.Role != models.ParticipantRoleApprover {
		t.Fatalf("role = %s, want approver", d.Role)
	}
}

// TestRequestTaskChanges_QueuesAssigneeRun verifies the changes_
// requested decision queues a task_changes_requested run carrying
// the comment for the assignee.
func TestRequestTaskChanges_QueuesAssigneeRun(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskWithAssignee(t, deps.db, "ch1", "ws-d", "C", "in_review", 2, "asg-1")
	mustAddParticipant(t, deps, "ch1", "agent-rev", models.ParticipantRoleReviewer)

	q := &stubApprovalQueuer{}
	deps.svc.SetApprovalReactivityQueuer(q)

	_, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeAgent, "agent-rev", "ch1", "tighten the diff")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(q.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(q.runs))
	}
	got := q.runs[0]
	if got.AgentID != "asg-1" || got.Reason != "task_changes_requested" {
		t.Errorf("run = %+v", got)
	}
	if got.DecisionComment != "tighten the diff" {
		t.Errorf("comment lost: %q", got.DecisionComment)
	}
}

// TestApproveTask_QueuesReadyToCloseOnFinalApproval — when the last
// approver approves and the task is in_review, the assignee wakes
// with task_ready_to_close.
func TestApproveTask_QueuesReadyToCloseOnFinalApproval(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskWithAssignee(t, deps.db, "rt1", "ws-d", "R", "in_review", 2, "asg-2")
	mustAddParticipant(t, deps, "rt1", "agent-A", models.ParticipantRoleApprover)
	mustAddParticipant(t, deps, "rt1", "agent-B", models.ParticipantRoleApprover)
	q := &stubApprovalQueuer{}
	deps.svc.SetApprovalReactivityQueuer(q)

	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-A", "rt1", ""); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	// One approver still pending → no run yet.
	if len(q.runs) != 0 {
		t.Fatalf("expected no run yet, got %v", q.runs)
	}
	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-B", "rt1", ""); err != nil {
		t.Fatalf("second approve: %v", err)
	}
	if len(q.runs) != 1 {
		t.Fatalf("runs = %d, want 1: %#v", len(q.runs), q.runs)
	}
	if q.runs[0].Reason != "task_ready_to_close" {
		t.Fatalf("reason = %s", q.runs[0].Reason)
	}
}

// -- Status gate (B3) --

// TestUpdateTaskStatus_NoApprovers_NoGate transitions to done freely
// when the task has no approvers configured.
func TestUpdateTaskStatus_NoApprovers_NoGate(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "gn1", "ws-g", "GN", "in_review", 2)

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "gn1",
		NewStatus: "done",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
}

// TestUpdateTaskStatus_GatedWhenApproverPending returns ApprovalsPendingError
// and redirects the persisted state to in_review (REVIEW).
func TestUpdateTaskStatus_GatedWhenApproverPending(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "gp1", "ws-g", "GP", "in_progress", 2)
	mustAddParticipant(t, deps, "gp1", "agent-A", models.ParticipantRoleApprover)

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "gp1",
		NewStatus: "done",
	})
	var pending *dashboard.ApprovalsPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("err = %v, want *ApprovalsPendingError", err)
	}
	if len(pending.Pending) != 1 || pending.Pending[0] != "agent-A" {
		t.Fatalf("pending = %v", pending.Pending)
	}
	// The persisted state should be REVIEW (gated redirect).
	state := readTaskState(t, deps, "gp1")
	if state != "REVIEW" {
		t.Errorf("state = %q, want REVIEW", state)
	}
}

// TestUpdateTaskStatus_GatedWithPartialApprovals — 2 approvers, 1
// approved, 1 pending → 409.
func TestUpdateTaskStatus_GatedWithPartialApprovals(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "gh1", "ws-g", "GH", "in_review", 2)
	mustAddParticipant(t, deps, "gh1", "agent-A", models.ParticipantRoleApprover)
	mustAddParticipant(t, deps, "gh1", "agent-B", models.ParticipantRoleApprover)
	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-A", "gh1", ""); err != nil {
		t.Fatalf("approve a: %v", err)
	}

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "gh1",
		NewStatus: "done",
	})
	var pending *dashboard.ApprovalsPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("err = %v", err)
	}
	if len(pending.Pending) != 1 || pending.Pending[0] != "agent-B" {
		t.Fatalf("pending = %v", pending.Pending)
	}
}

// TestUpdateTaskStatus_AllApproved transitions to done normally.
func TestUpdateTaskStatus_AllApproved(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ga1", "ws-g", "GA", "in_review", 2)
	mustAddParticipant(t, deps, "ga1", "agent-A", models.ParticipantRoleApprover)
	mustAddParticipant(t, deps, "ga1", "agent-B", models.ParticipantRoleApprover)
	for _, a := range []string{"agent-A", "agent-B"} {
		if _, err := deps.svc.ApproveTask(context.Background(),
			models.DeciderTypeAgent, a, "ga1", ""); err != nil {
			t.Fatalf("approve %s: %v", a, err)
		}
	}

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "ga1", NewStatus: "done",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if state := readTaskState(t, deps, "ga1"); state != "COMPLETED" {
		t.Errorf("state = %q, want COMPLETED", state)
	}
}

// TestUpdateTaskStatus_ReworkSupersedes — moving in_review → todo
// supersedes prior decisions.
func TestUpdateTaskStatus_ReworkSupersedes(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "rw1", "ws-g", "RW", "REVIEW", 2)
	mustAddParticipant(t, deps, "rw1", "agent-A", models.ParticipantRoleApprover)
	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-A", "rw1", ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "rw1", NewStatus: "todo",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	rows, _ := deps.svc.ListTaskDecisions(context.Background(), "rw1")
	if len(rows) != 0 {
		t.Fatalf("active decisions after rework = %d, want 0", len(rows))
	}
}

// TestUpdateTaskStatus_ReopenSupersedes — moving done → todo also clears.
func TestUpdateTaskStatus_ReopenSupersedes(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ro1", "ws-g", "RO", "COMPLETED", 2)
	mustAddParticipant(t, deps, "ro1", "agent-A", models.ParticipantRoleApprover)
	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-A", "ro1", ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "ro1", NewStatus: "todo",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	rows, _ := deps.svc.ListTaskDecisions(context.Background(), "ro1")
	if len(rows) != 0 {
		t.Fatalf("active decisions after reopen = %d", len(rows))
	}
}

// -- Endpoint tests (B4) --

func TestApproveTaskEndpoint_201(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ep1", "ws-d", "E", "in_review", 2)
	addApprover(t, deps.router, "ep1", "user")

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/ep1/approve",
		strings.NewReader(`{"comment":"ok"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestRequestChangesEndpoint_400OnEmpty(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ep2", "ws-d", "E", "in_review", 2)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/ep2/request-changes",
		strings.NewReader(`{"comment":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestListTaskDecisionsEndpoint(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ep3", "ws-d", "E", "in_review", 2)
	addApprover(t, deps.router, "ep3", "user")
	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeUser, "user", "ep3", ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/tasks/ep3/decisions", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp dashboard.DecisionListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(resp.Decisions))
	}
}

func TestUpdateTaskEndpoint_409OnApprovalGate(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ep4", "ws-d", "E", "in_progress", 2)
	addApprover(t, deps.router, "ep4", "agent-A")

	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/office/tasks/ep4",
		strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "in_review" {
		t.Errorf("response status = %v, want in_review", body["status"])
	}
}

// TestUpdateTaskEndpoint_409PendingApproversIncludesNames verifies that the
// resolvePendingApprovers helper enriches the bare profile IDs with the
// agent's display name so the frontend toast renders "Cannot mark done:
// awaiting approval from <names>" instead of bare UUIDs.
func TestUpdateTaskEndpoint_409PendingApproversIncludesNames(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ep5", "ws-d", "E", "in_progress", 2)
	addApprover(t, deps.router, "ep5", "agent-named")
	addApprover(t, deps.router, "ep5", "agent-unknown")
	deps.agents.names = map[string]string{"agent-named": "Reviewer Alice"}
	// agent-unknown intentionally omitted: stub returns nil → fallback to ID

	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/office/tasks/ep5",
		strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	approvers, ok := body["pending_approvers"].([]interface{})
	if !ok {
		t.Fatalf("pending_approvers missing or wrong type: %T", body["pending_approvers"])
	}
	if len(approvers) != 2 {
		t.Fatalf("approvers = %d, want 2", len(approvers))
	}
	byID := make(map[string]string, len(approvers))
	for _, raw := range approvers {
		row, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("approver entry wrong shape: %T", raw)
		}
		id, _ := row["agent_profile_id"].(string)
		name, _ := row["name"].(string)
		if id == "" {
			t.Errorf("approver missing agent_profile_id: %v", row)
		}
		byID[id] = name
	}
	if got := byID["agent-named"]; got != "Reviewer Alice" {
		t.Errorf("agent-named name = %q, want %q", got, "Reviewer Alice")
	}
	// fallback path: lookup returned nil → name defaults to the bare ID
	if got := byID["agent-unknown"]; got != "agent-unknown" {
		t.Errorf("agent-unknown name = %q, want fallback %q", got, "agent-unknown")
	}
}

// -- Inbox (B6) --

func TestInbox_TaskReviewRequest_ForUser(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ib1", "ws-i", "Title", "in_review", 2)
	insertTestTask(t, deps.db, "ib2", "ws-i", "Other", "in_review", 2)
	addApprover(t, deps.router, "ib1", "agent-A")
	// ib2: no participants → no inbox row

	items, err := deps.svc.GetInboxItems(context.Background(), "ws-i")
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	count := 0
	for _, it := range items {
		if it.Type == "task_review_request" && it.EntityID == "ib1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("review request items for ib1 = %d, want 1", count)
	}
}

func TestInbox_TaskReviewRequest_AgentScoped(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ai1", "ws-i", "T", "in_review", 2)
	addApprover(t, deps.router, "ai1", "agent-X")

	items, err := deps.svc.GetAgentInboxItems(context.Background(), "ws-i", "agent-X")
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	items2, _ := deps.svc.GetAgentInboxItems(context.Background(), "ws-i", "agent-OTHER")
	if len(items2) != 0 {
		t.Fatalf("other agent items = %d, want 0", len(items2))
	}
}

// -- helpers --

// mustAddParticipant inserts a participant row directly via the repo.
func mustAddParticipant(t *testing.T, deps *testDeps, taskID, agentID, role string) {
	t.Helper()
	if err := deps.repo.AddTaskParticipant(context.Background(), taskID, agentID, role); err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
}

// insertTestTaskWithAssignee inserts a task row plus a 'runner'
// participant in workflow_step_participants so the runner-projection
// returns the assignee. ADR 0005 Wave F: tasks no longer carry an
// assignee column directly.
func insertTestTaskWithAssignee(t *testing.T, db sqlxExecutor, id, wsID, title, state string, priority int, assignee string) {
	t.Helper()
	stepID := "step-" + id
	_, err := db.Exec(`
		INSERT INTO tasks (
			id, workspace_id, title, state, priority, identifier,
			workflow_step_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, title, state, intPriorityLabel(priority), id, stepID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if assignee == "" {
		return
	}
	if _, err := db.Exec(`
		INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', ?, 0, 0)
	`, "p-runner-"+id, stepID, id, assignee); err != nil {
		t.Fatalf("insert runner participant: %v", err)
	}
}

// readTaskState pulls the current state column for a task.
func readTaskState(t *testing.T, deps *testDeps, taskID string) string {
	t.Helper()
	var state string
	if err := deps.db.QueryRow(`SELECT state FROM tasks WHERE id = ?`, taskID).Scan(&state); err != nil {
		t.Fatalf("read state: %v", err)
	}
	return state
}
