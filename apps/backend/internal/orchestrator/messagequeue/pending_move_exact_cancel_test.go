package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/testutil"
	_ "github.com/mattn/go-sqlite3"
)

const (
	exactWorkspaceID       = "11111111-1111-4111-8111-111111111111"
	exactCallerTaskID      = "22222222-2222-4222-8222-222222222222"
	exactCallerSessionID   = "33333333-3333-4333-8333-333333333333"
	exactCallerExecutionID = "44444444-4444-4444-8444-444444444444"
	exactTargetTaskID      = "55555555-5555-4555-8555-555555555555"
	exactTargetSessionID   = "66666666-6666-4666-8666-666666666666"
	exactCurrentWorkflowID = "77777777-7777-4777-8777-777777777777"
	exactCurrentStepID     = "88888888-8888-4888-8888-888888888888"
	exactTargetWorkflowID  = "99999999-9999-4999-8999-999999999999"
	exactTargetStepID      = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	exactMoveID            = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

type exactCancelFixture struct {
	repo                 Repository
	sql                  *sqlx.DB
	actor                PendingMoveCancellationActor
	match                ExactPendingMoveMatch
	controlPendingMoveID string
}

func newExactCancelFixture(t *testing.T) exactCancelFixture {
	t.Helper()
	raw, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	return newExactCancelFixtureWithDB(t, db)
}

func newExactCancelFixtureWithDB(t *testing.T, db *sqlx.DB) exactCancelFixture {
	t.Helper()
	if _, err := db.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL);
		CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL);
		CREATE TABLE workflow_steps (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			parent_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL,
			workflow_step_id TEXT NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}',
			updated_at TIMESTAMP
		);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			state TEXT NOT NULL,
			agent_execution_id TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE executors_running (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL UNIQUE,
			task_id TEXT NOT NULL,
			agent_execution_id TEXT NOT NULL,
			status TEXT NOT NULL
		);
		CREATE TABLE workspace_coordinator_grants (
			workspace_id TEXT PRIMARY KEY,
			coordinator_task_id TEXT NOT NULL,
			created_by_user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`); err != nil {
		t.Fatalf("create exact-cancel fixture schema: %v", err)
	}
	execExactFixtureSQL(t, db, `INSERT INTO workspaces (id, owner_id) VALUES (?, 'owner-a')`, exactWorkspaceID)
	execExactFixtureSQL(t, db, `INSERT INTO workflows (id, workspace_id) VALUES (?, ?), (?, ?)`,
		exactCurrentWorkflowID, exactWorkspaceID, exactTargetWorkflowID, exactWorkspaceID)
	execExactFixtureSQL(t, db, `INSERT INTO workflow_steps (id, workflow_id) VALUES (?, ?), (?, ?)`,
		exactCurrentStepID, exactCurrentWorkflowID, exactTargetStepID, exactTargetWorkflowID)
	execExactFixtureSQL(t, db, `
		INSERT INTO tasks (id, workspace_id, parent_id, workflow_id, workflow_step_id) VALUES
			(?, ?, '', ?, ?), (?, ?, ?, ?, ?)
	`, exactCallerTaskID, exactWorkspaceID, exactCurrentWorkflowID, exactCurrentStepID,
		exactTargetTaskID, exactWorkspaceID, exactCallerTaskID, exactCurrentWorkflowID, exactCurrentStepID)
	execExactFixtureSQL(t, db, `
		INSERT INTO task_sessions (id, task_id, state, agent_execution_id) VALUES
			(?, ?, 'RUNNING', ?), (?, ?, 'WAITING_FOR_INPUT', '')
	`, exactCallerSessionID, exactCallerTaskID, exactCallerExecutionID,
		exactTargetSessionID, exactTargetTaskID)
	execExactFixtureSQL(t, db, `
		INSERT INTO executors_running (id, session_id, task_id, agent_execution_id, status)
			VALUES (?, ?, ?, ?, 'running')
	`, exactCallerSessionID, exactCallerSessionID, exactCallerTaskID, exactCallerExecutionID)
	execExactFixtureSQL(t, db, `
		INSERT INTO workspace_coordinator_grants
			(workspace_id, coordinator_task_id, created_by_user_id, created_at, updated_at)
			VALUES (?, ?, 'owner-a', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, exactWorkspaceID, exactCallerTaskID)
	repo, err := NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	move := &PendingMove{
		MoveID: exactMoveID, TaskID: exactTargetTaskID, WorkflowID: exactTargetWorkflowID,
		WorkflowStepID: exactTargetStepID, Actor: "agent", SenderSessionID: exactCallerSessionID,
	}
	if err := repo.SetPendingMove(context.Background(), exactTargetSessionID, move); err != nil {
		t.Fatalf("seed pending move: %v", err)
	}
	controlMove := &PendingMove{
		MoveID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", TaskID: exactCallerTaskID,
		WorkflowID: exactCurrentWorkflowID, WorkflowStepID: exactCurrentStepID,
	}
	if err := repo.SetPendingMove(context.Background(), exactCallerSessionID, controlMove); err != nil {
		t.Fatalf("seed control pending move: %v", err)
	}
	execExactFixtureSQL(t, db, `UPDATE tasks SET metadata = ? WHERE id = ?`,
		`{"tags":["preserve-me"]}`, exactTargetTaskID)
	execExactFixtureSQL(t, db, `
		INSERT INTO queued_messages (id, session_id, task_id, position, content, queued_at)
			VALUES (?, ?, ?, 1, ?, CURRENT_TIMESTAMP)
	`, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", exactTargetSessionID, exactTargetTaskID, "withheld prompt")

	return exactCancelFixture{
		repo: repo,
		sql:  db,
		actor: PendingMoveCancellationActor{
			Kind: "coordinator", ID: exactCallerTaskID, UserID: "owner-a", WorkspaceID: exactWorkspaceID,
			CallerTaskID: exactCallerTaskID, CallerSessionID: exactCallerSessionID,
			CallerExecutionID: exactCallerExecutionID,
		},
		match: ExactPendingMoveMatch{
			PendingMoveID: move.ID, SessionID: exactTargetSessionID, TaskID: exactTargetTaskID,
			MoveID: exactMoveID, WorkflowID: exactTargetWorkflowID,
			ExpectedCurrentWorkflowStepID: exactCurrentStepID,
			ExpectedTargetWorkflowStepID:  exactTargetStepID,
		},
		controlPendingMoveID: controlMove.ID,
	}
}

func execExactFixtureSQL(t *testing.T, db *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(query), args...); err != nil {
		t.Fatalf("seed exact-cancel fixture: %v", err)
	}
}

