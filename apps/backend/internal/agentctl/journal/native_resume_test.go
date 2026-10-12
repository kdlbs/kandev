package journal

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
func TestNativeResumeRetiresUnknownWithoutInventingOutcome(t *testing.T) {
	ctx := context.Background()
	j, event := terminalSubmissionFixture(t, SubmissionInterruptedUnknown, false)
	retired, err := j.RetireSubmission(ctx, event.SubmissionID, event.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, retired.Retired)
	require.Equal(t, SubmissionInterruptedUnknown, retired.State)
	require.Equal(t, []byte("prompt"), retired.Payload)
	require.Equal(t, event.HarnessGeneration, retired.HarnessGeneration)
	unresolved, err := j.HasUnresolvedSubmissions(ctx)
	require.NoError(t, err)
	require.False(t, unresolved)
	work, err := j.HasUnresolvedWork(ctx)
	require.NoError(t, err)
	require.False(t, work)
	descriptor, err := j.RecoveryDescriptor(ctx, event.SessionID, event.IncarnationID, event.HarnessGeneration, event.StreamID)
	require.NoError(t, err)
	require.False(t, descriptor.Unresolved)
	_, err = j.PutSubmission(ctx, retired)
	require.ErrorIs(t, err, ErrSubmissionState, "retirement must never make the original prompt replayable")
	_, err = j.RetireSubmission(ctx, event.SubmissionID, event.HarnessGeneration)
	require.NoError(t, err, "a repeated explicit acknowledgement is idempotent")
}

func TestNativeResumeCannotRetireLiveWork(t *testing.T) {
	for _, state := range []SubmissionState{SubmissionPrepared, SubmissionAccepted, SubmissionDispatching} {
		t.Run(string(state), func(t *testing.T) {
			j, event := terminalSubmissionFixture(t, state, false)
			_, err := j.RetireSubmission(context.Background(), event.SubmissionID, event.HarnessGeneration)
			require.ErrorIs(t, err, ErrSubmissionGeneration)
			stored, err := j.GetSubmission(context.Background(), event.SubmissionID)
			require.NoError(t, err)
			require.False(t, stored.Retired)
			require.Equal(t, state, stored.State)
		})
	}
}

func TestRecoveredUnknownHistorySurvivesGenerationAndReopen(t *testing.T) {
	ctx := context.Background()
	j, event := terminalSubmissionFixture(t, SubmissionInterruptedUnknown, false)
	retired, err := j.RetireSubmission(ctx, event.SubmissionID, event.HarnessGeneration+1)
	require.NoError(t, err)
	require.Equal(t, SubmissionInterruptedUnknown, retired.State)
	require.Equal(t, []byte("prompt"), retired.Payload)
	config := Config{Path: filepath.Join(filepath.Dir(j.config.Path), "delivery.bbolt")}
	require.NoError(t, j.Close())
	reopened, err := Open(config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	stored, err := reopened.GetSubmission(ctx, event.SubmissionID)
	require.NoError(t, err)
	require.True(t, stored.Retired)
	require.Equal(t, SubmissionInterruptedUnknown, stored.State)
	unresolved, err := reopened.HasUnresolvedSubmissions(ctx)
	require.NoError(t, err)
	require.False(t, unresolved)
}

func TestNativeResumeRetiresUnprovenTerminalLabelsWithoutChangingThem(t *testing.T) {
	for _, state := range []SubmissionState{SubmissionFailed, SubmissionCompleted, SubmissionCancelled} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			j, event := terminalSubmissionFixture(t, state, false)
			before, err := j.GetSubmission(ctx, event.SubmissionID)
			require.NoError(t, err)
			retired, err := j.RetireSubmission(ctx, before.ID, before.HarnessGeneration)
			require.NoError(t, err)
			require.True(t, retired.Retired)
			require.Equal(t, before.State, retired.State)
			require.Equal(t, before.Payload, retired.Payload)
			require.Equal(t, before.Hash, retired.Hash)
			require.False(t, retired.TerminalEventRetained)
			require.Zero(t, retired.TerminalSequence)
			require.NoError(t, j.Close())
			reopened, err := Open(j.config)
			require.NoError(t, err)
			t.Cleanup(func() { _ = reopened.Close() })
			stored, err := reopened.GetSubmission(ctx, before.ID)
			require.NoError(t, err)
			require.True(t, stored.Retired)
			require.Equal(t, state, stored.State)
			unresolved, err := reopened.HasUnresolvedSubmissions(ctx)
			require.NoError(t, err)
			require.False(t, unresolved)
		})
	}
}

func TestNativeResumeRetiresUnknownWithRetainedTerminal(t *testing.T) {
	j, event := terminalSubmissionFixture(t, SubmissionInterruptedUnknown, false)
	_, appendErr := j.Append(context.Background(), event)
	require.NoError(t, appendErr)
	before, err := j.GetSubmission(context.Background(), event.SubmissionID)
	require.NoError(t, err)
	require.True(t, before.TerminalEventRetained)
	after, err := j.RetireSubmission(context.Background(), event.SubmissionID, event.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, after.Retired)
	require.Equal(t, before.State, after.State)
	require.Equal(t, before.TerminalSequence, after.TerminalSequence)
}
