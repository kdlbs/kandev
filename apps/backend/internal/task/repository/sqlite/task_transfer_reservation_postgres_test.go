package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// The actual transfer must reject a prior row owner before task/FK writes,
// release every partial reservation, audit a conflict and preserve key retry.
func TestTransferRelationReservationPriorRowOwner(t *testing.T) {
	for _, relation := range []string{"tasks", "task_step_transitions"} {
		t.Run(relation, func(t *testing.T) {
			repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
			repo.db.SetMaxOpenConns(4)
			holder, err := repo.db.BeginTxx(context.Background(), nil)
			require.NoError(t, err)
			defer func() { _ = holder.Rollback() }()
			var locked string
			if relation == "tasks" {
				err = holder.QueryRow(`SELECT id FROM tasks WHERE id=$1 FOR UPDATE`, tasks[0].ID).Scan(&locked)
			} else {
				var id int64
				err = holder.QueryRow(`SELECT id FROM task_step_transitions WHERE task_id=$1 ORDER BY id LIMIT 1 FOR UPDATE`, tasks[0].ID).Scan(&id)
			}
			require.NoError(t, err)
			before, err := repo.GetTask(context.Background(), tasks[0].ID)
			require.NoError(t, err)
			command := postgresRaceTransferCommand(tasks[0], "reservation-prior-owner")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err = repo.TransferTask(ctx, command)
			require.ErrorIs(t, err, repoerrors.ErrTaskTransferConflict)
			after, err := repo.GetTask(context.Background(), tasks[0].ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			var failed, conflicts, reservations int
			require.NoError(t, repo.db.QueryRow(`SELECT count(*) FROM task_transfer_audit WHERE result='failed'`).Scan(&failed))
			require.NoError(t, repo.db.QueryRow(`SELECT count(*) FROM task_transfer_audit WHERE result='conflict'`).Scan(&conflicts))
			require.Equal(t, 0, failed)
			require.Equal(t, 1, conflicts)
			require.NoError(t, repo.db.QueryRow(`SELECT count(*) FROM pg_locks WHERE locktype='relation' AND mode='ExclusiveLock' AND relation IN (SELECT oid FROM pg_class WHERE relnamespace=current_schema()::regnamespace)`).Scan(&reservations))
			require.Zero(t, reservations, "partial reservation must be fully rolled back")
			require.NoError(t, holder.Rollback())
			receipt, err := repo.TransferTask(context.Background(), command)
			require.NoError(t, err, "conflict must not poison manual same-key retry")
			require.NotContains(t, receipt.PreservationCounts, "tasks")
			var ledgerCount int
			require.NoError(t, repo.db.QueryRow(`SELECT count(*) FROM task_step_transitions WHERE task_id=$1`, tasks[0].ID).Scan(&ledgerCount))
			require.Equal(t, ledgerCount, receipt.PreservationCounts["task_step_transitions"])
			again, err := repo.TransferTask(context.Background(), command)
			require.NoError(t, err)
			require.Equal(t, receipt.PreservationDigest, again.PreservationDigest)
			require.Equal(t, receipt.OperationID, again.OperationID)
		})
	}
}
