package lifecycle

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeProjectionWaitContinuousProgressExhaustsOriginalBudget(t *testing.T) {
	// Initial cursor lag returns before the closing status read.
	for _, scenario := range []struct {
		name           string
		initialLag     bool
		maxStatusReads int32
	}{
		{name: "projection lag", initialLag: true, maxStatusReads: 7},
		{name: "stream progress", initialLag: false, maxStatusReads: 8},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			manager, execution, repository, store := newProjectionWaitFixture(t)
			if !scenario.initialLag {
				repository.cursor.ReceivedSequence = 2
				repository.cursor.ProjectedSequence = 2
			}
			var statusReads atomic.Int32
			execution.agentctl = nativeResumeJournalPeer(t, store, false, func(ctx context.Context) error {
				if statusReads.Add(1) < 3 {
					return nil
				}
				event, err := store.Append(ctx, journal.Event{
					SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
					StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
				})
				if err != nil {
					return err
				}
				return store.Acknowledge(ctx, "stream", event.Sequence)
			})
			baseRead := repository.onRead
			ctx, cancel := context.WithTimeout(t.Context(), 750*time.Millisecond)
			defer cancel()
			deadline, _ := ctx.Deadline()
			repository.onRead = func(ctx context.Context, read int) error {
				if read < 3 {
					return baseRead(ctx, read)
				}
				currentDeadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.Equal(t, deadline, currentDeadline, "progress must not extend the original caller deadline")
				stream, err := store.GetStream(ctx, "stream")
				if err != nil {
					return err
				}
				repository.cursor.ReceivedSequence = int64(stream.HighWater)
				repository.cursor.ProjectedSequence = int64(stream.HighWater)
				return nil
			}
			proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, execution.SessionID)
			require.Nil(t, proof)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorIs(t, err, ErrDurableAdoptionBlocked)
			require.GreaterOrEqual(t, statusReads.Load(), int32(3), "exercise a changed closing status")
			require.LessOrEqual(t, statusReads.Load(), scenario.maxStatusReads, "every retry must wait for the 200 ms cadence")
		})
	}
}
