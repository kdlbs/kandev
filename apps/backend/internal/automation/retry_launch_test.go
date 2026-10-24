package automation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// These synthetic pre-launch fixtures seed task-operation state independently
// of unified admission. Provenance does not certify a launch or acceptance.
func recordSyntheticRetryAdmission(t *testing.T, store *Store, run *AutomationRun) {
	t.Helper()
	require.NotEmpty(t, run.RetryGroupID)
	require.Empty(t, run.SessionID)
	require.Empty(t, run.TurnID)
	_, err := store.db.Exec(store.db.Rebind(`INSERT INTO automation_retry_launch_protocols(run_id,task_id,version) VALUES (?,?,1) ON CONFLICT(run_id) DO UPDATE SET version=1,task_id=excluded.task_id`), run.ID, run.TaskID)
	require.NoError(t, err)
}

func admittedLaunchRun(t *testing.T, store *Store, id string, continuation ...string) *AutomationRun {
	t.Helper()
	ctx := context.Background()
	installRetryLaunchTestTables(t, store)
	a := &Automation{ID: "launch-automation-" + id, WorkspaceID: "workspace", Name: "launch", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	if len(continuation) > 0 {
		_, err := store.db.Exec(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`, continuation[0], "task-"+id)
		require.NoError(t, err)
	}
	run := &AutomationRun{ID: "launch-run-" + id, AutomationID: a.ID, Status: RunStatusTriggered, RetryGroupID: "launch-group-" + id, RetryGroupGeneration: 1, RetryState: RetryStateTriggered, TaskID: "task-" + id}
	require.NoError(t, store.CreateRetryAdmission(ctx, run, &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive}))
	return run
}

func TestRetryLaunchClaimRetainsImmutableIdentityAcrossQueueAndInstances(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "queue")
	identity, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	require.NotEmpty(t, identity.SessionID)
	require.NotEmpty(t, identity.TurnID)
	require.NotEmpty(t, identity.SubmissionID)
	require.True(t, strings.HasPrefix(identity.SubmissionID, "prompt:retry:"), "reserve the canonical submission identity before delivery normalization")
	claim, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	other := &Store{db: store.db, ro: store.ro}
	_, err = other.ClaimRetryLaunch(ctx, run.ID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	require.NoError(t, store.DeferRetryLaunch(ctx, claim))
	replay, err := other.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, identity.SessionID, replay.SessionID)
	require.Equal(t, identity.TurnID, replay.TurnID)
	require.Equal(t, identity.SubmissionID, replay.SubmissionID)
	require.NotEqual(t, claim.Token, replay.Token)
	require.ErrorIs(t, store.AdmitRetryLaunch(ctx, claim, "old-execution"), ErrRetryGenerationMismatch)
	require.NoError(t, other.AdmitRetryLaunch(ctx, replay, "execution"))
	require.NoError(t, store.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
	_, err = store.ClaimRetryLaunch(ctx, run.ID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	require.NoError(t, other.AcceptRetryLaunch(ctx, replay, "execution"))
	require.NoError(t, other.AcceptRetryLaunch(ctx, replay, "execution"))
	conflicting := *replay
	conflicting.TurnID = "other-turn"
	require.ErrorIs(t, store.AcceptRetryLaunch(ctx, &conflicting, "execution"), ErrRetryGenerationMismatch)
	bound, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, identity.TurnID, bound.TurnID)
	require.Equal(t, ThreadActionCreated, bound.ThreadAction)
	require.Equal(t, "original", bound.ThreadReason)
}

func TestRetryLaunchStopFencesAdmissionAndRetainsUncertainOwnership(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "stop")
	_, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	claim, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	launch, err := store.RequestRetryLaunchStop(ctx, run.ID)
	require.NoError(t, err)
	require.True(t, launch.CancelRequested)
	require.ErrorIs(t, store.AdmitRetryLaunch(ctx, claim, "execution"), ErrRetryGenerationMismatch)
	require.ErrorIs(t, store.CancelRetryGroup(ctx, run.RetryGroupID, 1), ErrRetryLaunchUncertain)
	retained, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, claim.SessionID, retained.SessionID)
	require.NoError(t, store.CompleteRetryLaunchStop(ctx, claim))
	require.NoError(t, store.CancelRetryGroup(ctx, run.RetryGroupID, 1))
}

func TestRetryLaunchAcceptedReceiptSurvivesCancellationRace(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "accepted", "session")
	var err error
	_, err = store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "session", ThreadActionResumed, "continued")
	require.NoError(t, err)
	claim, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.AdmitRetryLaunch(ctx, claim, "execution"))
	_, err = store.RequestRetryLaunchStop(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.AcceptRetryLaunch(ctx, claim, "execution"))
	require.ErrorIs(t, store.CancelRetryGroupsByAutomation(ctx, run.AutomationID), ErrRetryLaunchUncertain)
	_, err = store.DeleteAutomationWithCleanup(ctx, run.AutomationID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	retained, err := store.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RetryLaunchAccepted, retained.State)
	require.True(t, retained.CancelRequested)
}

func TestRetryLegacyLaunchIdentityGuardsRejectOlderSessionAndTurnWrites(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "legacy-guard")
	_, err := store.db.Exec(`DELETE FROM automation_retry_launch_protocols WHERE run_id = ?`, run.ID)
	require.NoError(t, err)
	// These are the repository INSERT boundaries used by an older dispatcher.
	_, err = store.db.Exec(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`, "old-session", run.TaskID)
	require.Error(t, err)
	_, err = store.db.Exec(`INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES (?,?,?)`, "old-turn", run.TaskID, "existing-session")
	require.Error(t, err)
	_, err = store.db.Exec(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`, "ordinary-session", "ordinary-task")
	require.NoError(t, err)
	_, err = store.db.Exec(`INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES (?,?,?)`, "ordinary-turn", "ordinary-task", "ordinary-session")
	require.NoError(t, err)
	_, err = store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "")
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	_, err = store.GetRetryLaunch(ctx, run.ID)
	require.Error(t, err)
	retained, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTaskCreated, retained.Status)
	require.Contains(t, retained.ErrorMessage, "unresolved")
}

func TestRetryLaunchDatabaseBindingCannotBeRewrittenByOlderDispatcher(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "immutable")
	_, err := store.db.Exec(`UPDATE automation_runs SET task_id = ? WHERE id = ?`, "foreign-task", run.ID)
	require.Error(t, err)
	_, err = store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	for _, statement := range []string{`UPDATE automation_runs SET session_id = 'foreign-session' WHERE id = ?`, `UPDATE automation_runs SET turn_id = 'foreign-turn' WHERE id = ?`, `UPDATE automation_runs SET thread_reason = 'changed' WHERE id = ?`, `UPDATE automation_runs SET thread_action = 'resumed' WHERE id = ?`} {
		_, err = store.db.Exec(statement, run.ID)
		require.Error(t, err)
	}
}

func TestRetryCapacityAdmissionIsAtomicAcrossStoreInstances(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "capacity.db") + "?_busy_timeout=5000"
	database, err := sqlx.Open("sqlite3", dsn)
	require.NoError(t, err)
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database, database)
	require.NoError(t, err)
	first := admittedLaunchRun(t, store, "capacity")
	_, err = store.db.Exec(`UPDATE automation_runs SET status = ? WHERE id = ?`, RunStatusFailed, first.ID)
	require.NoError(t, err)
	otherDatabase, err := sqlx.Open("sqlite3", dsn)
	require.NoError(t, err)
	otherDatabase.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = otherDatabase.Close() })
	other, err := NewStore(otherDatabase, otherDatabase)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, candidate := range []*Store{store, other} {
		go func(index int, candidate *Store) {
			<-start
			id := []string{"capacity-a", "capacity-b"}[index]
			run := &AutomationRun{ID: id, AutomationID: first.AutomationID, Status: RunStatusTriggered, RetryGroupID: "group-" + id, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			results <- candidate.CreateRetryAdmission(ctx, run, &RetryGroup{ID: run.RetryGroupID, AutomationID: first.AutomationID, Generation: 1, State: RetryGroupLive})
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
	runs, err := store.ListOpenRuns(ctx, first.AutomationID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
}

func TestRetryTerminalEvidenceRequiresExecutionAndImmutableTurn(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "terminal")
	_, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	launch, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.AdmitRetryLaunch(ctx, launch, "execution"))
	require.NoError(t, store.AcceptRetryLaunch(ctx, launch, "execution"))
	require.NoError(t, store.ObserveRetryLaunchTerminal(ctx, launch.TaskID, launch.SessionID, launch.TurnID, "foreign-execution"))
	require.ErrorIs(t, store.MarkRetrySucceeded(ctx, run.ID, 1), ErrRetryLaunchUncertain)
	require.NoError(t, store.ObserveRetryLaunchTerminal(ctx, launch.TaskID, launch.SessionID, "foreign-turn", "execution"))
	require.ErrorIs(t, store.MarkRetrySucceeded(ctx, run.ID, 1), ErrRetryLaunchUncertain)
	require.NoError(t, store.ObserveRetryLaunchTerminal(ctx, launch.TaskID, launch.SessionID, launch.TurnID, "execution"))
	require.NoError(t, store.MarkRetrySucceeded(ctx, run.ID, 1))
	retained, err := store.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RetryLaunchStopped, retained.State)
}

func TestRetryUnresolvedOwnershipCannotBeDiscardedByOlderMutations(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "retain")
	_, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	require.ErrorIs(t, store.DeleteRun(ctx, run.ID), ErrRetryLaunchUncertain)
	require.ErrorIs(t, store.DeleteAllRuns(ctx, run.AutomationID), ErrRetryLaunchUncertain)
	require.ErrorIs(t, store.DeleteAutomation(ctx, run.AutomationID), ErrRetryLaunchUncertain)
	for _, query := range []string{`DELETE FROM automation_runs WHERE id = ?`, `UPDATE automation_runs SET status = 'failed' WHERE id = ?`} {
		_, err = store.db.Exec(query, run.ID)
		require.Error(t, err)
	}
	_, err = store.RequestRetryLaunchStop(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.CancelRetryGroup(ctx, run.RetryGroupID, 1))
	require.NoError(t, store.DeleteRun(ctx, run.ID))
}

func TestRetryRetiredAdmissionStillRejectsDelayedOlderCreation(t *testing.T) {
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "retired")
	assertRetiredRetryAdmission(t, store, run)
}

func assertRetiredRetryAdmission(t *testing.T, store *Store, run *AutomationRun) {
	t.Helper()
	ctx := context.Background()
	reserved, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	claim, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.AdmitRetryLaunch(ctx, claim, "known-execution"))
	require.NoError(t, store.AcceptRetryLaunch(ctx, claim, "known-execution"))
	_, err = store.RequestRetryLaunchStop(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.CompleteRetryLaunchStop(ctx, claim))
	require.NoError(t, store.CancelRetryRun(ctx, run.ID, run.RetryGroupGeneration))
	for _, afterDelete := range []bool{false, true} {
		if afterDelete {
			require.NoError(t, store.DeleteRun(ctx, run.ID))
		}
		_, err = store.db.Exec(store.db.Rebind(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`), map[bool]string{false: "delayed-after-stop", true: "delayed-after-delete"}[afterDelete], run.TaskID)
		require.Error(t, err, "a stop acknowledgment for one execution cannot authorize an older fresh launch")
		lateTask := map[bool]string{false: "late-task-after-stop", true: "late-task-after-delete"}[afterDelete]
		_, err = store.db.Exec(store.db.Rebind(`INSERT INTO tasks(id,metadata) VALUES (?,?)`), lateTask, `{"automation_run_id":"`+run.ID+`"}`)
		require.NoError(t, err)
		_, err = store.db.Exec(store.db.Rebind(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`), "session-"+lateTask, lateTask)
		require.Error(t, err, "late task creation cannot erase its retired run owner")
		_, err = store.db.Exec(store.db.Rebind(`INSERT INTO task_session_turns(id,task_id,task_session_id) VALUES (?,?,?)`), "turn-"+lateTask, lateTask, reserved.SessionID)
		require.Error(t, err)
	}
	retained, err := store.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, reserved.SessionID, retained.SessionID)
	require.True(t, retained.CancelRequested)
	_, err = store.db.Exec(store.db.Rebind(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`), "ordinary-after-delete", "ordinary-task")
	require.NoError(t, err)
}

func TestRetrySupersessionFencesAdmissionWithoutDiscardingDispatchOwnership(t *testing.T) {
	for _, admit := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "dispatching"}[admit], func(t *testing.T) {
			ctx := context.Background()
			store := setupTestStore(t)
			run := admittedLaunchRun(t, store, "supersede")
			_, err := store.db.Exec(`UPDATE automations SET max_concurrent_runs = 2 WHERE id = ?`, run.AutomationID)
			require.NoError(t, err)
			_, err = store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
			require.NoError(t, err)
			var claim *RetryLaunch
			if admit {
				claim, err = store.ClaimRetryLaunch(ctx, run.ID)
				require.NoError(t, err)
				require.NoError(t, store.AdmitRetryLaunch(ctx, claim, "execution"))
			}
			next := &AutomationRun{ID: "next-run", AutomationID: run.AutomationID, Status: RunStatusTriggered, RetryGroupID: "next-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			require.NoError(t, store.CreateRetryAdmission(ctx, next, &RetryGroup{ID: next.RetryGroupID, AutomationID: run.AutomationID, Generation: 1, State: RetryGroupLive}))
			old, err := store.GetRetryLaunch(ctx, run.ID)
			require.NoError(t, err)
			require.True(t, old.CancelRequested)
			_, err = store.ClaimRetryLaunch(ctx, run.ID)
			require.ErrorIs(t, err, ErrRetryLaunchUncertain)
			if admit {
				require.Equal(t, RetryLaunchDispatching, old.State)
				require.ErrorIs(t, store.VerifyRetryLaunch(ctx, claim, "execution"), ErrRetryGenerationMismatch)
				require.NoError(t, store.AcceptRetryLaunch(ctx, claim, "execution"))
				retained, err := store.GetRun(ctx, run.ID)
				require.NoError(t, err)
				require.Equal(t, RunStatusTaskCreated, retained.Status)
			} else {
				require.Equal(t, RetryLaunchStopped, old.State)
				retained, err := store.GetRun(ctx, run.ID)
				require.NoError(t, err)
				require.Equal(t, RetryStateSuperseded, retained.RetryState)
				require.Equal(t, RunStatusFailed, retained.Status)
			}
		})
	}
}

