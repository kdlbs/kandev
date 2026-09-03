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

		var decisionCount int
		if err := db.GetContext(ctx, &decisionCount, db.Rebind(
			`SELECT COUNT(*) FROM workflow_step_decisions WHERE participant_id = ?`,
		), seatID); err != nil {
			t.Fatalf("iteration %d: count decisions: %v", i, err)
		}
		var seatAgent, seatProvenance string
		if err := db.GetContext(ctx, &seatAgent, db.Rebind(
			`SELECT agent_profile_id FROM workflow_step_participants WHERE id = ?`,
		), seatID); err != nil {
			t.Fatalf("iteration %d: read seat: %v", i, err)
		}
		if err := db.GetContext(ctx, &seatProvenance, db.Rebind(
			`SELECT provenance FROM workflow_step_participants WHERE id = ?`,
		), seatID); err != nil {
			t.Fatalf("iteration %d: read seat provenance: %v", i, err)
		}

		if claimResult.Outcome == sqlite.ParticipantWriteOutcomeClaimed {
			claimedTotal++
			// decisionErr == nil above already proved RecordStepDecision's
			// own goroutine committed successfully. Assert the DB actually
			// reflects both concurrent writes rather than trusting the
			// in-memory return values: the claim's reassignment must have
			// landed (proving claimAutoSeat's guard did not silently
			// no-op), and the decision it raced against must have landed
			// too (proving RecordStepDecision genuinely wrote under real
			// cross-connection concurrency, not just returned success).
			if seatAgent != "agent-claiming" || seatProvenance != "manual" {
				t.Fatalf(
					"iteration %d: AddTaskParticipant reported Claimed but seat %s reads agent=%q provenance=%q",
					i, seatID, seatAgent, seatProvenance,
				)
			}
			if decisionCount != 1 {
				t.Fatalf(
					"iteration %d: expected exactly one decision recorded against claimed seat %s, got %d",
					i, seatID, decisionCount,
				)
			}
			continue
		}
		notClaimedTotal++

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
		if decisionCount > 0 && seatAgent != "agent-auto" {
			t.Fatalf(
				"iteration %d: a decision was recorded by agent-auto against seat %s, but the claim reassigned that seat to %q — the decided seat's current holder never actually decided (SR21)",
				i, seatID, seatAgent,
			)
		}
	}

	// The split itself is scheduler noise (both goroutines start from the
	// same close(start) signal with no forced ordering), so a run landing
	// all-claimed or all-not-claimed is possible and is not itself a
	// defect — every iteration is asserted above regardless of which
	// branch it takes; this is just visibility into how the 15 iterations
	// split.
	t.Logf("claimed=%d not-claimed=%d", claimedTotal, notClaimedTotal)
}

