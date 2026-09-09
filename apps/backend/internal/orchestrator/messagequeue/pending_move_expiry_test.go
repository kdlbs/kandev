package messagequeue

import (
	"context"
	"testing"
	"time"
)

func TestPendingMove_IsStaleAt(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	ttl := time.Hour

	cases := []struct {
		name     string
		queuedAt time.Time
		ttl      time.Duration
		want     bool
	}{
		{name: "well inside ttl", queuedAt: now.Add(-time.Minute), ttl: ttl, want: false},
		{name: "exactly at ttl is not yet stale", queuedAt: now.Add(-ttl), ttl: ttl, want: false},
		{name: "just past ttl", queuedAt: now.Add(-ttl - time.Nanosecond), ttl: ttl, want: true},
		{name: "long past ttl", queuedAt: now.Add(-9 * 24 * time.Hour), ttl: ttl, want: true},
		// An unset queued_at column and a clock skew must not be able to
		// mass-expire the table. Same rejection the idle-session reaper
		// applies to zero/future UpdatedAt.
		{name: "zero queued_at is never stale", queuedAt: time.Time{}, ttl: ttl, want: false},
		{name: "future queued_at is never stale", queuedAt: now.Add(time.Hour), ttl: ttl, want: false},
		{name: "zero ttl disables expiry", queuedAt: now.Add(-9 * 24 * time.Hour), ttl: 0, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			move := &PendingMove{QueuedAt: tc.queuedAt}
			if got := move.IsStaleAt(now, tc.ttl); got != tc.want {
				t.Fatalf("IsStaleAt(%v, %v) = %v, want %v", tc.queuedAt, tc.ttl, got, tc.want)
			}
		})
	}

	t.Run("nil move is never stale", func(t *testing.T) {
		var move *PendingMove
		if move.IsStaleAt(now, ttl) {
			t.Fatal("nil move reported stale")
		}
	})
}

// TestPendingMoveTTL_IsGenerouslyAboveAnyRealTurn pins the intent of the
// constant rather than its exact value: it must be far above the seconds-to-
// minutes window a deferred move legitimately occupies, and far below the
// multi-day replay that motivated it.
func TestPendingMoveTTL_IsGenerouslyAboveAnyRealTurn(t *testing.T) {
	if PendingMoveTTL < 4*time.Hour {
		t.Fatalf("PendingMoveTTL = %v; too tight to survive a long but legitimate turn", PendingMoveTTL)
	}
	if PendingMoveTTL > 72*time.Hour {
		t.Fatalf("PendingMoveTTL = %v; too loose to prevent a days-later replay", PendingMoveTTL)
	}
}

// pendingMoveRepos exercises both Repository implementations so the sweep's
// two new methods cannot drift between production SQLite and the in-memory
// double the orchestrator tests run against.
func pendingMoveRepos(t *testing.T) map[string]Repository {
	t.Helper()
	return map[string]Repository{
		"sqlite": newTestSQLiteRepo(t),
		"memory": NewMemoryRepository(),
	}
}

