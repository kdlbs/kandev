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
	"github.com/kandev/kandev/internal/office/shared"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// noopActivity implements shared.ActivityLogger for tests.
type noopActivity struct{}

func (n *noopActivity) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (n *noopActivity) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// newTestRoutineService creates a RoutineService backed by in-memory SQLite.
func newTestRoutineService(t *testing.T) *routines.RoutineService {
	t.Helper()
	svc, _ := newTestRoutineServiceWithDB(t)
	return svc
}

// newTestRoutineServiceWithDB is newTestRoutineService plus the backing
// *sqlx.DB, for tests that need to reach into shared task or routine tables.
func newTestRoutineServiceWithDB(t *testing.T) (*routines.RoutineService, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	return routines.NewRoutineService(repo, log, &noopActivity{}), db
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
		ConcurrencyPolicy: models.RoutineConcurrencyPolicy(policy),
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
	// No workflow ensurer / task creator wired on this service, so this
	// routine (despite having a task_template) takes the lightweight
	// path and terminates immediately at "done" — see D1 in the
	// office-routine-runs triage.
	if run.Status != "done" {
		t.Errorf("status = %q, want done", run.Status)
	}
	if run.Source != "manual" {
		t.Errorf("source = %q, want manual", run.Source)
	}
	if run.DispatchFingerprint == "" {
		t.Error("expected fingerprint to be set")
	}
}

// TestDispatch_SkipIfActive exercises skip_if_active on the heavy path,
// where "active" is real: run1's task genuinely has not reached a
// terminal step yet (office-routine-runs D2). Wired lightweight (the
// pre-fix shape of this test), the gate can never engage — a
// lightweight run never sits in an active status (D1) — so the test
// would pass by asserting behaviour nothing exercises.
func TestDispatch_SkipIfActive(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Skip Test", "skip_if_active")

	// First run should succeed and create a task.
	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	// Second run with the same fingerprint should be skipped: run1's
	// task is still open.
	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "skipped" {
		t.Errorf("second run status = %q, want skipped", run2.Status)
	}
}

// TestDispatch_CoalesceIfActive is the coalesce_if_active counterpart of
// TestDispatch_SkipIfActive; see its comment for why this needs the
// heavy path.
func TestDispatch_CoalesceIfActive(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

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

// TestDispatch_HeavyRoutine_SyncRunStatusUnblocksNextFire proves the
// gate closes and reopens rather than bricking after the first fire
// (the office-routine-runs symptom: 323 consecutive coalesces after one
// fire). Each cycle fires, expects a fresh task_created run (not
// skipped/coalesced against a prior cycle), then simulates the linked
// task reaching Done via SyncRunStatus — the seam finalizeDone calls in
// production.
func TestDispatch_HeavyRoutine_SyncRunStatusUnblocksNextFire(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Sync Cycle", "coalesce_if_active")

	for i := 1; i <= 3; i++ {
		run, err := svc.FireManual(ctx, routine.ID, nil)
		if err != nil {
			t.Fatalf("fire %d: %v", i, err)
		}
		if run.Status != "task_created" {
			t.Fatalf("fire %d status = %q, want task_created (previous cycle's gate should already be closed)", i, run.Status)
		}
		if run.LinkedTaskID == "" {
			t.Fatalf("fire %d: expected a linked task", i)
		}
		if err := svc.SyncRunStatus(ctx, run.LinkedTaskID, "done"); err != nil {
			t.Fatalf("sync run status after fire %d: %v", i, err)
		}
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

	// No workflow ensurer / task creator wired: lightweight path, done
	// on both fires (always_create never even consults the active-run
	// gate, so this exercises only the "successful lightweight fire
	// terminates" half of D1).
	if run1.Status != "done" || run2.Status != "done" {
		t.Errorf("both runs should be done, got %q and %q", run1.Status, run2.Status)
	}
}

// TestDispatch_DifferentVarsNotSkipped is on the heavy path so the
// active-run gate is real: without it, a lightweight run is never
// "active" (D1) and the assertion would pass regardless of whether
// fingerprints actually differ.
func TestDispatch_DifferentVarsNotSkipped(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Diff Vars", "skip_if_active")

	run1, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "A"})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	// Different variable -> different fingerprint -> not skipped, even
	// though run1's task is still open.
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
	failed     []string
}

func (f *fakeWakeupEnqueuer) CreateWakeupRequest(_ context.Context, req *routines.WakeupRequest) error {
	f.created = append(f.created, req)
	return nil
}

func (f *fakeWakeupEnqueuer) Dispatch(_ context.Context, requestID string) error {
	f.dispatched = append(f.dispatched, requestID)
	return nil
}