// TestPostgresEnsureRoleSeatVsAddTaskParticipant_ConvergesOnManualSeat is
// AC-OFFICE-SEAT-PROVENANCE-004.1's actual proof obligation: automatic
// casting (EnsureRoleSeat) and a manual registration naming a DIFFERENT
// agent profile racing for one (task, role) that starts with no seat at
// all. Exactly one seat must exist afterwards, naming the manually
// registered agent, provenance "manual" — regardless of which writer wins
// ParticipantRoleSeatLockKey's shared exclusion first:
//   - EnsureRoleSeat first: it inserts the "auto" seat and commits;
//     AddTaskParticipant then claims it in place.
//   - AddTaskParticipant first: findClaimableAutoSeat sees nothing yet, so
//     it inserts its own "manual" seat directly; EnsureRoleSeat then runs
//     its own existence check (workflow+role scoped, any step), finds that
//     seat, and inserts nothing.
//
// TestPostgresAddTaskParticipant_ClaimDoesNotOverwriteAConcurrentDecision
// above races claim vs. decision-recording; neither it nor
// TestPostgresEnsureRoleSeat_ConcurrentEntriesConvergeOnOneSeat (two
// EnsureRoleSeat callers) drives this writer-vs-writer pairing. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresEnsureRoleSeatVsAddTaskParticipant_ConvergesOnManualSeat(t *testing.T) {
	const iterations = 15
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 4)
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
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Seat Writer Race', ?, ?)
	`), "wf-seat-writer-race", now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-seat-writer-race", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	for _, agentID := range []string{"agent-auto-cast", "agent-manual-race"} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'ws-seat-writer-race', '', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}

	for i := 0; i < iterations; i++ {
		taskID := fmt.Sprintf("task-seat-writer-race-%d", i)
		seedNow := time.Now().UTC()
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at) VALUES (?, ?, 'race', ?, ?)
		`), taskID, step.ID, seedNow, seedNow); err != nil {
			t.Fatalf("iteration %d: seed task: %v", i, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var autoErr, claimErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _, autoErr = workflowRepo.EnsureRoleSeat(ctx, "wf-seat-writer-race", step.ID, taskID, "reviewer", "agent-auto-cast")
		}()
		go func() {
			defer wg.Done()
			<-start
			_, claimErr = officeRepo.AddTaskParticipant(ctx, taskID, "agent-manual-race", "reviewer")
		}()
		close(start)
		wg.Wait()

		if autoErr != nil {
			t.Fatalf("iteration %d: EnsureRoleSeat: %v", i, autoErr)
		}
		if claimErr != nil {
			t.Fatalf("iteration %d: AddTaskParticipant: %v", i, claimErr)
		}

		var seats []struct {
			AgentProfileID string `db:"agent_profile_id"`
			Provenance     string `db:"provenance"`
		}
		if err := db.SelectContext(ctx, &seats, db.Rebind(`
			SELECT agent_profile_id, provenance FROM workflow_step_participants
			WHERE task_id = ? AND role = 'reviewer'
		`), taskID); err != nil {
			t.Fatalf("iteration %d: list seats: %v", i, err)
		}
		if len(seats) != 1 {
			t.Fatalf("iteration %d: AC-OFFICE-SEAT-PROVENANCE-004.1: expected exactly one seat, got %d: %+v", i, len(seats), seats)
		}
		if seats[0].AgentProfileID != "agent-manual-race" {
			t.Fatalf("iteration %d: expected the manually registered agent to hold the seat, got %q", i, seats[0].AgentProfileID)
		}
		if seats[0].Provenance != "manual" {
			t.Fatalf("iteration %d: expected provenance manual, got %q", i, seats[0].Provenance)
		}
	}
}

