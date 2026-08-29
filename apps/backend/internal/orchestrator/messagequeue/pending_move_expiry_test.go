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

func TestRepository_DeletePendingMove(t *testing.T) {
	for name, repo := range pendingMoveRepos(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// A missing row is a successful no-op, not an error: the sweep
			// races takes and transfers, and losing that race is normal.
			removed, err := repo.DeletePendingMove(ctx, "sess-absent")
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

			removed, err = repo.DeletePendingMove(ctx, "sess-a")
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
			removed, err = repo.DeletePendingMove(ctx, "sess-a")
			if err != nil {
				t.Fatalf("second delete of sess-a: %v", err)
			}
			if removed {
				t.Fatal("second delete reported a removal")
			}
		})
	}
}
