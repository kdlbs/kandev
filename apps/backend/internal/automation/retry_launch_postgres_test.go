package automation

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func postgresRetryLaunchStore(t *testing.T) (*Store, *sqlx.DB) {
	t.Helper()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := database.Exec(`CREATE TABLE IF NOT EXISTS task_creation_authorizations(kind TEXT,creation_id TEXT,task_id TEXT,PRIMARY KEY(kind,creation_id));CREATE TABLE tasks(id TEXT PRIMARY KEY,metadata TEXT DEFAULT '{}',archived_at TIMESTAMPTZ);
 CREATE TABLE task_sessions(id TEXT PRIMARY KEY,task_id TEXT,is_primary INTEGER DEFAULT 0,state TEXT DEFAULT 'RUNNING',queue_incarnation_id TEXT DEFAULT '',metadata TEXT DEFAULT '{}');
 CREATE TABLE task_session_turns(id TEXT PRIMARY KEY,task_id TEXT,task_session_id TEXT,metadata TEXT DEFAULT '{}');
 CREATE TABLE harness_session_generations(session_id TEXT,incarnation_id TEXT,generation BIGINT);
 CREATE TABLE session_recovery_blocks(id TEXT PRIMARY KEY,session_id TEXT,incarnation_id TEXT,expected_generation BIGINT,reason TEXT,state TEXT,consumer_reference TEXT,created_at TIMESTAMPTZ,updated_at TIMESTAMPTZ,UNIQUE(session_id,incarnation_id,expected_generation,reason));`)
	require.NoError(t, err)
	store, err := NewStore(database, database)
	require.NoError(t, err)
	return store, database
}

func TestPostgresRetryLegacyAdmissionAndWarmQuarantine(t *testing.T) {
	ctx := context.Background()
	store, database := postgresRetryLaunchStore(t)
	a := &Automation{ID: "legacy-auto", WorkspaceID: "workspace", Name: "legacy", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	_, err := database.Exec(`INSERT INTO tasks(id,metadata) VALUES ('legacy-task','{"automation_run_id":"legacy-run"}'),('ordinary-task','{}');
 INSERT INTO task_sessions(id,task_id,queue_incarnation_id) VALUES ('legacy-session','legacy-task','actual-incarnation');
 INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES ('existing-turn','legacy-task','legacy-session');
 INSERT INTO harness_session_generations VALUES ('legacy-session','actual-incarnation',3)`)
	require.NoError(t, err)
	run := &AutomationRun{ID: "legacy-run", AutomationID: a.ID, Status: RunStatusTriggered, RetryGroupID: "legacy-group", RetryGroupGeneration: 1}
	require.NoError(t, store.CreateRun(ctx, run))
	var block struct {
		ID          string `db:"id"`
		SessionID   string `db:"session_id"`
		Incarnation string `db:"incarnation_id"`
		Generation  int64  `db:"expected_generation"`
		State       string `db:"state"`
	}
	query := `SELECT id,session_id,incarnation_id,expected_generation,state FROM session_recovery_blocks WHERE session_id='legacy-session'`
	require.NoError(t, database.GetContext(ctx, &block, query))
	require.Equal(t, "actual-incarnation", block.Incarnation)
	require.EqualValues(t, 3, block.Generation)
	require.Equal(t, "open", block.State)
	originalID := block.ID
	// The task metadata owns this run before its task_id projection is bound.
	_, err = database.Exec(`INSERT INTO task_sessions(id,task_id) VALUES ('older-session','legacy-task')`)
	require.Error(t, err)
	_, err = database.Exec(`INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES ('older-turn','legacy-task','legacy-session')`)
	require.Error(t, err)
	_, err = database.Exec(`UPDATE automation_runs SET status='failed' WHERE id='legacy-run'`)
	require.Error(t, err)
	_, err = database.Exec(`DELETE FROM automation_runs WHERE id='legacy-run'`)
	require.Error(t, err)
	_, err = database.Exec(`INSERT INTO task_sessions(id,task_id) VALUES ('ordinary-session','ordinary-task'); INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES ('ordinary-turn','ordinary-task','ordinary-session')`)
	require.NoError(t, err)
	_, err = NewStore(database, database)
	require.NoError(t, err)
	require.NoError(t, database.GetContext(ctx, &block, query))
	require.Equal(t, originalID, block.ID, "schema replay retains the canonical quarantine")
	var ordinaryBlocks int
	require.NoError(t, database.GetContext(ctx, &ordinaryBlocks, `SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id='ordinary-session'`))
	require.Zero(t, ordinaryBlocks)
}

func secondPostgresRetryStore(t *testing.T, first *sqlx.DB) *Store {
	t.Helper()
	var schema string
	require.NoError(t, first.Get(&schema, `SELECT current_schema()`))
	require.Regexp(t, regexp.MustCompile(`^kandev_test_[a-z0-9]+$`), schema)
	database, err := sqlx.Open("pgx", testutil.PostgresDSNWithSearchPath(t, testutil.PostgresDSNFromEnv(t), schema))
	require.NoError(t, err)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = database.Close() })
	var backendFirst, backendSecond int
	require.NoError(t, first.Get(&backendFirst, `SELECT pg_backend_pid()`))
	require.NoError(t, database.Get(&backendSecond, `SELECT pg_backend_pid()`))
	require.NotEqual(t, backendFirst, backendSecond, "the proof requires independent database connections")
	store, err := NewStore(database, database)
	require.NoError(t, err)
	return store
}

