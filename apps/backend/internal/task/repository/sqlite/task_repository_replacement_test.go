package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
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
	conn, err := sqlx.Open("sqlite3", "file:"+path+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=250&_cache=private")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return NewWithInitializedDB(conn, conn, nil)
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5
func TestTaskRepositoryReplacementSQLiteSerialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serialized.db")
	holder := independentReplacementSQLite(t, path)
	require.NoError(t, holder.initSchemaContext(context.Background()))
	seedReplacementStore(t, holder)
	worker := independentReplacementSQLite(t, path)
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
	// A real independent writer conflict proves database serialization, rather
	// than shared-pool queueing or scheduler delay.
	_, err := worker.db.Exec(`UPDATE tasks SET updated_at=updated_at WHERE id='replacement-task'`)
	require.ErrorContains(t, err, "locked")

	var called atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	cancelFinished := make(chan struct{})
	t.Cleanup(func() { cancel(); <-cancelFinished })
	go func() {
		defer close(cancelFinished)
		_, err := worker.ReplaceTaskRepositories(ctx, "replacement-task", func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
			called.Store(true)
			return nil, nil
		})
		cancelled <- err
	}()
	require.Eventually(t, func() bool { return worker.db.Stats().InUse == 1 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-cancelled, context.Canceled)
	require.False(t, called.Load())

	successful := make(chan error, 1)
	successorFinished := make(chan struct{})
	successCtx, successCancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(func() { successCancel(); unblock(); <-successorFinished })
	var successorCalled atomic.Bool
	go func() {
		defer close(successorFinished)
		_, err := worker.ReplaceTaskRepositories(successCtx, "replacement-task", func(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
			successorCalled.Store(true)
			if len(snapshot.Repositories) != 2 || snapshot.Repositories[0].BranchPolicyName != "Complete" {
				return nil, errors.New("successor did not observe committed predecessor")
			}
			return []*models.TaskRepository{{RepositoryID: "old-b", BaseBranch: "winner"}}, nil
		})
		successful <- err
	}()
	require.Eventually(t, func() bool { return worker.db.Stats().InUse == 1 }, time.Second, time.Millisecond)
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