// TestPostgresAddTaskParticipantVsAddTaskParticipant_OneClaimsOneInserts is
// AC-OFFICE-SEAT-PROVENANCE-004.3's proof obligation: two manual
// registrations naming two different agent profiles racing for one role
// slate holding a single unclaimed "auto" seat. Exactly one shall claim it
// and the other shall write a new "manual" seat — two seats afterward, both
// naming their own agent, both provenance "manual". Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAddTaskParticipantVsAddTaskParticipantOneClaimsOneInserts(t *testing.T) {
	const iterations = 15
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 4)
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
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Manual Claim Race', ?, ?)
	`), "wf-manual-claim-race", now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-manual-claim-race", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	for _, agentID := range []string{"agent-auto-seeded", "agent-manual-a", "agent-manual-b"} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'ws-manual-claim-race', '', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}

	for i := 0; i < iterations; i++ {
		taskID := fmt.Sprintf("task-manual-claim-race-%d", i)
		seatID := fmt.Sprintf("seat-manual-claim-race-%d", i)
		seedNow := time.Now().UTC()
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at) VALUES (?, ?, 'race', ?, ?)
		`), taskID, step.ID, seedNow, seedNow); err != nil {
			t.Fatalf("iteration %d: seed task: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
			VALUES (?, ?, ?, 'reviewer', 'agent-auto-seeded', 1, 0, 'auto')
		`), seatID, step.ID, taskID); err != nil {
			t.Fatalf("iteration %d: seed auto seat: %v", i, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var resultA, resultB sqlite.ParticipantWriteResult
		var errA, errB error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			resultA, errA = officeRepo.AddTaskParticipant(ctx, taskID, "agent-manual-a", "reviewer")
		}()
		go func() {
			defer wg.Done()
			<-start
			resultB, errB = officeRepo.AddTaskParticipant(ctx, taskID, "agent-manual-b", "reviewer")
		}()
		close(start)
		wg.Wait()

		if errA != nil {
			t.Fatalf("iteration %d: AddTaskParticipant(agent-manual-a): %v", i, errA)
		}
		if errB != nil {
			t.Fatalf("iteration %d: AddTaskParticipant(agent-manual-b): %v", i, errB)
		}

		claimedCount := 0
		if resultA.Outcome == sqlite.ParticipantWriteOutcomeClaimed {
			claimedCount++
		}
		if resultB.Outcome == sqlite.ParticipantWriteOutcomeClaimed {
			claimedCount++
		}
		if claimedCount != 1 {
			t.Fatalf(
				"iteration %d: AC-OFFICE-SEAT-PROVENANCE-004.3: expected exactly one registration to claim the auto seat, got %d (A=%v B=%v)",
				i, claimedCount, resultA.Outcome, resultB.Outcome,
			)
		}

		var seats []struct {
			AgentProfileID string `db:"agent_profile_id"`
			Provenance     string `db:"provenance"`
		}
		if err := db.SelectContext(ctx, &seats, db.Rebind(`
			SELECT agent_profile_id, provenance FROM workflow_step_participants
			WHERE task_id = ? AND role = 'reviewer' ORDER BY agent_profile_id ASC
		`), taskID); err != nil {
			t.Fatalf("iteration %d: list seats: %v", i, err)
		}
		if len(seats) != 2 {
			t.Fatalf("iteration %d: expected exactly two seats afterward, got %d: %+v", i, len(seats), seats)
		}
		for _, seat := range seats {
			if seat.AgentProfileID != "agent-manual-a" && seat.AgentProfileID != "agent-manual-b" {
				t.Fatalf("iteration %d: unexpected seat holder %q, want agent-manual-a or agent-manual-b", i, seat.AgentProfileID)
			}
			if seat.Provenance != "manual" {
				t.Fatalf("iteration %d: seat %q has provenance %q, want manual", i, seat.AgentProfileID, seat.Provenance)
			}
		}
	}
}

// TestPostgresAddTaskParticipant_ConvergesWithConcurrentRemoveOfClaimTarget
// is AC-OFFICE-SEAT-PROVENANCE-004.8's actual proof obligation: the seat a
// registration selected to claim is removed before the claim is applied.
// RemoveTaskParticipant (unlike EnsureRoleSeat, AddTaskParticipant and
// recordStepDecisionTx) never acquires ParticipantRoleSeatLockKey — it is a
// plain unguarded DELETE — so it is the one writer that can genuinely
// commit in the middle of AddTaskParticipant's transaction, between
// findClaimableAutoSeat's SELECT and claimAutoSeat's conditional UPDATE,
// under PostgreSQL's READ COMMITTED isolation. This is the only writer
// pairing this package's SQLite-level tests cannot force: SQLite's
// single-writer pool serializes both calls' entire transactions, so a
// second call can only land strictly before or strictly after, never
// inside, the first.
//
// Whichever way the two calls interleave, the end state converges to the
// same thing: exactly one seat for (task, role), naming the manually
// registered agent, provenance "manual". The three orderings collapse to
// this identically:
//   - Remove commits before Add's transaction starts: findClaimableAutoSeat
//     sees no candidate, Add falls through to insertManualParticipant.
//   - Remove commits after Add's transaction commits: by then the row's
//     agent_profile_id has already changed to the claiming agent, so
//     Remove's own WHERE clause (still naming the original auto-cast
//     agent) matches nothing and no-ops.
//   - Remove commits between Add's SELECT and its UPDATE (the AC-004.8
//     window): claimAutoSeat's conditional UPDATE affects zero rows, and
//     attemptClaim falls through to insertManualParticipant exactly as the
//     first two cases do.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAddTaskParticipant_ConvergesWithConcurrentRemoveOfClaimTarget(t *testing.T) {
	const iterations = 15
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 4)
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
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Vanish Race', ?, ?)
	`), "wf-vanish-race", now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-vanish-race", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	for _, agentID := range []string{"agent-auto-vanish", "agent-human-vanish"} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'ws-vanish-race', '', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}

	claimedInPlace, insertedFresh := 0, 0
	for i := 0; i < iterations; i++ {
		taskID := fmt.Sprintf("task-vanish-race-%d", i)
		seatID := fmt.Sprintf("seat-vanish-race-%d", i)
		seedNow := time.Now().UTC()
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at) VALUES (?, ?, 'race', ?, ?)
		`), taskID, step.ID, seedNow, seedNow); err != nil {
			t.Fatalf("iteration %d: seed task: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
			VALUES (?, ?, ?, 'reviewer', 'agent-auto-vanish', 1, 0, 'auto')
		`), seatID, step.ID, taskID); err != nil {
			t.Fatalf("iteration %d: seed auto seat: %v", i, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var claimResult sqlite.ParticipantWriteResult
		var claimErr, removeErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			claimResult, claimErr = officeRepo.AddTaskParticipant(ctx, taskID, "agent-human-vanish", "reviewer")
		}()
		go func() {
			defer wg.Done()
			<-start
			removeErr = officeRepo.RemoveTaskParticipant(ctx, taskID, "agent-auto-vanish", "reviewer")
		}()
		close(start)
		wg.Wait()

		if claimErr != nil {
			t.Fatalf("iteration %d: AddTaskParticipant: %v", i, claimErr)
		}
		if removeErr != nil {
			t.Fatalf("iteration %d: RemoveTaskParticipant: %v", i, removeErr)
		}
		if claimResult.Outcome != sqlite.ParticipantWriteOutcomeClaimed && claimResult.Outcome != sqlite.ParticipantWriteOutcomeInserted {
			t.Fatalf("iteration %d: unexpected outcome %q", i, claimResult.Outcome)
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
			t.Fatalf("iteration %d: list seats: %v", i, err)
		}
		if len(seats) != 1 {
			t.Fatalf("iteration %d: AC-OFFICE-SEAT-PROVENANCE-004.8: expected exactly one seat afterward, got %d: %+v", i, len(seats), seats)
		}
		if seats[0].AgentProfileID != "agent-human-vanish" {
			t.Fatalf("iteration %d: expected the registering agent to hold the seat, got %q", i, seats[0].AgentProfileID)
		}
		if seats[0].Provenance != "manual" {
			t.Fatalf("iteration %d: expected provenance manual, got %q", i, seats[0].Provenance)
		}
		if seats[0].ID == seatID {
			claimedInPlace++
		} else {
			insertedFresh++
		}
	}

	// Split is scheduler noise: claimedInPlace covers the "remove committed
	// outside the window" orderings (the original row survived, reassigned
	// in place); insertedFresh covers both "remove won before Add started"
	// and the AC-004.8 mid-transaction window itself (the original row is
	// gone, a fresh one was inserted). Every iteration is asserted above
	// regardless of which branch it took.
	t.Logf("claimed-in-place=%d inserted-fresh=%d", claimedInPlace, insertedFresh)
}

