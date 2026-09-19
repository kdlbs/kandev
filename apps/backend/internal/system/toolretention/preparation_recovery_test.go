package toolretention

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestPreparationRecoversAfterFinishPersistenceFailure(t *testing.T) {
	for _, failure := range []string{"write", "start operation"} {
		t.Run(failure, func(t *testing.T) {
			s := testService(t)
			ctx := context.Background()
			calls := 0
			s.opts.CreateBackup = func(context.Context) (string, error) {
				calls++
				if failure == "write" {
					_, err := s.pool.Writer().Exec(`CREATE TRIGGER fail_finish BEFORE UPDATE ON settings BEGIN SELECT RAISE(FAIL, 'unavailable'); END`)
					require.NoError(t, err)
				}
				return "verified-receipt", nil
			}
			_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
			require.NoError(t, err)
			require.Error(t, s.tick(ctx)) // No tasks table also makes startOperation fail.
			if failure == "write" {
				_, err = s.pool.Writer().Exec(`DROP TRIGGER fail_finish`)
				require.NoError(t, err)
			}
			require.NoError(t, s.tick(ctx))
			status, err := s.Get(ctx)
			require.NoError(t, err)
			require.Equal(t, stateFailed, status.Preparation.State)
			require.Equal(t, stateFailed, status.Operation.State)
			require.False(t, status.Policy.Enabled)
			require.NoError(t, s.tick(ctx))
			require.Equal(t, 1, calls, "recovery must not silently repeat the backup or approve cleanup")
			_, err = s.Save(ctx, Update{Enabled: true, Age: status.Policy.Age, Revision: status.Policy.Revision, BackupChoice: choiceBackup})
			require.NoError(t, err, "preparation must be retryable")
		})
	}
}

func TestFailActiveTerminatesPreparation(t *testing.T) {
	for _, state := range []string{stateRunning, stateFailed, stateCancelled} {
		t.Run(state, func(t *testing.T) {
			s := testService(t)
			ctx := context.Background()
			_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
			require.NoError(t, err)
			_, err = s.change(ctx, func(r *record, _ *sqlx.Tx) error {
				r.Preparation.State = stateRunning
				r.Operation = &Operation{ID: "backup", Kind: choiceBackup, State: state}
				return nil
			})
			require.NoError(t, err)
			s.failActive(ctx)
			status, err := s.Get(ctx)
			require.NoError(t, err)
			require.Equal(t, stateFailed, status.Preparation.State)
			require.Equal(t, stateFailed, status.Operation.State)
			require.False(t, status.Policy.Enabled)
		})
	}
}

func TestPreparationRecoveryPreservesReplacement(t *testing.T) {
	for _, replacement := range []string{"revision", "operation", "disabled"} {
		t.Run(replacement, func(t *testing.T) {
			s := testService(t)
			ctx := context.Background()
			_, err := s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
			require.NoError(t, err)
			observed, err := s.change(ctx, func(r *record, _ *sqlx.Tx) error {
				r.Preparation.State = stateRunning
				r.Operation = &Operation{ID: "old-backup", Kind: choiceBackup, State: stateRunning}
				return nil
			})
			require.NoError(t, err)
			newer, err := s.change(ctx, func(r *record, _ *sqlx.Tx) error {
				switch replacement {
				case "revision":
					r.Policy.Revision++
				case "operation":
					r.Operation.ID = "new-backup"
				case "disabled":
					r.Policy.Revision++
					r.Preparation = Preparation{State: stateNone}
					r.Operation.State = stateCancelled
				}
				return nil
			})
			require.NoError(t, err)
			require.NoError(t, s.recoverPreparation(ctx, observed))
			actual, err := readRecord(ctx, s.pool.Reader())
			require.NoError(t, err)
			require.Equal(t, newer, actual)
		})
	}
}
