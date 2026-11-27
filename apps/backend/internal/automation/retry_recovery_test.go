package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecoverRetryLedgerPreservesLiveLeaseUntilExpiration(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	automation := &Automation{
		ID: "automation-recovery-live-lease", WorkspaceID: "workspace-recovery-live-lease",
		Name: "recovery live lease", Enabled: true,
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{
		ID: "group-recovery-live-lease", AutomationID: automation.ID,
		Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		ID: "run-recovery-live-lease", AutomationID: automation.ID,
		Status: RunStatusTriggered, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{
		ID: "intent-recovery-live-lease", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted,
	}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "operation-recovery-live-lease", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationRequested,
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "run-recovery-live-lease:1", RunID: run.ID,
		SnapshotVersion: 1, State: retryOutboxPending,
	}))
	outboxLease, err := store.ClaimRetryOutbox(ctx, "run-recovery-live-lease:1", time.Now().UTC(), time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, outboxLease.LeaseToken)

	leased, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationLeased, leased.State)
	require.NotEmpty(t, leased.LeaseToken)

	liveAt := time.Now().UTC()
	require.NoError(t, store.RecoverRetryLedger(ctx, liveAt))
	var liveOutbox RetryOutbox
	require.NoError(t, store.db.GetContext(ctx, &liveOutbox, `SELECT * FROM automation_retry_outbox WHERE event_id = ?`, outboxLease.EventID))
	require.Equal(t, retryOutboxLeased, liveOutbox.State)
	require.Equal(t, outboxLease.LeaseToken, liveOutbox.LeaseToken)
	liveLease, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationLeased, liveLease.State)
	require.Equal(t, leased.LeaseToken, liveLease.LeaseToken)
	require.True(t, liveLease.LeaseExpiresAt.After(liveAt))
	_, err = store.db.ExecContext(ctx, `UPDATE automation_run_operations SET lease_expires_at = NULL WHERE run_id = ?`, run.ID)
	require.NoError(t, err)
	require.NoError(t, store.RecoverRetryLedger(ctx, liveAt))
	malformedLease, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationRequested, malformedLease.State)
	_, err = store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	if _, err := store.BeginRetryTaskOperation(ctx, run.ID, 1); !errors.Is(err, ErrRetryOperationUndispatchable) {
		t.Fatalf("unexpired operation lease error = %v", err)
	}

	expiredAt := liveAt.Add(2 * time.Minute)
	_, err = store.db.ExecContext(ctx, store.db.Rebind(
		`UPDATE automation_run_operations SET lease_expires_at = ? WHERE run_id = ?`),
		expiredAt.Add(-time.Second), run.ID)
	require.NoError(t, err)
	require.NoError(t, store.RecoverRetryLedger(ctx, expiredAt))
	pendingOutbox, err := store.ListPendingRetryOutbox(ctx, expiredAt)
	require.NoError(t, err)
	require.Len(t, pendingOutbox, 1)
	recovered, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationRequested, recovered.State)
	require.Empty(t, recovered.LeaseToken)
	require.Nil(t, recovered.LeaseExpiresAt)
	reacquired, err := store.BeginRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.NoError(t, store.CommitRetryTaskOperation(ctx, run.ID, 1, reacquired.LeaseToken, "stable-task"))
	require.NoError(t, store.RecoverRetryLedger(ctx, time.Now().UTC()))
	committed, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationCommitted, committed.State)
	require.Equal(t, "stable-task", committed.ExternalTaskID)
	require.NoError(t, store.CommitRetryContinuationOperation(ctx, run.ID, 1, "", RunDispatch{
		TaskID: "stable-task", SessionID: "stable-session", TurnID: "stable-turn",
	}))
	upgraded, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, retryOperationCommitted, upgraded.State)
	require.Equal(t, "stable-task", upgraded.ExternalTaskID)
	require.Equal(t, "stable-session", upgraded.ExternalSessionID)
	require.Equal(t, "stable-turn", upgraded.ExternalTurnID)

	err = store.CommitRetryContinuationOperation(ctx, run.ID, 1, "", RunDispatch{
		TaskID: "stable-task", SessionID: "different-session", TurnID: "different-turn",
	})
	require.ErrorIs(t, err, ErrRetryGenerationMismatch)
	unchanged, err := store.GetRetryTaskOperation(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, "stable-session", unchanged.ExternalSessionID)
	require.Equal(t, "stable-turn", unchanged.ExternalTurnID)
}

func TestRetryOutboxClaimIsExclusiveAndAcknowledgementIsTokenFenced(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "exclusive-outbox-event", RunID: "exclusive-outbox-run",
		SnapshotVersion: 1, State: retryOutboxPending,
	}))
	now := time.Now().UTC()
	claimed, err := store.ClaimRetryOutbox(ctx, "exclusive-outbox-event", now, time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, claimed.LeaseToken)

	_, err = store.ClaimRetryOutbox(ctx, "exclusive-outbox-event", now, time.Minute)
	require.ErrorIs(t, err, ErrRetryOutboxLeaseHeld)
	require.ErrorIs(t, store.AcknowledgeRetryEvent(
		ctx, "exclusive-outbox-event", "stale-token", "exclusive-outbox-run", 1,
	), ErrRetryOutboxLeaseHeld)
	require.NoError(t, store.AcknowledgeRetryEvent(
		ctx, claimed.EventID, claimed.LeaseToken, "exclusive-outbox-run", 1,
	))
}