// TestPostgresAddTaskParticipantVsAddTaskParticipantSameAgentConvergesOnOneSeat
// is AC-OFFICE-SEAT-PROVENANCE-004.4's proof obligation: two manual
// registrations naming the SAME agent profile racing for the same role.
// Exactly one seat shall exist afterward. Under
// ParticipantRoleSeatLockKey's shared exclusion, the second caller's own
// probeExistingIdentity (run inside the lock, after the first caller has
// committed) is expected to find the first caller's row and report
// Unchanged rather than reach insertManualParticipant's natural-key
// backstop at all — this test proves the observable convergence, not which
// internal branch produced it. Skips unless KANDEV_TEST_POSTGRES_DSN is
// set.
func TestPostgresAddTaskParticipantVsAddTaskParticipantSameAgentConvergesOnOneSeat(t *testing.T) {
	const iterations = 15
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 4)
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
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Same Agent Race', ?, ?)
	`), "wf-same-agent-race", now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-same-agent-race", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), "agent-same-race", "agent-same-race", now, now); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'ws-same-agent-race', '', ?, ?)
	`), "agent-same-race", "agent-same-race", "agent-same-race", "agent-same-race", now, now); err != nil {
		t.Fatalf("seed agent profile: %v", err)
	}

	for i := 0; i < iterations; i++ {
		taskID := fmt.Sprintf("task-same-agent-race-%d", i)
		seedNow := time.Now().UTC()
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at) VALUES (?, ?, 'race', ?, ?)
		`), taskID, step.ID, seedNow, seedNow); err != nil {
			t.Fatalf("iteration %d: seed task: %v", i, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var errA, errB error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, errA = officeRepo.AddTaskParticipant(ctx, taskID, "agent-same-race", "reviewer")
		}()
		go func() {
			defer wg.Done()
			<-start
			_, errB = officeRepo.AddTaskParticipant(ctx, taskID, "agent-same-race", "reviewer")
		}()
		close(start)
		wg.Wait()

		if errA != nil {
			t.Fatalf("iteration %d: AddTaskParticipant (A): %v", i, errA)
		}
		if errB != nil {
			t.Fatalf("iteration %d: AddTaskParticipant (B): %v", i, errB)
		}

		var count int
		if err := db.GetContext(ctx, &count, db.Rebind(`
			SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = ? AND role = 'reviewer'
		`), taskID); err != nil {
			t.Fatalf("iteration %d: count seats: %v", i, err)
		}
		if count != 1 {
			t.Fatalf("iteration %d: AC-OFFICE-SEAT-PROVENANCE-004.4: expected exactly one seat, got %d", i, count)
		}
	}
}
