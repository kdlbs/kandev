package routines_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// noopActivity implements shared.ActivityLogger for tests.
type noopActivity struct{}

func (n *noopActivity) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (n *noopActivity) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// newTestRoutineService creates a RoutineService backed by in-memory SQLite.
func newTestRoutineService(t *testing.T) *routines.RoutineService {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	return routines.NewRoutineService(repo, log, &noopActivity{})
}

// createTestRoutine creates a test routine in the DB.
func createTestRoutine(t *testing.T, svc interface {
	CreateRoutine(ctx context.Context, r *models.Routine) error
}, name, policy string) *models.Routine {
	t.Helper()
	r := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              name,
		TaskTemplate:      `{"title":"{{name}} - {{date}}","description":"Run for {{date}}"}`,
		Status:            "active",
		ConcurrencyPolicy: policy,
		Variables:         `{"name":{"default":"Daily Check"}}`,
	}
	if err := svc.CreateRoutine(context.Background(), r); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return r
}

func TestFireManual_Basic(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Manual Test", "always_create")
	run, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "Security Scan"})
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.Status != "task_created" {
		t.Errorf("status = %q, want task_created", run.Status)
	}
	if run.Source != "manual" {
		t.Errorf("source = %q, want manual", run.Source)
	}
	if run.DispatchFingerprint == "" {
		t.Error("expected fingerprint to be set")
	}
}

func TestDispatch_SkipIfActive(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Skip Test", "skip_if_active")

	// First run should succeed.
	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	// Second run with same fingerprint should be skipped.
	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "skipped" {
		t.Errorf("second run status = %q, want skipped", run2.Status)
	}
}

func TestDispatch_CoalesceIfActive(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Coalesce Test", "coalesce_if_active")

	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "coalesced" {
		t.Errorf("second run status = %q, want coalesced", run2.Status)
	}
	if run2.CoalescedIntoRunID != run1.ID {
		t.Errorf("coalesced_into = %q, want %q", run2.CoalescedIntoRunID, run1.ID)
	}
}

func TestDispatch_AlwaysCreate(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Always Test", "always_create")

	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if run1.Status != "task_created" || run2.Status != "task_created" {
		t.Errorf("both runs should be task_created, got %q and %q", run1.Status, run2.Status)
	}
}

func TestDispatch_DifferentVarsNotSkipped(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Diff Vars", "skip_if_active")

	run1, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "A"})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	// Different variable -> different fingerprint -> not skipped.
	run2, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "B"})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "task_created" {
		t.Errorf("second run with different vars should be task_created, got %q", run2.Status)
	}
}

// -- PR 3 (heavy / lightweight materialisation) tests --

// fakeWakeupEnqueuer captures wakeup-request enqueue + dispatch calls
// so the lightweight-path test can assert on the payload + idempotency
// key the routines service produced.
type fakeWakeupEnqueuer struct {
	created    []*routines.WakeupRequest
	dispatched []string
}

func (f *fakeWakeupEnqueuer) CreateWakeupRequest(_ context.Context, req *routines.WakeupRequest) error {
	f.created = append(f.created, req)
	return nil
}

func (f *fakeWakeupEnqueuer) Dispatch(_ context.Context, requestID string) error {
	f.dispatched = append(f.dispatched, requestID)
	return nil
}

// fakeWorkflowEnsurer / fakeTaskCreator capture the heavy-path inputs
// and return synthetic ids so the heavy-flow test can assert on them.
type fakeWorkflowEnsurer struct {
	calledForWS string
}

func (f *fakeWorkflowEnsurer) EnsureRoutineWorkflow(_ context.Context, ws string) (string, error) {
	f.calledForWS = ws
	return "wf-routine-1", nil
}

type fakeTaskCreator struct {
	captured struct {
		workspaceID, workflowID, assignee, title, description string
	}
}

func (f *fakeTaskCreator) CreateOfficeTaskInWorkflow(
	_ context.Context, workspaceID, _, assignee, workflowID, title, description string,
) (string, error) {
	f.captured.workspaceID = workspaceID
	f.captured.workflowID = workflowID
	f.captured.assignee = assignee
	f.captured.title = title
	f.captured.description = description
	return "task-routine-1", nil
}

