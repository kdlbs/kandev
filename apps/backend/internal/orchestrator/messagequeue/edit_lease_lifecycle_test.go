package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidateEditLeasesForTaskRejectsStaleUpdates(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-lifecycle", "task-lifecycle", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	lease, err := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	svc.InvalidateEditLeasesForTask(entry.TaskID)

	_, err = svc.UpdateMessageWithLease(ctx, entry.SessionID, entry.ID, lease.LeaseID,
		"operation-1", "connection-a", lease.TargetRevision, "stale edit", nil, nil)
	require.ErrorIs(t, err, ErrEditLeaseNotFound)
}
