package messagequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEditLeasePreventsAutomaticMergeIntoTarget(t *testing.T) {
	svc := setupService(t)
	svc.SetAutoMergeEnabled(true)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-auto-merge", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	_, err = svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)

	entries := svc.GetStatus(ctx, first.SessionID).Entries
	require.Len(t, entries, 2)
	require.Equal(t, "first", entries[0].Content)
	require.Equal(t, "second", entries[1].Content)
}

func TestEditLeasePreventsManualMergeIntoTarget(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-manual-merge", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	_, err = svc.MergeIntoAbove(ctx, first.SessionID, second.ID, "user")
	require.ErrorIs(t, err, ErrEditConflict)

	entries := svc.GetStatus(ctx, first.SessionID).Entries
	require.Len(t, entries, 2)
	require.Equal(t, "first", entries[0].Content)
	require.Equal(t, "second", entries[1].Content)
	require.False(t, errors.Is(err, ErrEntryNotFound))
}
func TestEditLeaseBlocksReorderOfTarget(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-reorder", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	err = svc.ReorderEntries(ctx, first.SessionID, []string{second.ID, first.ID})
	require.ErrorIs(t, err, ErrEditConflict)

	entries := svc.GetStatus(ctx, first.SessionID).Entries
	require.Len(t, entries, 2)
	require.Equal(t, first.ID, entries[0].ID)
	require.Equal(t, second.ID, entries[1].ID)
}

func TestEditLeaseInvalidatedBySessionReplacement(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-replace", "task", "original", "", "user", false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	replacement := *entry
	replacement.Content = "restored"
	require.NoError(t, svc.RestoreSession(ctx, entry.SessionID, []QueuedMessage{replacement}, nil))

	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "stale edit", nil, nil)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
	require.Equal(t, "restored", svc.GetStatus(ctx, entry.SessionID).Entries[0].Content)
}

func TestEditLeaseBlocksAppendIntoTarget(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-append", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	second, err := svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, second.ID, "connection-a")
	require.NoError(t, err)

	_, _, err = svc.AppendContent(ctx, first.SessionID, "task", " appended", "", "user", false, nil)
	require.ErrorIs(t, err, ErrEditConflict)
	require.Equal(t, "second", svc.GetStatus(ctx, first.SessionID).Entries[1].Content)
}

