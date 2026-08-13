package github

import (
	"context"
	"testing"
	"time"
)

// TestSetTaskPRDisposition_PublishesOnChangeNotOnIdenticalRePatch closes the
// gap flagged during PR review: no test previously exercised the event-bus
// side of SetTaskPRDisposition end to end (the controller test harness wires
// a nil event bus). AC-29 requires a publish when the disposition changes
// and no publish — nor an advance of disposition_recorded_at — on an
// identical re-PATCH.
func TestSetTaskPRDisposition_PublishesOnChangeNotOnIdenticalRePatch(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	tp := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "acme", Repo: "demo",
		PRNumber: 1, PRURL: "https://github.com/acme/demo/pull/1", PRTitle: "closed pr",
		State: "closed", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })

	disposition := TaskPRDispositionDuplicate
	updated, err := svc.SetTaskPRDisposition(ctx, "ws-1", tp.ID, &disposition, nil)
	if err != nil {
		t.Fatalf("SetTaskPRDisposition: %v", err)
	}
	if updated == nil || updated.Disposition == nil || *updated.Disposition != TaskPRDispositionDuplicate {
		t.Fatalf("updated disposition = %+v, want %q", updated, TaskPRDispositionDuplicate)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Fatalf("published events after first PATCH = %d, want 1", got)
	}
	firstRecordedAt := updated.DispositionRecordedAt
	if firstRecordedAt == nil {
		t.Fatal("DispositionRecordedAt = nil, want set after first PATCH")
	}

	updated, err = svc.SetTaskPRDisposition(ctx, "ws-1", tp.ID, &disposition, nil)
	if err != nil {
		t.Fatalf("SetTaskPRDisposition (identical re-PATCH): %v", err)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Fatalf("published events after identical re-PATCH = %d, want still 1 (no republish)", got)
	}
	if updated.DispositionRecordedAt == nil || !updated.DispositionRecordedAt.Equal(*firstRecordedAt) {
		t.Fatalf("DispositionRecordedAt after identical re-PATCH = %v, want unchanged %v",
			updated.DispositionRecordedAt, firstRecordedAt)
	}
}
