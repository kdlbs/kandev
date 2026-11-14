package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPostgresSendOperationDuplicateClaimsUseIdentityConflict(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, item := range []struct {
		repo *Repository
		id   string
	}{{repoA, "row-claim-a"}, {repoB, "row-claim-b"}} {
		wait.Add(1)
		go func(repo *Repository, rowID string) {
			defer wait.Done()
			<-start
			results <- repo.ClaimSendOperation(ctx, testSendOperation(
				rowID, "operation-race", "sender-race", "session-race",
			))
		}(item.repo, item.id)
	}
	close(start)
	wait.Wait()
	close(results)

	claimed := 0
	conflicted := 0
	for err := range results {
		switch {
		case err == nil:
			claimed++
		case errors.Is(err, models.ErrSendOperationNotClaimed):
			conflicted++
		default:
			t.Fatalf("duplicate claim returned %v, want ErrSendOperationNotClaimed", err)
		}
	}
	require.Equal(t, 1, claimed)
	require.Equal(t, 1, conflicted)
	if _, err := repoA.GetSendOperation(ctx, "sender-race", "session-race", "operation-race"); err != nil {
		t.Fatalf("winning operation readback: %v", err)
	}
}

func TestPostgresSendOperationPrunePreservesConcurrentSettlement(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	operation := testSendOperation("row-prune-race", "operation-prune-race", "sender-prune-race", "session-prune-race")
	require.NoError(t, repoA.ClaimSendOperation(ctx, operation))
	_, err := repoA.db.ExecContext(ctx, repoA.db.Rebind(`
		UPDATE task_message_send_operations SET created_at = ? WHERE id = ?
	`), time.Now().UTC().Add(-sendOperationStalePending-time.Hour), operation.ID)
	require.NoError(t, err)

	holder, err := repoB.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	var holderPID, waiterPID int
	require.NoError(t, holder.GetContext(ctx, &holderPID, `SELECT pg_backend_pid()`))
	require.NoError(t, repoA.db.GetContext(ctx, &waiterPID, `SELECT pg_backend_pid()`))
	var lockedID string
	require.NoError(t, holder.GetContext(ctx, &lockedID, repoB.db.Rebind(`
		SELECT id FROM task_message_send_operations WHERE id = ? FOR UPDATE
	`), operation.ID))
	require.Equal(t, operation.ID, lockedID)

	pruneDone := make(chan error, 1)
	go func() {
		_, _, pruneErr := repoA.PruneSendOperations(ctx, 10)
		pruneDone <- pruneErr
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		err := observer.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				 WHERE pid = $1 AND wait_event_type = 'Lock'
				   AND $2 = ANY(pg_blocking_pids(pid))
			)
		`, waiterPID, holderPID).Scan(&waiting)
		if err != nil {
			t.Errorf("inspect stale-prune lock wait: %v", err)
			return false
		}
		return waiting
	}, 5*time.Second, 10*time.Millisecond, "stale cleanup did not wait on the locked operation")

	_, err = holder.ExecContext(ctx, repoB.db.Rebind(`
		UPDATE task_message_send_operations
		   SET state = ?, delivery_status = ?, message_id = ?, settled_at = ?, updated_at = ?
		 WHERE id = ?
	`), models.SendOperationStateCommitted, models.SendOperationDeliverySent, "message-race",
		time.Now().UTC(), time.Now().UTC(), operation.ID)
	require.NoError(t, err)
	require.NoError(t, holder.Commit())

	select {
	case err := <-pruneDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatalf("stale cleanup did not finish after settlement committed: %v", ctx.Err())
	}

	loaded, err := repoA.GetSendOperation(context.Background(), operation.SenderTaskID, operation.SenderSessionID, operation.OperationID)
	require.NoError(t, err)
	require.Equal(t, models.SendOperationStateCommitted, loaded.State,
		"cleanup must re-check pending state after waiting for the settlement lock")
	require.Equal(t, "message-race", loaded.MessageID)
}
