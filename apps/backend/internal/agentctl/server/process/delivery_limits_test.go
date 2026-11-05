package process

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.5
func TestDeliveryEventTooLargeDoesNotWaitForACK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	store, err := journal.Open(journal.Config{Path: path, MaxEventBytes: 1024})
	require.NoError(t, err)
	manager := &Manager{
		cfg: &config.InstanceConfig{
			DurableJournalPath: path, SessionID: "session",
			DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1,
		},
		logger:          newTestLogger(t),
		deliveryJournal: store,
	}
	t.Cleanup(func() { require.NoError(t, manager.closeDeliveryJournal()) })

	started := time.Now()
	_, err = manager.persistDeliveryEvent(adapter.AgentEvent{
		Type: streams.EventTypeMessageChunk, SessionID: "session", Text: strings.Repeat("x", 4096),
	})
	require.ErrorIs(t, err, journal.ErrEventTooLarge)
	require.Less(t, time.Since(started), 2*time.Second, "a permanent size error must not wait for capacity or ACK")

	stream, streamErr := store.GetStream(context.Background(), "stream")
	require.ErrorIs(t, streamErr, journal.ErrStreamNotFound)
	require.Zero(t, stream.HighWater, "a permanently oversized event must not allocate a sequence")
	require.NotEqual(t, "storage_error", manager.DeliveryHealth().State)
}
