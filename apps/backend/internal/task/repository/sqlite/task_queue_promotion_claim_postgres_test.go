package sqlite

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresClaimQueuePromotionEntrySerializesWithConcurrentMove(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConn(t, dsn, 3)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	claimerDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)
	claimerRepo, err := NewWithDB(claimerDB, claimerDB, nil)
	require.NoError(t, err)

	for _, test := range []struct {
		name            string
		claimLegacyZero bool
	}{
		{name: "stale positive entry"},
		{name: "legacy zero entry", claimLegacyZero: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			taskID := "task-promotion-claim-concurrent-" + strings.ReplaceAll(test.name, " ", "-")
			oldEntryID := seedQueuePromotionClaimTask(t, repo, taskID)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			moveTx, err := repo.db.BeginTxx(ctx, repo.hierarchyTxOptions())
			require.NoError(t, err)
			defer func() { _ = moveTx.Rollback() }()
			require.NoError(t, repo.lockTaskHierarchy(ctx, moveTx, nil, []string{taskID}))
			require.NoError(t, repo.lockTaskRowInTx(ctx, moveTx, taskID))
			newEntryID, err := repo.recordStepTransition(ctx, moveTx, stepTransitionInput{
				taskID: taskID, fromWorkflowID: "workflow-" + taskID, fromWorkflowStepID: "step-source",
				toWorkflowID: "workflow-" + taskID, toWorkflowStepID: "step-newer", occurredAt: time.Now().UTC(),
			})
			require.NoError(t, err)
			require.Greater(t, newEntryID, oldEntryID)
			markerJSON, err := json.Marshal(map[string]interface{}{
				"from_step_id": "step-newer-source",
				models.QueuePromotionEntryTransitionIDKey: strconv.FormatInt(newEntryID, 10),
			})
			require.NoError(t, err)
			_, err = moveTx.ExecContext(ctx, repo.db.Rebind(`
				UPDATE tasks
				SET workflow_step_id = ?,
				    metadata = jsonb_set(metadata::jsonb, ARRAY[?]::text[], ?::jsonb, true)::text
				WHERE id = ?
			`), "step-newer", models.MetaKeyQueuePromotionPending, string(markerJSON), taskID)
			require.NoError(t, err)

			var claimerPID int
			require.NoError(t, claimerDB.Get(&claimerPID, "SELECT pg_backend_pid()"))
			claimResult := make(chan struct {
				claimed bool
				err     error
			}, 1)
			claimDone := make(chan struct{})
			claimEntryID := oldEntryID
			if test.claimLegacyZero {
				claimEntryID = 0
			}
			go func() {
				claimed, claimErr := claimerRepo.ClaimQueuePromotionEntry(ctx, taskID, claimEntryID)
				claimResult <- struct {
					claimed bool
					err     error
				}{claimed: claimed, err: claimErr}
				close(claimDone)
			}()
			t.Cleanup(func() {
				_ = moveTx.Rollback()
				select {
				case <-claimDone:
				case <-time.After(5 * time.Second):
					t.Error("timed out waiting for the promotion claim during cleanup")
				}
			})
			require.NoError(t, waitForPostgresLock(ctx, observerDB, claimerPID, claimDone))
			require.NoError(t, moveTx.Commit())

			select {
			case result := <-claimResult:
				require.NoError(t, result.err)
				require.False(t, result.claimed, "a stale or legacy event must not claim the newer identified marker")
			case <-ctx.Done():
				t.Fatal("promotion claim did not finish after the move committed")
			}
			assertQueuePromotionMarker(t, repo, taskID, true, "step-newer-source")
		})
	}
}
