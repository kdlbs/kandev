package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
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

func TestListTriggersByRoutineIDs_Batch(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()

	r1 := &models.Routine{
		WorkspaceID: "ws-1", Name: "R1", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}
	r2 := &models.Routine{
		WorkspaceID: "ws-1", Name: "R2", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}
	r3 := &models.Routine{
		WorkspaceID: "ws-1", Name: "R3 (no triggers)", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}
	for _, r := range []*models.Routine{r1, r2, r3} {
		if err := repo.CreateRoutine(ctx, r); err != nil {
			t.Fatalf("create routine: %v", err)
		}
	}

	// t1 and t2 belong to r1 and get the same created_at (forced below), so
	// the id tiebreak is the only thing that can order them.
	t1 := &models.RoutineTrigger{ID: "trigger-b", RoutineID: r1.ID, Kind: "manual", Enabled: true}
	t2 := &models.RoutineTrigger{ID: "trigger-a", RoutineID: r1.ID, Kind: "manual", Enabled: true}
	t3 := &models.RoutineTrigger{RoutineID: r2.ID, Kind: "manual", Enabled: true}
	for _, tr := range []*models.RoutineTrigger{t1, t2, t3} {
		if err := repo.CreateRoutineTrigger(ctx, tr); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
	}

	tie := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx,
		`UPDATE office_routine_triggers SET created_at = ? WHERE id IN (?, ?)`,
		tie, t1.ID, t2.ID,
	); err != nil {
		t.Fatalf("force created_at tie: %v", err)
	}

	byRoutine, err := repo.ListTriggersByRoutineIDs(ctx, []string{r1.ID, r2.ID, r3.ID})
	if err != nil {
		t.Fatalf("batch list: %v", err)
	}
	if len(byRoutine) != 3 {
		t.Fatalf("map size = %d, want 3", len(byRoutine))
	}

	r1Triggers := byRoutine[r1.ID]
	if len(r1Triggers) != 2 {
		t.Fatalf("r1 triggers = %d, want 2", len(r1Triggers))
	}
	if r1Triggers[0].ID != "trigger-a" || r1Triggers[1].ID != "trigger-b" {
		t.Errorf("r1 trigger order = [%s, %s], want [trigger-a, trigger-b] (id tiebreak)",
			r1Triggers[0].ID, r1Triggers[1].ID)
	}

	if len(byRoutine[r2.ID]) != 1 {
		t.Fatalf("r2 triggers = %d, want 1", len(byRoutine[r2.ID]))
	}

	if r3Triggers, ok := byRoutine[r3.ID]; !ok || r3Triggers == nil || len(r3Triggers) != 0 {
		t.Errorf("r3 triggers = %v, want present and empty", r3Triggers)
	}

	empty, err := repo.ListTriggersByRoutineIDs(ctx, nil)
	if err != nil {
		t.Fatalf("empty batch list: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty input map size = %d, want 0", len(empty))
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
