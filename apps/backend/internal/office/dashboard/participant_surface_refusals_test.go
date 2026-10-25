package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAddTaskParticipant_HTTPRejectsEmptyAgentProfileID covers
// AC-OFFICE-SEAT-ASSURANCE-002.4, pinning
// AC-OFFICE-SEAT-PROVENANCE-005.1.
//
// The registration surface refuses a request naming no agent before the
// service or the store is reached. The slate must be untouched: a seat
// naming nobody is admitted by the natural key, counted by the quorum
// guard, and can never be woken or decided, so the refusal is the whole
// protection at this layer.
//
// Both add endpoints are covered because each names its own role, so a
// guard added to one body and not the other would leave half the surface
// open.
func TestAddTaskParticipant_HTTPRejectsEmptyAgentProfileID(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		role string
	}{
		{name: "reviewer", path: "reviewers", role: "reviewer"},
		{name: "approver", path: "approvers", role: "approver"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps(t)
			insertTestTask(t, deps.db, "empty-agent", "ws-empty", "E", "todo", 1)
			if _, err := deps.db.Exec(
				`UPDATE tasks SET workflow_step_id = 'step-empty' WHERE id = 'empty-agent'`); err != nil {
				t.Fatalf("bind task to step: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/office/tasks/empty-agent/"+tc.path,
				strings.NewReader(`{"agent_profile_id":""}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			deps.router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}

			var seats int
			if err := deps.db.Get(&seats, `
				SELECT COUNT(*) FROM workflow_step_participants
				WHERE task_id = 'empty-agent' AND role = ?
			`, tc.role); err != nil {
				t.Fatalf("count seats: %v", err)
			}
			if seats != 0 {
				t.Fatalf("seats = %d, want 0 (a refused registration writes nothing)", seats)
			}
		})
	}
}
