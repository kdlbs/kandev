package sqlite_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

func waitManagedDeletionPostgres(t *testing.T, ctx context.Context, observer *sqlx.DB, holderPID, waiterPID int) {
	t.Helper()
	for {
		var waiting bool
		require.NoError(t, observer.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND NOT granted AND locktype='transactionid') AND $2=ANY(pg_blocking_pids($1))`, waiterPID, holderPID).Scan(&waiting))
		if waiting {
			t.Logf("physical transaction lock: holder PID=%d, waiter PID=%d, observer PID=%d", holderPID, waiterPID, hierarchyBackendPID(t, observer.DB))
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("native PG writer never physically waited", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.1, AC-PLUGINS-MANAGED-COORDINATION-013.2, AC-PLUGINS-MANAGED-COORDINATION-013.3, AC-PLUGINS-MANAGED-COORDINATION-013.4, AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedDeletionAdmissionPostgresWaits(t *testing.T) {
	for _, boundary := range []string{"admit", "final"} {
		for _, mode := range []string{"revision", "detach", "identity", "replacement", "missing", "cancel", "owner"} {
			t.Run(boundary+"_"+mode, func(t *testing.T) {
				a, b, observer := newManagedAdmissionPostgresRepoPair(t)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "pgx")
				var claim *managed.DeleteClaim
				var err error
				if boundary == "final" || mode == "owner" {
					claim, err = a.AdmitManagedDeletion(ctx, request, "owner")
					require.NoError(t, err)
					if boundary == "final" {
						_, err = a.PrepareManagedDeletion(ctx, *claim, nativeDeletionInventory(t, ctx, a, request))
						require.NoError(t, err)
					}
				}
				holderPID, waiterPID := hierarchyBackendPID(t, a.DB()), hierarchyBackendPID(t, b.DB())
				holder, err := a.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
				require.NoError(t, err)
				waiterCtx, cancelWaiter := context.WithCancel(ctx)
				var workers sync.WaitGroup
				t.Cleanup(func() { cancelWaiter(); cancel(); _ = holder.Rollback(); workers.Wait() })
				var isolation string
				require.NoError(t, holder.QueryRowContext(ctx, `SHOW transaction_isolation`).Scan(&isolation))
				require.Equal(t, "read committed", isolation)
				if mode == "owner" {
					_, err = holder.ExecContext(ctx, `SELECT id FROM task_resource_cleanup_jobs WHERE id=$1 FOR UPDATE`, claim.JobID)
				} else {
					_, err = holder.ExecContext(ctx, `SELECT id FROM tasks WHERE id=$1 FOR UPDATE`, request.TaskID)
				}
				require.NoError(t, err)
				done := make(chan error, 1)
				workers.Add(1)
				go func() {
					defer workers.Done()
					if boundary == "admit" {
						_, err := b.AdmitManagedDeletion(waiterCtx, request, "waiter")
						done <- err
					} else {
						_, err := b.FinalizeManagedDeletion(waiterCtx, *claim)
						done <- err
					}
				}()
				waitManagedDeletionPostgres(t, ctx, observer, holderPID, waiterPID)
				switch mode {
				case "revision":
					_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=jsonb_set(metadata::jsonb,'{"kandev.conversation_revision"}','"2"')::text WHERE id=$1`, request.TaskID)
				case "detach":
					_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=jsonb_set(metadata::jsonb,'{"kandev.detached"}','true')::text WHERE id=$1`, request.TaskID)
				case "identity":
					_, err = holder.ExecContext(ctx, `UPDATE tasks SET metadata=jsonb_set(metadata::jsonb,'{"kandev.installation_id"}','"foreign"')::text WHERE id=$1`, request.TaskID)
				case "replacement":
					_, err = holder.ExecContext(ctx, `UPDATE tasks SET created_at=created_at+INTERVAL '1 second' WHERE id=$1`, request.TaskID)
				case "missing":
					_, err = holder.ExecContext(ctx, `DELETE FROM tasks WHERE id=$1`, request.TaskID)
				case "owner":
					_, err = holder.ExecContext(ctx, `UPDATE task_resource_cleanup_jobs SET resource_snapshot=jsonb_set(resource_snapshot::jsonb,'{managed_delete,owner}','"foreign-owner"')::text WHERE id=$1`, claim.JobID)
				case "cancel":
					cancelWaiter()
				}
				require.NoError(t, err)
				if mode != "cancel" {
					require.NoError(t, holder.Commit())
				}
				select {
				case err = <-done:
				case <-ctx.Done():
					t.Fatal("native PG waiter did not settle", ctx.Err())
				}
				workers.Wait()
				if mode == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
					require.NoError(t, holder.Commit())
				} else {
					switch mode {
					case "revision":
						require.ErrorIs(t, err, managed.ErrRevision)
					case "owner":
						require.ErrorIs(t, err, managed.ErrDeletionOwned)
					case "missing":
						if boundary == "final" {
							require.ErrorIs(t, err, repoerrors.ErrTaskNotFound)
						} else {
							require.ErrorIs(t, err, managed.ErrNotFound)
						}
					default:
						require.ErrorIs(t, err, managed.ErrNotFound)
					}
				}
				if mode != "missing" {
					assertNativeDeletionRows(t, ctx, b, request, sessionID)
				} else {
					_, err = b.GetTask(ctx, request.TaskID)
					require.Error(t, err)
				}
				current, job, err := b.InspectManagedDeletion(ctx, request)
				require.NoError(t, err)
				if claim == nil {
					require.Nil(t, current)
					require.Nil(t, job)
				} else {
					require.NotEqual(t, managed.DeleteCommitted, current.Phase)
					require.Equal(t, models.TaskResourceCleanupStatePrepared, job.State)
					if mode == "owner" {
						require.Equal(t, "foreign-owner", current.Owner)
					}
					started, err := b.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
					require.ErrorIs(t, err, managed.ErrDeletionOwned)
					require.False(t, started)
				}
				t.Logf("%s_%s current guard after real wait: no task deletion or committed marker, isolation=%s", boundary, mode, isolation)
			})
		}
	}
	t.Run("deletion_wins_independent_revision_and_detach", func(t *testing.T) {
		a, b, _ := newManagedAdmissionPostgresRepoPair(t)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "pgx")
		claim, err := a.AdmitManagedDeletion(ctx, request, "owner")
		require.NoError(t, err)
		database := sqlx.NewDb(b.DB(), "pgx")
		store, err := state.NewStore(db.NewPool(database, database))
		require.NoError(t, err)
		other := taskservice.NewAgentConversationService(b, b, nil, store, nil)
		spec := pluginsdk.ManagedAgentConversationSpec{WorkspaceID: request.WorkspaceID, InstanceKey: request.InstanceKey, ExpectedRevision: 1, AgentProfileID: "profile", BasePrompt: "independent writer", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
		_, _, err = other.EnsureManaged(ctx, "plugin", "installation", spec, "update", "update-digest")
		require.Error(t, err)
		require.Error(t, other.DetachManagedForInstallation(ctx, "installation"))
		assertNativeDeletionRows(t, ctx, b, request, sessionID)
		task, err := b.GetTask(ctx, request.TaskID)
		require.NoError(t, err)
		require.Equal(t, uint64(1), managed.Revision(task))
		require.Equal(t, false, task.Metadata[models.MetaKeyManagedConversationDetached])
		_, err = a.PrepareManagedDeletion(ctx, *claim, nativeDeletionInventory(t, ctx, a, request))
		require.NoError(t, err)
		_, err = a.FinalizeManagedDeletion(ctx, *claim)
		require.NoError(t, err)
		_, err = b.GetTask(ctx, request.TaskID)
		require.Error(t, err)
		_, err = b.GetTaskSession(ctx, sessionID)
		require.Error(t, err)
		current, job, err := b.InspectManagedDeletion(ctx, request)
		require.NoError(t, err)
		require.Equal(t, managed.DeleteCommitted, current.Phase)
		started, err := b.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, started)
		started, err = b.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
		require.NoError(t, err)
		require.False(t, started)
	})
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.4, AC-PLUGINS-MANAGED-COORDINATION-013.5, AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedDeletionAdmissionPostgresRollback(t *testing.T) {
	for _, boundary := range []string{"admit", "final"} {
		t.Run(boundary, func(t *testing.T) {
			a, b, _ := newManagedAdmissionPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "pgx")
			var notified atomic.Int32
			a.SetTaskQueuePurgeNotifier(func(context.Context, string) { notified.Add(1) })
			var claim *managed.DeleteClaim
			var err error
			if boundary == "final" {
				claim, err = a.AdmitManagedDeletion(ctx, request, "owner")
				require.NoError(t, err)
				_, err = a.PrepareManagedDeletion(ctx, *claim, nativeDeletionInventory(t, ctx, a, request))
				require.NoError(t, err)
			}
			_, err = a.DB().ExecContext(ctx, `CREATE FUNCTION fail_native_managed_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'native rollback'; END $$`)
			require.NoError(t, err)
			table, event := "task_resource_cleanup_jobs", "INSERT"
			if boundary == "final" {
				table, event = "tasks", "DELETE"
			}
			_, err = a.DB().ExecContext(ctx, `CREATE TRIGGER fail_native_managed_delete BEFORE `+event+` ON `+table+` FOR EACH ROW EXECUTE FUNCTION fail_native_managed_delete()`)
			require.NoError(t, err)
			if boundary == "admit" {
				_, err = a.AdmitManagedDeletion(ctx, request, "owner")
			} else {
				_, err = a.FinalizeManagedDeletion(ctx, *claim)
			}
			require.Error(t, err)
			assertNativeDeletionRows(t, ctx, b, request, sessionID)
			require.Zero(t, notified.Load())
			current, job, err := b.InspectManagedDeletion(ctx, request)
			require.NoError(t, err)
			if boundary == "admit" {
				require.Nil(t, current)
				require.Nil(t, job)
			} else {
				require.Equal(t, managed.DeletePrepared, current.Phase)
				require.Equal(t, models.TaskResourceCleanupStatePrepared, job.State)
				started, err := b.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				require.False(t, started)
				foreign := *claim
				foreign.Owner = "foreign"
				_, err = b.ReleaseManagedDeletion(ctx, foreign)
				require.ErrorIs(t, err, managed.ErrDeletionOwned)
				released, err := a.ReleaseManagedDeletion(ctx, *claim)
				require.NoError(t, err)
				require.True(t, released)
			}
			_, err = a.DB().ExecContext(ctx, `DROP TRIGGER fail_native_managed_delete ON `+table)
			require.NoError(t, err)
			claim, err = a.AdmitManagedDeletion(ctx, request, "new-owner")
			require.NoError(t, err)
			_, err = a.PrepareManagedDeletion(ctx, *claim, nativeDeletionInventory(t, ctx, a, request))
			require.NoError(t, err)
			_, err = a.FinalizeManagedDeletion(ctx, *claim)
			require.NoError(t, err)
			require.Equal(t, int32(1), notified.Load())
			current, job, err = b.InspectManagedDeletion(ctx, request)
			require.NoError(t, err)
			require.Equal(t, managed.DeleteCommitted, current.Phase)
			_, err = b.GetTask(ctx, request.TaskID)
			require.Error(t, err)
			started, err := b.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, started)
			t.Log("native rollback preserved task/primary/transcript and rejected effects; real commit marked deletion and notified once")
		})
	}
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedDeletionAdmissionPostgresSnapshotAuthority(t *testing.T) {
	a, b, _ := newManagedAdmissionPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "pgx")
	job := &models.TaskResourceCleanupJob{OperationID: "ordinary", TaskID: request.TaskID, Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared, ResourceSnapshot: "{}"}
	require.NoError(t, a.CreateTaskResourceCleanupJob(ctx, job))
	forged, err := managed.WithDeletionEnvelope("{}", managed.DeleteClaim{DeleteRequest: request, Version: 1, JobID: job.ID, Owner: "forged", Phase: managed.DeleteCommitted})
	require.NoError(t, err)
	require.ErrorIs(t, b.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, forged), managed.ErrUnavailable)
	stored, err := a.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "{}", stored.ResourceSnapshot)
	require.Equal(t, models.TaskResourceCleanupStatePrepared, stored.State)
	require.NoError(t, b.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, `{"workspace_id":"delete-ws"}`))
	stored, err = a.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"workspace_id":"delete-ws"}`, stored.ResourceSnapshot)
	assertNativeDeletionRows(t, ctx, a, request, sessionID)
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-013.5, AC-PLUGINS-MANAGED-COORDINATION-013.7
func TestManagedConversationAdmissionPostgresBarrierErrors(t *testing.T) {
	for _, mode := range []string{"storage", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			a, b, observer := newManagedAdmissionPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, request, sessionID := seedNativeManagedDeletion(t, ctx, a, "pgx")
			input := managed.StateRequest{Identity: request.Identity, Kind: managed.PauseExact, Paused: true, ExpectedRevision: 1, OperationID: "pause", PayloadDigest: "pause-digest"}
			if mode == "storage" {
				_, err := a.DB().ExecContext(ctx, `ALTER TABLE task_resource_cleanup_jobs RENAME TO hidden_cleanup_jobs`)
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = a.DB().ExecContext(context.Background(), `ALTER TABLE IF EXISTS hidden_cleanup_jobs RENAME TO task_resource_cleanup_jobs`)
				})
				_, err = b.ChangeManagedConversationState(ctx, input)
				require.NotErrorIs(t, err, managed.ErrBusy)
				var native *pgconn.PgError
				require.ErrorAs(t, err, &native)
				require.Equal(t, "42P01", native.Code)
				_, err = a.DB().ExecContext(ctx, `ALTER TABLE hidden_cleanup_jobs RENAME TO task_resource_cleanup_jobs`)
				require.NoError(t, err)
			} else {
				cancelManagedBarrierPostgresRead(t, ctx, a.DB(), b, observer, input)
			}
			assertNativeDeletionRows(t, ctx, b, request, sessionID)
			current, err := b.GetTask(ctx, request.TaskID)
			require.NoError(t, err)
			require.Equal(t, uint64(1), managed.Revision(current))
			result, err := b.ChangeManagedConversationState(ctx, input)
			require.NoError(t, err)
			require.True(t, result.Changed)
		})
	}
}