func TestPostgresRepository_ExactCancelPendingMove(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	fixture := newExactCancelFixtureWithDB(t, db)

	result, err := fixture.repo.ExactCancelPendingMove(
		context.Background(), fixture.actor, fixture.match, "correlation-postgres",
	)
	if err != nil || result == nil || !result.Cancelled {
		t.Fatalf("postgres exact cancel result=%#v err=%v", result, err)
	}
	result, err = fixture.repo.ExactCancelPendingMove(
		context.Background(), fixture.actor, fixture.match, "correlation-postgres-retry",
	)
	if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("postgres retry result=%#v err=%v, want stable miss", result, err)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-001.1
func TestSQLiteRepository_ExactCancelPendingMove(t *testing.T) {
	fixture := newExactCancelFixture(t)
	ctx := context.Background()

	result, err := fixture.repo.ExactCancelPendingMove(ctx, fixture.actor, fixture.match, "correlation-success")
	if err != nil {
		t.Fatalf("exact cancel pending move: %v", err)
	}
	if result == nil || !result.Cancelled {
		t.Fatalf("result = %#v, want cancelled", result)
	}
	if result.PendingMoveID != fixture.match.PendingMoveID || result.MoveID != exactMoveID ||
		result.TaskID != exactTargetTaskID || result.SessionID != exactTargetSessionID ||
		result.WorkflowID != exactTargetWorkflowID || result.PriorCurrentWorkflowStepID != exactCurrentStepID ||
		result.PriorTargetWorkflowStepID != exactTargetStepID {
		t.Fatalf("success readback did not preserve exact relation snapshot: %#v", result)
	}
	if move, getErr := fixture.repo.GetPendingMove(ctx, exactTargetSessionID); getErr != nil || move != nil {
		t.Fatalf("target pending move after cancel = %#v, err=%v", move, getErr)
	}
	assertExactCancellationPreservedControlState(t, fixture)

	var audit PendingMoveCancellationAudit
	if err := fixture.sql.GetContext(ctx, &audit, `
		SELECT correlation_id, actor_kind, actor_id, pending_move_id, move_id, task_id, session_id,
			workflow_id, prior_current_workflow_step_id, prior_target_workflow_step_id, outcome, changed
		FROM pending_move_cancellation_audit WHERE correlation_id = ?
	`, "correlation-success"); err != nil {
		t.Fatalf("read success audit: %v", err)
	}
	if audit.Outcome != PendingMoveCancellationOutcomeCancelled || !audit.Changed ||
		audit.PendingMoveID != fixture.match.PendingMoveID || audit.ActorID != exactCallerTaskID {
		t.Fatalf("success audit = %#v", audit)
	}
}

func TestMemoryRepository_ExactPendingMoveAdministrationFailsClosed(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	const (
		sessionID = "22222222-2222-4222-8222-222222222222"
		taskID    = "33333333-3333-4333-8333-333333333333"
	)
	move := &PendingMove{
		MoveID: "44444444-4444-4444-8444-444444444444", TaskID: taskID,
		WorkflowID:     "55555555-5555-4555-8555-555555555555",
		WorkflowStepID: "77777777-7777-4777-8777-777777777777",
	}
	if err := repo.SetPendingMove(ctx, sessionID, move); err != nil {
		t.Fatalf("seed memory pending move: %v", err)
	}
	actor := PendingMoveCancellationActor{
		Kind: "coordinator", ID: "99999999-9999-4999-8999-999999999999",
		CallerTaskID: "99999999-9999-4999-8999-999999999999",
	}
	match := ExactPendingMoveMatch{
		PendingMoveID: move.ID, SessionID: sessionID, TaskID: taskID, MoveID: move.MoveID,
		WorkflowID:                    move.WorkflowID,
		ExpectedCurrentWorkflowStepID: "66666666-6666-4666-8666-666666666666",
		ExpectedTargetWorkflowStepID:  move.WorkflowStepID,
	}

	cancelled, cancelErr := repo.ExactCancelPendingMove(ctx, actor, match, "correlation-memory-cancel")
	if cancelled != nil || !errors.Is(cancelErr, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("memory cancellation result=%#v err=%v, want fail-closed denial", cancelled, cancelErr)
	}
	census, readErr := repo.ReadPendingMoveCensus(ctx, actor, taskID, "correlation-memory-read")
	if census != nil || !errors.Is(readErr, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("memory census result=%#v err=%v, want fail-closed denial", census, readErr)
	}
	stored, err := repo.GetPendingMove(ctx, sessionID)
	if err != nil || stored == nil || stored.ID != move.ID {
		t.Fatalf("fail-closed memory administration mutated row: move=%#v err=%v", stored, err)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-002.3
func TestSQLiteRepository_ExactCancelPendingMoveRejectsOutOfTreeTarget(t *testing.T) {
	fixture := newExactCancelFixture(t)
	const unrelatedTaskID = "12121212-1212-4212-8212-121212121212"
	const unrelatedSessionID = "13131313-1313-4313-8313-131313131313"
	const unrelatedMoveID = "14141414-1414-4414-8414-141414141414"

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
	match := fixture.match
	match.PendingMoveID = move.ID
	match.SessionID = unrelatedSessionID
	match.TaskID = unrelatedTaskID
	match.MoveID = unrelatedMoveID

	result, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, match, "correlation-out-of-tree")
	if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("out-of-tree cancellation result=%#v err=%v, want stable denial", result, err)
	}
	if got, getErr := fixture.repo.GetPendingMove(context.Background(), unrelatedSessionID); getErr != nil || got == nil {
		t.Fatalf("out-of-tree cancellation changed pending move=%#v err=%v", got, getErr)
	}
}

func assertExactCancellationPreservedControlState(t *testing.T, fixture exactCancelFixture) {
	t.Helper()
	var task struct {
		StepID   string `db:"workflow_step_id"`
		Metadata string `db:"metadata"`
	}
	if err := fixture.sql.Get(&task,
		`SELECT workflow_step_id, metadata FROM tasks WHERE id = ?`, exactTargetTaskID); err != nil {
		t.Fatalf("read preserved task state: %v", err)
	}
	if task.StepID != exactCurrentStepID || task.Metadata != `{"tags":["preserve-me"]}` {
		t.Fatalf("task state changed during exact cancellation: %#v", task)
	}
	var sessionState string
	if err := fixture.sql.Get(&sessionState,
		`SELECT state FROM task_sessions WHERE id = ?`, exactTargetSessionID); err != nil {
		t.Fatalf("read preserved session state: %v", err)
	}
	var queueCount int
	if err := fixture.sql.Get(&queueCount,
		`SELECT COUNT(*) FROM queued_messages WHERE session_id = ? AND content = 'withheld prompt'`, exactTargetSessionID); err != nil {
		t.Fatalf("read preserved queue state: %v", err)
	}
	var lockCount int
	if err := fixture.sql.Get(&lockCount, `SELECT COUNT(*) FROM queue_session_locks`); err != nil {
		t.Fatalf("read preserved queue lock state: %v", err)
	}
	control, err := fixture.repo.GetPendingMove(context.Background(), exactCallerSessionID)
	if sessionState != "WAITING_FOR_INPUT" || queueCount != 1 || lockCount != 2 || err != nil || control == nil ||
		control.ID != fixture.controlPendingMoveID {
		t.Fatalf("control state changed: session=%q queue=%d locks=%d control=%#v err=%v",
			sessionState, queueCount, lockCount, control, err)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-001.2
func TestSQLiteRepository_ExactCancelPendingMoveRejectsEveryPredicateMismatch(t *testing.T) {
	mutations := map[string]func(*ExactPendingMoveMatch){
		"pending move id": func(m *ExactPendingMoveMatch) { m.PendingMoveID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" },
		"session id":      func(m *ExactPendingMoveMatch) { m.SessionID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" },
		"task id":         func(m *ExactPendingMoveMatch) { m.TaskID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" },
		"move id":         func(m *ExactPendingMoveMatch) { m.MoveID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" },
		"workflow id":     func(m *ExactPendingMoveMatch) { m.WorkflowID = exactCurrentWorkflowID },
		"current step id": func(m *ExactPendingMoveMatch) { m.ExpectedCurrentWorkflowStepID = exactTargetStepID },
		"target step id":  func(m *ExactPendingMoveMatch) { m.ExpectedTargetWorkflowStepID = exactCurrentStepID },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			match := fixture.match
			mutate(&match)

			result, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, match, "correlation-mismatch")
			if result != nil || err != ErrPendingMoveNotFoundOrChanged {
				t.Fatalf("result=%#v err=%v, want stable not-found-or-changed", result, err)
			}
			move, getErr := fixture.repo.GetPendingMove(context.Background(), exactTargetSessionID)
			if getErr != nil || move == nil || move.ID != fixture.match.PendingMoveID {
				t.Fatalf("mismatch mutated pending row: move=%#v err=%v", move, getErr)
			}
			assertExactCancellationPreservedControlState(t, fixture)
			var changed bool
			if err := fixture.sql.GetContext(context.Background(), &changed,
				`SELECT changed FROM pending_move_cancellation_audit WHERE correlation_id = ?`, "correlation-mismatch"); err != nil {
				t.Fatalf("read mismatch audit: %v", err)
			}
			if changed {
				t.Fatal("mismatch audit reported a state change")
			}
		})
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-002.4
func TestSQLiteRepository_ExactCancelPendingMoveAuthorizationIsNonLeaking(t *testing.T) {
	cases := map[string]func(*exactCancelFixture){
		"ordinary agent": func(f *exactCancelFixture) { f.actor.Kind = "agent" },
		"forged caller task": func(f *exactCancelFixture) {
			f.actor.ID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			f.actor.CallerTaskID = f.actor.ID
		},
		"forged caller session": func(f *exactCancelFixture) {
			f.actor.CallerSessionID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		},
		"forged caller execution": func(f *exactCancelFixture) {
			f.actor.CallerExecutionID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
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
			result, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-denied")
			if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
				t.Fatalf("result=%#v err=%v, want stable non-leaking denial", result, err)
			}
			move, getErr := fixture.repo.GetPendingMove(context.Background(), exactTargetSessionID)
			if getErr != nil || move == nil || move.ID != fixture.match.PendingMoveID {
				t.Fatalf("denial mutated pending move: move=%#v err=%v", move, getErr)
			}
			assertExactCancellationPreservedControlState(t, fixture)
			var targetID string
			if err := fixture.sql.Get(&targetID,
				`SELECT pending_move_id FROM pending_move_cancellation_audit WHERE correlation_id = ?`, "correlation-denied"); err != nil {
				t.Fatalf("read denial audit: %v", err)
			}
			if targetID != "" {
				t.Fatalf("authorization denial audit leaked target ID %q", targetID)
			}
		})
	}
}

func TestSQLiteRepository_ExactCancelPendingMoveRejectsBrokenRelations(t *testing.T) {
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
			result, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-relation")
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
			var auditedTargetID string
			if err := fixture.sql.Get(&auditedTargetID,
				`SELECT pending_move_id FROM pending_move_cancellation_audit WHERE correlation_id = ?`,
				"correlation-relation"); err != nil {
				t.Fatalf("read broken-relation audit: %v", err)
			}
			if auditedTargetID != "" {
				t.Fatalf("broken-relation audit retained untrusted target ID %q", auditedTargetID)
			}
		})
	}
}

func TestSQLiteRepository_ExactDeleteRevalidatesStateAndAuthorization(t *testing.T) {
	cases := map[string]string{
		"current step changed":      `UPDATE tasks SET workflow_step_id = '` + exactTargetStepID + `' WHERE id = '` + exactTargetTaskID + `'`,
		"caller workspace changed":  `UPDATE tasks SET workspace_id = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc' WHERE id = '` + exactCallerTaskID + `'`,
		"caller session mismatched": `UPDATE task_sessions SET task_id = '` + exactTargetTaskID + `' WHERE id = '` + exactCallerSessionID + `'`,
		"caller session stopped":    `UPDATE task_sessions SET state = 'STOPPED' WHERE id = '` + exactCallerSessionID + `'`,
		"execution replaced":        `UPDATE executors_running SET agent_execution_id = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'`,
		"execution stopped":         `UPDATE executors_running SET status = 'stopped'`,
		"grant reassigned":          `UPDATE workspace_coordinator_grants SET coordinator_task_id = '` + exactTargetTaskID + `'`,
		"grant revoked":             `DELETE FROM workspace_coordinator_grants`,
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			repo := fixture.repo.(*sqliteRepository)
			ctx := context.Background()
			tx, err := repo.db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin exact delete revalidation: %v", err)
			}
			defer func() { _ = tx.Rollback() }()
			if err := repo.lockSessionTx(ctx, tx, fixture.match.SessionID); err != nil {
				t.Fatalf("lock exact delete session: %v", err)
			}
			if _, err := repo.readExactCancelTarget(ctx, tx, fixture.match.SessionID); err != nil {
				t.Fatalf("read initial exact target: %v", err)
			}
			if _, err := tx.Exec(mutation); err != nil {
				t.Fatalf("mutate after initial read: %v", err)
			}
			deleted, err := repo.deleteExactCancelTarget(ctx, tx, fixture.actor, fixture.match)
			if err != nil {
				t.Fatalf("revalidate exact delete: %v", err)
			}
			if deleted {
				t.Fatal("exact delete ignored state changed after its initial read")
			}
		})
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-001.3
func TestSQLiteRepository_ExactCancelPendingMoveConcurrentCallers(t *testing.T) {
	fixture := newExactCancelFixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-concurrent")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes, misses := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPendingMoveNotFoundOrChanged):
			misses++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 || misses != 1 {
		t.Fatalf("concurrent outcomes: successes=%d misses=%d, want 1/1", successes, misses)
	}
	var audit struct {
		Total   int `db:"total"`
		Changed int `db:"changed"`
	}
	if err := fixture.sql.Get(&audit, `
		SELECT COUNT(*) AS total, SUM(changed) AS changed
		FROM pending_move_cancellation_audit WHERE correlation_id = ?
	`, "correlation-concurrent"); err != nil {
		t.Fatalf("read concurrent cancellation audit: %v", err)
	}
	if audit.Total != 2 || audit.Changed != 1 {
		t.Fatalf("concurrent audit total=%d changed=%d, want 2/1", audit.Total, audit.Changed)
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-001.4
func TestSQLiteRepository_ExactCancelPendingMoveNeverDeletesReplacement(t *testing.T) {
	fixture := newExactCancelFixture(t)
	replacement := &PendingMove{
		MoveID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", TaskID: exactTargetTaskID,
		WorkflowID: exactTargetWorkflowID, WorkflowStepID: exactTargetStepID,
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-replacement")
		results <- err
	}()
	go func() {
		<-start
		results <- fixture.repo.SetPendingMove(context.Background(), exactTargetSessionID, replacement)
	}()
	close(start)
	errA, errB := <-results, <-results
	if errA != nil && !errors.Is(errA, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("first race result: %v", errA)
	}
	if errB != nil && !errors.Is(errB, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("second race result: %v", errB)
	}
	got, err := fixture.repo.GetPendingMove(context.Background(), exactTargetSessionID)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	if got == nil || got.MoveID != replacement.MoveID || got.ID != replacement.ID {
		t.Fatalf("replacement was deleted: got=%#v want=%#v", got, replacement)
	}
}

func TestSQLiteRepository_ExactCancelPendingMoveRetryIsEffectIdempotent(t *testing.T) {
	fixture := newExactCancelFixture(t)
	if _, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-first"); err != nil {
		t.Fatalf("first cancellation: %v", err)
	}
	result, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-retry")
	if result != nil || !errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
		t.Fatalf("retry result=%#v err=%v", result, err)
	}
}

