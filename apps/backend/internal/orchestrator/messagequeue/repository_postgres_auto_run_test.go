package messagequeue

import (
	"context"
	"testing"
)

// TestPostgresRepository_AutoRunOffSerializesBeforeAutomaticReserve proves
// policy and automatic reservation share the cross-process session lock. OFF
// enters the lock wait queue first, so the later reserve must observe OFF and
// leave the FIFO head pending after the held lock is released.
func TestPostgresRepository_AutoRunOffSerializesBeforeAutomaticReserve(t *testing.T) {
	repoA, repoB, extraDB := newTestPostgresRepoPair(t)
	ctx := context.Background()
	const sessionID = "auto-run-race"
	entry := defaultAutoMergeEntry(sessionID, "queued")
	if err := repoA.Insert(ctx, &entry, 0); err != nil {
		t.Fatalf("insert queued entry: %v", err)
	}
	repoC, err := NewSQLiteRepository(extraDB, extraDB)
	if err != nil {
		t.Fatalf("NewSQLiteRepository(extra): %v", err)
	}

	setterPID := pgBackendPID(t, repoB.(*sqliteRepository).db)
	reservePID := pgBackendPID(t, extraDB)
	lockTx, err := repoA.(*sqliteRepository).db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ($1)
		ON CONFLICT(session_id) DO NOTHING
	`, sessionID); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = $1 FOR UPDATE
	`, sessionID); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	setDone := make(chan error, 1)
	go func() {
		setDone <- repoB.SetAutoRun(ctx, sessionID, false)
	}()
	waitForWaitingLocks(t, lockTx, setterPID, 1, "Auto-run OFF on the session lock")

	type reserveResult struct {
		message *QueuedMessage
		enabled bool
		err     error
	}
	reserveDone := make(chan reserveResult, 1)
	go func() {
		message, enabled, reserveErr := repoC.ReserveHeadIfAutoRun(ctx, sessionID)
		reserveDone <- reserveResult{message: message, enabled: enabled, err: reserveErr}
	}()
	waitForWaitingLocks(t, lockTx, reservePID, 1, "automatic reserve on the session lock")

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-setDone; err != nil {
		t.Fatalf("set Auto-run OFF: %v", err)
	}
	result := <-reserveDone
	if result.err != nil {
		t.Fatalf("automatic reserve: %v", result.err)
	}
	if result.message != nil || result.enabled {
		t.Fatalf("automatic reserve = (%+v, %v), want (nil, false)", result.message, result.enabled)
	}
	entries, err := repoA.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("queue after paused reserve = %+v, want original entry", entries)
	}
}