func TestQueuedRetryDropCannotSettleAClaimedOrAcceptedLaunch(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "claimed"}[claimed], func(t *testing.T) {
			ctx := context.Background()
			store := setupTestStore(t)
			run := admittedLaunchRun(t, store, "drop")
			identity, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
			require.NoError(t, err)
			claim, err := store.ClaimRetryLaunch(ctx, run.ID)
			require.NoError(t, err)
			require.NoError(t, store.DeferRetryLaunch(ctx, claim))
			if claimed {
				reclaimed, claimErr := store.ClaimRetryLaunch(ctx, run.ID)
				require.NoError(t, claimErr)
				require.NotEqual(t, claim.Token, reclaimed.Token)
			}
			svc := NewService(store, nil, nil)
			handled, err := svc.CancelQueuedRetryLaunch(ctx, run.ID)
			require.True(t, handled)
			if claimed {
				require.ErrorIs(t, err, ErrRetryLaunchUncertain)
			} else {
				require.NoError(t, err)
			}
			retained, err := store.GetRetryLaunch(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, identity.SessionID, retained.SessionID)
			if claimed {
				require.Equal(t, RetryLaunchPreparing, retained.State)
			} else {
				require.Equal(t, RetryLaunchStopped, retained.State)
				require.True(t, retained.CancelRequested)
				_, err = store.ClaimRetryLaunch(ctx, run.ID)
				require.Error(t, err)
			}
		})
	}
}

