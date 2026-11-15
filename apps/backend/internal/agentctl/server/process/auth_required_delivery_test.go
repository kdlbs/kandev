package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestConnectionScopedStartupEventsAreNotJournaled(t *testing.T) {
	tests := []struct {
		name  string
		event adapter.AgentEvent
	}{
		{
			name: "authentication required",
			event: adapter.AgentEvent{
				Type:        streams.EventTypeAuthRequired,
				AuthMethods: []streams.AuthMethodInfo{{ID: "cursor_login", Name: "Cursor login"}},
			},
		},
		{
			name: "agent capabilities",
			event: adapter.AgentEvent{
				Type:          streams.EventTypeAgentCapabilities,
				SupportsImage: true,
				AuthMethods:   []streams.AuthMethodInfo{{ID: "cursor_login", Name: "Cursor login"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newAllToolManager(t, 1024)

			persisted, err := manager.persistDeliveryEvent(test.event)
			require.NoError(t, err)
			require.Empty(t, persisted.DeliveryStreamID)
			require.Zero(t, persisted.DeliverySequence)

			deliveryJournal, err := manager.DeliveryJournal()
			require.NoError(t, err)
			_, err = deliveryJournal.GetStream(context.Background(), "stream")
			require.ErrorIs(t, err, journal.ErrStreamNotFound)
		})
	}
}

func TestSessionBoundAgentCapabilitiesRemainJournaled(t *testing.T) {
	manager := newAllToolManager(t, 1024)

	persisted, err := manager.persistDeliveryEvent(adapter.AgentEvent{
		Type:      streams.EventTypeAgentCapabilities,
		SessionID: "native-session",
	})
	require.NoError(t, err)
	require.NotEmpty(t, persisted.DeliveryStreamID)
	require.NotZero(t, persisted.DeliverySequence)
}
