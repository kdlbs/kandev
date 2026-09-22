package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExactProfileAssignmentSchemaExists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	var tableName string
	if err := db.Get(&tableName, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'task_exact_profile_assignments'
	`); err != nil {
		t.Fatalf("exact profile assignment table is missing: %v", err)
	}
	if err := db.Get(&tableName, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'task_exact_profile_launch_attempt_bindings'`); err != nil {
		t.Fatalf("exact profile launch attempt binding table is missing: %v", err)
	}
	rows, err := db.Queryx(`PRAGMA table_info(task_exact_profile_launch_attempt_bindings)`)
	if err != nil {
		t.Fatalf("binding table info: %v", err)
	}
	defer rows.Close()
	type column struct {
		Name, Type  string
		NotNull, PK int
	}
	var got []column
	for rows.Next() {
		var c column
		var cid, def any
		if err := rows.Scan(&cid, &c.Name, &c.Type, &c.NotNull, &def, &c.PK); err != nil {
			t.Fatal(err)
		}
		got = append(got, c)
	}
	want := []column{{"task_id", "TEXT", 1, 1}, {"session_id", "TEXT", 1, 2}, {"execution_id", "TEXT", 1, 0}, {"attempt_id", "TEXT", 1, 0}, {"session_incarnation_id", "TEXT", 1, 0}, {"agent_profile_id", "TEXT", 1, 0}, {"profile_revision_nanos", "BIGINT", 1, 0}, {"generation", "BIGINT", 1, 0}, {"created_at", "TIMESTAMP", 1, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("binding columns = %#v, want %#v", got, want)
	}
	fkRows, err := db.Queryx(`PRAGMA foreign_key_list(task_exact_profile_launch_attempt_bindings)`)
	if err != nil {
		t.Fatal(err)
	}
	defer fkRows.Close()
	if !fkRows.Next() {
		t.Fatal("binding foreign key missing")
	}
	var id, seq int
	var table, from, to, onUpdate, onDelete, match string
	if err := fkRows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil || table != "tasks" || from != "task_id" || to != "id" || onDelete != "CASCADE" {
		t.Fatalf("binding foreign key=%s/%s/%s/%s err=%v", table, from, to, onDelete, err)
	}
}

