package journal

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.5
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.6
func TestRetiredFullyPrunedSourceStreamCannotReceiveReplacementOutput(t *testing.T) {
	for _, generation := range []uint64{1, 2} {
		t.Run(map[uint64]string{1: "same generation", 2: "successor generation"}[generation], func(t *testing.T) {
			ctx := context.Background()
			config := Config{Path: CheckStorage(t.TempDir(), "session").Path}
			j, err := Open(config)
			require.NoError(t, err)
			_, err = j.PutSubmission(ctx, Submission{ID: "prompt", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", Hash: "hash", Payload: []byte("prompt"), State: SubmissionInterruptedUnknown})
			require.NoError(t, err)
			event := Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "prompt", Type: "context_window", Payload: []byte(`{}`)}
			_, err = j.Append(ctx, event)
			require.NoError(t, err)
			require.NoError(t, j.Acknowledge(ctx, "stream", 1))
			_, err = j.RetireSubmission(ctx, "prompt", generation)
			require.NoError(t, err)
			require.NoError(t, j.Close())
			j, err = Open(config)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, j.Close()) })
			descriptor, err := j.RecoveryDescriptor(ctx, "session", "session", 1, "stream")
			require.NoError(t, err)
			require.Equal(t, generation > 1, descriptor.Stream.Sealed)
			_, err = j.Append(ctx, event)
			if generation > 1 {
				require.ErrorIs(t, err, ErrOwnerMismatch)
				_, err = j.RetireSubmission(ctx, "prompt", generation)
				require.NoError(t, err, "retirement after restart must be idempotent")
			} else {
				require.NoError(t, err, "ordinary native resume must keep its generation writable")
			}
		})
	}
}
