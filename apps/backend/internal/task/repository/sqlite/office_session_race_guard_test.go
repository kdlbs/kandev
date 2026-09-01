package sqlite

// Tests for the in-transaction office-session live-pair guard (Change 1) and
// the live-preferring lookup ordering (Change 2). See
// docs/specs/office/{requirements,system-design}/task-session-identity*.md.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// insertOfficeSessionWithStartedAt is insertOfficeSession
// (office_task_session_uniqueness_test.go) with an explicit started_at, so
// ordering tests can control which row is "newer" independent of insertion
// order.
func insertOfficeSessionWithStartedAt(t *testing.T, db *sqlx.DB, s officeSessionSeed, startedAt time.Time) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (
			id, task_id, agent_profile_id, executor_id, executor_profile_id, environment_id,
			repository_id, base_branch, base_commit_sha,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, review_status, is_passthrough, task_environment_id
		) VALUES (?, ?, ?, '', '', '', '', '', '',
		          '{}', '{}', '{}', '{}',
		          ?, '', '{}', ?, NULL, ?,
		          0, '', 0, '')
	`), s.id, s.taskID, s.agentProfileID, string(s.state), startedAt, now)
	if err != nil {
		t.Fatalf("insert office session %s: %v", s.id, err)
	}
}

// TestCreateOfficeTaskSessionRefusesConcurrentLivePair races two
// CreateOfficeTaskSession calls for the SAME (task_id, agent_profile_id)
// pair. Unlike TestCreateOfficeTaskSessionMarksOnlyTheFirstConcurrentSessionAsOrigin
// (which races two DIFFERENT pairs and asserts both succeed), this must
// converge to exactly one live row: one call succeeds, the other is refused
// with ErrOfficeSessionRaceConflict, and the refused call's row is never
// written (AC-001.1, AC-001.2, AC-001.6).
func TestCreateOfficeTaskSessionRefusesConcurrentLivePair(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-live-pair-race"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office live pair race"}))

	agent := "agent-live-pair"
	sessions := []*models.TaskSession{
		{ID: "office-live-pair-1", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated},
		{ID: "office-live-pair-2", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated},
	}
	errs := make([]error, len(sessions))
	var wg sync.WaitGroup
	for i, session := range sessions {
		wg.Add(1)
		go func(i int, session *models.TaskSession) {
			defer wg.Done()
			errs[i] = repo.CreateOfficeTaskSession(ctx, session)
		}(i, session)
	}
	wg.Wait()

	successCount, conflictCount := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrOfficeSessionRaceConflict):
			conflictCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, successCount, "exactly one concurrent create should win the pair")
	require.Equal(t, 1, conflictCount, "the loser should be refused with ErrOfficeSessionRaceConflict")

	created, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, created, 1, "the refused call must not have written a row")
}

func TestPostgresCreateOfficeTaskSessionRefusesConcurrentLivePair(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-office-live-pair-race-pg"
	now := time.Now().UTC()
	_, err = db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, "", "Office live pair race (Postgres)", now, now)
	require.NoError(t, err)

	agent := "agent-live-pair-pg"
	sessions := []*models.TaskSession{
		{ID: "office-live-pair-pg-1", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated},
		{ID: "office-live-pair-pg-2", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated},
	}
	errs := make([]error, len(sessions))
	var wg sync.WaitGroup
	for i, session := range sessions {
		wg.Add(1)
		go func(i int, session *models.TaskSession) {
			defer wg.Done()
			errs[i] = repo.CreateOfficeTaskSession(ctx, session)
		}(i, session)
	}
	wg.Wait()

	successCount, conflictCount := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrOfficeSessionRaceConflict):
			conflictCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, successCount)
	require.Equal(t, 1, conflictCount)

	created, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, created, 1)
}

// TestCreateOfficeTaskSessionAllowsCreateWhenExistingPairRowIsTerminal proves
// the guard's predicate is complement-of-terminal, not "any row for this
// pair": a pair whose only existing row is terminal must still allow a fresh
// create (AC-001.3, AC-001.4).
func TestCreateOfficeTaskSessionAllowsCreateWhenExistingPairRowIsTerminal(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-terminal-pair-retry"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office terminal pair retry"}))

	agent := "agent-terminal-retry"
	require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-terminal-retry-1", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCompleted,
	}))

	err := repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-terminal-retry-2", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCreated,
	})
	require.NoError(t, err, "a terminal-only pair must not block a fresh create")

	created, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, created, 2)
}

// TestCreateOfficeTaskSessionRefusesLiveWhenTerminalRowsAlsoExist proves the
// guard fires even when older terminal rows coexist with the live one — the
// predicate is "any live row exists", not "all rows are live".
func TestCreateOfficeTaskSessionRefusesLiveWhenTerminalRowsAlsoExist(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-live-amid-terminal"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office live amid terminal"}))

	agent := "agent-live-amid-terminal"
	require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-live-amid-terminal-old", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCompleted,
	}))
	require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-live-amid-terminal-live", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateRunning,
	}))

	err := repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-live-amid-terminal-blocked", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCreated,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOfficeSessionRaceConflict))

	created, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, created, 2, "the refused create must not have written a row")
}

// TestCreateOfficeTaskSessionRefusalLeavesExistingRowsUnmodified closes
// Review round 5's Finding C: AC-001.6 requires a refused create to leave
// every existing row for the pair unmodified — no row updated, cancelled,
// merged, or deleted, and no metadata moved between rows. Every other
// refusal test in this file asserts row COUNT only, which a future
// row-healing regression (e.g. cancelling the stale duplicate inside the
// guard) could satisfy while still mutating a row. This snapshots the full
// pre-existing rows before the refused call and asserts they are unchanged
// afterward.
func TestCreateOfficeTaskSessionRefusalLeavesExistingRowsUnmodified(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-refusal-leaves-rows-unmodified"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office refusal leaves rows unmodified"}))

	agent := "agent-refusal-unmodified"
	require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "refusal-unmodified-old", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCompleted,
	}))
	require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "refusal-unmodified-live", TaskID: taskID, AgentProfileID: agent,
		State:                  models.TaskSessionStateRunning,
		ExecutionProfileID:     "profile-before-refusal",
		DownstreamACPSessionID: "acp-session-before-refusal",
	}))

	before, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, before, 2)

	err = repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "refusal-unmodified-blocked", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCreated,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOfficeSessionRaceConflict))

	after, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, after, 2, "the refused create must not have written a row")
	require.Equal(t, before, after,
		"a refused create must leave every existing row for the pair byte-identical — no update, cancel, merge, or metadata move")
}

// TestCreateOfficeTaskSessionSkipsGuardForEmptyAgentProfileID proves the
// guard is bypassed entirely when agent_profile_id is empty (AC-001.5):
// two rows for the same task with no agent profile are unrestricted, exactly
// like today's kanban behavior.
func TestCreateOfficeTaskSessionSkipsGuardForEmptyAgentProfileID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-empty-agent-bypass"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office empty agent bypass"}))

	require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-empty-agent-1", TaskID: taskID, State: models.TaskSessionStateCreated,
	}))
	err := repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-empty-agent-2", TaskID: taskID, State: models.TaskSessionStateCreated,
	})
	require.NoError(t, err, "empty agent_profile_id must bypass the live-pair guard")

	created, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, created, 2)
}

// TestCreateOfficeTaskSessionRefusesLiveRowCreatedByNonOfficePath is the
// repository-layer half of AC-OFFICE-SESSION-IDENTITY-001.8: the guard
// applies to any live row for the pair, including one created by a
// non-office path such as a session spawned on an office task that inherited
// the spawner's profile. Every other guard test in this file seeds its
// pre-existing row through CreateOfficeTaskSession itself, so none of them
// proves the guard also fires against a row that was never inserted through
// the office path. `task_sessions` has no column distinguishing which path
// created a row, so the plain CreateTaskSession call here stands in for that
// non-office path: it produces a live row identical in shape to one the
// office guard would itself have written.
func TestCreateOfficeTaskSessionRefusesLiveRowCreatedByNonOfficePath(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-nonoffice-live-pair"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office guard vs non-office row"}))

	agent := "agent-nonoffice-spawned"
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "nonoffice-spawned-1", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateRunning,
	}))

	err := repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
		ID: "office-wakeup-1", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCreated,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOfficeSessionRaceConflict),
		"the office guard must refuse a live row for the pair even when that row was not created via CreateOfficeTaskSession")

	created, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, created, 1, "the refused office create must not have written a second row")
}

// TestCreateOfficeTaskSessionRefusesLiveForNonCanonicalLiveStates closes
// Review entry 19's MUST-FIX 3 (SQLite half): AC-OFFICE-SESSION-IDENTITY-001.4
// requires the guard to be the complement of the terminal set
// (COMPLETED/FAILED/CANCELLED), not an enumerated allow-list of whichever
// live states happen to be exercised elsewhere. Every other guard test in
// this file seeds CREATED or RUNNING; STARTING and WAITING_FOR_INPUT are
// live states none of them touch. If the guard's SQL predicate were rewritten
// as `state IN ('CREATED','RUNNING')` instead of `state NOT IN (terminal
// set)`, every other test here would stay green while a pair whose only row
// sits in one of these two states would wrongly allow a duplicate create.
//
// IDLE and a fabricated out-of-enum state close Review round 5's Finding B:
// IDLE is an office session's dominant resting state between turns, and no
// guard test at any layer previously seeded it as the blocking row — an
// allow-list regression omitting IDLE would have shipped green. The
// fabricated state proves the complement form extends to states that don't
// exist yet, per AC-001.4's "a state added later is treated identically"
// requirement.
func TestCreateOfficeTaskSessionRefusesLiveForNonCanonicalLiveStates(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateStarting,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateIdle,
		models.TaskSessionState("PAUSED_FUTURE_STATE"),
	} {
		t.Run(string(state), func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			taskID := "task-office-noncanonical-live-" + string(state)
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office non-canonical live state"}))

			agent := "agent-noncanonical-live"
			require.NoError(t, repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
				ID: "noncanonical-live-existing", TaskID: taskID, AgentProfileID: agent,
				State: state,
			}))

			err := repo.CreateOfficeTaskSession(ctx, &models.TaskSession{
				ID: "noncanonical-live-blocked", TaskID: taskID, AgentProfileID: agent,
				State: models.TaskSessionStateCreated,
			})
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrOfficeSessionRaceConflict),
				"a pair whose only row is %s must be treated as live, not terminal", state)

			created, err := repo.ListTaskSessions(ctx, taskID)
			require.NoError(t, err)
			require.Len(t, created, 1, "the refused create must not have written a row")
		})
	}
}

