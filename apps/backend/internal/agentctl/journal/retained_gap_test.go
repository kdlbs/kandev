//go:build linux

package journal

import (
	"context"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
func TestRetainedRecoveryVerifiesPrunedGapWithoutAcknowledgingOrCompleting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	j, err := Open(Config{Path: CheckStorage(root, "session").Path})
	require.NoError(t, err)
	payload := []byte(`{"text":"original prompt"}`)
	_, err = j.PutSubmission(ctx, Submission{ID: "prompt", SessionID: "session", IncarnationID: "session",
		HarnessGeneration: 1, StreamID: "stream", Hash: SubmissionHash(payload), Payload: payload, State: SubmissionDispatching})
	require.NoError(t, err)
	for range 6 {
		_, err = j.Append(ctx, Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1,
			StreamID: "stream", SubmissionID: "prompt", Type: "context_window", Payload: []byte(`{}`)})
		require.NoError(t, err)
	}
	require.NoError(t, j.Acknowledge(ctx, "stream", 6))
	require.NoError(t, j.Close())
	request := RetainedRecoveryRequest{Root: root, SessionID: "session", ExecutionID: "execution",
		IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "prompt",
		OriginalRuntime: processidentity.Identity{PID: 2147483647, GroupID: 2147483647, SessionID: 2147483647, BirthToken: "linux:prior-boot:1"},
		After:           1, Limit: 4, PrunedThrough: 6, ExpectedHash: SubmissionHash(payload)}
	result, err := ReadRetainedRecovery(ctx, request)
	require.NoError(t, err, "a proven dead owner must expose honest gap evidence without replaying unavailable events")
	require.True(t, result.HistoryGapVerified)
	require.True(t, result.ProcessTerminated)
	require.Empty(t, result.Events)
	require.Equal(t, SubmissionInterruptedUnknown, result.Submission.State)
	require.Zero(t, result.Submission.TerminalSequence)
	require.EqualValues(t, 6, result.Descriptor.Stream.Acknowledged)
	for _, condition := range []string{"unknown owner", "live owner", "hash", "range", "ack"} {
		t.Run(condition, func(t *testing.T) {
			bad := request
			switch condition {
			case "unknown owner":
				bad.OriginalRuntime = processidentity.Identity{}
			case "live owner":
				bad.OriginalRuntime, err = processidentity.Capture(os.Getpid())
				require.NoError(t, err)
			case "hash":
				bad.ExpectedHash = "conflicting"
			case "range":
				bad.PrunedThrough++
			case "ack":
				bad.Acknowledge = bad.After
			}
			_, err := ReadRetainedRecovery(ctx, bad)
			require.Error(t, err)
		})
	}
	j, err = Open(Config{Path: CheckStorage(root, "session").Path})
	require.NoError(t, err)
	defer func() { require.NoError(t, j.Close()) }()
	stored, err := j.GetSubmission(ctx, "prompt")
	require.NoError(t, err)
	require.Equal(t, payload, stored.Payload)
	require.False(t, stored.Retired, "retirement requires successful native restoration")
}
