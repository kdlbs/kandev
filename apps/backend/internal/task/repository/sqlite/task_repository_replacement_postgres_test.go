package sqlite

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1
func TestPostgresTaskRepositoryReplacementRollback(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	original := seedReplacementStore(t, repo)
	// Valid rows reach the second insert and fail in the real database.
	_, err = db.Exec(`CREATE FUNCTION reject_replacement() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.repository_id = 'next-a' THEN RAISE EXCEPTION 'replacement second insert rejected'; END IF; RETURN NEW; END $$`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER reject_replacement BEFORE INSERT ON task_repositories FOR EACH ROW EXECUTE FUNCTION reject_replacement()`)
	require.NoError(t, err)
	result, err := repo.ReplaceTaskRepositories(context.Background(), "replacement-task", func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
		return replacementStoreRows(), nil
	})
	require.ErrorContains(t, err, "second insert")
	require.Nil(t, result)
	rows, err := repo.ListTaskRepositories(context.Background(), "replacement-task")
	require.NoError(t, err)
	require.Equal(t, original, rows)
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5
func TestPostgresTaskRepositoryReplacementSerialization(t *testing.T) {
	for _, cancelWaiting := range []bool{false, true} {
		t.Run(fmtPostgresReplacementCase(cancelWaiting), func(t *testing.T) {
			dsn := testutil.PostgresDSNFromEnv(t)
			db := testutil.OpenIsolatedPostgres(t, dsn)
			originalRepo, err := NewWithDB(db, db, nil)
			require.NoError(t, err)
			original := seedReplacementStore(t, originalRepo)
			holderDB := openSecondPostgresConnection(t, dsn, db)
			workerDB := openSecondPostgresConnection(t, dsn, db)
			observer := openSecondPostgresConnection(t, dsn, db)
			// The operation must explicitly choose READ COMMITTED even when the
			// connection's session default would otherwise freeze the pre-wait snapshot.
			_, err = workerDB.Exec(`SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL REPEATABLE READ`)
			require.NoError(t, err)
			holder := NewWithInitializedDB(holderDB, holderDB, nil)
			worker := NewWithInitializedDB(workerDB, workerDB, nil)
			var holderPID, workerPID, observerPID int
			require.NoError(t, holderDB.Get(&holderPID, "SELECT pg_backend_pid()"))
			require.NoError(t, workerDB.Get(&workerPID, "SELECT pg_backend_pid()"))
			require.NoError(t, observer.Get(&observerPID, "SELECT pg_backend_pid()"))
			require.NotEqual(t, holderPID, workerPID)
			require.NotEqual(t, holderPID, observerPID)
			require.NotEqual(t, workerPID, observerPID)
			holderCtx, cancelHolder := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelHolder()
			held, release := make(chan struct{}), make(chan struct{})
			holderDone := make(chan error, 1)
			holderFinished := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(func() { unblock(); <-holderFinished })
			go func() {
				defer close(holderFinished)
				_, err := holder.ReplaceTaskRepositories(holderCtx, "replacement-task", func(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
					close(held)
					<-release
					if cancelWaiting {
						return nil, context.Canceled
					}
					return replacementStoreRows(), nil
				})
				holderDone <- err
			}()
			select {
			case <-held:
			case <-holderFinished:
				require.FailNow(t, "holder exited before invoking callback", "error: %v", <-holderDone)
			case <-holderCtx.Done():
				unblock()
				<-holderFinished
				require.FailNow(t, "holder did not enter callback", "error: %v", <-holderDone)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var called atomic.Bool
			finished := make(chan struct{})
			workerDone := make(chan error, 1)
			t.Cleanup(func() { cancel(); unblock(); <-finished })
			go func() {
				defer close(finished)
				_, err := worker.ReplaceTaskRepositories(ctx, "replacement-task", func(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
					called.Store(true)
					if len(snapshot.Repositories) != 2 || snapshot.Repositories[0].BranchPolicyName != "Complete" {
						return nil, errors.New("successor did not observe committed predecessor")
					}
					row := *snapshot.Repositories[0]
					row.RepositoryID = "old-b"
					row.BaseBranch = "winner"
					row.Position = 0
					return []*models.TaskRepository{&row}, nil
				})
				workerDone <- err
			}()
			waitErr := waitForPostgresLock(ctx, observer, workerPID, finished)
			if waitErr != nil {
				cancel()
				unblock()
				<-workerDone
				<-holderDone
				require.NoError(t, waitErr)
			}
			calledBeforeRelease := called.Load()
			if cancelWaiting {
				cancel()
				require.Error(t, <-workerDone)
				require.False(t, called.Load())
				unblock()
				require.ErrorIs(t, <-holderDone, context.Canceled)
			} else {
				unblock()
				require.NoError(t, <-holderDone)
				require.NoError(t, <-workerDone)
				require.True(t, called.Load())
			}
			require.False(t, calledBeforeRelease, "canonical read/finalizer must follow the observed physical lock wait")
			rows, err := originalRepo.ListTaskRepositories(context.Background(), "replacement-task")
			require.NoError(t, err)
			if cancelWaiting {
				require.Equal(t, original, rows)
			} else {
				require.Len(t, rows, 1)
				require.Equal(t, "winner", rows[0].BaseBranch)
				require.Equal(t, "Complete", rows[0].BranchPolicyName)
				require.Equal(t, "new/{title}", rows[0].BranchPolicyBranchTemplate)
				require.Equal(t, replacementStoreRows()[0].Metadata, rows[0].Metadata)
			}
		})
	}
}

func fmtPostgresReplacementCase(cancel bool) string {
	if cancel {
		return "cancel_waiter"
	}
	return "committed_predecessor"
}