// TestGetTaskSessionByTaskAndAgentPrefersLiveOverNewerTerminal is the
// 62201cdb-shaped livelock regression test: a terminal row started AFTER a
// live row must not shadow it. Before Change 2, ORDER BY started_at DESC
// alone would return the newer terminal row, causing the office scheduler to
// treat a live pair as if it had no session and create a duplicate.
func TestGetTaskSessionByTaskAndAgentPrefersLiveOverNewerTerminal(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-lookup-livelock"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office lookup livelock"}))
	agent := "agent-lookup-livelock"

	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	insertOfficeSessionWithStartedAt(t, repo.db, officeSessionSeed{
		id: "lookup-livelock-live", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateRunning,
	}, older)
	insertOfficeSessionWithStartedAt(t, repo.db, officeSessionSeed{
		id: "lookup-livelock-terminal", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCompleted,
	}, newer)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agent)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "lookup-livelock-live", got.ID, "the live row must win over a newer terminal row")
}

// TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescThenIDDesc covers
// the total tiebreak: multiple live rows for the same pair (legacy
// duplicate-pair data, AC-003.6) resolve deterministically by started_at
// DESC, then id DESC when started_at ties exactly.
func TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescThenIDDesc(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-lookup-tiebreak"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office lookup tiebreak"}))
	agent := "agent-lookup-tiebreak"

	same := time.Now().UTC()
	insertOfficeSessionWithStartedAt(t, repo.db, officeSessionSeed{
		id: "lookup-tiebreak-a", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, same)
	insertOfficeSessionWithStartedAt(t, repo.db, officeSessionSeed{
		id: "lookup-tiebreak-b", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, same)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agent)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "lookup-tiebreak-b", got.ID, "equal started_at must tiebreak on id DESC")
}

// TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescBeforeID exercises
// the started_at DESC clause itself, not just its id DESC tiebreak: two live
// rows with DISTINCT started_at values, where the row with the LATER
// started_at has the lexicographically SMALLER id. Ordering by id DESC alone
// (or with started_at DESC dropped or reordered after id) would pick the
// wrong row here, unlike TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescThenIDDesc
// above, whose two rows share one started_at and so cannot tell started_at
// DESC apart from an id-only ordering.
func TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescBeforeID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-lookup-started-at-precedence"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office lookup started_at precedence"}))
	agent := "agent-lookup-started-at-precedence"

	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	insertOfficeSessionWithStartedAt(t, repo.db, officeSessionSeed{
		id: "lookup-started-at-precedence-z-older", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, older)
	insertOfficeSessionWithStartedAt(t, repo.db, officeSessionSeed{
		id: "lookup-started-at-precedence-a-newer", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, newer)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agent)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "lookup-started-at-precedence-a-newer", got.ID,
		"the later started_at must win even though its id sorts lower than the older row's")
}

func TestPostgresGetTaskSessionByTaskAndAgentPrefersLiveOverNewerTerminal(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-office-lookup-livelock-pg"
	now := time.Now().UTC()
	_, err = db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, "", "Office lookup livelock (Postgres)", now, now)
	require.NoError(t, err)
	agent := "agent-lookup-livelock-pg"

	older := now.Add(-time.Hour)
	newer := now
	insertOfficeSessionWithStartedAt(t, db, officeSessionSeed{
		id: "lookup-livelock-pg-live", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateRunning,
	}, older)
	insertOfficeSessionWithStartedAt(t, db, officeSessionSeed{
		id: "lookup-livelock-pg-terminal", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCompleted,
	}, newer)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agent)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "lookup-livelock-pg-live", got.ID)
}

// TestPostgresGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescThenIDDesc
// is the PostgreSQL dialect counterpart of
// TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescThenIDDesc: two
// live rows sharing one started_at value tiebreak deterministically on id
// DESC, proving the ordering clause carries the same total order on this
// dialect, not just SQLite's.
func TestPostgresGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescThenIDDesc(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-office-lookup-tiebreak-pg"
	now := time.Now().UTC()
	_, err = db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, "", "Office lookup tiebreak (Postgres)", now, now)
	require.NoError(t, err)
	agent := "agent-lookup-tiebreak-pg"

	same := now
	insertOfficeSessionWithStartedAt(t, db, officeSessionSeed{
		id: "lookup-tiebreak-pg-a", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, same)
	insertOfficeSessionWithStartedAt(t, db, officeSessionSeed{
		id: "lookup-tiebreak-pg-b", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, same)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agent)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "lookup-tiebreak-pg-b", got.ID, "equal started_at must tiebreak on id DESC")
}

// TestPostgresGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescBeforeID
// is the PostgreSQL dialect counterpart of
// TestGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescBeforeID: two live
// rows with distinct started_at values where the later row has the
// lexicographically smaller id, proving the query orders by started_at DESC
// ahead of id DESC rather than the reverse.
func TestPostgresGetTaskSessionByTaskAndAgentOrdersLiveByStartedAtDescBeforeID(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-office-lookup-started-at-precedence-pg"
	now := time.Now().UTC()
	_, err = db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, "", "Office lookup started_at precedence (Postgres)", now, now)
	require.NoError(t, err)
	agent := "agent-lookup-started-at-precedence-pg"

	older := now.Add(-time.Hour)
	newer := now
	insertOfficeSessionWithStartedAt(t, db, officeSessionSeed{
		id: "lookup-started-at-precedence-pg-z-older", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, older)
	insertOfficeSessionWithStartedAt(t, db, officeSessionSeed{
		id: "lookup-started-at-precedence-pg-a-newer", taskID: taskID, agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	}, newer)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agent)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "lookup-started-at-precedence-pg-a-newer", got.ID,
		"the later started_at must win even though its id sorts lower than the older row's")
}

// TestGetTaskSessionByTaskAndAgentReturnsNilForAbsentPairOrEmptyIdentifiers
// covers AC-002's nil,nil contract: no matching row, and both
// empty-identifier short-circuits, must not be treated as an error.
func TestGetTaskSessionByTaskAndAgentReturnsNilForAbsentPairOrEmptyIdentifiers(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const taskID = "task-office-lookup-absent"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Office lookup absent"}))

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, "agent-never-seen")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = repo.GetTaskSessionByTaskAndAgent(ctx, "", "agent-never-seen")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = repo.GetTaskSessionByTaskAndAgent(ctx, taskID, "")
	require.NoError(t, err)
	require.Nil(t, got)
}