func TestRetryFailedDisableRevokesFutureWorkButRetainsAcceptance(t *testing.T) {
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "failed-disable")
	assertFailedRetryDisableRetainsOwnership(t, store, run)
}

func assertFailedRetryDisableRetainsOwnership(t *testing.T, store *Store, run *AutomationRun) {
	t.Helper()
	ctx := context.Background()
	limit := 2
	require.NoError(t, store.UpdateAutomation(ctx, run.AutomationID, &UpdateAutomationRequest{MaxConcurrentRuns: &limit}))
	_, err := store.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", ThreadActionCreated, "original")
	require.NoError(t, err)
	claim, err := store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.AdmitRetryLaunch(ctx, claim, "in-flight-execution"))
	due := time.Now().Add(-time.Minute)
	future := &AutomationRun{ID: "future-run", AutomationID: run.AutomationID, DedupKey: "independent-future", Status: RunStatusScheduledRetry, RetryGroupID: "future-group", RetryGroupGeneration: 1, RetryState: RetryStateScheduled, RetryScheduledAt: &due}
	require.NoError(t, store.CreateRetryAdmission(ctx, future, &RetryGroup{ID: future.RetryGroupID, AutomationID: run.AutomationID, Generation: 1, State: RetryGroupLive}))
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := NewService(store, nil, log)
	stopErr := errors.New("termination acknowledgement unavailable")
	svc.SetRunStopper(runStopperStub{err: stopErr})
	require.ErrorIs(t, svc.DisableAutomation(ctx, run.AutomationID), stopErr)
	disabled, err := store.GetAutomation(ctx, run.AutomationID)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	cancelledFuture, err := store.GetRun(ctx, future.ID)
	require.NoError(t, err)
	require.Equal(t, RetryStateCancelled, cancelledFuture.RetryState)
	for _, id := range []string{run.RetryGroupID, future.RetryGroupID} {
		group, err := store.GetRetryGroup(ctx, id)
		require.NoError(t, err)
		require.Equal(t, RetryGroupCancelled, group.State)
		require.Greater(t, group.Generation, int64(1))
	}
	require.NoError(t, svc.EnableAutomation(ctx, run.AutomationID))
	_, _, err = store.ClaimDueRetry(ctx, time.Now().Add(time.Minute), time.Minute)
	require.ErrorIs(t, err, ErrNoDueRetry)
	require.NoError(t, store.AcceptRetryLaunch(ctx, claim, "in-flight-execution"), "late acceptance must remain owned after revocation")
	accepted, err := store.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RetryLaunchAccepted, accepted.State)
	require.True(t, accepted.CancelRequested)
	visible, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.NotEmpty(t, visible.ErrorMessage, "late acceptance cannot erase an unresolved stop")
	require.Equal(t, claim.SessionID, accepted.SessionID)
	require.Equal(t, claim.TurnID, accepted.TurnID)
	require.Equal(t, claim.SubmissionID, accepted.SubmissionID)
	_, err = store.ClaimRetryLaunch(ctx, run.ID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
	_, err = store.DeleteAutomationWithCleanup(ctx, run.AutomationID)
	require.ErrorIs(t, err, ErrRetryLaunchUncertain)
}

