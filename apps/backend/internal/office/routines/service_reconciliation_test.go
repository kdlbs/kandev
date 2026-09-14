package routines_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

type dispatchErrorWakeupEnqueuer struct{ fakeWakeupEnqueuer }

func (f *dispatchErrorWakeupEnqueuer) Dispatch(context.Context, string) error {
	return errors.New("dispatcher unavailable")
}

type sequenceTaskCreator struct {
	ids  []string
	next int
}

func (f *sequenceTaskCreator) CreateOfficeTaskInWorkflow(
	context.Context, string, string, string, string, string, string,
) (string, error) {
	if f.next >= len(f.ids) {
		return "task-overflow", nil
	}
	id := f.ids[f.next]
	f.next++
	return id, nil
}

func TestDispatch_LightweightRoutine_SourceKeysAreUnique(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Manual keys",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "same"}); err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "same"}); err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if len(enq.created) != 2 {
		t.Fatalf("created wakeups = %d, want 2", len(enq.created))
	}
	if enq.created[0].IdempotencyKey == enq.created[1].IdempotencyKey {
		t.Fatalf("distinct manual fires reused idempotency key %q", enq.created[0].IdempotencyKey)
	}
}

func TestDispatch_LightweightRoutine_WebhookRequestKeyIsStable(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Webhook keys",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	trigger := &models.RoutineTrigger{ID: "trigger-1", RoutineID: routine.ID, Kind: "webhook"}
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if _, err := svc.DispatchRoutineRunWithIdempotencyKey(
		ctx, routine, trigger, "webhook", map[string]string{"branch": "main"}, "delivery-1"); err != nil {
		t.Fatalf("first webhook: %v", err)
	}
	if _, err := svc.DispatchRoutineRunWithIdempotencyKey(
		ctx, routine, trigger, "webhook", map[string]string{"branch": "main"}, "delivery-1"); err != nil {
		t.Fatalf("retry webhook: %v", err)
	}
	if len(enq.created) != 2 {
		t.Fatalf("created wakeups = %d, want 2", len(enq.created))
	}
	if enq.created[0].IdempotencyKey != enq.created[1].IdempotencyKey {
		t.Fatalf("retry changed idempotency key from %q to %q", enq.created[0].IdempotencyKey, enq.created[1].IdempotencyKey)
	}
}

func TestDispatch_LightweightRoutine_DispatchFailureFailsRun(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Dispatch failure",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	enq := &dispatchErrorWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err == nil {
		t.Fatal("expected the dispatch error to reach the caller")
	}
	if run.Status != models.RoutineRunStatusFailed {
		t.Errorf("status = %q, want failed", run.Status)
	}
	if len(enq.failed) != 1 {
		t.Errorf("failed wakeups = %d, want 1", len(enq.failed))
	}
}

func TestDispatch_HeavyRoutine_LongRunningTaskStillGates(t *testing.T) {
	svc, db := newTestRoutineServiceWithDB(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Stale Gate", "skip_if_active")

	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Fatalf("first run status = %q, want task_created", run1.Status)
	}

	if _, err := db.ExecContext(ctx,
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tasks (id, state) VALUES (?, ?)`, run1.LinkedTaskID, "IN_PROGRESS"); err != nil {
		t.Fatalf("insert linked task: %v", err)
	}

	longAgo := time.Now().UTC().AddDate(0, -1, 0)
	if _, err := db.ExecContext(ctx,
		`UPDATE office_routine_runs SET created_at = ? WHERE id = ?`, longAgo, run1.ID,
	); err != nil {
		t.Fatalf("backdate run: %v", err)
	}

	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != models.RoutineRunStatusSkipped {
		t.Errorf("second run status = %q, want skipped (live task must keep gating)", run2.Status)
	}
}

func TestDispatch_HeavyRoutine_RepairsNewestButStillBlocksOnOlder(t *testing.T) {
	svc, db := newTestRoutineServiceWithDB(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	creator := &sequenceTaskCreator{ids: []string{"task-old", "task-new", "task-third"}}
	svc.SetTaskCreator(creator)

	if _, err := db.ExecContext(ctx,
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tasks (id, state) VALUES (?, ?), (?, ?)`, "task-old", "IN_PROGRESS", "task-new", "COMPLETED"); err != nil {
		t.Fatalf("insert linked tasks: %v", err)
	}

	routine := createTestRoutine(t, svc, "Multiple active rows", "always_create")
	first, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first fire: %v", err)
	}
	second, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if first.LinkedTaskID != "task-old" || second.LinkedTaskID != "task-new" {
		t.Fatalf("linked tasks = %q, %q, want task-old, task-new", first.LinkedTaskID, second.LinkedTaskID)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE office_routine_runs SET created_at = CASE id WHEN ? THEN ? WHEN ? THEN ? END WHERE id IN (?, ?)`,
		first.ID, time.Now().UTC().Add(-2*time.Minute), second.ID, time.Now().UTC().Add(-time.Minute), first.ID, second.ID); err != nil {
		t.Fatalf("order routine runs: %v", err)
	}
	routine.ConcurrencyPolicy = models.ConcurrencyPolicySkipIfActive
	if err := svc.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update policy: %v", err)
	}

	third, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("third fire: %v", err)
	}
	if third.Status != models.RoutineRunStatusSkipped {
		t.Fatalf("third run status = %q, want skipped while older task remains active", third.Status)
	}
	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.ID == second.ID && run.Status != models.RoutineRunStatusDone {
			t.Errorf("newest repaired run status = %q, want done", run.Status)
		}
	}
}

func TestDispatch_HeavyRoutine_TerminalTaskStatesReleaseGate(t *testing.T) {
	cases := []struct {
		name          string
		state         string
		archived      bool
		wantOldStatus models.RoutineRunStatus
	}{
		{name: "failed", state: "FAILED", wantOldStatus: models.RoutineRunStatusFailed},
		{name: "archived", state: "IN_PROGRESS", archived: true, wantOldStatus: models.RoutineRunStatusCancelled},
		{name: "missing", wantOldStatus: models.RoutineRunStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, db := newTestRoutineServiceWithDB(t)
			ctx := context.Background()
			svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
			svc.SetTaskCreator(&fakeTaskCreator{})
			routine := createTestRoutine(t, svc, "Terminal "+tc.name, "skip_if_active")
			first, err := svc.FireManual(ctx, routine.ID, nil)
			if err != nil {
				t.Fatalf("first fire: %v", err)
			}

			if _, err := db.ExecContext(ctx,
				`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
				t.Fatalf("create tasks table: %v", err)
			}
			if tc.name != "missing" {
				var archivedAt any
				if tc.archived {
					archivedAt = time.Now().UTC()
				}
				if _, err := db.ExecContext(ctx,
					`INSERT INTO tasks (id, state, archived_at) VALUES (?, ?, ?)`, first.LinkedTaskID, tc.state, archivedAt); err != nil {
					t.Fatalf("insert linked task: %v", err)
				}
			}

			second, err := svc.FireManual(ctx, routine.ID, nil)
			if err != nil {
				t.Fatalf("second fire: %v", err)
			}
			if second.Status != models.RoutineRunStatusTaskCreated {
				t.Fatalf("second run status = %q, want task_created", second.Status)
			}
			runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
			if err != nil {
				t.Fatalf("list runs: %v", err)
			}
			for _, run := range runs {
				if run.ID == first.ID && run.Status != tc.wantOldStatus {
					t.Errorf("old run status = %q, want %q", run.Status, tc.wantOldStatus)
				}
			}
		})
	}
}