func TestExactProfileBindingSchemaReopenLeavesLegacySessionsUnbound(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), assignment); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordExactProfileLaunchReceipt(t.Context(), &models.ExactProfileLaunchReceipt{TaskID: assignment.TaskID, SessionID: "legacy-session", AgentProfileID: assignment.AgentProfileID, Generation: assignment.Generation, ProfileRevision: assignment.ProfileRevision, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewWithDB(repo.db, repo.ro, nil)
	if err != nil {
		t.Fatalf("reopen exact schema: %v", err)
	}
	var bindings int
	if err := reopened.db.GetContext(t.Context(), &bindings, `SELECT COUNT(*) FROM task_exact_profile_launch_attempt_bindings WHERE task_id = ?`, assignment.TaskID); err != nil {
		t.Fatal(err)
	}
	if bindings != 0 {
		t.Fatalf("legacy launch bindings = %d, want 0", bindings)
	}
	if storedAssignment, err := reopened.GetExactProfileAssignment(t.Context(), assignment.TaskID); err != nil || !exactProfileAssignmentsEqual(storedAssignment, assignment) {
		t.Fatalf("legacy assignment=%#v err=%v", storedAssignment, err)
	}
	if receipt, err := reopened.GetExactProfileLaunchReceipt(t.Context(), assignment.TaskID, "legacy-session"); err != nil || receipt == nil {
		t.Fatalf("legacy receipt = %#v, err=%v", receipt, err)
	}
}

func TestExactProfileBindingSchemaUpgradesPriorExactSchema(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.db.Exec(`DROP TABLE task_exact_profile_launch_attempt_bindings`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AssignExactProfileAssignment(t.Context(), assignment); err != nil {
		t.Fatal(err)
	}
	receipt := &models.ExactProfileLaunchReceipt{TaskID: assignment.TaskID, SessionID: "prior-session", AgentProfileID: assignment.AgentProfileID, Generation: assignment.Generation, ProfileRevision: assignment.ProfileRevision, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if _, err := repo.RecordExactProfileLaunchReceipt(t.Context(), receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithDB(repo.db, repo.ro, nil); err != nil {
		t.Fatalf("upgrade prior exact schema: %v", err)
	}
	var bindings int
	if err := repo.db.GetContext(t.Context(), &bindings, `SELECT COUNT(*) FROM task_exact_profile_launch_attempt_bindings`); err != nil || bindings != 0 {
		t.Fatalf("upgraded bindings=%d err=%v", bindings, err)
	}
	stored, err := repo.GetExactProfileLaunchReceipt(t.Context(), assignment.TaskID, receipt.SessionID)
	if err != nil || stored == nil || stored.Outcome != receipt.Outcome {
		t.Fatalf("preserved prior receipt=%#v err=%v", stored, err)
	}
	storedAssignment, err := repo.GetExactProfileAssignment(t.Context(), assignment.TaskID)
	if err != nil || !exactProfileAssignmentsEqual(storedAssignment, assignment) {
		t.Fatalf("preserved prior assignment=%#v err=%v", storedAssignment, err)
	}
}

func TestExactProfileAttemptBindingGuardsReceipt(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), assignment); err != nil {
		t.Fatal(err)
	}
	binding := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "bound-session", ExecutionID: "execution-1", AttemptID: "attempt-1", SessionIncarnationID: "incarnation-1", AgentProfileID: assignment.AgentProfileID, ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	createExactProfileAttemptSession(t, repo, binding)
	receipt := &models.ExactProfileLaunchReceipt{TaskID: binding.TaskID, SessionID: binding.SessionID, AgentProfileID: binding.AgentProfileID, ProfileRevision: binding.ProfileRevision, Generation: binding.Generation, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), binding, receipt); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("missing binding changed=%v err=%v", changed, err)
	}
	if _, err := repo.BindExactProfileLaunchAttempt(t.Context(), binding); err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), binding, receipt); err != nil || !changed {
		t.Fatalf("current receipt changed=%v err=%v", changed, err)
	}
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), binding, receipt); err != nil || changed {
		t.Fatalf("replay changed=%v err=%v", changed, err)
	}
	stale := *binding
	stale.AttemptID = "attempt-old"
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), &stale, receipt); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("stale binding changed=%v err=%v", changed, err)
	}
}

func TestExactProfileAttemptRejectsSupersededAssignment(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), assignment); err != nil {
		t.Fatal(err)
	}
	binding := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "superseded-session", ExecutionID: "execution-old", AttemptID: "attempt-old", SessionIncarnationID: "incarnation-old", AgentProfileID: assignment.AgentProfileID, ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	createExactProfileAttemptSession(t, repo, binding)
	if _, err := repo.BindExactProfileLaunchAttempt(t.Context(), binding); err != nil {
		t.Fatal(err)
	}
	next := *assignment
	next.Generation++
	next.ProfileRevision = next.ProfileRevision.Add(time.Second)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), &next); err != nil {
		t.Fatal(err)
	}
	receipt := &models.ExactProfileLaunchReceipt{TaskID: binding.TaskID, SessionID: binding.SessionID, AgentProfileID: binding.AgentProfileID, ProfileRevision: binding.ProfileRevision, Generation: binding.Generation, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), binding, receipt); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("superseded receipt changed=%v err=%v", changed, err)
	}
}

