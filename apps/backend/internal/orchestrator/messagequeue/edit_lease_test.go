package messagequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEditLeaseProtectsTargetWithoutPausingQueuePolicy(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, "session-lease", "task", "second", "", "user", false, nil)
	require.NoError(t, err)

	lease, err := svc.BeginEdit(ctx, second.SessionID, second.ID, "connection-a")
	require.NoError(t, err)
	require.Equal(t, int64(0), lease.TargetRevision)

	got, ok, autoRun := svc.ReserveQueuedWithAutoRun(ctx, first.SessionID)
	require.True(t, autoRun)
	require.True(t, ok)
	require.Equal(t, first.ID, got.ID)
	got, ok, autoRun = svc.ReserveQueuedWithAutoRun(ctx, second.SessionID)
	require.True(t, autoRun)
	require.False(t, ok)
	require.Nil(t, got)

	_, err = svc.BeginEdit(ctx, second.SessionID, second.ID, "connection-b")
	require.ErrorIs(t, err, ErrEditConflict)

	revision, err := svc.UpdateMessageWithLease(ctx, second.SessionID, second.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	revision, err = svc.UpdateMessageWithLease(ctx, second.SessionID, second.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	_, err = svc.UpdateMessageWithLease(ctx, second.SessionID, second.ID, lease.LeaseID,
		"operation-2", "connection-a", lease.TargetRevision, "stale", nil, nil)
	require.ErrorIs(t, err, ErrEditRevisionConflict)

	require.NoError(t, svc.EndEdit(ctx, second.SessionID, second.ID, lease.LeaseID, "connection-a"))
	got, ok, _ = svc.ReserveQueuedWithAutoRun(ctx, second.SessionID)
	require.True(t, ok)
	require.Equal(t, second.ID, got.ID)
}

func TestEditLeaseRejectsOperationHashMismatchWithoutMutation(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-operation", "task", "before", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)

	revision, err := svc.UpdateMessageWithLease(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation-1", "connection",
		lease.TargetRevision, "after", nil, nil,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	prepared := false
	_, err = svc.UpdateMessageWithLeaseAfterValidation(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation-1", "connection",
		lease.TargetRevision, "different", []MessageAttachment{{Type: "image", Data: "new"}},
		nil,
		func(context.Context) error {
			prepared = true
			return nil
		},
		nil,
	)
	require.ErrorIs(t, err, ErrEditRevisionConflict)
	require.False(t, prepared)

	status := svc.GetStatus(ctx, entry.SessionID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, "after", status.Entries[0].Content)
	require.Equal(t, int64(1), svc.editRevisions[svc.editLeaseKey(entry.SessionID, entry.ID)])
}

func TestReserveQueuedWithAutoRunReportsPolicyWhenEditBlocksHead(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-policy", "task", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.SetAutoRun(ctx, entry.SessionID, false))
	_, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	got, ok, autoRun := svc.ReserveQueuedWithAutoRun(ctx, entry.SessionID)
	require.False(t, autoRun)
	require.False(t, ok)
	require.Nil(t, got)
}

func TestEditLeaseRejectsForeignReleaseAndUpdate(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-foreign", "task", "body", "", "user", false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	err = svc.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection-b")
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-b", 0, "tampered", nil, nil)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	require.False(t, errors.Is(err, ErrEditRevisionConflict))
}
func TestEditLeaseEndRejectsExpiredLease(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-expiry", "task", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	svc.editLeases[svc.editLeaseKey(entry.SessionID, entry.ID)].ExpiresAt = time.Now().UTC().Add(-time.Second)

	err = svc.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection-a")
	require.ErrorIs(t, err, ErrEditLeaseNotFound)

	_, ok := svc.TakeQueued(ctx, entry.SessionID)
	require.True(t, ok, "an expired lease must not continue blocking delivery")
}

func TestEditLeaseBlocksTargetedDrains(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-target", "task", "body", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	got, ok := svc.TakeQueued(ctx, entry.SessionID)
	require.False(t, ok)
	require.Nil(t, got)
	_, err = svc.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry})
	require.ErrorIs(t, err, ErrEditConflict)
}
func TestClaimSendNowClearsOrdinaryEditState(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-send-now", "task", "before", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection", lease.TargetRevision, "after", nil, nil)
	require.NoError(t, err)

	key := svc.editLeaseKey(entry.SessionID, entry.ID)
	require.Contains(t, svc.editRevisions, key)
	require.Contains(t, svc.editRevisionTaskIDs, key)
	svc.editLeases[key].ExpiresAt = time.Now().UTC().Add(-time.Second)
	current, err := svc.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	claim, err := svc.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*current})
	require.NoError(t, err)
	require.Len(t, claim.Sources, 1)
	require.NotContains(t, svc.editLeases, key)
	require.NotContains(t, svc.editRevisions, key)
	require.NotContains(t, svc.editRevisionTaskIDs, key)
}

func TestEditLeaseReleaseForDisconnectedConnectionUnblocksTarget(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-disconnect", "task", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	require.Equal(t, 1, svc.ReleaseEditLeasesForConnection("connection-a"))

	got, ok := svc.TakeQueued(ctx, entry.SessionID)
	require.True(t, ok)
	require.Equal(t, entry.ID, got.ID)
}

func TestTakeQueuedIfAutoRunDoesNotDeleteDurableLifecycleEntry(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, _, accepted, err := svc.QueueLifecycleMessageWithCoalesceKey(
		ctx, "session-lifecycle-auto-run", "task", "lifecycle", "", QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": "github_pr_automation"}, "lifecycle-key", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)

	got, ok := svc.TakeQueuedIfAutoRun(ctx, entry.SessionID)
	require.False(t, ok)
	require.Nil(t, got)

	status := svc.GetStatus(ctx, entry.SessionID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, entry.ID, status.Entries[0].ID)
}

func TestLeasedUpdateRollsBackPreparedStateOnPreparationError(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-prepare", "task", "before", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	claimed, rolledBack := false, false
	_, err = svc.UpdateMessageWithLeaseAfterValidation(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation-1", "connection-a",
		lease.TargetRevision, "after", nil, nil,
		func(context.Context) error {
			claimed = true
			return errors.New("attachment claim failed")
		},
		func(context.Context) error {
			rolledBack = true
			return nil
		},
	)
	require.Error(t, err)
	require.True(t, claimed)
	require.True(t, rolledBack)

	status := svc.GetStatus(ctx, entry.SessionID)
	require.Equal(t, "before", status.Entries[0].Content)
}

func TestQueueCoalesceReplacementInvalidatesEditLease(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, replaced, err := svc.QueueMessageWithCoalesceKey(
		ctx, "session-lease-replace", "task", "before", "", QueuedByUser,
		false, nil, nil, "coalesce-key", true,
	)
	require.NoError(t, err)
	require.False(t, replaced)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	replacement, replaced, err := svc.QueueMessageWithCoalesceKey(
		ctx, entry.SessionID, "task", "replacement", "", QueuedByUser,
		false, nil, nil, "coalesce-key", true,
	)
	require.NoError(t, err)
	require.True(t, replaced)
	require.Equal(t, entry.ID, replacement.ID)

	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "stale", nil, nil)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
}
func TestEndEditAfterSaveReportsOnlyFinalizedUpdates(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-save", "task", "before", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	saved, err := svc.EndEditAfterSave(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection")
	require.NoError(t, err)
	require.False(t, saved)

	lease, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection", lease.TargetRevision, "after", nil, nil)
	require.NoError(t, err)

	saved, err = svc.EndEditAfterSave(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection")
	require.NoError(t, err)
	require.True(t, saved)
}
