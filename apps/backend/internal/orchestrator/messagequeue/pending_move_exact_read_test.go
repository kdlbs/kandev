package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/testutil"
)

type pendingMoveCensusAuditTarget struct {
	PendingMoveID string `db:"pending_move_id"`
	MoveID        string `db:"move_id"`
	TaskID        string `db:"task_id"`
	SessionID     string `db:"session_id"`
	WorkflowID    string `db:"workflow_id"`
	CurrentStepID string `db:"prior_current_workflow_step_id"`
	TargetStepID  string `db:"prior_target_workflow_step_id"`
}

func assertPendingMoveCensusAuditTargetRedacted(t *testing.T, db *sqlx.DB, correlationID string) {
	t.Helper()
	var auditedTarget pendingMoveCensusAuditTarget
	if err := db.Get(&auditedTarget, `
		SELECT pending_move_id, move_id, task_id, session_id, workflow_id,
			prior_current_workflow_step_id, prior_target_workflow_step_id
		FROM pending_move_cancellation_audit WHERE correlation_id = ?
	`, correlationID); err != nil {
		t.Fatalf("read denied census audit: %v", err)
	}
	if auditedTarget != (pendingMoveCensusAuditTarget{}) {
		t.Fatalf("denied census audit retained untrusted target identity: %#v", auditedTarget)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.1
func TestSQLiteRepository_ReadPendingMoveCensusFound(t *testing.T) {
	fixture := newExactCancelFixture(t)
	ctx := context.Background()

	result, err := fixture.repo.ReadPendingMoveCensus(ctx, fixture.actor, exactTargetTaskID, "correlation-census-found")
	if err != nil {
		t.Fatalf("read pending move census: %v", err)
	}
	if result == nil || !result.Found {
		t.Fatalf("result = %#v, want found", result)
	}
	if result.PendingMoveID != fixture.match.PendingMoveID || result.MoveID != exactMoveID ||
		result.TaskID != exactTargetTaskID || result.SessionID != exactTargetSessionID ||
		result.WorkflowID != exactTargetWorkflowID || result.CurrentWorkflowStepID != exactCurrentStepID ||
		result.TargetWorkflowStepID != exactTargetStepID {
		t.Fatalf("census result did not report the exact relation snapshot: %#v", result)
	}

	// The census must never mutate: the same row can still be cancelled after
	// being read.
	cancelResult, cancelErr := fixture.repo.ExactCancelPendingMove(ctx, fixture.actor, fixture.match, "correlation-after-census")
	if cancelErr != nil || cancelResult == nil || !cancelResult.Cancelled {
		t.Fatalf("cancellation after census result=%#v err=%v, want the read to be non-mutating", cancelResult, cancelErr)
	}

	var audit PendingMoveCancellationAudit
	if err := fixture.sql.GetContext(ctx, &audit, `
		SELECT correlation_id, actor_kind, actor_id, pending_move_id, move_id, task_id, session_id,
			workflow_id, prior_current_workflow_step_id, prior_target_workflow_step_id, outcome, changed, action
		FROM pending_move_cancellation_audit WHERE correlation_id = ?
	`, "correlation-census-found"); err != nil {
		t.Fatalf("read census audit: %v", err)
	}
	if audit.Outcome != PendingMoveCensusOutcomeFound || audit.Changed || audit.Action != "read" ||
		audit.PendingMoveID != fixture.match.PendingMoveID || audit.ActorID != exactCallerTaskID {
		t.Fatalf("census audit = %#v", audit)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.2
func TestSQLiteRepository_ReadPendingMoveCensusZeroRow(t *testing.T) {
	fixture := newExactCancelFixture(t)
	ctx := context.Background()
	if _, err := fixture.repo.TakePendingMove(ctx, exactTargetSessionID); err != nil {
		t.Fatalf("drain target pending move: %v", err)
	}

	result, err := fixture.repo.ReadPendingMoveCensus(ctx, fixture.actor, exactTargetTaskID, "correlation-census-zero-row")
	if err != nil {
		t.Fatalf("read pending move census: %v", err)
	}
	if result == nil || result.Found {
		t.Fatalf("result = %#v, want an authoritative zero-row census", result)
	}
	if result.TaskID != exactTargetTaskID {
		t.Fatalf("zero-row census dropped the requested task ID: %#v", result)
	}

	var audit PendingMoveCancellationAudit
	if err := fixture.sql.GetContext(ctx, &audit, `
		SELECT outcome, changed, action FROM pending_move_cancellation_audit WHERE correlation_id = ?
	`, "correlation-census-zero-row"); err != nil {
		t.Fatalf("read zero-row audit: %v", err)
	}
	if audit.Outcome != PendingMoveCensusOutcomeZeroRow || audit.Changed || audit.Action != "read" {
		t.Fatalf("zero-row audit = %#v", audit)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.3
func TestSQLiteRepository_ReadPendingMoveCensusRejectsBrokenRelations(t *testing.T) {
	cases := map[string]string{
		"session belongs to another task":          `UPDATE task_sessions SET task_id = '` + exactCallerTaskID + `' WHERE id = '` + exactTargetSessionID + `'`,
		"current step belongs to another workflow": `UPDATE tasks SET workflow_step_id = '` + exactTargetStepID + `' WHERE id = '` + exactTargetTaskID + `'`,
		"target step belongs to another workflow":  `UPDATE pending_moves SET workflow_step_id = '` + exactCurrentStepID + `' WHERE session_id = '` + exactTargetSessionID + `'`,
		"target task crosses workspace":            `UPDATE tasks SET workspace_id = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc' WHERE id = '` + exactTargetTaskID + `'`,
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			if _, err := fixture.sql.Exec(mutation); err != nil {
				t.Fatalf("mutate relation: %v", err)
			}

			result, err := fixture.repo.ReadPendingMoveCensus(
				context.Background(), fixture.actor, exactTargetTaskID, "correlation-census-relation",
			)
			if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
				t.Fatalf("result=%#v err=%v, want stable relation mismatch", result, err)
			}
			var count int
			if err := fixture.sql.Get(&count, `SELECT COUNT(*) FROM pending_moves WHERE id = ?`, fixture.match.PendingMoveID); err != nil {
				t.Fatalf("count pending row: %v", err)
			}
			if count != 1 {
				t.Fatalf("broken relation removed pending row, count=%d", count)
			}
			assertPendingMoveCensusAuditTargetRedacted(t, fixture.sql, "correlation-census-relation")
		})
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.3
func TestSQLiteRepository_ReadPendingMoveCensusAuthorizationIsNonLeaking(t *testing.T) {
	cases := map[string]func(*exactCancelFixture){
		"ordinary agent": func(f *exactCancelFixture) { f.actor.Kind = "agent" },
		"forged caller task": func(f *exactCancelFixture) {
			f.actor.ID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			f.actor.CallerTaskID = f.actor.ID
		},
		"cross workspace actor": func(f *exactCancelFixture) {
			f.actor.WorkspaceID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		},
		"foreign owner": func(f *exactCancelFixture) { f.actor.UserID = "owner-b" },
		"revoked grant": func(f *exactCancelFixture) {
			if _, err := f.sql.Exec(`DELETE FROM workspace_coordinator_grants`); err != nil {
				t.Fatalf("revoke grant: %v", err)
			}
		},
		"stopped execution": func(f *exactCancelFixture) {
			if _, err := f.sql.Exec(`UPDATE executors_running SET status = 'stopped'`); err != nil {
				t.Fatalf("stop execution: %v", err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			mutate(&fixture)
			result, err := fixture.repo.ReadPendingMoveCensus(
				context.Background(), fixture.actor, exactTargetTaskID, "correlation-census-denied",
			)
			if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
				t.Fatalf("result=%#v err=%v, want stable non-leaking denial", result, err)
			}
			move, getErr := fixture.repo.GetPendingMove(context.Background(), exactTargetSessionID)
			if getErr != nil || move == nil || move.ID != fixture.match.PendingMoveID {
				t.Fatalf("denial mutated pending move: move=%#v err=%v", move, getErr)
			}
			assertPendingMoveCensusAuditTargetRedacted(t, fixture.sql, "correlation-census-denied")
		})
	}
}

func TestPostgresRepository_ReadPendingMoveCensusUsesOneSnapshot(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	fixture := newExactCancelFixtureWithDB(t, db)

	var schema string
	if err := db.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatalf("read postgres schema: %v", err)
	}
	openPeer := func() *sqlx.DB {
		peer, err := sqlx.Open("pgx", dsn)
		if err != nil {
			t.Fatalf("open postgres peer: %v", err)
		}
		peer.SetMaxOpenConns(1)
		peer.SetMaxIdleConns(1)
		t.Cleanup(func() { _ = peer.Close() })
		if _, err := peer.Exec(`SET search_path TO ` + schema); err != nil {
			t.Fatalf("set postgres peer search path: %v", err)
		}
		return peer
	}
	mutator := openPeer()
	poller := openPeer()

	if _, err := db.Exec(`
		ALTER TABLE pending_moves RENAME TO pending_moves_census_snapshot_backing;
		CREATE FUNCTION pending_move_census_snapshot_barrier() RETURNS boolean
		LANGUAGE plpgsql VOLATILE AS $$
		BEGIN
			PERFORM pg_advisory_xact_lock(hashtextextended(current_schema(), 0));
			RETURN true;
		END;
		$$;
		CREATE VIEW pending_moves AS
			SELECT pending.* FROM pending_moves_census_snapshot_backing pending
			WHERE pending_move_census_snapshot_barrier();
	`); err != nil {
		t.Fatalf("install census statement barrier: %v", err)
	}

	mutationTx, err := mutator.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin target mutation: %v", err)
	}
	defer func() { _ = mutationTx.Rollback() }()
	if _, err := mutationTx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(current_schema(), 0))`); err != nil {
		t.Fatalf("acquire census statement barrier: %v", err)
	}
	if _, err := mutationTx.Exec(`UPDATE tasks SET workspace_id = $1 WHERE id = $2`,
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc", exactTargetTaskID); err != nil {
		t.Fatalf("stage target reparent: %v", err)
	}

	backendPID := pgBackendPID(t, db)
	type censusResponse struct {
		result *PendingMoveCensusResult
		err    error
	}
	responseCh := make(chan censusResponse, 1)
	go func() {
		result, readErr := fixture.repo.ReadPendingMoveCensus(
			context.Background(), fixture.actor, exactTargetTaskID, "correlation-census-snapshot",
		)
		responseCh <- censusResponse{result: result, err: readErr}
	}()

	waitForWaitingLocks(t, poller, backendPID, 1, "census pending-row read")
	if err := mutationTx.Commit(); err != nil {
		t.Fatalf("commit target reparent: %v", err)
	}

	select {
	case response := <-responseCh:
		if response.err != nil || response.result == nil || !response.result.Found {
			t.Fatalf("census result=%#v err=%v, want the authorization snapshot", response.result, response.err)
		}
		if response.result.PendingMoveID != fixture.match.PendingMoveID ||
			response.result.CurrentWorkflowStepID != exactCurrentStepID {
			t.Fatalf("census returned a mixed relation snapshot: %#v", response.result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("census did not finish after releasing its statement barrier")
	}
}

func TestPendingMoveCensusTransactionUsesRepeatableRead(t *testing.T) {
	options := pendingMoveCensusTxOptions()
	if options == nil || options.Isolation != sql.LevelRepeatableRead {
		t.Fatalf("census transaction options = %#v, want repeatable read", options)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.3
func TestSQLiteRepository_ReadPendingMoveCensusRejectsOutOfTreeTarget(t *testing.T) {
	fixture := newExactCancelFixture(t)
	const unrelatedTaskID = "15151515-1515-4515-8515-151515151515"
	const unrelatedSessionID = "16161616-1616-4616-8616-161616161616"
	const unrelatedMoveID = "17171717-1717-4717-8717-171717171717"

	execExactFixtureSQL(t, fixture.sql, `
		INSERT INTO tasks (id, workspace_id, parent_id, workflow_id, workflow_step_id)
		VALUES (?, ?, '', ?, ?)
	`, unrelatedTaskID, exactWorkspaceID, exactCurrentWorkflowID, exactCurrentStepID)
	execExactFixtureSQL(t, fixture.sql, `
		INSERT INTO task_sessions (id, task_id, state) VALUES (?, ?, 'WAITING_FOR_INPUT')
	`, unrelatedSessionID, unrelatedTaskID)
	move := &PendingMove{
		MoveID: unrelatedMoveID, TaskID: unrelatedTaskID, WorkflowID: exactTargetWorkflowID,
		WorkflowStepID: exactTargetStepID,
	}
	if err := fixture.repo.SetPendingMove(context.Background(), unrelatedSessionID, move); err != nil {
		t.Fatalf("seed unrelated pending move: %v", err)
	}

	result, err := fixture.repo.ReadPendingMoveCensus(
		context.Background(), fixture.actor, unrelatedTaskID, "correlation-census-out-of-tree",
	)
	if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("out-of-tree census result=%#v err=%v, want stable denial", result, err)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.6
func TestSQLiteRepository_ReadPendingMoveCensusAuditFailureIsSanitized(t *testing.T) {
	fixture := newExactCancelFixture(t)
	if _, err := fixture.sql.Exec(`
		CREATE TRIGGER fail_pending_move_census_audit
		BEFORE INSERT ON pending_move_cancellation_audit
		WHEN NEW.action = 'read'
		BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END;
	`); err != nil {
		t.Fatalf("install census audit failure trigger: %v", err)
	}

	result, err := fixture.repo.ReadPendingMoveCensus(
		context.Background(), fixture.actor, exactTargetTaskID, "correlation-census-audit-failure",
	)
	if result != nil || !errors.Is(err, ErrPendingMoveReadFailed) {
		t.Fatalf("result=%#v err=%v, want sanitized read failure", result, err)
	}
	move, getErr := fixture.repo.GetPendingMove(context.Background(), exactTargetSessionID)
	if getErr != nil || move == nil || move.ID != fixture.match.PendingMoveID {
		t.Fatalf("failed census mutated pending row: move=%#v err=%v", move, getErr)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-004.4
func TestService_ReadPendingMoveRejectsInvalidTaskID(t *testing.T) {
	for _, tc := range []struct {
		name      string
		taskID    string
		present   bool
		canonical bool
	}{
		{name: "missing", taskID: "", present: false, canonical: false},
		{name: "malformed", taskID: "not-a-uuid", present: true, canonical: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			svc := NewService(fixture.repo, DefaultMaxPerSession, logger.Default())
			correlationID := "correlation-invalid-census-" + tc.name

			result, err := svc.ReadPendingMove(context.Background(), fixture.actor, tc.taskID, correlationID)
			if result != nil || !errors.Is(err, ErrPendingMoveInvalidArgument) {
				t.Fatalf("result=%#v err=%v, want invalid argument", result, err)
			}
			var audit struct {
				TaskID               string `db:"task_id"`
				Action               string `db:"action"`
				IdentifiersPresent   bool   `db:"identifiers_present"`
				IdentifiersCanonical bool   `db:"identifiers_canonical"`
			}
			if err := fixture.sql.Get(&audit, `
				SELECT task_id, action, identifiers_present, identifiers_canonical
				FROM pending_move_cancellation_audit WHERE correlation_id = ?
			`, correlationID); err != nil {
				t.Fatalf("read invalid census audit: %v", err)
			}
			if audit.TaskID != "" || audit.Action != "read" ||
				audit.IdentifiersPresent != tc.present || audit.IdentifiersCanonical != tc.canonical {
				t.Fatalf("invalid census audit leaked target or shape: %#v", audit)
			}
		})
	}
}
