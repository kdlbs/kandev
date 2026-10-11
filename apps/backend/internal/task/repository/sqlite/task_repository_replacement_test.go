package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func seedReplacementStore(t *testing.T, repo *Repository) []*models.TaskRepository {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "replacement-ws", Name: "Replacement"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "replacement-task", WorkspaceID: "replacement-ws", Title: "Replacement", Priority: "medium"}))
	for _, id := range []string{"old-a", "old-b", "next-a", "next-b"} {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "replacement-ws", Name: id}))
	}
	for i, id := range []string{"old-a", "old-b"} {
		require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "link-" + id, TaskID: "replacement-task", RepositoryID: id, BaseBranch: "main", Position: i, BranchPolicyID: "policy", BranchPolicyName: "Saved", BranchPolicyBaseBranch: "main", BranchPolicyBranchTemplate: "topic/{title}", BranchPolicyPullRequestTarget: "release", Metadata: map[string]interface{}{"nested": []interface{}{id, float64(4)}}}))
	}
	rows, err := repo.ListTaskRepositories(ctx, "replacement-task")
	require.NoError(t, err)
	return rows
}

func replacementStoreRows() []*models.TaskRepository {
	return []*models.TaskRepository{{RepositoryID: "next-b", BaseBranch: "develop", Position: 0, BranchPolicyID: "policy", BranchPolicyName: "Complete", BranchPolicyBaseBranch: "develop", BranchPolicyBranchTemplate: "new/{title}", BranchPolicyPullRequestTarget: "release", Metadata: map[string]interface{}{"nested": []interface{}{"value", float64(6)}}}, {RepositoryID: "next-a", BaseBranch: "main", CheckoutBranch: "topic", Position: 1}}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1
func TestTaskRepositoryReplacementStoreRollback(t *testing.T) {
	for _, kind := range []string{"second_insert", "encoding", "callback", "missing_parent", "foreign_owner"} {
		t.Run(kind, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			original := seedReplacementStore(t, repo)
			taskID := "replacement-task"
			if kind == "second_insert" {
				_, err := repo.db.Exec(`CREATE TRIGGER replacement_reject BEFORE INSERT ON task_repositories WHEN NEW.repository_id = 'next-a' BEGIN SELECT RAISE(ABORT, 'second insert rejected'); END`)
				require.NoError(t, err)
			}
			if kind == "missing_parent" {
				taskID = "missing"
			}
			result, err := repo.ReplaceTaskRepositories(context.Background(), taskID, func(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
				require.Equal(t, original, snapshot.Repositories)
				if kind == "callback" {
					return nil, errors.New("invalid canonical policy")
				}
				rows := replacementStoreRows()
				if kind == "foreign_owner" {
					rows[1].TaskID = "other-task"
				}
				if kind == "encoding" {
					rows[1].Metadata = map[string]interface{}{"unencodable": make(chan int)}
				}
				return rows, nil
			})
			require.Error(t, err)
			require.Nil(t, result)
			rows, err := repo.ListTaskRepositories(context.Background(), "replacement-task")
			require.NoError(t, err)
			require.Equal(t, original, rows)
		})
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2
func TestTaskRepositoryReplacementCompleteSet(t *testing.T) {
	repo := newRepoForEntityTests(t)
	original := seedReplacementStore(t, repo)
	input := replacementStoreRows()
	result, err := repo.ReplaceTaskRepositories(context.Background(), "replacement-task", func(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
		require.Equal(t, original, snapshot.Repositories)
		require.False(t, snapshot.EnvironmentExists)
		return input, nil
	})
	require.NoError(t, err)
	require.Len(t, result, 2)
	actual, err := repo.ListTaskRepositories(context.Background(), "replacement-task")
	require.NoError(t, err)
	require.Equal(t, actual, result)
	for i, row := range result {
		require.Equal(t, input[i].RepositoryID, row.RepositoryID)
		require.Equal(t, input[i].Metadata, row.Metadata)
		require.Equal(t, input[i].BranchPolicyBranchTemplate, row.BranchPolicyBranchTemplate)
		require.NotEmpty(t, row.ID)
		require.Equal(t, "replacement-task", row.TaskID)
		require.Empty(t, input[i].ID)
	}
	result[0].Metadata["nested"] = "caller edit"
	actual, err = repo.ListTaskRepositories(context.Background(), "replacement-task")
	require.NoError(t, err)
	require.NotEqual(t, result[0].Metadata, actual[0].Metadata)
	_, err = repo.ReplaceTaskRepositories(context.Background(), "replacement-task", func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) { return nil, nil })
	require.NoError(t, err)
	actual, err = repo.ListTaskRepositories(context.Background(), "replacement-task")
	require.NoError(t, err)
	require.Empty(t, actual)
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1
func TestTaskRepositoryReplacementCancellation(t *testing.T) {
	repo := newRepoForEntityTests(t)
	original := seedReplacementStore(t, repo)
	ctx, cancel := context.WithCancel(context.Background())
	result, err := repo.ReplaceTaskRepositories(ctx, "replacement-task", func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
		cancel()
		return replacementStoreRows(), nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	actual, err := repo.ListTaskRepositories(context.Background(), "replacement-task")
	require.NoError(t, err)
	require.Equal(t, original, actual)
}

func independentReplacementSQLite(t *testing.T, path string) *Repository {
	t.Helper()
	conn := replacementSQLiteDB(t, path)
	return NewWithInitializedDB(conn, conn, nil)
}

func replacementSQLiteDB(t *testing.T, path string) *sqlx.DB {
	t.Helper()
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	conn := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return conn
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5
func TestTaskRepositoryReplacementSQLiteSerialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serialized.db")
	holder := independentReplacementSQLite(t, path)
	require.NoError(t, holder.initSchemaContext(context.Background()))
	seedReplacementStore(t, holder)
	worker := independentReplacementSQLite(t, path)
	observer := replacementSQLiteDB(t, path)
	_, err := observer.Exec(`PRAGMA busy_timeout=0`)
	require.NoError(t, err)
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	holderFinished := make(chan struct{})
	go func() {
		defer close(holderFinished)
		_, err := holder.ReplaceTaskRepositories(context.Background(), "replacement-task", func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
			close(held)
			<-release
			return replacementStoreRows(), nil
		})
		done <- err
	}()
	<-held
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); <-holderFinished })
	// The independent zero-timeout observer proves that the holder owns SQLite's
	// writer; the successor below must then enter the actual SQLite lock wait.
	_, err = observer.Exec(`UPDATE tasks SET updated_at=updated_at WHERE id='replacement-task'`)
	var busy sqlite3.Error
	require.ErrorAs(t, err, &busy)
	require.Equal(t, sqlite3.ErrBusy, busy.Code)

	var cancelledCallback atomic.Bool
	cancelCtx, cancelRequest := context.WithCancel(context.Background())
	var busyTimeout int
	require.NoError(t, worker.db.Get(&busyTimeout, `PRAGMA busy_timeout`))
	require.Equal(t, 5000, busyTimeout)
	cancelWaitCtx, cancelWait := context.WithTimeout(context.Background(), time.Duration(busyTimeout+1000)*time.Millisecond)
	cancelResult := make(chan error, 1)
	cancelFinished := make(chan struct{})
	cancelStarted := make(chan struct{})
	go func() {
		defer close(cancelFinished)
		close(cancelStarted)
		_, err := worker.ReplaceTaskRepositories(cancelCtx, "replacement-task", func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
			cancelledCallback.Store(true)
			return nil, nil
		})
		cancelResult <- err
	}()
	t.Cleanup(func() { cancelRequest(); cancelWait(); unblock(); <-cancelFinished })
	select {
	case <-cancelStarted:
	case <-cancelWaitCtx.Done():
		t.Fatal("cancellation waiter did not start", cancelWaitCtx.Err())
	}
	waitingCancelWorker := waitTaskRepositoryReplacementSQLiteBegin(t, cancelWaitCtx, cancelFinished, "cancellation waiter")
	_, err = observer.ExecContext(cancelWaitCtx, `UPDATE tasks SET updated_at=updated_at WHERE id='replacement-task'`)
	require.ErrorAs(t, err, &busy)
	require.Equal(t, sqlite3.ErrBusy, busy.Code)
	require.Equal(t, waitingCancelWorker, taskRepositoryReplacementSQLiteBeginWorker(), "same cancellation waiter remains in SQLite's writer wait")
	cancelRequest()
	select {
	case err = <-cancelResult:
		require.ErrorIs(t, err, context.Canceled)
	case <-cancelWaitCtx.Done():
		t.Fatal("canceled replacement did not settle while the holder remained locked", cancelWaitCtx.Err())
	}
	<-cancelFinished
	_, err = observer.ExecContext(cancelWaitCtx, `UPDATE tasks SET updated_at=updated_at WHERE id='replacement-task'`)
	require.ErrorAs(t, err, &busy)
	require.Equal(t, sqlite3.ErrBusy, busy.Code)
	require.False(t, cancelledCallback.Load())

	successful := make(chan error, 1)
	successorFinished := make(chan struct{})
	successCtx, successCancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(func() { successCancel(); unblock(); <-successorFinished })
	var successorCalled atomic.Bool
	successorStarted := make(chan struct{})
	go func() {
		defer close(successorFinished)
		close(successorStarted)
		_, err := worker.ReplaceTaskRepositories(successCtx, "replacement-task", func(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
			successorCalled.Store(true)
			if len(snapshot.Repositories) != 2 || snapshot.Repositories[0].BranchPolicyName != "Complete" {
				return nil, errors.New("successor did not observe committed predecessor")
			}
			return []*models.TaskRepository{{RepositoryID: "old-b", BaseBranch: "winner"}}, nil
		})
		successful <- err
	}()
	select {
	case <-successorStarted:
	case <-successCtx.Done():
		t.Fatal("successor did not start", successCtx.Err())
	}
	waitingWorker := waitTaskRepositoryReplacementSQLiteBegin(t, successCtx, successorFinished, "successor")
	_, err = observer.ExecContext(successCtx, `UPDATE tasks SET updated_at=updated_at WHERE id='replacement-task'`)
	require.ErrorAs(t, err, &busy)
	require.Equal(t, sqlite3.ErrBusy, busy.Code)
	require.Equal(t, waitingWorker, taskRepositoryReplacementSQLiteBeginWorker(), "same successor remains in SQLite's writer wait")
	require.False(t, successorCalled.Load())
	unblock()
	require.NoError(t, <-done)
	require.NoError(t, <-successful)
	require.True(t, successorCalled.Load())
	rows, err := holder.ListTaskRepositories(context.Background(), "replacement-task")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "winner", rows[0].BaseBranch)
}

