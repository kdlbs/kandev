package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

type retentionEventLockCounter struct {
	taskSessionExecutor
	commands int
}

func (c *retentionEventLockCounter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	c.commands++
	return c.taskSessionExecutor.ExecContext(ctx, query, args...)
}

func TestDeliveryRetentionEventLockCommands(t *testing.T) {
	pool := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	pool.SetMaxOpenConns(4)
	for _, sessions := range [][]string{{"a"}, {"b", "a", "b", ""}} {
		tx, err := pool.BeginTxx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		counter := &retentionEventLockCounter{taskSessionExecutor: tx}
		err = lockAgentDeliveryEventWrites(t.Context(), counter, pool.DriverName(), sessions, []string{"z", "y", "z", ""})
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			t.Fatal(rollbackErr)
		}
		if err != nil {
			t.Fatal(err)
		}
		if counter.commands != 1 {
			t.Errorf("event lock acquisition used %d network commands, want one", counter.commands)
		}
	}
}

func TestDeliveryRetentionEventLockOrder(t *testing.T) {
	pool := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	pool.SetMaxOpenConns(4)
	keys := []string{sessionTurnLockNamespace + "a", sessionTurnLockNamespace + "b",
		agentDeliverySessionLockNamespace + "a", agentDeliverySessionLockNamespace + "b",
		agentDeliveryStreamLockNamespace + "y", agentDeliveryStreamLockNamespace + "z"}
	for _, blocked := range []int{1, 2, 4, 5} {
		t.Run(fmt.Sprintf("prefix_%d", blocked), func(t *testing.T) {
			exerciseRetentionEventLockBarrier(t, pool, keys, blocked, false)
		})
	}
	t.Run("cancel", func(t *testing.T) { exerciseRetentionEventLockBarrier(t, pool, keys, 3, true) })
}

func exerciseRetentionEventLockBarrier(t *testing.T, pool *sqlx.DB, keys []string, blocked int, cancelBlocked bool) {
	t.Helper()
	holder, err := pool.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if err := lockAgentDeliveryKey(t.Context(), holder, keys[blocked]); err != nil {
		t.Fatal(err)
	}
	candidate, err := pool.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = candidate.Rollback() }()
	if _, err := candidate.ExecContext(t.Context(), `SET LOCAL plan_cache_mode = force_generic_plan`); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := candidate.QueryRowContext(t.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done, finished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(finished)
		done <- lockAgentDeliveryEventWrites(ctx, candidate, pool.DriverName(), []string{"b", "a", "b", ""}, []string{"z", "y", "z", ""})
	}()
	if err := waitForPostgresLock(t.Context(), pool, pid, finished); err != nil {
		t.Fatal(err)
	}
	assertRetentionEventLockPrefix(t, pool, pid, keys, blocked)
	if cancelBlocked {
		cancel()
		if err := <-done; err == nil {
			t.Fatal("canceled lock wait succeeded")
		}
		_ = candidate.Rollback()
		assertRetentionNoEventLocks(t, pool, pid)
		if err := holder.Rollback(); err != nil {
			t.Fatal(err)
		}
		retry, err := pool.BeginTxx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = retry.Rollback() }()
		if err := lockAgentDeliveryEventWrites(t.Context(), retry, pool.DriverName(), []string{"a", "b"}, []string{"y", "z"}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := holder.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := candidate.Commit(); err != nil {
		t.Fatal(err)
	}
	assertRetentionNoEventLocks(t, pool, pid)
}

func assertRetentionEventLockPrefix(t *testing.T, pool *sqlx.DB, pid int, keys []string, blocked int) {
	t.Helper()
	for index, key := range keys {
		var granted bool
		err := pool.QueryRowContext(t.Context(), `SELECT granted FROM pg_locks
   WHERE pid=$1 AND locktype='advisory' AND objsubid=1
    AND classid=((hashtextextended($2,0)>>32)&4294967295)::oid
    AND objid=(hashtextextended($2,0)&4294967295)::oid`, pid, key).Scan(&granted)
		if index > blocked {
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("later lock %s acquired before barrier: %t %v", key, granted, err)
			}
		} else if err != nil || granted != (index < blocked) {
			t.Fatalf("ordered lock %s: granted=%t error=%v blocked=%d", key, granted, err, blocked)
		}
	}
}

func assertRetentionNoEventLocks(t *testing.T, pool *sqlx.DB, pid int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		if err := pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_locks WHERE pid=$1 AND locktype='advisory'`, pid).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("transaction retained %d advisory locks after cancellation: %v", count, ctx.Err())
		}
	}
}

func TestDeliveryRetentionEventLockFreshOwner(t *testing.T) {
	repo := newRetentionTestRepo(t, "Postgres")
	repo.db.SetMaxOpenConns(4)
	session := createRetentionBenchmarkSession(t, repo)
	prepareRetentionBenchmarkOwner(t, repo)
	event := &models.AgentDeliveryEvent{SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, StreamID: "retention-fresh-owner-lock", Sequence: 1, EventType: "message_chunk",
		Payload: []byte(`{"type":"message_chunk","text":"output"}`), ReceivedAt: repo.nowUTC()}
	setRetentionBenchmarkEventOwner(event)
	if err := repo.prepareAgentDeliveryOwnerStatement(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	holder, err := repo.db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if err := lockSessionTurnWrites(t.Context(), holder, repo.db.DriverName(), session.ID); err != nil {
		t.Fatal(err)
	}
	candidate, err := repo.db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = candidate.Rollback() }()
	var pid int
	if err := candidate.QueryRowContext(t.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	done, finished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(finished)
		if err := lockAgentDeliveryEventWrites(t.Context(), candidate, repo.db.DriverName(), []string{session.ID}, []string{event.StreamID}); err != nil {
			done <- err
			return
		}
		_, err := repo.receiveAgentDeliveryEventTx(t.Context(), candidate, event, 1, agentDeliveryEventHash(event))
		done <- err
	}()
	if err := waitForPostgresLock(t.Context(), repo.db, pid, finished); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(t.Context(), repo.db.Rebind(`UPDATE executors_running SET agent_execution_id=? WHERE session_id=?`), "replacement-execution", session.ID); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
		t.Fatalf("authority after previous owner commit: %v", err)
	}
	if err := candidate.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", event.StreamID, 0)
	assertRetentionRowCount(t, repo, "agent_delivery_cursors", "stream_id = ?", event.StreamID, 0)
}