func cancelManagedBarrierPostgresRead(t *testing.T, ctx context.Context, holderDB *sql.DB, waiter *tasksqlite.Repository, observer *sqlx.DB, input managed.StateRequest) {
	t.Helper()
	holderPID, waiterPID := hierarchyBackendPID(t, holderDB), hierarchyBackendPID(t, waiter.DB())
	holder, err := holderDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	require.NoError(t, err)
	attempt, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	t.Cleanup(func() { cancel(); _ = holder.Rollback(); workers.Wait() })
	_, err = holder.ExecContext(ctx, `LOCK TABLE task_resource_cleanup_jobs IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)
	done := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := waiter.ChangeManagedConversationState(attempt, input)
		done <- err
	}()
	waitManagedBarrierPostgresTableLock(t, ctx, observer, holderPID, waiterPID)
	cancel()
	select {
	case err = <-done:
	case <-ctx.Done():
		t.Fatal("cancelled native barrier did not settle", ctx.Err())
	}
	workers.Wait()
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, managed.ErrBusy)
	require.Eventually(t, func() bool { return waiter.DB().Stats().InUse == 0 }, 2*time.Second, 10*time.Millisecond,
		"canceled transaction should release its pooled connection")
	var held bool
	require.NoError(t, observer.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks l JOIN pg_class c ON c.oid=l.relation WHERE l.pid=$1 AND l.granted AND l.mode='AccessExclusiveLock' AND c.relname='task_resource_cleanup_jobs')`, holderPID).Scan(&held))
	require.True(t, held, "cancellation settled while actual holder remains locked")
	require.NoError(t, holder.Rollback())
}

func waitManagedBarrierPostgresTableLock(t *testing.T, ctx context.Context, observer *sqlx.DB, holderPID, waiterPID int) {
	t.Helper()
	for {
		var waiting bool
		require.NoError(t, observer.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks l JOIN pg_class c ON c.oid=l.relation WHERE l.pid=$1 AND NOT l.granted AND c.relname='task_resource_cleanup_jobs') AND $2=ANY(pg_blocking_pids($1))`, waiterPID, holderPID).Scan(&waiting))
		if waiting {
			t.Logf("actual cleanup-table read wait: holder PID=%d waiter PID=%d", holderPID, waiterPID)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("native cleanup-table read never physically waited", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
