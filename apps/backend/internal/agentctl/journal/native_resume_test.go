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
	for _, state := range []SubmissionState{SubmissionPrepared, SubmissionAccepted, SubmissionDispatching, SubmissionCompleted} {
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