func TestLegacyStopReceiptMustMatchTheRetainedOperationTuple(t *testing.T) {
	store := setupTestStore(t)
	run := admittedLaunchRun(t, store, "legacy-operation")
	assertLegacyStopOperationTuple(t, store, run)
}

func assertLegacyStopOperationTuple(t *testing.T, store *Store, run *AutomationRun) {
	t.Helper()
	ctx := context.Background()
	_, err := store.db.Exec(store.db.Rebind(`UPDATE automation_retry_launch_protocols SET version=0 WHERE run_id=?`), run.ID)
	require.NoError(t, err)
	_, err = store.db.Exec(store.db.Rebind(`UPDATE automation_run_operations SET state=?,external_task_id=?,external_session_id=?,external_turn_id=? WHERE run_id=?`), retryOperationAmbiguous, run.TaskID, "accepted-session", "accepted-turn", run.ID)
	require.NoError(t, err)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := NewService(store, nil, log)
	owned, err := svc.ListOpenRunsByTaskID(ctx, run.TaskID)
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.Equal(t, "accepted-session", owned[0].SessionID)
	require.Equal(t, "accepted-turn", owned[0].TurnID)
	stored, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Empty(t, stored.SessionID, "operation hydration must not manufacture accepted binding")
	require.Empty(t, stored.TurnID)
	observed := *run
	observed.SessionID = "foreign-session"
	observed.TurnID = "accepted-turn"
	require.ErrorIs(t, store.recordLegacyRetryStop(ctx, &observed), ErrRetryGenerationMismatch)
	observed.SessionID = "accepted-session"
	observed.RetryGroupGeneration++
	require.ErrorIs(t, store.recordLegacyRetryStop(ctx, &observed), ErrRetryGenerationMismatch)
	observed.RetryGroupGeneration = run.RetryGroupGeneration
	require.NoError(t, store.RevokeRetryAdmissions(ctx, run.AutomationID, run.RetryGroupID))
	require.NoError(t, store.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
	var operationState string
	require.NoError(t, store.db.Get(&operationState, store.db.Rebind(`SELECT state FROM automation_run_operations WHERE run_id=?`), run.ID))
	require.Equal(t, retryOperationAbandoned, operationState, "revocation retains observed identities even when dispatch is abandoned")
	_, err = store.db.Exec(store.db.Rebind(`UPDATE automation_retry_launch_protocols SET task_id=? WHERE run_id=?`), "foreign-owner", run.ID)
	require.NoError(t, err)
	require.ErrorIs(t, store.recordLegacyRetryStop(ctx, &observed), ErrRetryGenerationMismatch)
	_, err = store.db.Exec(store.db.Rebind(`UPDATE automation_retry_launch_protocols SET task_id=? WHERE run_id=?`), run.TaskID, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.recordLegacyRetryStop(ctx, &observed))
	var evidence struct {
		TaskID  string `db:"task_id"`
		Version int    `db:"version"`
	}
	require.NoError(t, store.db.Get(&evidence, store.db.Rebind(`SELECT task_id,version FROM automation_retry_launch_protocols WHERE run_id=?`), run.ID))
	require.Equal(t, run.TaskID, evidence.TaskID)
	require.Equal(t, -1, evidence.Version)
	require.NoError(t, store.CancelRetryRun(ctx, run.ID, run.RetryGroupGeneration))
	require.NoError(t, store.DeleteRun(ctx, run.ID))
	_, err = store.db.Exec(store.db.Rebind(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`), "delayed-after-operation-stop", run.TaskID)
	require.Error(t, err, "resolved acceptance must retain admission revocation")
}

func assertRetrySchemaProbeCoverage(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	present, err := retryRequiredTablesPresent(ctx, store.db, "tasks", "task_sessions")
	require.NoError(t, err)
	require.True(t, present)
	present, err = retryRequiredTablesPresent(ctx, store.db, "tasks", "missing_retry_fixture_table")
	require.NoError(t, err)
	require.False(t, present, "partial schema cannot authorize retry guards")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	present, err = retryRequiredTablesPresent(cancelled, store.db, "tasks")
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, present, "unavailable schema evidence must fail closed")
}

func TestRetrySchemaProbesRespectMissingTablesAndCancellation(t *testing.T) {
	store := setupTestStore(t)
	admittedLaunchRun(t, store, "schema-probes")
	assertRetrySchemaProbeCoverage(t, store)
}

func installRetryLaunchTestTables(t *testing.T, store *Store) {
	t.Helper()
	_, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS task_creation_authorizations(kind TEXT,creation_id TEXT,task_id TEXT,PRIMARY KEY(kind,creation_id));CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY, metadata TEXT DEFAULT '{}', archived_at TIMESTAMP);CREATE TABLE IF NOT EXISTS task_sessions(id TEXT PRIMARY KEY,task_id TEXT,is_primary INTEGER DEFAULT 0,state TEXT DEFAULT 'RUNNING',metadata TEXT DEFAULT '{}');CREATE TABLE IF NOT EXISTS task_session_turns(id TEXT PRIMARY KEY,task_id TEXT,task_session_id TEXT,metadata TEXT DEFAULT '{}')`)
	require.NoError(t, err)
	require.NoError(t, store.installRetryIdentityGuards(context.Background()))
}

func reserveFiredRetryLaunch(t *testing.T, svc *Service, runID, taskID, sessionID string, action ThreadAction) *RetryLaunch {
	t.Helper()
	ctx := context.Background()
	installRetryLaunchTestTables(t, svc.store)
	if sessionID != "" {
		_, err := svc.store.db.Exec(`INSERT INTO task_sessions(id,task_id) VALUES (?,?)`, sessionID, taskID)
		require.NoError(t, err)
	}
	require.NoError(t, svc.store.BindRunTask(ctx, runID, taskID, ""))
	launch, err := svc.ReserveRetryLaunch(ctx, runID, taskID, sessionID, action, "fixture")
	require.NoError(t, err)
	return launch
}
