package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func newActivityEntry(wsID, actorType, actorID, action, targetType, targetID string) *models.ActivityEntry {
	return &models.ActivityEntry{
		WorkspaceID: wsID,
		ActorType:   models.ActivityActorType(actorType),
		ActorID:     actorID,
		Action:      models.ActivityAction(action),
		TargetType:  models.ActivityTargetType(targetType),
		TargetID:    targetID,
		Details:     "{}",
	}
}

// Pins that AppendRunEvent assigns monotonically increasing seq per
// run, ListRunEvents returns them in seq order, and afterSeq filters
// out earlier rows. Two distinct runs get independent seq sequences.
func TestRunEvents_AppendAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, payload := range []string{
		`{"step":1}`, `{"step":2}`, `{"step":3}`,
	} {
		if _, err := repo.AppendRunEvent(ctx, "run-A", "step", "info", payload); err != nil {
			t.Fatalf("append run-A: %v", err)
		}
	}
	if _, err := repo.AppendRunEvent(ctx, "run-B", "init", "info", "{}"); err != nil {
		t.Fatalf("append run-B: %v", err)
	}

	got, err := repo.ListRunEvents(ctx, "run-A", -1, 0)
	if err != nil {
		t.Fatalf("list run-A: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events for run-A, got %d", len(got))
	}
	for i, e := range got {
		if e.Seq != i {
			t.Errorf("event[%d].seq = %d, want %d", i, e.Seq, i)
		}
		if e.EventType != "step" {
			t.Errorf("event[%d].event_type = %q, want step", i, e.EventType)
		}
	}

	// afterSeq filters earlier rows
	tail, err := repo.ListRunEvents(ctx, "run-A", 0, 0)
	if err != nil {
		t.Fatalf("list afterSeq=0: %v", err)
	}
	if len(tail) != 2 {
		t.Fatalf("expected 2 events afterSeq=0, got %d", len(tail))
	}

	// run-B is independent
	other, err := repo.ListRunEvents(ctx, "run-B", -1, 0)
	if err != nil {
		t.Fatalf("list run-B: %v", err)
	}
	if len(other) != 1 || other[0].Seq != 0 || other[0].EventType != "init" {
		t.Fatalf("unexpected run-B events: %+v", other)
	}
}

// Pins that ListTasksTouchedByRun returns the distinct task ids the
// agent acted on under a given run, sourced from the activity_log
// run_id column.
func TestActivityLog_ListTasksTouchedByRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	insert := func(runID, taskID, action string) {
		t.Helper()
		entry := newActivityEntry("ws-1", "agent", "agent-1", action, "task", taskID)
		entry.RunID = runID
		if err := repo.CreateActivityEntry(ctx, entry); err != nil {
			t.Fatalf("create entry: %v", err)
		}
	}
	insert("run-1", "task-A", "task.updated")
	insert("run-1", "task-B", "task.commented")
	insert("run-1", "task-A", "task.commented") // duplicate task → distinct
	insert("run-2", "task-C", "task.updated")
	insert("", "task-D", "task.updated") // not under a run → ignored

	got, err := repo.ListTasksTouchedByRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct tasks for run-1, got %d (%v)", len(got), got)
	}
	seen := map[string]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if !seen["task-A"] || !seen["task-B"] {
		t.Errorf("expected task-A + task-B, got %v", got)
	}
	if seen["task-C"] || seen["task-D"] {
		t.Errorf("leaked task from another run / no-run row: %v", got)
	}
}
