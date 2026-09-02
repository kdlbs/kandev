package sqlite_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// openIsolatedPostgresMultiConnForClaimRace is workflow/repository's private
// openIsolatedPostgresMultiConn helper, duplicated here (as it already is in
// internal/task/repository/sqlite) rather than exported: this test needs a
// genuine multi-connection pool — testutil.OpenIsolatedPostgres pins
// SetMaxOpenConns(1), which would force AddTaskParticipant's transaction and
// RecordStepDecision's transaction to queue for the single connection and
// never actually interleave, defeating the point of a race test. It cannot
// live in internal/workflow/repository's test files instead: that package
// exercises AddTaskParticipant (owned by internal/office/repository/sqlite,
// which itself imports internal/workflow/repository for
// ParticipantRoleSeatLockKey), so a workflow/repository-internal test file
// importing office/repository/sqlite would be an import cycle. This file
// lives in the external sqlite_test package specifically to import both
// without one.
func openIsolatedPostgresMultiConnForClaimRace(t *testing.T, dsn string, maxConns int) *sqlx.DB {
	t.Helper()
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	setup, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres (schema setup): %v", err)
	}
	if _, err := setup.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setup.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	_ = setup.Close()
	t.Cleanup(func() {
		if cleanup, cerr := sqlx.Open("pgx", dsn); cerr == nil {
			_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
			_ = cleanup.Close()
		}
	})

	var scopedDSN string
	if strings.Contains(dsn, "://") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		scopedDSN = dsn + sep + "options=" + url.QueryEscape("-c search_path="+schema)
	} else {
		scopedDSN = dsn + " options='-c search_path=" + schema + "'"
	}
	db, err := sqlx.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatalf("open postgres (scoped, %d conns): %v", maxConns, err)
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestPostgresAddTaskParticipant_ClaimDoesNotOverwriteAConcurrentDecision is
// SR21's proof obligation: RecordStepDecision locks on decisionLockNamespace
// while EnsureRoleSeat/AddTaskParticipant lock on participantsLockNamespace —
// two different advisory-lock namespaces that never contend with each other
// on Postgres. Before this fix, AddTaskParticipant's claim UPDATE had no
// decision guard, so a decision committing between the "is this seat
// undecided" read and the claim's UPDATE could be silently reattributed:
// the seat's agent_profile_id would end up naming the claiming agent even
// though the decision on file was recorded by the original auto-cast agent.
// The fix makes the claim's UPDATE conditional on
// `NOT EXISTS (SELECT 1 FROM workflow_step_decisions WHERE participant_id = ?)`
// and falls through to inserting a fresh seat when that affects zero rows
// (see claimAutoSeat in participants.go).
//
// This only proves anything under genuine cross-connection concurrency:
// SQLite's SetMaxOpenConns(1) serializes every transaction for free and
// would pass whether or not the guard exists. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAddTaskParticipant_ClaimDoesNotOverwriteAConcurrentDecision(t *testing.T) {
	const iterations = 15
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 4)
	ctx := context.Background()

	// AddTaskParticipant's identity probe (agentProfileExistsTx) reads
	// agent_profiles, owned by the settings store schema — bring it up
	// before the office schema, matching NewWithDB's ordering contract.
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
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Claim Race', ?, ?)
	`), "wf-claim-race", now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-claim-race", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	// Both raced agents must be live agent_profiles rows (with a
	// satisfied agents(id) foreign key) for the identity probe to
	// treat them as real — Postgres enforces that FK where SQLite's
	// default connection does not.
	for _, agentID := range []string{"agent-auto", "agent-claiming"} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'ws-claim-race', '', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}

	claimedTotal, notClaimedTotal := 0, 0
	for i := 0; i < iterations; i++ {
		taskID := fmt.Sprintf("task-claim-race-%d", i)
		seatID := fmt.Sprintf("seat-claim-race-%d", i)
		seedNow := time.Now().UTC()

		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at) VALUES (?, ?, 'race', ?, ?)
		`), taskID, step.ID, seedNow, seedNow); err != nil {
			t.Fatalf("iteration %d: seed task: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
			VALUES (?, ?, ?, 'reviewer', 'agent-auto', 1, 0, 'auto')
		`), seatID, step.ID, taskID); err != nil {
			t.Fatalf("iteration %d: seed auto seat: %v", i, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var claimResult sqlite.ParticipantWriteResult
		var claimErr, decisionErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			claimResult, claimErr = officeRepo.AddTaskParticipant(ctx, taskID, "agent-claiming", "reviewer")
		}()
		go func() {
			defer wg.Done()
			<-start
			decisionErr = workflowRepo.RecordStepDecision(ctx, &models.WorkflowStepDecision{
				TaskID: taskID, StepID: step.ID, ParticipantID: seatID,
				Decision: "approved", DeciderID: "agent-auto", Role: "reviewer",
			})
		}()
		close(start)
		wg.Wait()

		if claimErr != nil {
			t.Fatalf("iteration %d: AddTaskParticipant: %v", i, claimErr)
		}
		if decisionErr != nil {
			t.Fatalf("iteration %d: RecordStepDecision: %v", i, decisionErr)
		}

		// AC-002.5 protects a decision already on file when the claim
		// examines the seat — it does not promise anything about a decision
		// recorded afterward for an agent a claim has since, legitimately,
		// displaced (RecordAgentDecision's own ResolveParticipantRole
		// pre-check is what keeps a genuinely displaced agent from
		// reaching RecordStepDecision in production; this repository-level
		// test drives RecordStepDecision directly and race-times it against
		// the claim on purpose). So this assertion only applies to the
		// branch where the claim did NOT win: if it had, ParticipantRoleSeatLockKey's
		// shared exclusion (both writers now serialize on it, closing the
		// window claimAutoSeat's NOT EXISTS guard alone could not) guarantees
		// no decision existed yet when the claim's guard ran.
		if claimResult.Outcome == sqlite.ParticipantWriteOutcomeClaimed {
			claimedTotal++
			continue
		}
		notClaimedTotal++

		var decisionCount int
		if err := db.GetContext(ctx, &decisionCount, db.Rebind(
			`SELECT COUNT(*) FROM workflow_step_decisions WHERE participant_id = ?`,
		), seatID); err != nil {
			t.Fatalf("iteration %d: count decisions: %v", i, err)
		}
		var seatAgent string
		if err := db.GetContext(ctx, &seatAgent, db.Rebind(
			`SELECT agent_profile_id FROM workflow_step_participants WHERE id = ?`,
		), seatID); err != nil {
			t.Fatalf("iteration %d: read seat: %v", i, err)
		}

		if decisionCount > 0 && seatAgent != "agent-auto" {
			t.Fatalf(
				"iteration %d: a decision was recorded by agent-auto against seat %s, but the claim reassigned that seat to %q — the decided seat's current holder never actually decided (SR21)",
				i, seatID, seatAgent,
			)
		}
	}

	// Logged, not asserted: which branch wins each iteration is scheduler
	// noise (both goroutines start from the same close(start) signal with
	// no forced ordering), so a run landing all-claimed or all-not-claimed
	// is possible and is not itself a defect. The correctness check inside
	// the loop above is what actually proves SR21 on every "not claimed"
	// iteration; this is just visibility into how the 15 iterations split.
	t.Logf("claimed=%d not-claimed=%d", claimedTotal, notClaimedTotal)
}
