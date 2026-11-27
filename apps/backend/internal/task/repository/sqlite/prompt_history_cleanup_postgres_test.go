package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestPostgresDeleteTaskSerializesWithFallbackClaim(t *testing.T) {
	repoA, repoB, claimDB := newTaskPostgresRepoPair(t)
	repoA.db.SetMaxOpenConns(2)
	claimRepo, err := NewWithDB(claimDB, claimDB, nil)
	if err != nil {
		t.Fatalf("init fallback claim repository: %v", err)
	}
	ctx := context.Background()
	seedTaskWithSession(t, repoA, "task-prompt-delete-race", "workspace-prompt-delete-race", "session-prompt-delete-race")
	session, err := repoA.GetTaskSession(ctx, "session-prompt-delete-race")
	if err != nil {
		t.Fatalf("read session incarnation: %v", err)
	}

	const barrierKey = "prompt-sequence-delete-trigger-barrier"
	if _, err := repoA.db.ExecContext(ctx, `
		CREATE FUNCTION hold_prompt_session_delete() RETURNS trigger AS $$
		BEGIN
			PERFORM pg_advisory_xact_lock(hashtextextended('`+barrierKey+`', 0));
			RETURN OLD;
		END;
		$$ LANGUAGE plpgsql
	`); err != nil {
		t.Fatalf("create delete barrier trigger function: %v", err)
	}
	if _, err := repoA.db.ExecContext(ctx, `
		CREATE TRIGGER hold_prompt_session_delete
		AFTER DELETE ON task_sessions
		FOR EACH ROW EXECUTE FUNCTION hold_prompt_session_delete()
	`); err != nil {
		t.Fatalf("create delete barrier trigger: %v", err)
	}

	blocker, err := repoA.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	if _, err := blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, barrierKey); err != nil {
		t.Fatalf("hold delete trigger barrier: %v", err)
	}

	deletePID := pgBackendPID(t, repoB.db)
	deleteDone := make(chan struct{})
	var deleteErr error
	go func() {
		deleteErr = repoB.DeleteTask(ctx, "task-prompt-delete-race")
		close(deleteDone)
	}()
	if err := waitForPostgresLock(ctx, repoA.db, deletePID, deleteDone); err != nil {
		t.Fatalf("wait for task deletion trigger barrier: %v", err)
	}

	claimPID := pgBackendPID(t, claimRepo.db)
	claimDone := make(chan struct{})
	var claimed bool
	var claimErr error
	go func() {
		claimed, claimErr = claimRepo.ClaimInitialPromptFallback(ctx, "session-prompt-delete-race", session.QueueIncarnationID)
		close(claimDone)
	}()
	if err := waitForPostgresLock(ctx, repoA.db, claimPID, claimDone); err != nil {
		t.Fatalf("wait for fallback claim to serialize with deletion: %v", err)
	}
	if err := blocker.Commit(); err != nil {
		t.Fatalf("release delete trigger barrier: %v", err)
	}

	select {
	case <-deleteDone:
	case <-time.After(5 * time.Second):
		t.Fatal("task deletion did not finish")
	}
	if deleteErr != nil {
		t.Fatalf("delete task: %v", deleteErr)
	}
	select {
	case <-claimDone:
	case <-time.After(5 * time.Second):
		t.Fatal("fallback claim did not finish")
	}
	if claimErr != nil {
		t.Fatalf("fallback claim: %v", claimErr)
	}
	if claimed {
		t.Fatal("fallback claim succeeded after the owning session was deleted")
	}
	if _, err := claimRepo.GetTask(ctx, "task-prompt-delete-race"); err == nil {
		t.Fatal("task still exists after deletion")
	}
	if _, err := claimRepo.GetTaskSession(ctx, "session-prompt-delete-race"); err == nil {
		t.Fatal("session still exists after task deletion")
	}
	var markers int
	if err := claimRepo.db.GetContext(ctx, &markers, claimRepo.db.Rebind(`
		SELECT COUNT(*) FROM task_session_prompt_seq WHERE task_session_id = ?
	`), "session-prompt-delete-race"); err != nil {
		t.Fatal(err)
	}
	if markers != 0 {
		t.Fatalf("orphan prompt sequence markers = %d, want 0", markers)
	}
}
