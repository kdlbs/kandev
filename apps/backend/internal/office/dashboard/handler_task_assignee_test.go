package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// stubAssigneeWriter stands in for the task service, which owns both the
// caller authorization and the assignee validation. It records what it was
// asked and can reject, so the handler test covers the refusal path without
// building a workspace, a membership table and an authenticated session.
type stubAssigneeWriter struct {
	gotTaskID string
	gotUserID string
	reject    error
	apply     func(taskID, userID string)
}

func (s *stubAssigneeWriter) SetHumanAssignee(_ context.Context, taskID, userID string) error {
	s.gotTaskID, s.gotUserID = taskID, userID
	if s.reject != nil {
		return s.reject
	}
	if s.apply != nil {
		s.apply(taskID, userID)
	}
	return nil
}

func patchTask(t *testing.T, deps *testDeps, taskID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/office/tasks/"+taskID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	return w
}

func taskAssigneeUserID(t *testing.T, deps *testDeps, taskID string) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/"+taskID, nil)
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get task: status = %d body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Task struct {
			AssigneeUserID string `json:"assigneeUserId"`
		} `json:"task"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	return body.Task.AssigneeUserID
}

// TestUpdateTask_SetsHumanAssignee covers the round trip: PATCH stores the
// human assignee, the resolver is consulted, and GET returns it.
func TestUpdateTask_SetsHumanAssignee(t *testing.T) {
	deps := newTestDeps(t)
	writer := &stubAssigneeWriter{apply: func(taskID, userID string) {
		// Stand in for the task service's persistence so the GET assertions
		// below observe a real column write.
		if _, err := deps.db.Exec(`UPDATE tasks SET assignee_user_id = ? WHERE id = ?`, userID, taskID); err != nil {
			t.Fatalf("stub persist: %v", err)
		}
	}}
	deps.svc.SetHumanAssigneeWriter(writer)
	insertTestTask(t, deps.db, "ha1", "ws-d", "Assignable", "todo", 2)

	if w := patchTask(t, deps, "ha1", `{"assignee_user_id":"user-42"}`); w.Code != http.StatusOK {
		t.Fatalf("patch: status = %d body = %s", w.Code, w.Body.String())
	}
	if writer.gotTaskID != "ha1" || writer.gotUserID != "user-42" {
		t.Fatalf("writer saw (%q, %q), want (ha1, user-42)", writer.gotTaskID, writer.gotUserID)
	}
	if got := taskAssigneeUserID(t, deps, "ha1"); got != "user-42" {
		t.Fatalf("assignee = %q, want user-42", got)
	}

	// Empty string is the documented "unassign" value, distinct from an
	// omitted field, which must leave the assignee untouched.
	if w := patchTask(t, deps, "ha1", `{"priority":"high"}`); w.Code != http.StatusOK {
		t.Fatalf("patch priority: status = %d body = %s", w.Code, w.Body.String())
	}
	if got := taskAssigneeUserID(t, deps, "ha1"); got != "user-42" {
		t.Fatalf("omitted field cleared the assignee: got %q", got)
	}
	if w := patchTask(t, deps, "ha1", `{"assignee_user_id":""}`); w.Code != http.StatusOK {
		t.Fatalf("patch unassign: status = %d body = %s", w.Code, w.Body.String())
	}
	if got := taskAssigneeUserID(t, deps, "ha1"); got != "" {
		t.Fatalf("assignee = %q after unassign, want empty", got)
	}
}

// TestUpdateTask_UnreachableAssigneeIs400 covers the one rejection a normal
// user can trigger by accident: the picker offers the whole user directory
// because reach cannot be computed in the browser, so picking someone outside
// the workspace must read as a bad request with a showable message, not a 500.
func TestUpdateTask_UnreachableAssigneeIs400(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetHumanAssigneeWriter(&stubAssigneeWriter{reject: taskservice.ErrAssigneeCannotReachWorkspace})
	insertTestTask(t, deps.db, "ha5", "ws-d", "Assignable", "todo", 2)

	w := patchTask(t, deps, "ha5", `{"assignee_user_id":"outsider"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want 400", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cannot see this workspace") {
		t.Fatalf("body = %s, want the reason to be showable", w.Body.String())
	}
}

// TestUpdateTask_HumanAssigneeNotFoundIsNot500 pins the other half of the
// 404-vs-403 rule: a caller who cannot see the task at all must not learn it
// exists, and must not be told the server broke either.
func TestUpdateTask_HumanAssigneeNotFoundIsNot500(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetHumanAssigneeWriter(&stubAssigneeWriter{reject: repoerrors.ErrTaskNotFound})
	insertTestTask(t, deps.db, "ha4", "ws-d", "Assignable", "todo", 2)

	w := patchTask(t, deps, "ha4", `{"assignee_user_id":"user-42"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want 404", w.Code, w.Body.String())
	}
}

// TestUpdateTask_HumanAssigneeRejectedByWriter is the reach rule surfacing
// through the office endpoint: a user who cannot see the workspace is not a
// valid assignee, and the write must not land.
func TestUpdateTask_HumanAssigneeRejectedByWriter(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetHumanAssigneeWriter(&stubAssigneeWriter{reject: taskservice.ErrForbidden})
	insertTestTask(t, deps.db, "ha2", "ws-d", "Assignable", "todo", 2)

	w := patchTask(t, deps, "ha2", `{"assignee_user_id":"outsider"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s, want 403", w.Code, w.Body.String())
	}
	if got := taskAssigneeUserID(t, deps, "ha2"); got != "" {
		t.Fatalf("assignee = %q after a rejected write, want empty", got)
	}
}

// TestUpdateTask_HumanAssigneeWithoutWriterIsRefused pins the fail-closed
// contract. It matters more here than on the sibling fields: PATCH
// /office/tasks/:id carries no :wsId, so the office workspace-scope middleware
// does not gate it, and writing the column directly would let any signed-in
// user assign a task in a workspace they cannot reach.
func TestUpdateTask_HumanAssigneeWithoutWriterIsRefused(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ha3", "ws-d", "Assignable", "todo", 2)

	w := patchTask(t, deps, "ha3", `{"assignee_user_id":"user-42"}`)
	if w.Code == http.StatusOK {
		t.Fatalf("unvalidated assignment returned 200: %s", w.Body.String())
	}
	if got := taskAssigneeUserID(t, deps, "ha3"); got != "" {
		t.Fatalf("assignee = %q with no writer wired, want empty", got)
	}
}