func TestExactProfileAttemptRejectsSupersededSessionAndAdmitsSuccessor(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), assignment); err != nil {
		t.Fatal(err)
	}
	old := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "successor-session", ExecutionID: "execution-old", AttemptID: "attempt-old", SessionIncarnationID: "incarnation-old", AgentProfileID: assignment.AgentProfileID, ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	createExactProfileAttemptSession(t, repo, old)
	if _, err := repo.BindExactProfileLaunchAttempt(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	nextAssignment := *assignment
	nextAssignment.Generation++
	nextAssignment.ProfileRevision = nextAssignment.ProfileRevision.Add(time.Second)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), &nextAssignment); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`, "incarnation-successor", old.SessionID); err != nil {
		t.Fatal(err)
	}
	successor := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: old.SessionID, ExecutionID: "execution-successor", AttemptID: "attempt-successor", SessionIncarnationID: "incarnation-successor", AgentProfileID: nextAssignment.AgentProfileID, ProfileRevision: nextAssignment.ProfileRevision, Generation: nextAssignment.Generation, ExpectedPrior: old}
	if changed, err := repo.BindExactProfileLaunchAttempt(t.Context(), successor); err != nil || !changed {
		t.Fatalf("replace binding = (%v, %v)", changed, err)
	}
	oldReceipt := &models.ExactProfileLaunchReceipt{TaskID: old.TaskID, SessionID: old.SessionID, AgentProfileID: old.AgentProfileID, ProfileRevision: old.ProfileRevision, Generation: old.Generation, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), old, oldReceipt); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("old receipt = (%v, %v)", changed, err)
	}
	successorReceipt := &models.ExactProfileLaunchReceipt{TaskID: successor.TaskID, SessionID: successor.SessionID, AgentProfileID: successor.AgentProfileID, ProfileRevision: successor.ProfileRevision, Generation: successor.Generation, Outcome: models.ExactProfileLaunchOutcomeFailedClosed}
	if changed, err := repo.RecordExactProfileLaunchReceiptForAttempt(t.Context(), successor, successorReceipt); err != nil || !changed {
		t.Fatalf("successor receipt = (%v, %v)", changed, err)
	}
}

func TestExactProfileAttemptBindingRequiresCurrentAssignmentSessionAndCAS(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.AssignExactProfileAssignment(t.Context(), assignment); err != nil {
		t.Fatal(err)
	}
	missingSession := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "missing", ExecutionID: "execution", AttemptID: "attempt", SessionIncarnationID: "incarnation", AgentProfileID: assignment.AgentProfileID, ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	if changed, err := repo.BindExactProfileLaunchAttempt(t.Context(), missingSession); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("missing session bind = (%v, %v)", changed, err)
	}
	current := &models.ExactProfileLaunchAttemptBinding{TaskID: assignment.TaskID, SessionID: "cas-session", ExecutionID: "execution-current", AttemptID: "attempt-current", SessionIncarnationID: "incarnation-current", AgentProfileID: assignment.AgentProfileID, ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation}
	createExactProfileAttemptSession(t, repo, current)
	missingAssignment := *current
	missingAssignment.AgentProfileID = "profile-missing"
	if changed, err := repo.BindExactProfileLaunchAttempt(t.Context(), &missingAssignment); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("missing assignment bind = (%v, %v)", changed, err)
	}
	if _, err := repo.BindExactProfileLaunchAttempt(t.Context(), current); err != nil {
		t.Fatal(err)
	}
	forged := *current
	forged.ExecutionID = "execution-forged"
	forged.AttemptID = "attempt-forged"
	if changed, err := repo.BindExactProfileLaunchAttempt(t.Context(), &forged); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("replacement without CAS = (%v, %v)", changed, err)
	}
	wrongPrior := *current
	wrongPrior.AttemptID = "wrong-prior"
	forged.ExpectedPrior = &wrongPrior
	if changed, err := repo.BindExactProfileLaunchAttempt(t.Context(), &forged); changed || !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("replacement with forged CAS = (%v, %v)", changed, err)
	}
}

func createExactProfileAttemptSession(t *testing.T, repo *Repository, binding *models.ExactProfileLaunchAttemptBinding) {
	t.Helper()
	if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: binding.SessionID, TaskID: binding.TaskID, QueueIncarnationID: binding.SessionIncarnationID, State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertExactProfileAssignmentGuardsGeneration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment-generation.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	now := time.Now().UTC().Round(0)
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-exact-assignment", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: now, Generation: 1, SourceWorkflowID: "workflow-1", SourceWorkflowStepID: "step-1",
		SourceTaskState: "TODO",
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		assignment.TaskID, assignment.WorkspaceID, "Exact profile assignment", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	changed, err := repo.UpsertExactProfileAssignment(context.Background(), assignment)
	if err != nil {
		t.Fatalf("create assignment: %v", err)
	}
	if !changed {
		t.Fatal("initial assignment was not recorded")
	}

	stale := *assignment
	stale.AgentProfileID = "profile-stale"
	changed, err = repo.UpsertExactProfileAssignment(context.Background(), &stale)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
		t.Fatalf("replay assignment error = %v, want generation rejection", err)
	}
	if changed {
		t.Fatal("same generation with a different payload must not replace assignment")
	}
}

func TestAssignExactProfileAssignmentActivatesAtomically(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exact-profile-assignment-atomic.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	now := time.Now().UTC().Round(0)
	assignment := &models.ExactProfileAssignment{
		TaskID: "task-atomic", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: now, Generation: 1,
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		assignment.TaskID, assignment.WorkspaceID, "Exact profile assignment", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	changed, err := repo.AssignExactProfileAssignment(context.Background(), assignment)
	if err != nil || !changed {
		t.Fatalf("assign exact profile = (%v, %v), want (true, nil)", changed, err)
	}
	stored, err := repo.GetExactProfileAssignment(context.Background(), assignment.TaskID)
	if err != nil || stored == nil || !stored.Active {
		t.Fatalf("stored assignment = %#v, %v; want active assignment", stored, err)
	}

	changed, err = repo.AssignExactProfileAssignment(context.Background(), assignment)
	if err != nil || changed {
		t.Fatalf("replay exact profile = (%v, %v), want (false, nil)", changed, err)
	}
}

func TestAssignExactProfileAssignmentRejectsTaskLaneChangeAtomically(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()
	assignment.SourceWorkflowID = "workflow-1"
	assignment.SourceWorkflowStepID = "step-expected"
	assignment.SourceTaskState = "TODO"

	result, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET workflow_id = ?, workflow_step_id = ?, state = ? WHERE id = ?
	`), assignment.SourceWorkflowID, "step-changed", assignment.SourceTaskState, assignment.TaskID)
	if err != nil {
		t.Fatalf("move task fixture: %v", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		t.Fatalf("move task fixture rows = %d, %v", rows, rowsErr)
	}

	changed, err := repo.AssignExactProfileAssignment(ctx, assignment)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) || changed {
		t.Fatalf("assignment after lane change = (%v, %v), want fenced rejection", changed, err)
	}
	stored, getErr := repo.GetExactProfileAssignment(ctx, assignment.TaskID)
	if getErr != nil || stored != nil {
		t.Fatalf("stored assignment = %#v, %v; want nil", stored, getErr)
	}
}

