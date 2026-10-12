package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
)

// TestAddTaskReviewer_ClaimSucceedsWhenDisplacedRunCancellationFails covers
// AC-OFFICE-SEAT-ASSURANCE-002.7, pinning the second sentence of
// AC-OFFICE-SEAT-PROVENANCE-006.6.
//
// That sentence is the only place the contract accepts a residual: the claim
// has already committed, the displaced agent's queued run cannot be
// cancelled, and the registration must still report success rather than
// failing an operator's action over cleanup it cannot undo. What it leaves
// behind is one run still runnable for an agent no longer seated in the
// role, and a recorded failure so that is visible rather than silent.
//
// The runs table is renamed aside for the duration of the call rather than
// dropped: after the restore the run is still there and still runnable,
// which is the residual itself. A dropped table would take the evidence with
// it. The rename touches only this test's own isolated in-memory database.
func TestAddTaskReviewer_ClaimSucceedsWhenDisplacedRunCancellationFails(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build observed logger: %v", err)
	}

	deps := newTestDepsWithLogger(t, log)
	insertTestTask(t, deps.db, "cancelfail", "ws-cancelfail", "C", "todo", 2)
	const stepID = "step-cancelfail"

	if _, err := deps.db.Exec(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-claiming', '', 'agent-claiming', 'agent-claiming', 'ws-cancelfail', '', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed claiming agent profile: %v", err)
	}
	if _, err := deps.db.Exec(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES ('auto-seat-cancelfail', ?, 'cancelfail', 'reviewer', 'agent-auto', 1, 0, 'auto')
	`, stepID); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}

	// The run the step-entry fan-out queued for the agent about to be
	// displaced. Seeded before the table goes away.
	run := &models.Run{
		AgentProfileID: "agent-auto",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"cancelfail","workflow_step_id":"` + stepID + `"}`,
		Status:         models.RunStatusQueued,
		CoalescedCount: 1,
	}
	if err := deps.repo.CreateRun(t.Context(), run); err != nil {
		t.Fatalf("seed displaced run: %v", err)
	}

	// Take the runs store away for the duration of the registration, and
	// put it back whatever happens.
	if _, err := deps.db.Exec(`ALTER TABLE runs RENAME TO runs_stashed`); err != nil {
		t.Fatalf("stash runs table: %v", err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		if _, err := deps.db.Exec(`ALTER TABLE runs_stashed RENAME TO runs`); err != nil {
			t.Fatalf("restore runs table: %v", err)
		}
	}
	t.Cleanup(restore)

	body := `{"agent_profile_id":"agent-claiming"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/cancelfail/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	restore()

	// 1. The request succeeds: the claim committed, and cleanup it cannot
	//    complete must not be reported to the operator as a failed action.
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (cancellation is best-effort): %s", w.Code, w.Body.String())
	}

	// 2. The claimed slate is exactly as a successful claim leaves it.
	var seats int
	if err := deps.db.Get(&seats,
		`SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = 'cancelfail' AND role = 'reviewer'`,
	); err != nil {
		t.Fatalf("count seats: %v", err)
	}
	if seats != 1 {
		t.Fatalf("seats = %d, want 1 (claimed in place)", seats)
	}
	var agentID, provenance string
	if err := deps.db.QueryRow(
		`SELECT agent_profile_id, provenance FROM workflow_step_participants WHERE id = 'auto-seat-cancelfail'`,
	).Scan(&agentID, &provenance); err != nil {
		t.Fatalf("read claimed seat: %v", err)
	}
	if agentID != "agent-claiming" || provenance != "manual" {
		t.Fatalf("seat = (agent=%q provenance=%q), want (agent-claiming, manual)", agentID, provenance)
	}

	// 3. The failure is recorded. Asserted on fields, not message text.
	entries := logs.FilterFieldKey("agent_profile_id").All()
	found := false
	for _, e := range entries {
		fields := e.ContextMap()
		if fields["task_id"] == "cancelfail" &&
			fields["step_id"] == stepID &&
			fields["agent_profile_id"] == "agent-auto" &&
			fields["error"] != nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no warning naming the task, step and displaced agent with an error; observed: %+v",
			logs.All())
	}

	// 4. The displaced agent's run is still runnable — the residual the
	//    criterion describes, and the reason the table was renamed rather
	//    than dropped.
	gotRun, err := deps.repo.GetRunByID(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("get displaced run: %v", err)
	}
	if gotRun.Status != models.RunStatusQueued {
		t.Fatalf("displaced run status = %q, want %q (still runnable)", gotRun.Status, models.RunStatusQueued)
	}
}