func waitTaskRepositoryReplacementSQLiteBegin(t *testing.T, ctx context.Context, finished <-chan struct{}, operation string) string {
	t.Helper()
	for {
		if worker := taskRepositoryReplacementSQLiteBeginWorker(); worker != "" {
			return worker
		}
		select {
		case <-finished:
			t.Fatalf("%s returned before waiting for SQLite's writer", operation)
		case <-ctx.Done():
			t.Fatalf("%s SQLite writer wait was not observed: %v", operation, ctx.Err())
		default:
			runtime.Gosched()
		}
	}
}

func taskRepositoryReplacementSQLiteBeginWorker() string {
	buffer := make([]byte, 1<<20)
	sections := strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n")
	for _, parent := range sections {
		if !strings.Contains(parent, ".ReplaceTaskRepositories(") || !strings.Contains(parent, "(*SQLiteConn).begin(") {
			continue
		}
		owner := strings.Fields(parent)[1]
		for _, child := range sections {
			if strings.Contains(child, "_Cfunc__sqlite3_step_row_internal(") && strings.Contains(child, "created by github.com/mattn/go-sqlite3.(*SQLiteStmt).exec in goroutine "+owner+"\n") {
				return owner + "/" + strings.Fields(child)[1]
			}
		}
	}
	return ""
}
