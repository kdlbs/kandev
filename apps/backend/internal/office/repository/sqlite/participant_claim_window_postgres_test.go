package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// waitUntilBlockedBy polls until at least one backend is blocked waiting on
// holderPID, and fails the test if that never happens before the deadline.
//
// Reaching this successfully is the proof the AC-004.8 window was entered:
// the registration's claim search has already selected the locked seat as
// its candidate (a plain SELECT is not blocked by a row lock under MVCC) and
// its conditional UPDATE is now waiting on that lock (an UPDATE is). A
// timeout here means the window was NOT entered, which must fail loudly
// rather than let the case report coverage it did not exercise.
func waitUntilBlockedBy(t *testing.T, db *sqlx.DB, holderPID int) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var blocked int
		if err := db.GetContext(ctx, &blocked, `
			SELECT count(*) FROM pg_stat_activity
			WHERE pg_blocking_pids(pid) @> ARRAY[$1]::int[]
		`, holderPID); err != nil {
			t.Fatalf("probe blocked backends: %v", err)
		}
		if blocked > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no backend became blocked on holder pid %d within the deadline: "+
		"the claim's conditional UPDATE never waited on the row lock, so the "+
		"AC-OFFICE-SEAT-PROVENANCE-004.8 window was not entered and this case proves nothing", holderPID)
}

// TestPostgresAddTaskParticipant_ClaimTargetRemovedInsideTheWriteWindow
// covers AC-OFFICE-SEAT-ASSURANCE-002.10, pinning
// AC-OFFICE-SEAT-PROVENANCE-004.8 deterministically.
//
// The window: a seat is selected as claimable, removed before the
// conditional write applies, and the registration must fall through to
// inserting its own seat rather than completing having written nothing.
//
// participants.go's comment on that path says the window "cannot be forced
// deterministically", and
// TestPostgresAddTaskParticipant_ConvergesWithConcurrentRemoveOfClaimTarget
// stands in for it with fifteen concurrent iterations whose split it logs
// but does not assert — so a run that never entered the window passes
// identically to one that did.
//
// The premise is wrong, and the reason is in the code: the claim search is a
// plain read, which a row lock does not block, and the claim is a
// conditional UPDATE of one row by id, which it does. An outside session can
// therefore stand exactly between them:
//
//  1. A holding session takes a row lock on the seat. Nothing else is locked.
//  2. The registration runs. Its advisory lock is uncontended, its identity
//     probe misses, its claim search reads the seat through the row lock and
//     selects it, and its conditional write blocks.
//  3. The test waits until that block is observable. This is the proof.
//  4. The holder deletes the seat and commits.
//  5. The blocked write re-evaluates, matches no row, reports zero rows
//     affected, and the registration falls through and inserts.
//
// No production code changes for this: the window is entered on the shipped
// path, not on a path that exists only under test. The fifteen-iteration
// case is kept — it exercises genuine cross-connection concurrency, which
// this one deliberately does not.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAddTaskParticipant_ClaimTargetRemovedInsideTheWriteWindow(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 5)
	ctx := context.Background()

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	workflowRepo, err := workflowrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init workflow repo: %v", err)
	}
	officeRepo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Claim Window', ?, ?)
	`), "wf-claim-window", now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-claim-window", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	for _, agentID := range []string{"agent-auto-window", "agent-human-window"} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'ws-claim-window', '', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}

	const taskID = "task-claim-window"
	const seatID = "seat-claim-window"
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at) VALUES (?, ?, 'window', ?, ?)
	`), taskID, step.ID, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES (?, ?, ?, 'reviewer', 'agent-auto-window', 1, 0, 'auto')
	`), seatID, step.ID, taskID); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}

	// 1. The holding session locks the seat row and nothing else.
	holder, err := db.Connx(ctx)
	if err != nil {
		t.Fatalf("acquire holder connection: %v", err)
	}
	defer func() { _ = holder.Close() }()
	holdTx, err := holder.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	defer func() { _ = holdTx.Rollback() }()

	var holderPID int
	if err := holdTx.GetContext(ctx, &holderPID, `SELECT pg_backend_pid()`); err != nil {
		t.Fatalf("read holder pid: %v", err)
	}
	var lockedID string
	if err := holdTx.GetContext(ctx, &lockedID,
		`SELECT id FROM workflow_step_participants WHERE id = $1 FOR UPDATE`, seatID); err != nil {
		t.Fatalf("lock seat row: %v", err)
	}

	// 2. The registration selects the locked seat, then blocks on the write.
	type addResult struct {
		result sqlite.ParticipantWriteResult
		err    error
	}
	done := make(chan addResult, 1)
	go func() {
		r, aerr := officeRepo.AddTaskParticipant(ctx, taskID, "agent-human-window", "reviewer")
		done <- addResult{result: r, err: aerr}
	}()

	// 3. Wait until the block is observable — the window itself.
	waitUntilBlockedBy(t, db, holderPID)

	// 4. The holder removes the selected seat and commits.
	if _, err := holdTx.ExecContext(ctx,
		`DELETE FROM workflow_step_participants WHERE id = $1`, seatID); err != nil {
		t.Fatalf("delete seat inside the window: %v", err)
	}
	if err := holdTx.Commit(); err != nil {
		t.Fatalf("commit holder tx: %v", err)
	}

	// 5. The blocked write matches no row and the registration inserts.
	var got addResult
	select {
	case got = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("AddTaskParticipant did not return after the claim target was removed")
	}
	if got.err != nil {
		t.Fatalf("AddTaskParticipant: %v", got.err)
	}
	if got.result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q: a zero-row conditional write must fall through to an insert, "+
			"not be reported as a claim", got.result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if got.result.DisplacedAgentProfileID != "" {
		t.Errorf("displaced agent = %q, want empty (nothing was claimed)", got.result.DisplacedAgentProfileID)
	}

	var seats []struct {
		ID             string `db:"id"`
		AgentProfileID string `db:"agent_profile_id"`
		Provenance     string `db:"provenance"`
	}
	if err := db.SelectContext(ctx, &seats, db.Rebind(`
		SELECT id, agent_profile_id, provenance FROM workflow_step_participants
		WHERE task_id = ? AND role = 'reviewer'
	`), taskID); err != nil {
		t.Fatalf("read slate: %v", err)
	}
	if len(seats) != 1 {
		t.Fatalf("slate = %+v, want exactly 1 seat: an implementation reporting a claim on a "+
			"zero-row write would leave this empty", seats)
	}
	if seats[0].AgentProfileID != "agent-human-window" {
		t.Errorf("seat agent = %q, want agent-human-window", seats[0].AgentProfileID)
	}
	if seats[0].Provenance != string(models.ParticipantProvenanceManual) {
		t.Errorf("seat provenance = %q, want manual", seats[0].Provenance)
	}
	if seats[0].ID == lockedID {
		t.Errorf("seat id = %q, want a fresh seat: the removed seat cannot be the one that survived", seats[0].ID)
	}
}
