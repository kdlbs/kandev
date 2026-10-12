package toolretention

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

func approveTestPolicy(t *testing.T, s *Service) int64 {
	t.Helper()
	ctx := context.Background()
	status, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "skip"})
	require.NoError(t, err)
	_, err = s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		r.Policy.Enabled = true
		r.Preparation.State = "ready"
		r.ApprovedRevision = r.Policy.Revision
		return nil
	})
	require.NoError(t, err)
	return status.Policy.Revision
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3, 002.5, 003.3
func TestCleanupPreservesMetadataAndRechecksActivity(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "protect"}[active], func(t *testing.T) {
			s := testService(t)
			before := seedAnalysis(t, s)
			rev := approveTestPolicy(t, s)
			ctx := context.Background()
			_, err := s.Run(ctx, rev)
			require.NoError(t, err)
			if active {
				_, err = s.pool.Writer().Exec(`UPDATE task_sessions SET state='RUNNING'`)
				require.NoError(t, err)
			}
			for i := 0; i < 10; i++ {
				require.NoError(t, s.stepScan(ctx))
				status, err := s.Get(ctx)
				require.NoError(t, err)
				if status.Operation.State != "running" {
					break
				}
			}
			status, err := s.Get(ctx)
			require.NoError(t, err)
			require.Equal(t, "succeeded", status.Operation.State)
			var raw, updated string
			require.NoError(t, s.pool.Reader().QueryRow(`SELECT metadata,CAST(updated_at AS TEXT) FROM task_session_messages WHERE id='message'`).Scan(&raw, &updated))
			require.Equal(t, "2020-01-01", updated)
			if active {
				require.Equal(t, before, raw)
				require.Zero(t, status.Operation.RemovedMessages)
				return
			}
			var metadata map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &metadata))
			require.Contains(t, metadata, "payload_retention")
			require.Equal(t, "tool1", metadata["tool_call_id"])
			require.Equal(t, "echo result", metadata["title"])
			require.EqualValues(t, 1, status.Operation.RemovedMessages)
			require.EqualValues(t, len(before)-len(raw), status.Operation.PayloadBytes)
		})
	}
}

func TestDisableStopsUncommittedCleanupAndRestartKeepsCounts(t *testing.T) {
	s := testService(t)
	before := seedAnalysis(t, s)
	rev := approveTestPolicy(t, s)
	ctx := context.Background()
	_, err := s.Run(ctx, rev)
	require.NoError(t, err)
	_, err = s.Save(ctx, Update{Age: Age{3, "months"}, Revision: rev})
	require.NoError(t, err)
	reopened := New(s.pool)
	require.NoError(t, reopened.stepScan(ctx))
	var raw string
	require.NoError(t, s.pool.Reader().Get(&raw, `SELECT metadata FROM task_session_messages WHERE id='message'`))
	require.Equal(t, before, raw)
	status, err := reopened.Get(ctx)
	require.NoError(t, err)
	require.False(t, status.Policy.Enabled)
	require.Equal(t, "cancelled", status.Operation.State)
}

func TestEmptyCleanupDoesNotReverifyTheWholeBackup(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	s.opts.CreateBackup = func(context.Context) (string, error) { return "verified-receipt", nil }
	verified := 0
	s.opts.VerifyBackup = func(context.Context, string) error { verified++; return nil }
	_, err := s.pool.Writer().Exec(`UPDATE task_sessions SET state='RUNNING'`)
	require.NoError(t, err)
	_, err = s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "backup"})
	require.NoError(t, err)
	require.NoError(t, s.stepPreparation(ctx))
	require.NoError(t, s.stepScan(ctx))
	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "succeeded", status.Operation.State)
	require.Zero(t, verified)
	require.Zero(t, status.Operation.RemovedMessages)
}