func TestRepository_ListPendingMoves(t *testing.T) {
	for name, repo := range pendingMoveRepos(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			records, err := repo.ListPendingMoves(ctx)
			if err != nil {
				t.Fatalf("list on empty table: %v", err)
			}
			if len(records) != 0 {
				t.Fatalf("empty table returned %d records", len(records))
			}

			// session_id is UNIQUE but task_id is not: one task legitimately
			// carries several armed rows keyed to different sessions. The
			// sweep has to see all of them.
			queuedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
			seed := []struct {
				sessionID string
				move      PendingMove
			}{
				{"sess-a", PendingMove{MoveID: "m-a", TaskID: "task-1", WorkflowID: "wf-1", WorkflowStepID: "step-blocked", Position: 2, QueuedAt: queuedAt, Actor: "agent", SenderSessionID: "sender-a"}},
				{"sess-b", PendingMove{MoveID: "m-b", TaskID: "task-1", WorkflowID: "wf-1", WorkflowStepID: "step-work", QueuedAt: queuedAt}},
				{"sess-c", PendingMove{MoveID: "m-c", TaskID: "task-2", WorkflowID: "wf-1", WorkflowStepID: "step-qa", QueuedAt: queuedAt}},
			}
			for _, entry := range seed {
				move := entry.move
				if err := repo.SetPendingMove(ctx, entry.sessionID, &move); err != nil {
					t.Fatalf("arm %s: %v", entry.sessionID, err)
				}
			}

			records, err = repo.ListPendingMoves(ctx)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(records) != len(seed) {
				t.Fatalf("listed %d records, want %d", len(records), len(seed))
			}

			bySession := make(map[string]PendingMove, len(records))
			for _, record := range records {
				bySession[record.SessionID] = record.Move
			}
			for _, entry := range seed {
				got, ok := bySession[entry.sessionID]
				if !ok {
					t.Fatalf("session %s missing from listing", entry.sessionID)
				}
				if got.MoveID != entry.move.MoveID || got.TaskID != entry.move.TaskID ||
					got.WorkflowID != entry.move.WorkflowID || got.WorkflowStepID != entry.move.WorkflowStepID ||
					got.Position != entry.move.Position || got.Actor != entry.move.Actor ||
					got.SenderSessionID != entry.move.SenderSessionID {
					t.Fatalf("session %s round-tripped as %+v, want %+v", entry.sessionID, got, entry.move)
				}
				if !got.QueuedAt.Equal(entry.move.QueuedAt) {
					t.Fatalf("session %s queued_at = %v, want %v", entry.sessionID, got.QueuedAt, entry.move.QueuedAt)
				}
			}
		})
	}
}

func TestRepository_DeletePendingMoveIfMatch(t *testing.T) {
	for name, repo := range pendingMoveRepos(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// A missing row is a successful no-op, not an error: the sweep
			// races takes and transfers, and losing that race is normal.
			absent := PendingMoveRecord{SessionID: "sess-absent", Move: PendingMove{MoveID: "absent", QueuedAt: time.Now().UTC()}}
			removed, err := repo.DeletePendingMoveIfMatch(ctx, absent, "")
			if err != nil {
				t.Fatalf("delete absent row: %v", err)
			}
			if removed {
				t.Fatal("delete of an absent row reported a removal")
			}

			for _, sessionID := range []string{"sess-a", "sess-b"} {
				if err := repo.SetPendingMove(ctx, sessionID, &PendingMove{
					MoveID: "m-" + sessionID, TaskID: "task-1", WorkflowStepID: "step-x",
				}); err != nil {
					t.Fatalf("arm %s: %v", sessionID, err)
				}
			}

			moveA, err := repo.GetPendingMove(ctx, "sess-a")
			if err != nil || moveA == nil {
				t.Fatalf("load sess-a before delete: move=%+v err=%v", moveA, err)
			}
			recordA := PendingMoveRecord{SessionID: "sess-a", Move: *moveA}
			removed, err = repo.DeletePendingMoveIfMatch(ctx, recordA, "")
			if err != nil {
				t.Fatalf("delete sess-a: %v", err)
			}
			if !removed {
				t.Fatal("delete of an armed row reported no removal")
			}
			if move, err := repo.GetPendingMove(ctx, "sess-a"); err != nil || move != nil {
				t.Fatalf("sess-a still armed after delete: move=%+v err=%v", move, err)
			}

			// Deleting one session must not touch its siblings.
			move, err := repo.GetPendingMove(ctx, "sess-b")
			if err != nil {
				t.Fatalf("get sess-b: %v", err)
			}
			if move == nil {
				t.Fatal("sibling row sess-b was removed by an unrelated delete")
			}

			// Deleting twice is idempotent.
			removed, err = repo.DeletePendingMoveIfMatch(ctx, recordA, "")
			if err != nil {
				t.Fatalf("second delete of sess-a: %v", err)
			}
			if removed {
				t.Fatal("second delete reported a removal")
			}
		})
	}
}

