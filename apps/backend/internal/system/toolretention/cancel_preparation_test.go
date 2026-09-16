package toolretention

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type blockedPreparation struct {
	service     *Service
	status      Status
	interrupted <-chan struct{}
	stopped     <-chan struct{}
}

func startWriterBlockedPreparation(t *testing.T, onInterrupted ...func()) blockedPreparation {
	t.Helper()
	s := testService(t)
	entered := make(chan struct{})
	interrupted := make(chan struct{})
	done := make(chan error, 1)
	stopped := make(chan struct{})
	worker, cancel := context.WithCancel(context.Background())
	s.opts.CreateBackup = func(ctx context.Context) (string, error) {
		conn, err := s.pool.Writer().Connx(ctx)
		if err != nil {
			return "", err
		}
		defer conn.Close() //nolint:errcheck // The connection is returned to the test pool.
		close(entered)
		<-ctx.Done()
		for _, fn := range onInterrupted {
			fn()
		}
		close(interrupted)
		return "", ctx.Err()
	}
	_, err := s.Save(context.Background(), Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
	require.NoError(t, err)
	go func() { done <- s.tick(worker); close(stopped) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("backup worker did not stop")
		}
	})
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("backup did not start: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("backup did not acquire writer")
	}
	status, err := s.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, stateRunning, status.Preparation.State)
	return blockedPreparation{s, status, interrupted, stopped}
}

func TestInterruptedPreparationPersistenceFailureStaysDisabled(t *testing.T) {
	for _, action := range []string{"cancel", "disable"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			blocked := startWriterBlockedPreparation(t, cancel)
			var err error
			if action == "cancel" {
				_, err = blocked.service.Cancel(ctx, blocked.status.Operation.ID)
			} else {
				_, err = blocked.service.Save(ctx, Update{Age: blocked.status.Policy.Age, Revision: blocked.status.Policy.Revision})
			}
			require.ErrorIs(t, err, context.Canceled)
			r, err := readRecord(context.Background(), blocked.service.pool.Reader())
			require.NoError(t, err)
			require.False(t, r.Policy.Enabled)
			require.Zero(t, r.ApprovedRevision)
			require.False(t, approved(&r))
			require.Equal(t, stateRunning, r.Preparation.State)
			require.Equal(t, choiceBackup, r.Operation.Kind)
			select {
			case <-blocked.stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("backup worker did not stop")
			}
			require.NoError(t, blocked.service.tick(context.Background()))
			status, err := blocked.service.Get(context.Background())
			require.NoError(t, err)
			require.Equal(t, stateFailed, status.Preparation.State)
			require.Equal(t, stateFailed, status.Operation.State)
			require.False(t, status.Policy.Enabled)
			_, err = blocked.service.Save(context.Background(), Update{Enabled: true, Age: status.Policy.Age, Revision: status.Policy.Revision, BackupChoice: choiceBackup})
			require.NoError(t, err, "normal worker recovery must make preparation retryable")
		})
	}
}

func TestPreparationInterruptionDoesNotCancelNewerWork(t *testing.T) {
	for _, changed := range []string{"operation", "revision"} {
		t.Run(changed, func(t *testing.T) {
			s := testService(t)
			_, err := s.Save(context.Background(), Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
			require.NoError(t, err)
			observed, err := s.change(context.Background(), func(r *record, _ *sqlx.Tx) error {
				r.Preparation.State = stateRunning
				r.Operation = &Operation{ID: "original", Kind: choiceBackup, State: stateRunning}
				return nil
			})
			require.NoError(t, err)
			newer, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.activeCancel = cancel
			s.activeRevision = observed.Policy.Revision
			s.activeID = observed.Operation.ID
			_, err = s.interruptValidated(context.Background(), func(r *record) error {
				_, updateErr := s.change(context.Background(), func(current *record, _ *sqlx.Tx) error {
					if changed == "operation" {
						current.Operation.ID = "newer-operation"
					} else {
						current.Policy.Revision++
					}
					s.activeID = current.Operation.ID
					s.activeRevision = current.Policy.Revision
					return nil
				})
				return updateErr
			})
			if changed == "operation" {
				require.ErrorContains(t, err, "conflict")
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, newer.Err(), "stale observation must not cancel new work")
		})
	}
}

func TestPreparationHandoffInterruptsWriterHeldByBackup(t *testing.T) {
	for _, id := range []string{"", "previous-operation"} {
		for _, action := range []string{"cancel", "disable"} {
			t.Run(id+"/"+action, func(t *testing.T) {
				blocked := startWriterBlockedPreparation(t)
				// Reproduce the interval between the durable operation and activeID publication.
				blocked.service.mu.Lock()
				blocked.service.activeID = id
				blocked.service.mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var status Status
				var err error
				if action == "cancel" {
					status, err = blocked.service.Cancel(ctx, blocked.status.Operation.ID)
				} else {
					status, err = blocked.service.Save(ctx, Update{Age: blocked.status.Policy.Age, Revision: blocked.status.Policy.Revision})
				}
				require.NoError(t, err)
				require.Equal(t, stateCancelled, status.Operation.State)
				require.Equal(t, stateNone, status.Preparation.State)
			})
		}
	}
}

func TestCancelPreparationInterruptsWriterHeldByBackup(t *testing.T) {
	for _, action := range []string{"cancel", "disable"} {
		t.Run(action, func(t *testing.T) {
			blocked := startWriterBlockedPreparation(t)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var status Status
			var err error
			if action == "cancel" {
				status, err = blocked.service.Cancel(ctx, blocked.status.Operation.ID)
			} else {
				status, err = blocked.service.Save(ctx, Update{Age: blocked.status.Policy.Age, Revision: blocked.status.Policy.Revision})
			}
			require.NoError(t, err, "request must interrupt the backup before acquiring its writer")
			select {
			case <-blocked.interrupted:
			default:
				t.Fatal("backup was not interrupted")
			}
			require.False(t, status.Policy.Enabled)
			require.Equal(t, stateNone, status.Preparation.State)
			require.Equal(t, stateCancelled, status.Operation.State)
		})
	}
}

func TestInvalidPreparationRequestsDoNotInterruptBackup(t *testing.T) {
	blocked := startWriterBlockedPreparation(t)
	for _, action := range []string{"wrong operation", "stale revision", "missing backup choice", "invalid age", "invalid choice"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var err error
			want := "conflict"
			update := Update{Age: blocked.status.Policy.Age, Revision: blocked.status.Policy.Revision}
			switch action {
			case "wrong operation":
				_, err = blocked.service.Cancel(ctx, "different-operation")
			case "stale revision":
				update.Revision--
				_, err = blocked.service.Save(ctx, update)
			case "missing backup choice":
				update.Enabled = true
				want = "backup_choice_required"
				_, err = blocked.service.Save(ctx, update)
			case "invalid age":
				update.Age.Value = 0
				want = "invalid_age"
				_, err = blocked.service.Save(ctx, update)
			case "invalid choice":
				update.BackupChoice = "invalid"
				want = "invalid_backup_choice"
				_, err = blocked.service.Save(ctx, update)
			}
			require.ErrorContains(t, err, want)
			select {
			case <-blocked.interrupted:
				t.Fatal("invalid request interrupted backup")
			default:
			}
		})
	}
}

