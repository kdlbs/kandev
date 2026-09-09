package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func TestRoutine_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Daily Check",
		Description:       "Run daily checks",
		TaskTemplate:      `{"title":"Daily check"}`,
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Daily Check" {
		t.Errorf("name = %q, want %q", got.Name, "Daily Check")
	}

	routines, err := repo.ListRoutines(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(routines) != 1 {
		t.Fatalf("list count = %d, want 1", len(routines))
	}

	routine.Name = "Updated Routine"
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := repo.DeleteRoutine(ctx, routine.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestRoutineTrigger_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Trigger Test",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	now := time.Now().UTC()
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "UTC",
		NextRunAt:      &now,
		Enabled:        true,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if trigger.ID == "" {
		t.Fatal("expected trigger ID to be set")
	}

	triggers, err := repo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}

	due, err := repo.GetDueTriggers(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("get due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due trigger, got %d", len(due))
	}

	claimed, err := repo.ClaimTrigger(ctx, trigger.ID, now)
	if err != nil || !claimed {
		t.Fatalf("claim: claimed=%v, err=%v", claimed, err)
	}

	// Second claim should fail (CAS).
	claimed2, err := repo.ClaimTrigger(ctx, trigger.ID, now)
	if err != nil {
		t.Fatalf("second claim err: %v", err)
	}
	if claimed2 {
		t.Error("second claim should fail CAS")
	}

	if err := repo.DeleteRoutineTrigger(ctx, trigger.ID); err != nil {
		t.Fatalf("delete trigger: %v", err)
	}
}

func TestRoutineTrigger_WebhookLookup(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Webhook Test",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	trigger := &models.RoutineTrigger{
		RoutineID:   routine.ID,
		Kind:        "webhook",
		PublicID:    "wh-abc123",
		SigningMode: "bearer",
		Secret:      "my-secret",
		Enabled:     true,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	found, err := repo.GetTriggerByPublicID(ctx, "wh-abc123")
	if err != nil {
		t.Fatalf("get by public ID: %v", err)
	}
	if found.ID != trigger.ID {
		t.Errorf("ID = %q, want %q", found.ID, trigger.ID)
	}
}

func TestRoutineRun_ActiveFingerprint(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "FP Test",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	run := &models.RoutineRun{
		RoutineID:           routine.ID,
		Source:              "manual",
		Status:              "task_created",
		TriggerPayload:      "{}",
		DispatchFingerprint: "fp-123",
	}
	if err := repo.CreateRoutineRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	active, err := repo.GetActiveRunForFingerprint(ctx, routine.ID, "fp-123")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active == nil {
		t.Fatal("expected active run")
	}

	// Non-matching fingerprint should return nil.
	none, err := repo.GetActiveRunForFingerprint(ctx, routine.ID, "fp-999")
	if err != nil {
		t.Fatalf("get none: %v", err)
	}
	if none != nil {
		t.Error("expected nil for non-matching fingerprint")
	}

}

func TestRoutineRun_Create(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Runner",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	run := &models.RoutineRun{
		RoutineID:      routine.ID,
		Source:         "manual",
		Status:         "received",
		TriggerPayload: "{}",
	}
	if err := repo.CreateRoutineRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.ID == "" {
		t.Fatal("expected run ID to be set")
	}
}

// TestGetTaskTerminalStatus brings up just enough of the shared `tasks`
// table (owned by internal/task/repository/sqlite in the real app) for
// GetTaskTerminalStatus to query against it. office/repository/sqlite's
// own initSchema deliberately does not create this table (see
// TestInitSchema_AllTablesExist) — it shares the production DB's table
// rather than owning it.
func TestGetTaskTerminalStatus(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `INSERT INTO tasks (id, state, archived_at) VALUES (?, ?, NULL), (?, ?, NULL), (?, ?, NULL), (?, ?, NULL), (?, ?, ?)`,
		"task-done", "COMPLETED", "task-cancelled", "CANCELLED", "task-failed", "FAILED", "task-open", "IN_PROGRESS", "task-archived", "IN_PROGRESS", time.Now().UTC()); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	cases := []struct {
		taskID string
		want   string
	}{
		{"task-done", "done"},
		{"task-cancelled", "cancelled"},
		{"task-failed", "failed"},
		{"task-open", ""},
		{"task-archived", "cancelled"},
		{"task-missing", "missing"},
	}
	for _, c := range cases {
		got, err := repo.GetTaskTerminalStatus(ctx, c.taskID)
		if err != nil {
			t.Fatalf("GetTaskTerminalStatus(%q): %v", c.taskID, err)
		}
		if got != c.want {
			t.Errorf("GetTaskTerminalStatus(%q) = %q, want %q", c.taskID, got, c.want)
		}
	}
}

func TestUpdateRunStatusIfTaskCreated(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Conditional Close",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	run := &models.RoutineRun{
		RoutineID:           routine.ID,
		Source:              "manual",
		Status:              "task_created",
		TriggerPayload:      "{}",
		DispatchFingerprint: "fp-cond",
		LinkedTaskID:        "task-1",
	}
	if err := repo.CreateRoutineRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	closed, err := repo.UpdateRunStatusIfTaskCreated(ctx, run.ID, models.RoutineRunStatusDone, "task-1")
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	if !closed {
		t.Fatal("expected first close to succeed from task_created")
	}

	// A second attempt against the now-closed row must not win — this is
	// the race SyncRunStatus and the concurrency gate's inline check can
	// both hit, and it must not rewrite a "done" row to "cancelled" or
	// move completed_at.
	closedAgain, err := repo.UpdateRunStatusIfTaskCreated(ctx, run.ID, models.RoutineRunStatusCancelled, "task-1")
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if closedAgain {
		t.Fatal("second close against an already-closed run must report false")
	}

	got, err := repo.GetRoutineRunByLinkedTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("get by linked task: %v", err)
	}
	if got.Status != models.RoutineRunStatusDone {
		t.Errorf("status = %q, want done (unchanged by the losing second close)", got.Status)
	}
}

func TestGetRoutineRunByLinkedTaskID_EmptyTaskIDGuarded(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Empty Guard",
		TaskTemplate:      "",
		Status:            "active",
		ConcurrencyPolicy: "always_create",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	// Lightweight run: linked_task_id defaults to ''.
	run := &models.RoutineRun{
		RoutineID:           routine.ID,
		Source:              "manual",
		Status:              "done",
		TriggerPayload:      "{}",
		DispatchFingerprint: "fp-empty",
	}
	if err := repo.CreateRoutineRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	got, err := repo.GetRoutineRunByLinkedTaskID(ctx, "")
	if err != nil {
		t.Fatalf("lookup with empty taskID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty taskID, got run %q (would have matched an arbitrary lightweight run)", got.ID)
	}
}
