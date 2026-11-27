package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-FIELD-UPDATES-001.8, AC-TASKS-FIELD-UPDATES-001.9, AC-TASKS-FIELD-UPDATES-001.10, AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergePostgresPhysicalWait(t *testing.T) {
	for _, variant := range []string{"alpha_first", "port_first", "title_owner", "cancel"} {
		t.Run(variant, func(t *testing.T) {
			a, b, observer := newHierarchyPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "merge-ws", Name: "Merge"}))
			require.NoError(t, a.CreateTask(ctx, &models.Task{ID: "subject", WorkspaceID: "merge-ws", Title: "Original", Metadata: map[string]interface{}{"keep": true}}))
			before, err := b.GetTask(ctx, "subject")
			require.NoError(t, err)
			pid := hierarchyBackendPID(t, b.DB())
			holder, err := a.DB().BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = holder.Rollback() }()
			// No workspace or metadata advisory lock: every physical row writer participates.
			_, err = holder.ExecContext(ctx, `SELECT id FROM tasks WHERE id='subject' FOR UPDATE`)
			require.NoError(t, err)
			overlay := map[string]interface{}{"port_forwarding_enabled": true}
			current := map[string]interface{}{"keep": true, "alpha": "current"}
			title := "Original"
			if variant == "port_first" {
				overlay = map[string]interface{}{"alpha": "current"}
				current = map[string]interface{}{"keep": true, "port_forwarding_enabled": true}
			}
			if variant == "title_owner" {
				title = "Current title"
				current[models.MetaKeyDeferredLaunch] = map[string]interface{}{"prompt": "current"}
				current[models.MetaKeyAgentTitlePending] = true
				current[models.MetaKeyAgentTitleOwnerSessionID] = "owner"
				current["nullable"] = nil
				overlay[models.MetaKeyDeferredLaunch] = "forged"
				overlay[models.MetaKeyAgentTitleOwnerSessionID] = "forged"
			}
			waiterCtx, cancelWaiter := context.WithCancel(ctx)
			result := make(chan error, 1)
			go func() { result <- b.MergeTaskMetadata(waiterCtx, "subject", overlay) }()
			joined := false
			defer func() {
				cancelWaiter()
				_ = holder.Rollback()
				if !joined {
					<-result
				}
			}()
			waitHierarchyPostgresLocks(t, holder, pid, 1, variant)
			var waitType, waitEvent string
			var blockers int
			err = observer.QueryRowContext(ctx, `SELECT wait_event_type,wait_event,cardinality(pg_blocking_pids(pid)) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waitType, &waitEvent, &blockers)
			require.NoError(t, err)
			require.Equal(t, "Lock", waitType)
			require.Positive(t, blockers)
			t.Logf("%s: physical task-row wait PID=%d %s/%s blockers=%d", variant, pid, waitType, waitEvent, blockers)
			if variant == "cancel" {
				cancelWaiter()
				err = <-result
				joined = true
				require.ErrorIs(t, err, context.Canceled)
				require.NoError(t, holder.Commit())
				got, err := b.GetTask(ctx, "subject")
				require.NoError(t, err)
				require.Equal(t, before, got)
				return
			}
			encoded, err := json.Marshal(current)
			require.NoError(t, err)
			_, err = holder.ExecContext(ctx, `UPDATE tasks SET title=$1,metadata=$2,updated_at=CURRENT_TIMESTAMP WHERE id='subject'`, title, string(encoded))
			require.NoError(t, err)
			require.NoError(t, holder.Commit())
			err = <-result
			joined = true
			require.NoError(t, err)
			got, err := b.GetTask(ctx, "subject")
			require.NoError(t, err)
			require.Equal(t, title, got.Title)
			require.True(t, got.UpdatedAt.After(before.UpdatedAt))
			require.Equal(t, "current", got.Metadata["alpha"])
			require.Equal(t, true, got.Metadata["port_forwarding_enabled"])
			require.Equal(t, true, got.Metadata["keep"])
			if variant == "title_owner" {
				require.Equal(t, "current", got.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})["prompt"])
				require.True(t, models.IsAgentTitleOwner(got.Metadata, "owner"))
				require.Contains(t, got.Metadata, "nullable")
			}
			before.Title, before.Metadata, before.UpdatedAt = got.Title, got.Metadata, got.UpdatedAt
			require.Equal(t, before, got)
		})
	}
}
