package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestService_MergeEnabledDefaultsToTrue asserts a freshly constructed
// Service allows merges out of the box, matching the "enabled by default"
// product requirement for queued-message merging.
func TestService_MergeEnabledDefaultsToTrue(t *testing.T) {
	svc := setupService(t)
	assert.True(t, svc.MergeEnabled())
}

// TestService_MergeIntoAbove_DisabledRejectsMerge asserts SetMergeEnabled(false)
// makes MergeIntoAbove fail with ErrMergeDisabled and leaves the queue
// untouched, without needing per-call plumbing through every merge call site.
func TestService_MergeIntoAbove_DisabledRejectsMerge(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.QueueMessage(ctx, "s", "t", "above", "", "alice", false, nil)
	require.NoError(t, err)
	source, err := svc.QueueMessage(ctx, "s", "t", "source", "", "alice", false, nil)
	require.NoError(t, err)

	svc.SetMergeEnabled(false)

	_, err = svc.MergeIntoAbove(ctx, "s", source.ID, "alice")
	assert.ErrorIs(t, err, ErrMergeDisabled)
	assert.Equal(t, 2, svc.GetStatus(ctx, "s").Count)

	svc.SetMergeEnabled(true)
	merged, err := svc.MergeIntoAbove(ctx, "s", source.ID, "alice")
	require.NoError(t, err)
	assert.Equal(t, "above\n\nsource", merged.Content)
}

// TestGetStatus_ReflectsMergeEnabled asserts QueueStatus.MergeEnabled mirrors
// the live flag, so clients can hide the merge affordance without a separate
// settings round trip.
func TestGetStatus_ReflectsMergeEnabled(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	assert.True(t, svc.GetStatus(ctx, "s").MergeEnabled)

	svc.SetMergeEnabled(false)
	assert.False(t, svc.GetStatus(ctx, "s").MergeEnabled)
}