func TestActivateExactProfileAssignmentRejectsStaleGeneration(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.UpsertExactProfileAssignment(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	changed, err := repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 2)
	if err != nil || changed {
		t.Fatalf("stale activation = (%v, %v)", changed, err)
	}
	changed, err = repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 1)
	if err != nil || !changed {
		t.Fatalf("activation = (%v, %v)", changed, err)
	}
	changed, err = repo.ActivateExactProfileAssignment(context.Background(), assignment.TaskID, 1)
	if err != nil || changed {
		t.Fatalf("activation replay = (%v, %v)", changed, err)
	}
}

func TestExactProfileLaunchReceiptReplayDoesNotOverwrite(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	receipt := &models.ExactProfileLaunchReceipt{TaskID: assignment.TaskID, SessionID: "session-1", AgentProfileID: assignment.AgentProfileID, Generation: 1, ProfileRevision: assignment.ProfileRevision, Model: "model-1", Outcome: models.ExactProfileLaunchOutcomeApplied, InferenceStarted: true}
	changed, err := repo.RecordExactProfileLaunchReceipt(context.Background(), receipt)
	if err != nil || !changed {
		t.Fatalf("record = (%v, %v)", changed, err)
	}
	receipt.Outcome = models.ExactProfileLaunchOutcomeFailedClosed
	changed, err = repo.RecordExactProfileLaunchReceipt(context.Background(), receipt)
	if err != nil || changed {
		t.Fatalf("replay = (%v, %v)", changed, err)
	}
	stored, err := repo.GetExactProfileLaunchReceipt(context.Background(), assignment.TaskID, receipt.SessionID)
	if err != nil || stored == nil || stored.Outcome != models.ExactProfileLaunchOutcomeApplied {
		t.Fatalf("stored = %#v, %v", stored, err)
	}
	missing, err := repo.GetExactProfileLaunchReceipt(context.Background(), assignment.TaskID, "missing")
	if err != nil || missing != nil {
		t.Fatalf("missing = %#v, %v", missing, err)
	}
}

