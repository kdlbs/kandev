package toolretention

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"testing"
	"time"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.1, 003.3, 003.4
func TestDailyScheduleAndRestart(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	approveTestPolicy(t, s)
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	s.opts.Now = func() time.Time { return now }
	_, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error { next := now.Add(time.Hour); r.NextDueAt = &next; return nil })
	require.NoError(t, err)
	require.NoError(t, s.stepScheduled(ctx))
	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Nil(t, status.Operation)
	now = now.Add(time.Hour)
	require.NoError(t, s.stepScheduled(ctx))
	status, err = s.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, status.Operation)
	reopened := New(s.pool, Options{Now: func() time.Time { return now }})
	require.NoError(t, reopened.reconcile(ctx))
	require.NoError(t, reopened.stepScan(ctx))
	status, err = reopened.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "succeeded", status.Operation.State)
	require.Equal(t, now.Add(24*time.Hour), *status.NextDueAt)
	require.NoError(t, reopened.stepScan(ctx))
	again, err := reopened.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, status.Operation.RemovedMessages, again.Operation.RemovedMessages)
}

func TestInterruptedBackupRequiresExplicitRetry(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "backup"})
	require.NoError(t, err)
	_, err = s.change(ctx, func(r *record, tx *sqlx.Tx) error { r.Preparation.State = "running"; return nil })
	require.NoError(t, err)
	reopened := New(s.pool)
	require.NoError(t, reopened.reconcile(ctx))
	status, err := reopened.Get(ctx)
	require.NoError(t, err)
	require.False(t, status.Policy.Enabled)
	require.Equal(t, "failed", status.Preparation.State)
	require.Equal(t, "backup_interrupted", status.Preparation.Error)
}

func TestRuntimeStartStopDoesNotRunMaintenanceDuringReadiness(t *testing.T) {
	s := testService(t)
	for i := 0; i < 2; i++ {
		s.Start(context.Background())
		s.Start(context.Background())
		s.mu.Lock()
		running := s.cancel != nil
		s.mu.Unlock()
		require.True(t, running)
		s.Stop()
		s.Stop()
	}
	status, err := s.Get(context.Background())
	require.NoError(t, err)
	require.Nil(t, status.Operation)
}

func TestWorkBudgetStartsWhenWorkArrivesAfterIdle(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	s.opts.Now = func() time.Time { return now }
	var burst time.Time
	require.Equal(t, time.Minute, s.nextDelay(ctx, nil, &burst))
	require.True(t, burst.IsZero())
	_, err := s.Analyze(ctx, Age{3, "months"})
	require.NoError(t, err)
	require.Equal(t, 100*time.Millisecond, s.nextDelay(ctx, nil, &burst))
	require.Equal(t, now, burst)
	now = now.Add(30 * time.Second)
	require.Equal(t, time.Minute, s.nextDelay(ctx, nil, &burst))
	require.True(t, burst.IsZero())
}

func TestMaintenanceAdmissionDoesNotCountAsSchedulerFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous int
		err      error
		want     int
	}{
		{name: "initial deferral", previous: 0, err: errMaintenanceBusy, want: 0},
		{name: "preserves earlier failures", previous: 4, err: errMaintenanceBusy, want: 4},
		{name: "ordinary failure", previous: 4, err: errors.New("scan failed"), want: 5},
		{name: "success clears failures", previous: 4, err: nil, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, nextFailureCount(tc.previous, tc.err))
		})
	}
}
