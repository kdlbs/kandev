package toolretention

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCleanupCommitsBoundedProgressAndResumesWithoutDoubleCounting(t *testing.T) {
	s := testService(t)
	raw := seedAnalysis(t, s)
	ctx := context.Background()
	for i := 0; i < 249; i++ {
		_, err := s.pool.Writer().Exec(`INSERT INTO task_session_messages(id,task_session_id,task_id,created_at,updated_at,type,metadata,requests_input) VALUES(?,'session','task','2020-01-01','2020-01-01','tool_execute',?,0)`, fmt.Sprintf("message-%03d", i), raw)
		require.NoError(t, err)
	}
	rev := approveTestPolicy(t, s)
	_, err := s.Run(ctx, rev)
	require.NoError(t, err)
	require.NoError(t, s.stepScan(ctx))
	first, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "running", first.Operation.State)
	require.Greater(t, first.Operation.RemovedMessages, int64(0))
	require.LessOrEqual(t, first.Operation.Scanned, int64(batchRows))
	reopened := New(s.pool)
	for i := 0; i < 20; i++ {
		require.NoError(t, reopened.stepScan(ctx))
		got, getErr := reopened.Get(ctx)
		require.NoError(t, getErr)
		if got.Operation.State != "running" {
			break
		}
	}
	final, err := reopened.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "succeeded", final.Operation.State)
	require.EqualValues(t, 250, final.Operation.RemovedMessages)
	require.EqualValues(t, 1, final.Operation.EligibleTasks)
	require.NoError(t, reopened.stepScan(ctx))
	again, err := reopened.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, final.Operation, again.Operation)
}

func TestCancellationDoesNotCancelReplacementOperation(t *testing.T) {
	s := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.activeCancel = cancel
	s.activeRevision = 2
	s.activeID = "replacement"
	s.cancelOlder(2)
	require.NoError(t, ctx.Err())
	s.cancelOperation("old-operation")
	require.NoError(t, ctx.Err())
	s.cancelOlder(3)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestCompactionInvalidatesAnUnfinishedCursor(t *testing.T) {
	s := testService(t)
	raw := seedAnalysis(t, s)
	ctx := context.Background()
	for i := 0; i < 149; i++ {
		_, err := s.pool.Writer().Exec(`INSERT INTO task_session_messages(id,task_session_id,task_id,created_at,updated_at,type,metadata,requests_input) VALUES(?,'session','task','2020-01-01','2020-01-01','tool_execute',?,0)`, fmt.Sprintf("message-%03d", i), raw)
		require.NoError(t, err)
	}
	_, err := s.Analyze(ctx, Age{3, "months"})
	require.NoError(t, err)
	require.NoError(t, s.stepScan(ctx))
	before, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "running", before.Operation.State)
	_, err = s.pool.Writer().Exec(`VACUUM`)
	require.NoError(t, err)
	require.NoError(t, s.stepScan(ctx))
	after, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "partial", after.Operation.State)
	require.Equal(t, "database_changed", after.Operation.Error)
	require.Equal(t, before.Operation.EligibleMessages, after.Operation.EligibleMessages)
}