// TestDispatch_LightweightRoutine_EnqueuesWakeup verifies that a routine
// with an empty task_template enqueues a wakeup-request and dispatches
// it (without creating a task).
func TestDispatch_LightweightRoutine_EnqueuesWakeup(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Lightweight",
		TaskTemplate:           "", // lightweight
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	run, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "alpha"})
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.LinkedTaskID != "" {
		t.Errorf("lightweight path must not link a task, got %q", run.LinkedTaskID)
	}
	if run.Status != "task_created" {
		t.Errorf("status = %q, want task_created (run row reused even for taskless)", run.Status)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup-request created, got %d", len(enq.created))
	}
	got := enq.created[0]
	if got.AgentProfileID != "agent-1" {
		t.Errorf("agent_profile_id = %q, want agent-1", got.AgentProfileID)
	}
	if got.Source != "routine" {
		t.Errorf("source = %q, want routine", got.Source)
	}
	if got.Reason != "routine_dispatch" {
		t.Errorf("reason = %q, want routine_dispatch", got.Reason)
	}
	if !strings.Contains(got.Payload, `"routine_id":"`+routine.ID+`"`) {
		t.Errorf("payload missing routine_id, got %q", got.Payload)
	}
	if !strings.Contains(got.IdempotencyKey, "routine:"+routine.ID+":") {
		t.Errorf("idempotency_key missing routine prefix, got %q", got.IdempotencyKey)
	}
	if len(enq.dispatched) != 1 || enq.dispatched[0] != got.ID {
		t.Errorf("expected dispatch with the wakeup id, got %v", enq.dispatched)
	}
}

// TestDispatch_HeavyRoutine_CreatesTaskInRoutineWorkflow verifies a
// routine with a task_template materialises a real task pinned to the
// routine workflow id.
func TestDispatch_HeavyRoutine_CreatesTaskInRoutineWorkflow(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Heavy",
		TaskTemplate:           `{"title":"Daily review {{date}}","description":"D for {{date}}"}`,
		AssigneeAgentProfileID: "agent-coord",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
		Variables:              `{"date":{"default":"2026-01-01"}}`,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	wf := &fakeWorkflowEnsurer{}
	tc := &fakeTaskCreator{}
	svc.SetWorkflowEnsurer(wf)
	svc.SetTaskCreator(tc)

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.Status != "task_created" {
		t.Errorf("status = %q, want task_created", run.Status)
	}
	if run.LinkedTaskID != "task-routine-1" {
		t.Errorf("LinkedTaskID = %q, want task-routine-1", run.LinkedTaskID)
	}
	if wf.calledForWS != "ws-1" {
		t.Errorf("ensurer called with ws %q, want ws-1", wf.calledForWS)
	}
	if tc.captured.workflowID != "wf-routine-1" {
		t.Errorf("task created in workflow %q, want wf-routine-1", tc.captured.workflowID)
	}
	if tc.captured.assignee != "agent-coord" {
		t.Errorf("task assignee %q, want agent-coord", tc.captured.assignee)
	}
	if !strings.Contains(tc.captured.title, "Daily review") {
		t.Errorf("task title %q missing template prefix", tc.captured.title)
	}
}

// TestDispatch_HeavyRoutine_FallsBackWithoutDeps confirms that a heavy
// (task_template set) routine degrades to lightweight behaviour when
// the workflow ensurer or task creator is not wired — no task is
// created, no panic. Defensive when wiring is incomplete.
func TestDispatch_HeavyRoutine_FallsBackWithoutDeps(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "FallbackHeavy",
		TaskTemplate:           `{"title":"X","description":"Y"}`,
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.LinkedTaskID != "" {
		t.Errorf("expected empty linked_task_id when deps missing, got %q", run.LinkedTaskID)
	}
}

func TestListRoutineRuns(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Runs List", "always_create")
	for i := 0; i < 3; i++ {
		_, _ = svc.FireManual(ctx, routine.ID, nil)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runs))
	}
}