func TestPostgresRetryLaunchFencesIndependentInstancesAndLeaseExpiry(t *testing.T) {
	ctx := context.Background()
	store, database := postgresRetryLaunchStore(t)
	other := secondPostgresRetryStore(t, database)
	a := &Automation{ID: "launch-auto", WorkspaceID: "workspace", Name: "launch", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	run := &AutomationRun{ID: "launch-run", AutomationID: a.ID, TaskID: "launch-task", Status: RunStatusTriggered, RetryGroupID: "launch-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRetryAdmission(ctx, run, &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive}))
	reserved, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	claim, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	_, err = other.ClaimRetryLaunch(ctx, run.ID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	require.NoError(t, other.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
	require.ErrorIs(t, store.AdmitRetryLaunch(ctx, claim, "late-execution"), ErrRetryGenerationMismatch)
	_, err = other.ClaimRetryLaunch(ctx, run.ID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	retained, err := other.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, reserved.SessionID, retained.SessionID)
	require.Equal(t, reserved.TurnID, retained.TurnID)
	require.Equal(t, reserved.SubmissionID, retained.SubmissionID)
	require.ErrorIs(t, other.DeleteRun(ctx, run.ID), ErrRetryLaunchUncertain)
}

func TestPostgresRetryCapacityIsAtomicAcrossIndependentConnections(t *testing.T) {
	ctx := context.Background()
	store, database := postgresRetryLaunchStore(t)
	other := secondPostgresRetryStore(t, database)
	a := &Automation{ID: "capacity-auto", WorkspaceID: "workspace", Name: "capacity", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, candidate := range []*Store{store, other} {
		go func(index int, candidate *Store) {
			<-start
			id := []string{"first-capacity-run", "second-capacity-run"}[index]
			run := &AutomationRun{ID: id, AutomationID: a.ID, Status: RunStatusTriggered, RetryGroupID: "group-" + id, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			results <- candidate.CreateRetryAdmission(ctx, run, &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive})
		}(index, candidate)
	}
	close(start)
	left, right := <-results, <-results
	if left == nil {
		require.ErrorIs(t, right, ErrRetryCapacityReached)
	} else {
		require.ErrorIs(t, left, ErrRetryCapacityReached)
		require.NoError(t, right)
	}
	runs, err := store.ListOpenRuns(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
}

func TestPostgresRetryQueueAcceptanceAndCancellationRetainExactIdentity(t *testing.T) {
	ctx := context.Background()
	store, database := postgresRetryLaunchStore(t)
	other := secondPostgresRetryStore(t, database)
	a := &Automation{ID: "queue-auto", WorkspaceID: "workspace", Name: "queue", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	run := &AutomationRun{ID: "queue-run", AutomationID: a.ID, TaskID: "queue-task", Status: RunStatusTriggered, RetryGroupID: "queue-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRetryAdmission(ctx, run, &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive}))
	reserved, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	first, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.DeferRetryLaunch(ctx, first))
	replay, err := other.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, reserved.SessionID, replay.SessionID)
	require.Equal(t, reserved.TurnID, replay.TurnID)
	require.Equal(t, reserved.SubmissionID, replay.SubmissionID)
	require.ErrorIs(t, store.AdmitRetryLaunch(ctx, first, "stale-execution"), ErrRetryGenerationMismatch)
	_, err = database.Exec(`INSERT INTO task_sessions(id,task_id) VALUES ($1,$2)`, replay.SessionID, replay.TaskID)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES ($1,$2,$3)`, replay.TurnID, replay.TaskID, replay.SessionID)
	require.NoError(t, err)
	require.NoError(t, other.AdmitRetryLaunch(ctx, replay, "execution"))
	_, err = store.RequestRetryLaunchStop(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, other.AcceptRetryLaunch(ctx, replay, "execution"))
	require.ErrorIs(t, store.CancelRetryGroup(ctx, run.RetryGroupID, 1), ErrRetryLaunchUncertain)
	require.ErrorIs(t, store.DeleteAutomation(ctx, a.ID), ErrRetryLaunchUncertain)
	conflicting := *replay
	conflicting.Generation++
	require.ErrorIs(t, store.AcceptRetryLaunch(ctx, &conflicting, "execution"), ErrRetryGenerationMismatch)
	require.NoError(t, store.ObserveRetryLaunchTerminal(ctx, replay.TaskID, replay.SessionID, replay.TurnID, "different-execution"))
	require.ErrorIs(t, store.CancelRetryRun(ctx, run.ID, 1), ErrRetryLaunchUncertain)
	require.NoError(t, store.ObserveRetryLaunchTerminal(ctx, replay.TaskID, replay.SessionID, replay.TurnID, "execution"))
	require.NoError(t, store.CancelRetryRun(ctx, run.ID, 1))
	_, err = database.Exec(`UPDATE automation_runs SET thread_reason='rewritten' WHERE id=$1`, run.ID)
	require.Error(t, err, "terminal history remains immutable")
}

func TestPostgresRetryRetiredAdmissionStillRejectsDelayedOlderCreation(t *testing.T) {
	ctx := context.Background()
	store, _ := postgresRetryLaunchStore(t)
	a := &Automation{ID: "retired-auto", WorkspaceID: "workspace", Name: "retired", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	run := &AutomationRun{ID: "retired-run", AutomationID: a.ID, TaskID: "retired-task", Status: RunStatusTriggered, RetryGroupID: "retired-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRetryAdmission(ctx, run, &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive}))
	assertRetiredRetryAdmission(t, store, run)
}

func TestPostgresRetryFailedDisableRevokesFutureWorkButRetainsAcceptance(t *testing.T) {
	store, _ := postgresRetryLaunchStore(t)
	run := admittedLaunchRun(t, store, "failed-disable")
	assertFailedRetryDisableRetainsOwnership(t, store, run)
}

func TestPostgresLegacyStopReceiptMustMatchTheRetainedOperationTuple(t *testing.T) {
	store, _ := postgresRetryLaunchStore(t)
	run := admittedLaunchRun(t, store, "legacy-operation")
	assertLegacyStopOperationTuple(t, store, run)
}

func TestPostgresRetryUnprojectedOperationCapturesOwnerAndWarmQuarantine(t *testing.T) {
	ctx := context.Background()
	store, database := postgresRetryLaunchStore(t)
	a := &Automation{ID: "unprojected-auto", WorkspaceID: "workspace", Name: "unprojected", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	_, err := database.Exec(`INSERT INTO tasks(id) VALUES ('unprojected-task');INSERT INTO task_sessions(id,task_id) VALUES ('unprojected-session','unprojected-task')`)
	require.NoError(t, err)
	run := &AutomationRun{ID: "unprojected-run", AutomationID: a.ID, Status: RunStatusTriggered, RetryGroupID: "unprojected-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "unprojected-intent", RunID: run.ID, GroupGeneration: 1, State: retryIntentCreated}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{ID: "unprojected-operation", IntentID: intent.ID, RunID: run.ID, GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationAmbiguous, ExternalTaskID: "unprojected-task", ExternalSessionID: "unprojected-session", ExternalTurnID: "observed-turn"}))
	var owner string
	require.NoError(t, database.Get(&owner, `SELECT task_id FROM automation_retry_launch_protocols WHERE run_id='unprojected-run'`))
	require.Equal(t, "unprojected-task", owner)
	var blockState string
	require.NoError(t, database.Get(&blockState, `SELECT state FROM session_recovery_blocks WHERE session_id='unprojected-session'`))
	require.Equal(t, "open", blockState)
	_, err = database.Exec(`INSERT INTO task_sessions(id,task_id) VALUES ('delayed-unprojected','unprojected-task')`)
	require.Error(t, err)
	_, err = NewStore(database, database)
	require.NoError(t, err)
	require.NoError(t, database.Get(&owner, `SELECT task_id FROM automation_retry_launch_protocols WHERE run_id='unprojected-run'`))
	require.Equal(t, "unprojected-task", owner)
	stored, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Empty(t, stored.TaskID)
	require.Empty(t, stored.SessionID)
	require.Empty(t, stored.TurnID)
}

func TestPostgresRetrySchemaProbesRespectMissingTablesAndCancellation(t *testing.T) {
	store, _ := postgresRetryLaunchStore(t)
	assertRetrySchemaProbeCoverage(t, store)
}
