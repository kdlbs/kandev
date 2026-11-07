package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

type stubControlCapacity struct {
	observation sessioncapacity.Observation
}

func (s stubControlCapacity) SessionCapacityObservation(context.Context) sessioncapacity.Observation {
	return s.observation
}

func controlDashboardSchema(t *testing.T, deps *testDeps) {
	t.Helper()
	stmts := []string{
		`ALTER TABLE tasks ADD COLUMN wip_admitted INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE tasks ADD COLUMN queued_for_step_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE task_sessions ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS queued_messages (id TEXT PRIMARY KEY, task_id TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS task_step_transitions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			to_workflow_step_id TEXT NOT NULL DEFAULT '',
			occurred_at TIMESTAMP NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := deps.db.Exec(stmt); err != nil {
			t.Fatalf("control dashboard schema: %v", err)
		}
	}
}

func readControlDashboard(t *testing.T, deps *testDeps, workspaceID string) sqlite.ControlMetrics {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/office/workspaces/"+workspaceID+"/dashboard?view=control",
		nil,
	)
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("control dashboard status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var metrics sqlite.ControlMetrics
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("decode control dashboard: %v", err)
	}
	return metrics
}

func TestControlDashboardAddsWipAndSharedCapacityWaiting(t *testing.T) {
	deps := newTestDeps(t)
	controlDashboardSchema(t, deps)
	if _, err := deps.db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, state, workflow_step_id, queued_for_step_id, wip_admitted)
		VALUES
			('t-queued', 'ws-1', 'queued', 'IN_PROGRESS', 'step-wip', 'step-wip', 0),
			('t-admitted', 'ws-1', 'admitted', 'IN_PROGRESS', 'step-wip', 'step-wip', 1)
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	deps.svc.SetControlCapacitySource(stubControlCapacity{observation: sessioncapacity.Observation{
		Waiting: sessioncapacity.WaitingObservation{Known: true, Counts: map[string]int{
			sessioncapacity.WaitingReasonTotalLimit:         1,
			sessioncapacity.WaitingReasonGradeLimit:         2,
			sessioncapacity.WaitingReasonBothLimits:         1,
			sessioncapacity.WaitingReasonNeedsConfiguration: 1,
			sessioncapacity.WaitingReasonUnknownPopulation:  0,
		}},
	}})

	metrics := readControlDashboard(t, deps, "ws-1")
	if metrics.WipWaitingTasks != 1 {
		t.Fatalf("WipWaitingTasks = %d, want 1", metrics.WipWaitingTasks)
	}
	waiting := metrics.SharedCapacityWaiting
	if waiting == nil || !waiting.Known {
		t.Fatalf("SharedCapacityWaiting = %#v, want known", waiting)
	}
	if waiting.Grade != 3 || waiting.Total != 5 {
		t.Fatalf("SharedCapacityWaiting = %#v, want grade 3 total 5", waiting)
	}
}

func TestControlDashboardOmitsSharedCapacityWithoutASource(t *testing.T) {
	deps := newTestDeps(t)
	controlDashboardSchema(t, deps)

	metrics := readControlDashboard(t, deps, "ws-1")
	if metrics.SharedCapacityWaiting != nil {
		t.Fatalf("SharedCapacityWaiting = %#v, want omitted", metrics.SharedCapacityWaiting)
	}
}

func TestControlDashboardReportsUnknownSharedCapacity(t *testing.T) {
	deps := newTestDeps(t)
	controlDashboardSchema(t, deps)
	deps.svc.SetControlCapacitySource(stubControlCapacity{observation: sessioncapacity.Observation{
		Waiting: sessioncapacity.WaitingObservation{Known: false},
	}})

	metrics := readControlDashboard(t, deps, "ws-1")
	waiting := metrics.SharedCapacityWaiting
	if waiting == nil || waiting.Known || waiting.Grade != 0 || waiting.Total != 0 {
		t.Fatalf("SharedCapacityWaiting = %#v, want unknown with no counts", waiting)
	}
}