func TestSQLiteRepository_ExactCancelPendingMoveRollsBackOnAuditOrDeleteFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trigger string
	}{
		{name: "audit", trigger: `
			CREATE TRIGGER fail_pending_move_cancel_audit
			BEFORE INSERT ON pending_move_cancellation_audit
			BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END;
		`},
		{name: "delete", trigger: `
			CREATE TRIGGER fail_pending_move_cancel_delete
			BEFORE DELETE ON pending_moves
			BEGIN SELECT RAISE(ABORT, 'delete unavailable'); END;
		`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			if _, err := fixture.sql.Exec(tc.trigger); err != nil {
				t.Fatalf("install failure trigger: %v", err)
			}
			result, err := fixture.repo.ExactCancelPendingMove(context.Background(), fixture.actor, fixture.match, "correlation-failure")
			if result != nil || !errors.Is(err, ErrPendingMoveCancelFailed) {
				t.Fatalf("result=%#v err=%v, want sanitized cancellation failure", result, err)
			}
			var pendingCount, auditCount int
			if err := fixture.sql.Get(&pendingCount, `SELECT COUNT(*) FROM pending_moves WHERE id = ?`, fixture.match.PendingMoveID); err != nil {
				t.Fatalf("count pending row: %v", err)
			}
			if err := fixture.sql.Get(&auditCount, `SELECT COUNT(*) FROM pending_move_cancellation_audit WHERE correlation_id = ?`, "correlation-failure"); err != nil {
				t.Fatalf("count audit rows: %v", err)
			}
			if pendingCount != 1 || auditCount != 0 {
				t.Fatalf("failure was not atomic: pending=%d audit=%d", pendingCount, auditCount)
			}
		})
	}
}

