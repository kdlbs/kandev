package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// A disposable trigger pauses the actual transfer statement after reservation.
// It adds no production hook and never substitutes admission or preservation.
func TestTransferWorkspaceFirstFullComposition(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "new row owner and census", true: "late cancellation rollback"}[cancelled], func(t *testing.T) {
			f := newOfficeAtomicFixture(t, nil)
			_, cleanup, err := settingsstore.Provide(f.h.db, f.h.db, f.log)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })
			scope, cancelScope := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelScope()
			_, err = f.h.db.Exec(`UPDATE workspaces SET owner_id='owner',office_workflow_id='' WHERE id IN ('ws-1','ws-2')`)
			require.NoError(t, err)
			require.NoError(t, f.workflow.CreateStep(scope, &wfmodels.WorkflowStep{ID: "external-home", WorkflowID: "wf-other-workspace", Name: "Home", Position: 0}))
			require.NoError(t, f.h.repo.CreateTask(scope, &models.Task{ID: "other-task", WorkspaceID: "ws-2", WorkflowID: "wf-other-workspace", WorkflowStepID: "external-home", Title: "Other"}))
			require.NoError(t, f.h.repo.CreateTaskSession(scope, &models.TaskSession{ID: "transfer-session", TaskID: "atomic-task", State: models.TaskSessionStateWaitingForInput, IsPrimary: true, ReviewStatus: models.ReviewStatusPending}))
			before, err := f.h.repo.GetTask(scope, "atomic-task")
			require.NoError(t, err)
			session, err := f.h.repo.GetTaskSession(scope, "transfer-session")
			require.NoError(t, err)
			ledger, err := f.h.repo.GetLatestTaskStepTransitionID(scope, "atomic-task")
			require.NoError(t, err)
			command := models.TaskTransferCommand{TaskID: before.ID, ExpectedSourceWorkspaceID: before.WorkspaceID, ExpectedSourceWorkflowID: before.WorkflowID, ExpectedSourceStepID: before.WorkflowStepID, ExpectedTaskUpdatedAt: before.UpdatedAt, DestinationWorkspaceID: "ws-2", DestinationWorkflowID: "wf-other-workspace", DestinationStepID: "external-home", IdempotencyKey: "composition-transfer", PreservationPolicy: models.TaskTransferPreservationPolicyV1, Actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner"}, AuthorizedOwnerID: "owner", OwnerPredicateSet: true}
			_, err = f.h.db.Exec(`CREATE FUNCTION pause_owned_transfer() RETURNS trigger LANGUAGE plpgsql AS $body$ BEGIN PERFORM pg_advisory_xact_lock(4642,77); RETURN NEW; END $body$`)
			require.NoError(t, err)
			trigger := `CREATE TRIGGER pause_owned_transfer BEFORE UPDATE ON tasks FOR EACH ROW WHEN (OLD.id='atomic-task') EXECUTE FUNCTION pause_owned_transfer()`
			if cancelled {
				trigger = `CREATE TRIGGER pause_owned_transfer BEFORE INSERT ON task_transfer_operations FOR EACH ROW EXECUTE FUNCTION pause_owned_transfer()`
			}
			_, err = f.h.db.Exec(trigger)
			require.NoError(t, err)
			holder, err := f.h.db.BeginTxx(scope, nil)
			require.NoError(t, err)
			defer func() { _ = holder.Rollback() }()
			_, err = holder.ExecContext(scope, `SELECT pg_advisory_xact_lock(4642,77)`)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(scope)
			defer cancel()
			type result struct {
				receipt *models.TaskTransferReceipt
				err     error
			}
			done := make(chan result, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				receipt, err := f.h.repo.TransferTask(ctx, command)
				done <- result{receipt, err}
			}()
			t.Cleanup(func() { cancel(); _ = holder.Rollback(); <-finished })
			owner := awaitOwnedTransferPause(t, scope, f.h.db, done)
			var actual, expected []string
			require.NoError(t, f.h.db.SelectContext(scope, &actual, `SELECT c.relname FROM pg_locks l JOIN pg_class c ON c.oid=l.relation WHERE l.pid=$1 AND l.mode='ExclusiveLock' AND l.granted AND c.relnamespace=current_schema()::regnamespace ORDER BY c.relname`, owner))
			require.NoError(t, f.h.db.SelectContext(scope, &expected, `SELECT DISTINCT table_name FROM information_schema.columns WHERE table_schema=current_schema() AND (column_name='task_id' OR (table_name='task_workspace_groups' AND column_name='owner_task_id'))`))
			expected = append(expected, "tasks", "workspaces", "workflows", "workflow_steps", "task_transfer_operations", "task_transfer_audit", "task_transfer_serialization", "task_step_transitions", "office_labels", "office_task_tree_holds", "agent_profiles")
			set := map[string]bool{}
			for _, table := range expected {
				set[table] = true
			}
			expected = nil
			for table := range set {
				expected = append(expected, table)
			}
			sort.Strings(expected)
			require.Equal(t, expected, actual, "full production schemas must be covered by actual transfer reservation")
			if cancelled {
				cancel()
				outcome := <-done
				require.ErrorIs(t, outcome.err, context.Canceled)
				after, err := f.h.repo.GetTask(scope, "atomic-task")
				require.NoError(t, err)
				require.Equal(t, before, after)
				afterSession, err := f.h.repo.GetTaskSession(scope, "transfer-session")
				require.NoError(t, err)
				require.Equal(t, session, afterSession)
				afterLedger, err := f.h.repo.GetLatestTaskStepTransitionID(scope, "atomic-task")
				require.NoError(t, err)
				require.Equal(t, ledger, afterLedger)
				var count int
				require.NoError(t, f.h.db.QueryRowContext(scope, `SELECT count(*) FROM task_transfer_operations`).Scan(&count))
				require.Zero(t, count)
				require.NoError(t, f.h.db.QueryRowContext(scope, `SELECT count(*) FROM pg_locks WHERE pid=$1 AND mode='ExclusiveLock' AND locktype='relation'`, owner).Scan(&count))
				require.Zero(t, count)
				return
			}
			// Plain task reads remain available while EXCLUSIVE blocks new row owners.
			read, err := f.h.repo.GetTask(scope, "other-task")
			require.NoError(t, err)
			require.Equal(t, "Other", read.Title)
			writer, err := f.h.db.BeginTxx(scope, nil)
			require.NoError(t, err)
			defer func() { _ = writer.Rollback() }()
			var writerPID int
			require.NoError(t, writer.QueryRowContext(scope, `SELECT pg_backend_pid()`).Scan(&writerPID))
			wrote := make(chan error, 1)
			go func() {
				var id string
				err := writer.QueryRowContext(scope, `SELECT id FROM tasks WHERE id='other-task' FOR UPDATE`).Scan(&id)
				if err == nil {
					_, err = writer.ExecContext(scope, `UPDATE tasks SET description='concurrent row owner' WHERE id='other-task'`)
				}
				if err == nil {
					err = writer.Commit()
				}
				wrote <- err
			}()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			blocked := false
			for !blocked {
				select {
				case err := <-wrote:
					t.Fatalf("new unrelated row owner passed reservation: %v", err)
				case <-tick.C:
					require.NoError(t, f.h.db.QueryRowContext(scope, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND relation='tasks'::regclass AND mode='RowShareLock' AND NOT granted)`, writerPID).Scan(&blocked))
				case <-scope.Done():
					t.Fatal(scope.Err())
				}
			}
			require.NoError(t, holder.Rollback())
			outcome := <-done
			require.NoError(t, outcome.err)
			require.Equal(t, "ws-2", outcome.receipt.DestinationWorkspaceID)
			require.NoError(t, <-wrote)
			after, err := f.h.repo.GetTask(scope, "other-task")
			require.NoError(t, err)
			require.Equal(t, "concurrent row owner", after.Description)
		})
	}
}

// The channel is observed through a callback to keep the paused transfer's
// concrete result private to its test while waiting on an actual server lock.
func awaitOwnedTransferPause[T any](t *testing.T, ctx context.Context, db *sqlx.DB, done <-chan T) int {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case outcome := <-done:
			t.Fatalf("transfer returned before reaching the owned SQL barrier: %+v", outcome)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-tick.C:
			var pid int
			err := db.QueryRowContext(ctx, `SELECT pid FROM pg_locks WHERE locktype='advisory' AND classid=4642 AND objid=77 AND NOT granted LIMIT 1`).Scan(&pid)
			if err == nil {
				return pid
			}
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatal(err)
			}
		}
	}
}
