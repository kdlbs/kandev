package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	internaldb "github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// The blocker owns the workspace, not either moved task. Observe the actual
// independent backend wait, then commit a competing edge before releasing it.
func TestTaskHierarchyAdmissionPostgresWaits(t *testing.T) {
	a, b, _ := newHierarchyPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "wait-ws", Name: "Wait"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "wait-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	pid := hierarchyBackendPID(t, b.DB())
	holder, err := a.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(ctx, `SELECT id FROM workspaces WHERE id='wait-ws' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		parent := "a"
		_, err := b.UpdateTaskWithParentAdmission(ctx, snapshot, &parent, false, hierarchy.ValidateParent)
		result <- err
	}()
	// Cleanup always releases the held lock and joins the worker, including a
	// fatal physical-wait assertion. A worker never outlives its fixture.
	joined := false
	defer func() {
		_ = holder.Rollback()
		if !joined {
			<-result
		}
	}()
	waitHierarchyPostgresLocks(t, holder, pid, 1, "canonical parent admission")
	t.Logf("observed backend PID %d waiting on workspace owner transaction", pid)
	if _, err := holder.ExecContext(ctx, `UPDATE tasks SET parent_id='b' WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	err = <-result
	joined = true
	if !errors.Is(err, hierarchy.ErrInvalidParent) {
		t.Fatalf("waiter validated stale ancestors: %v", err)
	}
	got, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentID != "" || got.Title != "b" || !got.UpdatedAt.Equal(snapshot.UpdatedAt) {
		t.Fatalf("loser changed: %+v", got)
	}
	if err := b.CreateTask(ctx, &models.Task{ID: "c", WorkspaceID: "wait-ws", Title: "c"}); err != nil {
		t.Fatal(err)
	}
	leaf, err := b.GetTask(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	parent := "c"
	changed, err := b.UpdateTaskWithParentAdmission(ctx, leaf, &parent, true, hierarchy.ValidateParent)
	if err != nil || !changed {
		t.Fatalf("ordinary valid PG move: changed=%t error=%v", changed, err)
	}
	persisted, err := b.GetTask(ctx, leaf.ID)
	if err != nil || persisted.ParentID != parent || persisted.Title != "a" {
		t.Fatalf("committed valid PG move: task=%+v error=%v", persisted, err)
	}
}

func TestTaskHierarchyAdmissionTargetLifecycle(t *testing.T) {
	for _, operation := range []string{"archive", "unarchive", "delete", "detach", "promotion", "restore"} {
		t.Run(operation, func(t *testing.T) {
			a, b, _ := newHierarchyPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "lifecycle-ws", Name: "Lifecycle"}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"parent", "child"} {
				if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "lifecycle-ws", Title: id}); err != nil {
					t.Fatal(err)
				}
			}
			if operation == "unarchive" {
				if err := a.ArchiveTask(ctx, "parent"); err != nil {
					t.Fatal(err)
				}
			}
			if operation == "detach" || operation == "promotion" {
				if _, err := a.DB().Exec(`UPDATE tasks SET parent_id='parent' WHERE id='child'`); err != nil {
					t.Fatal(err)
				}
			}
			pid := hierarchyBackendPID(t, b.DB())
			holder, err := a.DB().BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Rollback() }()
			if _, err := holder.ExecContext(ctx, `SELECT id FROM workspaces WHERE id='lifecycle-ws' FOR UPDATE`); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "archive":
					err = b.ArchiveTask(ctx, "parent")
				case "unarchive":
					_, err = b.UnarchiveTask(ctx, "parent")
				case "delete":
					err = b.DeleteTask(ctx, "parent")
				case "detach":
					_, err = b.DetachTask(ctx, "child")
				case "promotion":
					err = b.ReparentDirectChildrenInWorkspace(ctx, "parent", "", "lifecycle-ws")
				case "restore":
					err = b.RestoreTaskParentIfUnchanged(ctx, "child", "", "parent", "")
				}
				result <- err
			}()
			joined := false
			defer func() {
				_ = holder.Rollback()
				if !joined {
					<-result
				}
			}()
			waitHierarchyPostgresLocks(t, holder, pid, 1, operation)
			t.Logf("%s: physical workspace wait PID %d", operation, pid)
			if operation == "delete" {
				if _, err := holder.ExecContext(ctx, `INSERT INTO tasks(id, workspace_id, title, parent_id, created_at, updated_at) VALUES ('late','lifecycle-ws','Late','parent',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
					t.Fatal(err)
				}
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			err = <-result
			joined = true
			if operation == "delete" {
				if !errors.Is(err, repoerrors.ErrTaskHierarchyConflict) {
					t.Fatalf("late-child delete=%v", err)
				}
				if _, err := b.GetTask(ctx, "parent"); err != nil {
					t.Fatalf("parent not retained: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Initialize all production owners before opening independent handles. Detach
// reads Office membership even for a task without explicit workspace metadata.
func newHierarchyPostgresRepoPair(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	first := testutil.OpenIsolatedPostgres(t, dsn)
	a, err := tasksqlite.NewWithDB(first, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := settingsstore.Provide(first, first, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := officesqlite.NewWithDB(first, first, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := workflowrepo.NewWithDB(first, first, nil); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := first.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	open := func() *sqlx.DB {
		raw, err := internaldb.OpenPostgres(hierarchySchemaDSN(dsn, schema), 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		db := sqlx.NewDb(raw, "pgx")
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.Exec(`SET search_path TO ` + schema); err != nil {
			t.Fatal(err)
		}
		return db
	}
	second, observer := open(), open()
	return a, tasksqlite.NewWithInitializedDB(second, second, nil), observer
}
func hierarchyBackendPID(t *testing.T, db *sql.DB) int {
	t.Helper()
	var pid int
	if err := db.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid
}
func waitHierarchyPostgresLocks(t *testing.T, tx *sql.Tx, pid, want int, operation string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		var count int
		err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks WHERE NOT granted AND locktype='transactionid' AND pid=$1`, pid).Scan(&count)
		if err != nil {
			t.Fatalf("%s physical lock observation: %v", operation, err)
		}
		if count >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s never physically waited: %v", operation, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestTaskHierarchyAdmissionCreateMove(t *testing.T) {
	for _, variant := range []string{"ordinary", "capacity", "feeder"} {
		t.Run(variant, func(t *testing.T) {
			a, b, _ := newHierarchyPostgresRepoPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			ctx = hierarchy.WithTaskCreationParentValidator(ctx, hierarchy.ValidateCreationParent)
			if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "create-ws", Name: "Create"}); err != nil {
				t.Fatal(err)
			}
			if err := a.CreateWorkflow(ctx, &models.Workflow{ID: "create-wf", WorkspaceID: "create-ws", Name: "Workflow"}); err != nil {
				t.Fatal(err)
			}
			if _, err := a.DB().Exec(`INSERT INTO workflow_steps(id,workflow_id,name,position) VALUES ('target','create-wf','Target',0),('feeder','create-wf','Feeder',1)`); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"a", "b"} {
				if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "create-ws", Title: id}); err != nil {
					t.Fatal(err)
				}
			}
			pid := hierarchyBackendPID(t, b.DB())
			holder, err := a.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Rollback() }()
			if _, err := holder.Exec(`SELECT id FROM workspaces WHERE id='create-ws' FOR UPDATE`); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				child := &models.Task{ID: "child", WorkspaceID: "create-ws", WorkflowID: "create-wf", WorkflowStepID: "target", Title: "Child", ParentID: "b"}
				var err error
				switch variant {
				case "ordinary":
					err = b.CreateTask(ctx, child)
				case "capacity":
					err = b.CreateTaskIfWorkflowStepHasCapacity(ctx, child, "target", 1)
				case "feeder":
					err = b.CreateTaskWithWorkflowStepAdmission(ctx, child, "target", 1, "feeder", 1)
				}
				result <- err
			}()
			joined := false
			defer func() {
				_ = holder.Rollback()
				if !joined {
					<-result
				}
			}()
			waitHierarchyPostgresLocks(t, holder, pid, 1, variant+" creation")
			t.Logf("%s insert: physical workspace wait PID %d", variant, pid)
			if _, err := holder.Exec(`UPDATE tasks SET parent_id='a' WHERE id='b'`); err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			err = <-result
			joined = true
			if !errors.Is(err, hierarchy.ErrSubtaskDepthExceeded) {
				t.Fatalf("stale creation=%v", err)
			}
			if _, err := b.GetTask(ctx, "child"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
				t.Fatalf("rejected child persisted: %v", err)
			}
		})
	}
}