// @covers AC-TASKS-PENDING-MOVE-CANCELLATION-002.5
func TestService_ExactCancelPendingMoveRejectsMalformedIdentifiersBeforeTargetLookup(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     string
		present   bool
		canonical bool
	}{
		{name: "missing", value: "", present: false, canonical: false},
		{name: "malformed", value: "not-a-uuid", present: true, canonical: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newExactCancelFixture(t)
			svc := NewService(fixture.repo, DefaultMaxPerSession, logger.Default())
			match := fixture.match
			match.PendingMoveID = tc.value
			correlationID := "correlation-invalid-" + tc.name

			result, err := svc.ExactCancelPendingMove(context.Background(), fixture.actor, match, correlationID)
			if result != nil || !errors.Is(err, ErrPendingMoveInvalidArgument) {
				t.Fatalf("result=%#v err=%v, want invalid argument", result, err)
			}
			move, getErr := fixture.repo.GetPendingMove(context.Background(), exactTargetSessionID)
			if getErr != nil || move == nil || move.ID != fixture.match.PendingMoveID {
				t.Fatalf("invalid request touched target: move=%#v err=%v", move, getErr)
			}
			var audit struct {
				PendingMoveID        string `db:"pending_move_id"`
				Outcome              string `db:"outcome"`
				IdentifiersPresent   bool   `db:"identifiers_present"`
				IdentifiersCanonical bool   `db:"identifiers_canonical"`
			}
			if err := fixture.sql.Get(&audit, `
				SELECT pending_move_id, outcome, identifiers_present, identifiers_canonical
				FROM pending_move_cancellation_audit WHERE correlation_id = ?
			`, correlationID); err != nil {
				t.Fatalf("read invalid audit: %v", err)
			}
			if audit.PendingMoveID != "" || audit.Outcome != PendingMoveCancellationOutcomeInvalidArgument ||
				audit.IdentifiersPresent != tc.present || audit.IdentifiersCanonical != tc.canonical {
				t.Fatalf("invalid audit leaked target or shape: %#v", audit)
			}
		})
	}
}
