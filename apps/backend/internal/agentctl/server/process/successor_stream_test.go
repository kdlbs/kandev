package process

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	agentstreams "github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestSuccessorDeliveryStreamAbandonmentRequiresNewerIdleOwner(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.Append(ctx, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "incarnation:g2", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	expected, err := store.GetStream(ctx, "incarnation:g2")
	require.NoError(t, err)
	manager := &Manager{
		cfg: &config.InstanceConfig{
			DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
			SessionID:          "session", DeliveryIncarnationID: "incarnation",
			DeliveryHarnessGeneration: 1, DeliveryStreamID: "incarnation:g1",
		},
		deliveryJournal: store,
		logger:          newTestLogger(t),
	}

	streams, err := manager.SuccessorDeliveryStreams(ctx)
	require.NoError(t, err)
	require.Equal(t, []journal.Stream{expected}, streams)
	require.NoError(t, manager.AbandonSuccessorDeliveryStream(ctx, expected))
	_, err = store.GetStream(ctx, expected.StreamID)
	require.ErrorIs(t, err, journal.ErrStreamNotFound)
	committed, err := manager.persistDeliveryBatchRaw(ctx, []adapter.AgentEvent{{
		Type: agentstreams.EventTypeMessageChunk, SessionID: "session", Text: "current owner",
	}})
	require.NoError(t, err)
	require.Equal(t, "incarnation:g1", committed[0].DeliveryStreamID)
	require.EqualValues(t, 1, committed[0].DeliveryHarnessGeneration)
	_, err = store.GetStream(ctx, expected.StreamID)
	require.ErrorIs(t, err, journal.ErrStreamNotFound, "the configured generation owner cannot append to the orphan")

	_, err = store.Append(ctx, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "incarnation:g2-live", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	live, err := store.GetStream(ctx, "incarnation:g2-live")
	require.NoError(t, err)
	manager.deliverySubmissionMu.Lock()
	require.ErrorIs(t, manager.AbandonSuccessorDeliveryStream(ctx, live), journal.ErrSubmissionState)
	manager.deliverySubmissionMu.Unlock()

	manager.cfg.DeliveryHarnessGeneration = 2
	require.ErrorIs(t, manager.AbandonSuccessorDeliveryStream(ctx, live), journal.ErrOwnerMismatch,
		"an owner cannot abandon its configured generation through the successor operation")
}

func TestFailedBootstrapStreamAbandonmentRequiresStoppedOwner(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.Append(ctx, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "incarnation:g2", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	manager := &Manager{
		cfg: &config.InstanceConfig{
			DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
			SessionID:          "session", DeliveryIncarnationID: "incarnation",
			DeliveryHarnessGeneration: 2, DeliveryStreamID: "incarnation:g2",
		},
		deliveryJournal: store,
		logger:          newTestLogger(t),
	}
	manager.status.Store(StatusRunning)
	require.ErrorIs(t, manager.AbandonFailedBootstrapDeliveryStream(ctx, "incarnation:g2", 2), journal.ErrSubmissionState)
	stream, err := store.GetStream(ctx, "incarnation:g2")
	require.NoError(t, err)
	require.False(t, stream.Sealed)

	manager.status.Store(StatusStopped)
	require.NoError(t, manager.AbandonFailedBootstrapDeliveryStream(ctx, "incarnation:g2", 2))
	_, err = store.GetStream(ctx, "incarnation:g2")
	require.ErrorIs(t, err, journal.ErrStreamNotFound)
}