func TestRepository_DeletePendingMoveIfMatchPreservesReplacement(t *testing.T) {
	for name, repo := range pendingMoveRepos(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			queuedAtA := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
			moveA := &PendingMove{
				MoveID: "move-a", TaskID: "task-a", WorkflowID: "wf-a",
				WorkflowStepID: "step-a", QueuedAt: queuedAtA,
			}
			if err := repo.SetPendingMove(ctx, "sess", moveA); err != nil {
				t.Fatalf("arm move A: %v", err)
			}

			records, err := repo.ListPendingMoves(ctx)
			if err != nil {
				t.Fatalf("list move A: %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("listed %d records, want 1", len(records))
			}

			moveB := &PendingMove{
				MoveID: "move-b", TaskID: "task-b", WorkflowID: "wf-b",
				WorkflowStepID: "step-b", QueuedAt: queuedAtA.Add(time.Hour),
			}
			if err := repo.SetPendingMove(ctx, "sess", moveB); err != nil {
				t.Fatalf("replace with move B: %v", err)
			}

			removed, err := repo.DeletePendingMoveIfMatch(ctx, records[0], "")
			if err != nil {
				t.Fatalf("delete inspected move A: %v", err)
			}
			if removed {
				t.Fatal("delete of inspected move A removed replacement move B")
			}

			got, err := repo.GetPendingMove(ctx, "sess")
			if err != nil {
				t.Fatalf("get replacement move B: %v", err)
			}
			if got == nil || got.MoveID != moveB.MoveID || !got.QueuedAt.Equal(moveB.QueuedAt) {
				t.Fatalf("replacement move B was not preserved: got %+v, want %+v", got, moveB)
			}
		})
	}
}

func TestSQLiteRepository_DeletePendingMoveIfMatchRollsBackPromptFailure(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	sqlRepo := repo.(*sqliteRepository)
	ctx := context.Background()
	queuedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	move := &PendingMove{
		MoveID: "move-a", TaskID: "task-a", WorkflowID: "wf-a",
		WorkflowStepID: "step-a", QueuedAt: queuedAt,
	}
	if err := repo.SetPendingMove(ctx, "sess", move); err != nil {
		t.Fatalf("arm pending move: %v", err)
	}
	prompt := &QueuedMessage{
		ID: "handoff", SessionID: "sess", TaskID: "task-a", Content: "target-step hand-off",
		QueuedAt: queuedAt, QueuedBy: QueuedByMoveTask,
	}
	if err := repo.Insert(ctx, prompt, 10); err != nil {
		t.Fatalf("queue hand-off prompt: %v", err)
	}
	records, err := repo.ListPendingMoves(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list pending move: records=%+v err=%v", records, err)
	}
	if _, err := sqlRepo.db.Exec(`
		CREATE TRIGGER fail_handoff_delete
		BEFORE DELETE ON queued_messages
		WHEN OLD.id = 'handoff'
		BEGIN
			SELECT RAISE(ABORT, 'injected prompt delete failure');
		END
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	removed, err := repo.DeletePendingMoveIfMatch(ctx, records[0], prompt.ID)
	if err == nil {
		t.Fatal("expected correlated prompt delete failure")
	}
	if removed {
		t.Fatal("failed transaction reported pending move removed")
	}
	if got, getErr := repo.GetPendingMove(ctx, "sess"); getErr != nil || got == nil {
		t.Fatalf("pending move was not rolled back: move=%+v err=%v", got, getErr)
	}
	if entries, listErr := repo.ListBySession(ctx, "sess"); listErr != nil || len(entries) != 1 {
		t.Fatalf("hand-off prompt was not preserved: entries=%+v err=%v", entries, listErr)
	}

	if _, err := sqlRepo.db.Exec(`DROP TRIGGER fail_handoff_delete`); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	removed, err = repo.DeletePendingMoveIfMatch(ctx, records[0], prompt.ID)
	if err != nil || !removed {
		t.Fatalf("retry exact delete: removed=%v err=%v", removed, err)
	}
	if entries, listErr := repo.ListBySession(ctx, "sess"); listErr != nil || len(entries) != 0 {
		t.Fatalf("hand-off prompt survived retry: entries=%+v err=%v", entries, listErr)
	}
}
