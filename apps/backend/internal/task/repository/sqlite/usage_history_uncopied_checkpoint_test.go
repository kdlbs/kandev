package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageHistoryUncopiedNativeCompletedCheckpoint(t *testing.T) {
	runUncopiedNativeCompletedCheckpoint(t, false)
}

func TestUsageHistoryPostgresUncopiedNativeCompletedCheckpoint(t *testing.T) {
	runUncopiedNativeCompletedCheckpoint(t, true)
}

func runUncopiedNativeCompletedCheckpoint(t *testing.T, postgres bool) {
	t.Helper()
	for _, prune := range []bool{false, true} {
		name := "delete"
		if prune {
			name = "prune"
		}
		t.Run(name, func(t *testing.T) {
			f := newUncopiedNativeFixture(t, postgres, 2)
			before := f.readLedger(t)
			// A previous binary could persist completion after recording an
			// ownership gap. Reproduce that durable state without a retained copy.
			if prune {
				require.Error(t, f.history.PreserveSessionUsage(context.Background(), f.proof.TaskID, f.session.ID, f.session.QueueIncarnationID))
			} else {
				require.Error(t, f.history.PreserveTaskUsage(context.Background(), f.proof.TaskID))
			}
			_, err := f.db.Exec(`UPDATE usage_preservation_cursors SET last_key = high_water, complete = 1`)
			require.NoError(t, err)
			_, err = f.db.Exec(`UPDATE usage_preservation_fences SET state = 'ready'`)
			require.NoError(t, err)
			f.restart(t)
			require.ErrorContains(t, f.cleanup(prune), "no accepted retained copy")
			require.Equal(t, before, f.readLedger(t))
			_, err = f.tasks.GetTaskSession(context.Background(), f.session.ID)
			require.NoError(t, err)
			f.restart(t)
			require.ErrorContains(t, f.cleanup(prune), "no accepted retained copy")
			require.Equal(t, before, f.readLedger(t))
			for _, event := range before {
				f.retainOriginalEvent(t, event)
			}
			_, err = f.db.Exec(f.db.Rebind(`UPDATE workspaces SET org_id = ? WHERE id = ?`), "org-b", f.proof.WorkspaceID)
			require.NoError(t, err)
			require.NoError(t, f.cleanup(prune))
			f.assertRetained(t, before)
		})
	}
}