func TestTaskHierarchyAdmissionPostgresCancellation(t *testing.T) {
	a, b, _ := newHierarchyPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "cancel-ws", Name: "Cancel"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "cancel-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	pid := hierarchyBackendPID(t, b.DB())
	holder, err := a.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(`SELECT id FROM workspaces WHERE id='cancel-ws' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	waiterCtx, cancelWaiter := context.WithCancel(ctx)
	defer cancelWaiter()
	result := make(chan error, 1)
	go func() {
		parent := "a"
		_, err := b.UpdateTaskWithParentAdmission(waiterCtx, snapshot, &parent, false, hierarchy.ValidateParent)
		result <- err
	}()
	joined := false
	defer func() {
		cancelWaiter()
		_ = holder.Rollback()
		if !joined {
			<-result
		}
	}()
	waitHierarchyPostgresLocks(t, holder, pid, 1, "cancelled canonical admission")
	t.Logf("cancellation: physical workspace wait PID %d", pid)
	cancelWaiter()
	err = <-result
	joined = true
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission=%v", err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	current, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if current.ParentID != "" || !current.UpdatedAt.Equal(snapshot.UpdatedAt) {
		t.Fatal("cancelled waiter changed current row")
	}
	parent := "a"
	if _, err := b.UpdateTaskWithParentAdmission(ctx, current, &parent, false, hierarchy.ValidateParent); err != nil {
		t.Fatalf("ordinary request after cancellation failed: %v", err)
	}
}

// A cancelled pgx query may retire its connection. Configure the namespace at
// connection startup so a replacement still reads this test's owned schema.
func hierarchySchemaDSN(dsn, schema string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			panic(err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	return dsn + " search_path=" + schema
}

func TestTaskHierarchyAdmissionDeferredMarker(t *testing.T) {
	a, b, _ := newHierarchyPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "marker-ws", Name: "Marker"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "child"} {
		if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "marker-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB().Exec(`UPDATE tasks SET parent_id='a', metadata='{"workspace":{"mode":"inherit_parent","group_id":"marker-group"},"keep":"current"}' WHERE id='child'`); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "marker-session", TaskID: "child", QueueIncarnationID: "marker-incarnation", State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := a.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	queue, err := messagequeue.NewSQLiteRepository(sqlx.NewDb(a.DB(), "pgx"), sqlx.NewDb(a.DB(), "pgx"))
	if err != nil {
		t.Fatal(err)
	}
	move := messagequeue.PendingMove{MoveID: "marker-move", SessionIncarnationID: session.QueueIncarnationID, TaskID: "child", QueuedAt: time.Now().UTC()}
	if err := queue.SetPendingMove(ctx, session.ID, &move); err != nil {
		t.Fatal(err)
	}
	pid := hierarchyBackendPID(t, b.DB())
	holder, err := a.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(ctx, `SELECT id FROM workspaces WHERE id='marker-ws' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		applied, err := b.MarkDeferredMoveAppliedForSession(ctx, "child", move.MoveID, messagequeue.PendingMoveRecord{SessionID: session.ID, Move: move})
		if err == nil && !applied {
			err = errors.New("marker not applied")
		}
		result <- err
	}()
	joined := false
	defer func() {
		_ = holder.Rollback()
		if !joined {
			<-result
		}
	}()
	waitHierarchyPostgresLocks(t, holder, pid, 1, "deferred-marker")
	t.Logf("deferred marker physical workspace wait PID %d", pid)
	if _, err := holder.ExecContext(ctx, `UPDATE tasks SET parent_id='b', metadata='{"workspace":{"mode":"shared_group","group_id":"marker-group"},"keep":"current"}' WHERE id='child'`); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	err = <-result
	joined = true
	if err != nil {
		t.Fatal(err)
	}
	current, err := b.GetTask(ctx, "child")
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := current.Metadata["workspace"].(map[string]interface{})
	applied, _ := current.Metadata[models.MetaKeyAppliedDeferredMoves].(map[string]interface{})
	if current.ParentID != "b" || workspace["mode"] != "shared_group" || workspace["group_id"] != "marker-group" || current.Metadata["keep"] != "current" || applied[move.MoveID] != true {
		t.Fatalf("current marker state=%+v", current)
	}
	if pending, err := queue.GetPendingMove(ctx, session.ID); err != nil || pending != nil {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
}