func TestUpsertExactProfileAssignmentAdvancesGeneration(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	if _, err := repo.UpsertExactProfileAssignment(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	next := *assignment
	next.Generation = 2
	next.AgentProfileID = "profile-2"
	changed, err := repo.UpsertExactProfileAssignment(context.Background(), &next)
	if err != nil || !changed {
		t.Fatalf("advance = (%v, %v)", changed, err)
	}
	changed, err = repo.UpsertExactProfileAssignment(context.Background(), &next)
	if err != nil || changed {
		t.Fatalf("replay = (%v, %v)", changed, err)
	}
}

func TestUpsertExactProfileAssignmentRejectsForeignWorkspace(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	assignment.WorkspaceID = "workspace-foreign"
	changed, err := repo.UpsertExactProfileAssignment(context.Background(), assignment)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) || changed {
		t.Fatalf("foreign workspace = (%v, %v)", changed, err)
	}
}

func newExactProfileAssignmentRepo(t *testing.T) (*Repository, *models.ExactProfileAssignment) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "exact.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Round(0)
	a := &models.ExactProfileAssignment{TaskID: "task-test", WorkspaceID: "workspace-1", AgentProfileID: "profile-1", ProfileRevision: now, Generation: 1}
	if _, err := db.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, a.TaskID, a.WorkspaceID, "Exact", now, now); err != nil {
		t.Fatal(err)
	}
	return repo, a
}

func TestFindExactProfileReusableSessionFiltersGenerationRevisionAndTerminalState(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()
	const revision int64 = 42
	for _, session := range []*models.TaskSession{
		{ID: "exact-terminal", TaskID: assignment.TaskID, State: models.TaskSessionStateCompleted, ExactProfileGeneration: 1, ExactProfileRevision: revision},
		{ID: "exact-wrong-generation", TaskID: assignment.TaskID, State: models.TaskSessionStateWaitingForInput, ExactProfileGeneration: 2, ExactProfileRevision: revision},
		{ID: "exact-wrong-revision", TaskID: assignment.TaskID, State: models.TaskSessionStateWaitingForInput, ExactProfileGeneration: 1, ExactProfileRevision: revision + 1},
		{ID: "exact-reusable", TaskID: assignment.TaskID, State: models.TaskSessionStateWaitingForInput, ExactProfileGeneration: 1, ExactProfileRevision: revision},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create session %s: %v", session.ID, err)
		}
	}

	matching, err := repo.FindExactProfileReusableSession(ctx, assignment.TaskID, 1, revision)
	if err != nil || matching == nil || matching.ID != "exact-reusable" {
		t.Fatalf("matching session = %#v, %v; want exact-reusable", matching, err)
	}
	for _, query := range []struct {
		generation int64
		revision   int64
	}{
		{generation: 2, revision: revision + 1},
		{generation: 3, revision: revision},
		{generation: 1, revision: revision + 2},
	} {
		found, err := repo.FindExactProfileReusableSession(ctx, assignment.TaskID, query.generation, query.revision)
		if err != nil || found != nil {
			t.Fatalf("lookup (%d, %d) = %#v, %v; want nil", query.generation, query.revision, found, err)
		}
	}
}

func TestExactProfileAssignmentRejectsInvalidInput(t *testing.T) {
	repo, assignment := newExactProfileAssignmentRepo(t)
	ctx := context.Background()

	invalid := *assignment
	invalid.AgentProfileID = ""
	changed, err := repo.UpsertExactProfileAssignment(ctx, &invalid)
	if !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) || changed {
		t.Fatalf("missing profile = (%v, %v), want invalid input", changed, err)
	}

	invalid = *assignment
	invalid.ProfileRevision = time.Time{}
	changed, err = repo.UpsertExactProfileAssignment(ctx, &invalid)
	if !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) || changed {
		t.Fatalf("missing revision = (%v, %v), want invalid input", changed, err)
	}

	invalid = *assignment
	invalid.Generation = 0
	changed, err = repo.UpsertExactProfileAssignment(ctx, &invalid)
	if !errors.Is(err, models.ErrExactProfileAssignmentGeneration) || changed {
		t.Fatalf("zero generation = (%v, %v), want generation rejection", changed, err)
	}

	receipt := &models.ExactProfileLaunchReceipt{
		TaskID: assignment.TaskID, SessionID: "session-invalid", AgentProfileID: assignment.AgentProfileID,
		Generation: 1, ProfileRevision: assignment.ProfileRevision, Outcome: "unknown",
	}
	changed, err = repo.RecordExactProfileLaunchReceipt(ctx, receipt)
	if !errors.Is(err, models.ErrExactProfileAssignmentInvalidInput) || changed {
		t.Fatalf("invalid receipt outcome = (%v, %v), want invalid input", changed, err)
	}
}