func (f *fakeWakeupEnqueuer) FailWakeupRequest(_ context.Context, requestID, _ string) error {
	f.failed = append(f.failed, requestID)
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
	// The run's own lifecycle ends once the wakeup-request is enqueued
	// and dispatched (D1) — it does not wait in task_created for the
	// wakeup dispatcher's own concurrency gate to resolve.
	if run.Status != "done" {
		t.Errorf("status = %q, want done", run.Status)
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
	// FireManual is event/user-triggered, not periodic (WO-46 Review round
	// 1, Blocker 1): it must get RunReasonRoutineDispatchEvent, never the
	// cron-only RunReasonRoutineDispatch the idle-skip gate treats as a
	// skippable periodic wake.
	if got.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("reason = %q, want %q", got.Reason, shared.RunReasonRoutineDispatchEvent)
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

// idempotencyConflictWakeupEnqueuer simulates a duplicate request identity
// already persisted by another caller.
type idempotencyConflictWakeupEnqueuer struct{}

func (idempotencyConflictWakeupEnqueuer) CreateWakeupRequest(context.Context, *routines.WakeupRequest) error {
	return routines.ErrWakeupAlreadyRequested
}

func (idempotencyConflictWakeupEnqueuer) Dispatch(context.Context, string) error { return nil }

func (idempotencyConflictWakeupEnqueuer) FailWakeupRequest(context.Context, string, string) error {
	return nil
}

// TestDispatch_LightweightRoutine_IdempotencyConflictIsNotFailed verifies
// that a duplicate request identity is recorded as done, not failed.
func TestDispatch_LightweightRoutine_IdempotencyConflictIsNotFailed(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Dedup Race",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	svc.SetWakeupEnqueuer(idempotencyConflictWakeupEnqueuer{})

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.Status != "done" {
		t.Errorf("status = %q, want done (idempotency conflict is success by another fire, not a failure)", run.Status)
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

func TestDispatch_HeavyRoutine_TruncatesRenderedTitle(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	longName := strings.Repeat("x", taskservice.TaskTitleMaxLength+10)
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Long title",
		TaskTemplate:           `{"title":"{{name}}","description":"Details: {{name}}"}`,
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
		Variables:              `{"name":{"default":"short"}}`,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	wf := &fakeWorkflowEnsurer{}
	tc := &fakeTaskCreator{}
	svc.SetWorkflowEnsurer(wf)
	svc.SetTaskCreator(tc)
	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": longName}); err != nil {
		t.Fatalf("fire manual: %v", err)
	}

	if gotLen := len([]rune(tc.captured.title)); gotLen != taskservice.TaskTitleMaxLength {
		t.Fatalf("captured title rune length = %d, want %d", gotLen, taskservice.TaskTitleMaxLength)
	}
	if !strings.Contains(tc.captured.description, longName) {
		t.Fatalf("description = %q, want full rendered value", tc.captured.description)
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
	// Falling back to lightweight must still resolve to a terminal
	// status, not strand the run in task_created with no task to ever
	// close it out.
	if run.Status != "done" {
		t.Errorf("status = %q, want done", run.Status)
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

// TestDispatch_LightweightRoutine_SubsequentFiresMaterialise is the
// card's explicit regression requirement: a lightweight routine must
// fire every time, under both concurrency policies, not just once. Pre-
// fix, materialiseLightweightRoutineRun left every successful fire in
// task_created, which both policies treat as "still active" forever.
func TestDispatch_LightweightRoutine_SubsequentFiresMaterialise(t *testing.T) {
	for _, policy := range []string{"skip_if_active", "coalesce_if_active"} {
		t.Run(policy, func(t *testing.T) {
			svc := newTestRoutineService(t)
			ctx := context.Background()

			routine := &models.Routine{
				WorkspaceID:            "ws-1",
				Name:                   "Coordinator heartbeat",
				TaskTemplate:           "", // lightweight
				AssigneeAgentProfileID: "agent-1",
				Status:                 "active",
				ConcurrencyPolicy:      models.RoutineConcurrencyPolicy(policy),
			}
			if err := svc.CreateRoutine(ctx, routine); err != nil {
				t.Fatalf("create routine: %v", err)
			}
			svc.SetWakeupEnqueuer(&fakeWakeupEnqueuer{})

			for i := 1; i <= 3; i++ {
				run, err := svc.FireManual(ctx, routine.ID, nil)
				if err != nil {
					t.Fatalf("fire %d: %v", i, err)
				}
				if run.Status != "done" {
					t.Fatalf("fire %d status = %q, want done (fire should materialise, not be gated by fire 1)", i, run.Status)
				}
			}
		})
	}
}

// TestDispatch_HeavyRoutine_GateSelfHealsWhenTaskTerminatesWithoutEvent is
// the R1 regression test: no production publisher of events.TaskMoved
// ever sets to_step_name (office-routine-runs review round 1), so
// SyncRunStatus is never reached by that event in production and cannot
// be the only way a heavy run's gate clears. This drives the actual
// production seam — FireManual -> dispatchRoutineRun ->
// applyConcurrencyPolicy — with no call to SyncRunStatus at all: the
// linked task's row is updated directly (as the real `tasks` table would
// be by an ordinary task-service move), and the gate must still notice
// and let the second fire through.
func TestDispatch_HeavyRoutine_GateSelfHealsWhenTaskTerminatesWithoutEvent(t *testing.T) {
	for _, policy := range []string{"skip_if_active", "coalesce_if_active"} {
		t.Run(policy, func(t *testing.T) {
			svc, db := newTestRoutineServiceWithDB(t)
			ctx := context.Background()
			svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
			svc.SetTaskCreator(&fakeTaskCreator{})

			routine := createTestRoutine(t, svc, "Self Heal "+policy, policy)

			run1, err := svc.FireManual(ctx, routine.ID, nil)
			if err != nil {
				t.Fatalf("first run: %v", err)
			}
			if run1.Status != "task_created" {
				t.Fatalf("first run status = %q, want task_created", run1.Status)
			}
			if run1.LinkedTaskID == "" {
				t.Fatalf("expected a linked task")
			}

			// Mirror the shared `tasks` table's shape and mark the linked
			// task COMPLETED directly — no TaskMoved event, no
			// SyncRunStatus call. This is what the row looks like after a
			// real task-service move in production.
			if _, err := db.ExecContext(ctx,
				`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
				t.Fatalf("create tasks table: %v", err)
			}
			if _, err := db.ExecContext(ctx,
				`INSERT INTO tasks (id, state) VALUES (?, ?)`, run1.LinkedTaskID, "COMPLETED"); err != nil {
				t.Fatalf("seed linked task: %v", err)
			}

			run2, err := svc.FireManual(ctx, routine.ID, nil)
			if err != nil {
				t.Fatalf("second run: %v", err)
			}
			if run2.Status != "task_created" {
				t.Fatalf("second run status = %q, want task_created (gate must self-heal once the linked task is terminal)", run2.Status)
			}
		})
	}
}

// TestDispatch_HeavyRoutine_GateSelfHealsOnFailedTask covers the third
// terminal outcome GetTaskTerminalStatus recognizes: a linked task that
// reaches FAILED (not just COMPLETED/CANCELLED) must also release the
// gate, and the stale run must be closed out as "failed" rather than
// "done" so routine history reflects what actually happened.
func TestDispatch_HeavyRoutine_GateSelfHealsOnFailedTask(t *testing.T) {
	for _, policy := range []string{"skip_if_active", "coalesce_if_active"} {
		t.Run(policy, func(t *testing.T) {
			svc, db := newTestRoutineServiceWithDB(t)
			ctx := context.Background()
			svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
			svc.SetTaskCreator(&fakeTaskCreator{})

			routine := createTestRoutine(t, svc, "Self Heal Failed "+policy, policy)

			run1, err := svc.FireManual(ctx, routine.ID, nil)
			if err != nil {
				t.Fatalf("first run: %v", err)
			}
			if run1.Status != "task_created" {
				t.Fatalf("first run status = %q, want task_created", run1.Status)
			}
			if run1.LinkedTaskID == "" {
				t.Fatalf("expected a linked task")
			}

			if _, err := db.ExecContext(ctx,
				`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
				t.Fatalf("create tasks table: %v", err)
			}
			if _, err := db.ExecContext(ctx,
				`INSERT INTO tasks (id, state) VALUES (?, ?)`, run1.LinkedTaskID, "FAILED"); err != nil {
				t.Fatalf("seed linked task: %v", err)
			}

			run2, err := svc.FireManual(ctx, routine.ID, nil)
			if err != nil {
				t.Fatalf("second run: %v", err)
			}
			if run2.Status != "task_created" {
				t.Fatalf("second run status = %q, want task_created (gate must self-heal once the linked task fails)", run2.Status)
			}

			var closedStatus string
			if err := db.GetContext(ctx, &closedStatus,
				`SELECT status FROM office_routine_runs WHERE id = ?`, run1.ID); err != nil {
				t.Fatalf("read closed-out run status: %v", err)
			}
			if closedStatus != "failed" {
				t.Errorf("run1 status after self-heal = %q, want failed", closedStatus)
			}
		})
	}
}

// TestDispatch_LastRunAt verifies routines.last_run_at is populated by a
// materialised fire and left untouched by a gated one — the card's
// fourth symptom (empty last_run_at despite hundreds of runs).
func TestDispatch_LastRunAt(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Last Run", "skip_if_active")

	before, err := svc.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get routine: %v", err)
	}
	if before.LastRunAt != nil {
		t.Fatalf("last_run_at = %v before any fire, want nil", before.LastRunAt)
	}

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("first fire: %v", err)
	}
	afterFirst, err := svc.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get routine: %v", err)
	}
	if afterFirst.LastRunAt == nil {
		t.Fatal("expected last_run_at to be set after a materialised fire")
	}
	firstStamp := *afterFirst.LastRunAt

	// Second fire is skipped (run1's task is still open) — last_run_at
	// must not advance for a fire that didn't materialise.
	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if run2.Status != "skipped" {
		t.Fatalf("second run status = %q, want skipped", run2.Status)
	}
	afterSecond, err := svc.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get routine: %v", err)
	}
	if !afterSecond.LastRunAt.Equal(firstStamp) {
		t.Errorf("last_run_at changed from %v to %v after a skipped fire", firstStamp, afterSecond.LastRunAt)
	}
}