func TestEditLeaseAllowsAppendToOtherEntry(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	first, err := svc.QueueMessage(ctx, "session-lease-append-other", "task", "first", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.QueueMessage(ctx, first.SessionID, "task", "second", "", "user", false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, first.SessionID, first.ID, "connection-a")
	require.NoError(t, err)

	merged, appended, err := svc.AppendContent(ctx, first.SessionID, "task", " appended", "", "user", false, nil)
	require.NoError(t, err)
	require.True(t, appended)
	require.Equal(t, "second\n\n---\n\n appended", merged.Content)
}
func TestPurgeTaskInvalidatesEditLeases(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lease-purge", "task-purge", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	removed, err := svc.PurgeTask(ctx, entry.TaskID)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	require.Empty(t, svc.editLeases)
}
func TestEditRevisionStateFollowsEntryLifecycle(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	entry, err := svc.QueueMessage(ctx, "session-edit-delete", "task-edit-delete", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.Contains(t, svc.editRevisions, svc.editLeaseKey(entry.SessionID, entry.ID))
	require.NoError(t, svc.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection-a"))
	require.NoError(t, svc.RemoveEntry(ctx, entry.SessionID, entry.ID))
	require.NotContains(t, svc.editRevisions, svc.editLeaseKey(entry.SessionID, entry.ID))
	require.NotContains(t, svc.editRevisionTaskIDs, svc.editLeaseKey(entry.SessionID, entry.ID))

	entry, err = svc.QueueMessage(ctx, "session-edit-transfer", "task-edit-transfer", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-b")
	require.NoError(t, err)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-2", "connection-b", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.NoError(t, svc.TransferSession(ctx, entry.SessionID, "session-edit-transfer-new"))
	require.NotContains(t, svc.editRevisions, svc.editLeaseKey(entry.SessionID, entry.ID))

	entry, err = svc.QueueMessage(ctx, "session-edit-purge", "task-edit-purge", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err = svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-c")
	require.NoError(t, err)
	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-3", "connection-c", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	_, err = svc.PurgeTask(ctx, entry.TaskID)
	require.NoError(t, err)
	require.NotContains(t, svc.editRevisions, svc.editLeaseKey(entry.SessionID, entry.ID))
	require.NotContains(t, svc.editRevisionTaskIDs, svc.editLeaseKey(entry.SessionID, entry.ID))
}
func TestPurgeTaskPreservesOtherEditRevisions(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	live, err := svc.QueueMessage(ctx, "session-live-edit", "task-live", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	other, err := svc.QueueMessage(ctx, "session-other-edit", "task-other", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, live.SessionID, live.ID, "connection-a")
	require.NoError(t, err)
	_, err = svc.BeginEdit(ctx, other.SessionID, other.ID, "connection-b")
	require.NoError(t, err)

	revision, err := svc.UpdateMessageWithLease(ctx, live.SessionID, live.ID, lease.LeaseID,
		"operation-live", "connection-a", lease.TargetRevision, "edited", nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)

	_, err = svc.PurgeTask(ctx, other.TaskID)
	require.NoError(t, err)

	revision, err = svc.UpdateMessageWithLease(ctx, live.SessionID, live.ID, lease.LeaseID,
		"operation-live-2", "connection-a", revision, "edited again", nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), revision)
}
func TestLeaseUpdateFinalizerRunsBeforeSessionTransfer(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-finalize-old", "task-finalize", "before", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	transferDone := make(chan error, 1)
	_, err = svc.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation-finalize", "connection-a",
		lease.TargetRevision, "after", nil, nil, nil, nil,
		func(context.Context, *QueuedMessage) error {
			go func() {
				transferDone <- svc.TransferSession(context.Background(), entry.SessionID, "session-finalize-new")
			}()
			select {
			case err := <-transferDone:
				t.Fatalf("session transfer completed before finalizer returned: %v", err)
			case <-time.After(25 * time.Millisecond):
			}
			return nil
		},
	)
	require.NoError(t, err)
	require.NoError(t, <-transferDone)
	require.Empty(t, svc.GetStatus(ctx, entry.SessionID).Entries)
	require.Equal(t, "after", svc.GetStatus(ctx, "session-finalize-new").Entries[0].Content)
}

func TestLeaseUpdateFinalizerReplayKeepsOriginalPreUpdateSnapshot(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	original := MessageAttachment{AttachmentID: "original"}
	replacement := MessageAttachment{AttachmentID: "replacement"}
	entry, err := svc.QueueMessage(
		ctx, "session-finalize-replay", "task-finalize", "before", "", QueuedByUser, false,
		[]MessageAttachment{original},
	)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)
	finalizeFailure := errors.New("finalize unavailable")
	finalizeCalls := 0
	finalize := func(_ context.Context, previous *QueuedMessage) error {
		finalizeCalls++
		require.Equal(t, []MessageAttachment{original}, previous.Attachments)
		if finalizeCalls == 1 {
			return finalizeFailure
		}
		return nil
	}

	revision, err := svc.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation-finalize-replay", "connection-a",
		lease.TargetRevision, "after", []MessageAttachment{replacement}, nil, nil, nil, finalize,
	)
	require.ErrorIs(t, err, finalizeFailure)
	require.Equal(t, int64(1), revision)
	require.Equal(t, []MessageAttachment{replacement}, svc.GetStatus(ctx, entry.SessionID).Entries[0].Attachments)

	revision, err = svc.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, entry.SessionID, entry.ID, lease.LeaseID, "operation-finalize-replay", "connection-a",
		lease.TargetRevision, "after", []MessageAttachment{replacement}, nil, nil, nil, finalize,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), revision)
	require.Equal(t, 2, finalizeCalls)
}
func TestEditLeaseChecksPhysicalHeadAndTail(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			svc := newAutoMergeTestServiceWithRepository(t, repo, 10)
			svc.SetAutoMergeEnabled(false)
			ctx := context.Background()

			t.Run("reserved head does not expose a leased visible row", func(t *testing.T) {
				head := &QueuedMessage{
					SessionID: "session-physical-head", TaskID: "task-head",
					Content: "head", QueuedBy: QueuedByWorkflow,
					Metadata: map[string]interface{}{MetadataLifecycleDurable: true},
				}
				require.NoError(t, repo.Insert(ctx, head, 0))
				_, err := svc.QueueMessage(ctx, head.SessionID, "task-user", "user", "", QueuedByUser, false, nil)
				require.NoError(t, err)
				reserved, ok := svc.ReserveQueued(ctx, head.SessionID)
				require.True(t, ok)
				require.Equal(t, head.ID, reserved.ID)
				user := svc.GetStatus(ctx, head.SessionID).Entries[0]
				_, err = svc.BeginEdit(ctx, user.SessionID, user.ID, "connection")
				require.NoError(t, err)

				recovered, ok := svc.ReserveQueued(ctx, head.SessionID)
				require.True(t, ok)
				require.Equal(t, head.ID, recovered.ID)
			})

			t.Run("reserved tail does not expose a leased visible row", func(t *testing.T) {
				user, err := svc.QueueMessage(ctx, "session-physical-tail", "task-user", "user", "", QueuedByUser, false, nil)
				require.NoError(t, err)
				tail := &QueuedMessage{
					SessionID: user.SessionID, TaskID: "task-tail",
					Content: "tail", QueuedBy: QueuedByWorkflow,
					Metadata: map[string]interface{}{MetadataLifecycleDurable: true},
				}
				require.NoError(t, repo.Insert(ctx, tail, 0))
				taken, ok := svc.TakeQueued(ctx, user.SessionID)
				require.True(t, ok)
				require.Equal(t, user.ID, taken.ID)
				reserved, ok := svc.ReserveQueued(ctx, user.SessionID)
				require.True(t, ok)
				require.Equal(t, tail.ID, reserved.ID)
				restored, err := svc.RestoreMessage(ctx, taken)
				require.NoError(t, err)
				_, err = svc.BeginEdit(ctx, restored.SessionID, restored.ID, "connection")
				require.NoError(t, err)

				inserted, appended, err := svc.AppendContent(ctx, restored.SessionID, "task-user", "new", "", QueuedByUser, false, nil)
				require.NoError(t, err)
				require.False(t, appended)
				require.NotEqual(t, restored.ID, inserted.ID)
				entries, _, err := svc.SnapshotSession(ctx, restored.SessionID)
				require.NoError(t, err)
				require.Len(t, entries, 3)
				require.Equal(t, restored.ID, entries[0].ID)
				require.Equal(t, tail.ID, entries[1].ID)
				require.Equal(t, inserted.ID, entries[2].ID)
			})
		})
	}
}
