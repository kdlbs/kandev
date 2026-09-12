package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// recordingTerminalShapeRecorder captures RecordCancelledRunTerminalShapes
// calls so tests can verify a displaced-participant cancellation reaches
// office_loop_terminal_total's classification path.
type recordingTerminalShapeRecorder struct {
	calls [][]runssqlite.CancelledRun
}

func (r *recordingTerminalShapeRecorder) RecordCancelledRunTerminalShapes(
	_ context.Context, cancelled []runssqlite.CancelledRun,
) {
	r.calls = append(r.calls, cancelled)
}

var _ dashboard.TerminalShapeRecorder = (*recordingTerminalShapeRecorder)(nil)

// TestAddTaskReviewer_ClaimingAutoSeatRecordsDisplacedRunTerminalShape is
// the Testing round 3 regression test. cancelDisplacedRun calls
// repo.CancelDisplacedParticipantRun directly, which bypassed
// office_loop_terminal_total entirely — the same bypass class Review
// round 1 fixed for the single-run CancelRun call sites. The setup mirrors
// TestAddTaskReviewer_ClaimingAutoSeatCancelsDisplacedRunAndLogsActivity;
// this test additionally asserts the terminal-shape recorder is invoked
// with the displaced run's cancelled row.
func TestAddTaskReviewer_ClaimingAutoSeatRecordsDisplacedRunTerminalShape(t *testing.T) {
	deps := newTestDeps(t)
	recorder := &recordingTerminalShapeRecorder{}
	deps.svc.SetTerminalShapeRecorder(recorder)
	insertTestTask(t, deps.db, "claim-shape1", "ws-claim-shape", "C", "todo", 2)
	stepID := "step-claim-shape1"

	if _, err := deps.db.Exec(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-claiming', '', 'agent-claiming', 'agent-claiming', 'ws-claim-shape', '', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed claiming agent profile: %v", err)
	}

	if _, err := deps.db.Exec(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES ('auto-seat-claim-shape1', ?, 'claim-shape1', 'reviewer', 'agent-auto', 1, 0, 'auto')
	`, stepID); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}

	run := &models.Run{
		AgentProfileID: "agent-auto",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"claim-shape1","workflow_step_id":"` + stepID + `"}`,
		Status:         models.RunStatusQueued,
		CoalescedCount: 1,
	}
	if err := deps.repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("seed displaced run: %v", err)
	}

	body := `{"agent_profile_id":"agent-claiming"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/claim-shape1/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("terminal shape recorder calls = %d, want 1", len(recorder.calls))
	}
	if len(recorder.calls[0]) != 1 || recorder.calls[0][0].ID != run.ID {
		t.Fatalf("recorded cancelled rows = %+v, want [%s]", recorder.calls[0], run.ID)
	}
}

// TestSetTerminalShapeRecorder_NilTolerated keeps the wiring optional: when
// no recorder is registered, the displaced-run cancellation itself must
// still succeed (dashboard tests run without the office service wired in).
func TestSetTerminalShapeRecorder_NilTolerated(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "claim-shape-nil", "ws-claim-shape-nil", "C", "todo", 2)
	stepID := "step-claim-shape-nil"

	if _, err := deps.db.Exec(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-claiming', '', 'agent-claiming', 'agent-claiming', 'ws-claim-shape-nil', '', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed claiming agent profile: %v", err)
	}
	if _, err := deps.db.Exec(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES ('auto-seat-claim-shape-nil', ?, 'claim-shape-nil', 'reviewer', 'agent-auto', 1, 0, 'auto')
	`, stepID); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}
	run := &models.Run{
		AgentProfileID: "agent-auto",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"claim-shape-nil","workflow_step_id":"` + stepID + `"}`,
		Status:         models.RunStatusQueued,
		CoalescedCount: 1,
	}
	if err := deps.repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("seed displaced run: %v", err)
	}

	body := `{"agent_profile_id":"agent-claiming"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/claim-shape-nil/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	gotRun, err := deps.repo.GetRunByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get displaced run: %v", err)
	}
	if gotRun.Status != "cancelled" {
		t.Fatalf("displaced run status = %q, want cancelled", gotRun.Status)
	}
}
