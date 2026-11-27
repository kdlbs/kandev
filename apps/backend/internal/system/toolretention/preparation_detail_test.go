package toolretention

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestLimitPreparationDetailInvalidUTF8(t *testing.T) {
	for _, message := range []string{
		"snapshot \xff failed",
		strings.Repeat("\xff", preparationDetailLimit),
		strings.Repeat("x", preparationDetailLimit-2) + "\xfftail",
	} {
		detail := limitPreparationDetail(message)
		if !utf8.ValidString(detail) || len(detail) > preparationDetailLimit {
			t.Errorf("detail must be valid UTF-8 within the byte limit: valid=%v bytes=%d", utf8.ValidString(detail), len(detail))
		}
		encoded, err := json.Marshal(detail)
		if err != nil {
			t.Fatal(err)
		}
		var stored string
		if err := json.Unmarshal(encoded, &stored); err != nil {
			t.Fatal(err)
		}
		if len(stored) > preparationDetailLimit {
			t.Errorf("persisted detail exceeds byte limit: %d", len(stored))
		}
	}
}

func failedPreparation(t *testing.T) (*Service, record) {
	t.Helper()
	s := testService(t)
	seedAnalysis(t, s)
	s.opts.CreateBackup = func(context.Context) (string, error) { return "", errors.New("snapshot failed") }
	_, err := s.Save(t.Context(), Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
	require.NoError(t, err)
	require.EqualError(t, s.stepPreparation(t.Context()), "backup_failed")
	r, err := readRecord(t.Context(), s.pool.Reader())
	require.NoError(t, err)
	require.Equal(t, "snapshot failed", r.PreparationDetail)
	return s, r
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestPreparationFailurePersistsBoundedUTF8(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	backupErr := errors.New(strings.Repeat("x\xff", preparationDetailLimit))
	s.opts.CreateBackup = func(context.Context) (string, error) { return "", backupErr }
	var logged error
	s.opts.Log = func(_ context.Context, _ string, err error) { logged = err }
	_, err := s.Save(t.Context(), Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
	require.NoError(t, err)
	require.EqualError(t, s.stepPreparation(t.Context()), "backup_failed")
	stored, err := readRecord(t.Context(), s.pool.Reader())
	require.NoError(t, err)
	require.True(t, utf8.ValidString(stored.PreparationDetail))
	require.LessOrEqual(t, len(stored.PreparationDetail), preparationDetailLimit)
	require.ErrorIs(t, logged, backupErr)
	status, err := s.Get(t.Context())
	require.NoError(t, err)
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "preparation_detail")
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestPreparationDetailClearedByRetrySuccessAndRecovery(t *testing.T) {
	for _, outcome := range []string{"pending", "success", "recovery"} {
		t.Run(outcome, func(t *testing.T) {
			s, failed := failedPreparation(t)
			if outcome == "pending" {
				_, err := s.Save(t.Context(), Update{Enabled: true, Age: failed.Policy.Age, Revision: failed.Policy.Revision, BackupChoice: choiceBackup})
				require.NoError(t, err)
			} else {
				prepared, err := s.change(t.Context(), func(r *record, _ *sqlx.Tx) error {
					r.Preparation.State = stateRunning
					r.Operation.State = stateRunning
					return nil
				})
				require.NoError(t, err)
				if outcome == "success" {
					require.NoError(t, s.finishPreparation(t.Context(), prepared, "verified-receipt", nil))
				} else {
					require.NoError(t, s.reconcile(t.Context()))
				}
			}
			stored, err := readRecord(t.Context(), s.pool.Reader())
			require.NoError(t, err)
			require.Empty(t, stored.PreparationDetail)
			want := map[string]string{"pending": statePending, "success": stateReady, "recovery": stateFailed}[outcome]
			require.Equal(t, want, stored.Preparation.State)
			if outcome == "recovery" {
				require.Equal(t, "backup_interrupted", stored.Preparation.Error)
			}
		})
	}
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestStalePreparationFailurePreservesCurrentDetail(t *testing.T) {
	for _, identity := range []string{"revision", "operation"} {
		t.Run(identity, func(t *testing.T) {
			s, failed := failedPreparation(t)
			current, err := s.change(t.Context(), func(r *record, _ *sqlx.Tx) error {
				r.Preparation.State = stateRunning
				r.Operation.State = stateRunning
				return nil
			})
			require.NoError(t, err)
			prepared := current
			if identity == "revision" {
				prepared.Policy.Revision--
			} else {
				op := *current.Operation
				op.ID = "obsolete-operation"
				prepared.Operation = &op
			}
			var logs int
			s.opts.Log = func(context.Context, string, error) { logs++ }
			require.EqualError(t, s.finishPreparation(t.Context(), prepared, "", errors.New("obsolete cause")), "backup_failed")
			stored, err := readRecord(t.Context(), s.pool.Reader())
			require.NoError(t, err)
			require.Equal(t, failed.PreparationDetail, stored.PreparationDetail)
			require.Equal(t, current.Policy.Revision, stored.Policy.Revision)
			require.Equal(t, current.Operation.ID, stored.Operation.ID)
			require.Equal(t, stateRunning, stored.Preparation.State)
			require.Zero(t, logs, "stale failures must not be logged as current failures")
		})
	}
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
func TestPreparationFailureLoggingRequiresCommittedFailure(t *testing.T) {
	for _, failure := range []string{"cancelled", "deadline", "persistence"} {
		t.Run(failure, func(t *testing.T) {
			s, _ := failedPreparation(t)
			prepared, err := s.change(t.Context(), func(r *record, _ *sqlx.Tx) error {
				r.Preparation.State = stateRunning
				r.Operation.State = stateRunning
				return nil
			})
			require.NoError(t, err)
			backupErr := errors.New("snapshot failed")
			ctx := t.Context()
			switch failure {
			case "cancelled":
				backupErr = context.Canceled
			case "deadline":
				backupErr = context.DeadlineExceeded
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Hour))
				defer cancel()
			default:
				_, err := s.pool.Writer().Exec(`CREATE TRIGGER fail_retention_write BEFORE UPDATE OF value ON settings BEGIN SELECT RAISE(ABORT, 'write rejected'); END`)
				require.NoError(t, err)
			}
			var logs int
			s.opts.Log = func(context.Context, string, error) { logs++ }
			require.Error(t, s.finishPreparation(ctx, prepared, "", backupErr))
			require.Zero(t, logs)
		})
	}
}