func TestPreparationInterruptionRetainsTransactionalCAS(t *testing.T) {
	for _, action := range []string{"cancel revision", "cancel operation", "disable revision"} {
		t.Run(action, func(t *testing.T) {
			s := testService(t)
			_, err := s.Save(context.Background(), Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
			require.NoError(t, err)
			observed, err := s.change(context.Background(), func(r *record, _ *sqlx.Tx) error {
				r.Preparation.State = stateRunning
				r.Operation = &Operation{ID: "original", Kind: choiceBackup, State: stateRunning}
				return nil
			})
			require.NoError(t, err)
			s.activeID = observed.Operation.ID
			s.activeRevision = observed.Policy.Revision
			var replacement record
			s.activeCancel = func() {
				// Commit a competing request after validation and before the writer CAS.
				var updateErr error
				replacement, updateErr = s.change(context.Background(), func(r *record, _ *sqlx.Tx) error {
					if action == "cancel operation" {
						r.Operation.ID = "replacement"
					} else {
						r.Policy.Revision++
					}
					return nil
				})
				require.NoError(t, updateErr)
			}
			if action == "disable revision" {
				_, err = s.Save(context.Background(), Update{Age: observed.Policy.Age, Revision: observed.Policy.Revision})
			} else {
				_, err = s.Cancel(context.Background(), observed.Operation.ID)
			}
			require.ErrorContains(t, err, "conflict")
			current, err := readRecord(context.Background(), s.pool.Reader())
			require.NoError(t, err)
			require.Equal(t, replacement, current, "stale request must not change durable state")
		})
	}
}

func TestPendingPreparationBecomingRunningCanBeInterrupted(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(map[bool]string{false: "no operation", true: "cancelled operation"}[previous], func(t *testing.T) {
			s := testService(t)
			status, err := s.Save(context.Background(), Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: choiceBackup})
			require.NoError(t, err)
			if previous {
				_, err = s.change(context.Background(), func(r *record, _ *sqlx.Tx) error {
					r.Operation = &Operation{ID: "previous", Kind: choiceBackup, State: stateCancelled}
					return nil
				})
				require.NoError(t, err)
			}
			entered := make(chan struct{})
			done := make(chan error, 1)
			worker, stop := context.WithCancel(context.Background())
			defer stop()
			s.opts.CreateBackup = func(ctx context.Context) (string, error) {
				conn, err := s.pool.Writer().Connx(ctx)
				if err != nil {
					return "", err
				}
				defer conn.Close() //nolint:errcheck // Return the connection after cancellation.
				close(entered)
				<-ctx.Done()
				return "", ctx.Err()
			}
			t.Cleanup(func() {
				stop()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("backup worker did not stop")
				}
			})
			update := Update{Age: status.Policy.Age, Revision: status.Policy.Revision}
			_, err = s.interruptValidated(context.Background(), func(r *record) error {
				require.Equal(t, statePending, r.Preparation.State)
				_, validationErr := validatePolicyUpdate(r, update, s.opts.Now())
				go func() { done <- s.tick(worker) }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("backup did not acquire writer")
				}
				return validationErr
			})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := s.pool.Writer().Connx(ctx)
			require.NoError(t, err, "pending observation must interrupt the now-running backup")
			require.NoError(t, conn.Close())
			status, err = s.Save(ctx, update)
			require.NoError(t, err)
			require.False(t, status.Policy.Enabled)
			require.Equal(t, stateNone, status.Preparation.State)
		})
	}
}
