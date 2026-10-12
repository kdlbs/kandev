package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresRecordSessionRecoveryResolutionUsesTransactionAdvisoryLock(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-recovery-resolution-lock-pg"
	const sessionID = "session-recovery-resolution-lock-pg"
	seedSessionForTurns(t, repo, taskID, sessionID)

	blocker, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin advisory lock blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback() })
	lockKey := "session-recovery-resolution:" + sessionID
	if _, err := blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		t.Fatalf("hold recovery resolution advisory lock: %v", err)
	}

	type result struct {
		stored bool
		err    error
	}
	done := make(chan result, 1)
	go func() {
		stored, recordErr := repo.RecordSessionRecoveryResolution(ctx, sessionID, models.SessionRecoveryResolution{
			ErrorStamp: "failure-before-recovery",
			AttemptID:  "resume-1",
			ResolvedAt: time.Date(2026, time.September, 30, 10, 0, 0, 0, time.UTC),
		})
		done <- result{stored: stored, err: recordErr}
	}()
	waitForPostgresLockWait(t, ctx, repo, "pg_advisory_xact_lock")
	select {
	case got := <-done:
		t.Fatalf("resolution write did not wait for the advisory lock: %+v", got)
	default:
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release recovery resolution advisory lock: %v", err)
	}
	select {
	case got := <-done:
		if got.err != nil || !got.stored {
			t.Fatalf("record recovery resolution after lock release = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovery resolution after lock release")
	}

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load resolved session: %v", err)
	}
	resolutions := models.LoadSessionRecoveryResolutions(session.Metadata)
	if len(resolutions) != 1 || resolutions[0].ErrorStamp != "failure-before-recovery" {
		t.Fatalf("stored recovery resolutions = %#v, want exact matching stamp", resolutions)
	}
}
