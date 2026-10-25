package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type resumedDeliveryCommitBus struct {
	MockEventBusWithTracking
	repo *retentionBootstrapRepository
}

func (b *resumedDeliveryCommitBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if subject == events.AgentACPSessionCreated {
		b.repo.generation.Store(retentionBootstrapGeneration())
	}
	return b.MockEventBusWithTracking.Publish(ctx, subject, event)
}

func TestResumedDeliveryBootstrapWaitsForFirstGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	execution, event := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	execution.ACPSessionID = "bootstrap-native"
	execution.agentctl = client
	processed := make(chan error, 1)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(manager.Wait)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.initialize" {
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success": true, "durable_delivery": map[string]any{"version": journal.CurrentVersion, "durable": true},
			})
			return response
		}
		if msg.Action == "agent.session.load" {
			go func() {
				_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
				processed <- err
			}()
			select {
			case <-repo.firstRead:
			case <-ctx.Done():
				return nil
			}
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true, "session_id": "bootstrap-native"})
			return response
		}
		return mock.defaultHandler(msg)
	}
	disconnected := connectAgentStream(t, mock, client)
	t.Cleanup(func() { client.Close(); <-disconnected })
	sm := NewSessionManager(newTestLogger(), nil)
	sm.SetDependencies(NewEventPublisher(&resumedDeliveryCommitBus{repo: repo}, newTestLogger()), nil, nil, nil)
	config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{
		Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP,
		SessionConfig: agents.SessionConfig{NativeSessionResume: true},
	}}
	require.NoError(t, sm.InitializeAndPrompt(ctx, execution, config, "", nil, nil, func(string) error { return nil }, StartModelPolicy{}, "", nil))
	select {
	case err := <-processed:
		require.NoError(t, err, "setup events must survive the native resume before SQL authority commits")
	case <-ctx.Done():
		t.Fatal("resumed setup event never processed")
	}
	require.EqualValues(t, 1, repo.receipts.Load())
}

func TestStreamProcessorFailureClosesQuietTransport(t *testing.T) {
	execution, event := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	foreign := retentionBootstrapGeneration()
	foreign.Generation = 2
	repo.generation.Store(foreign)
	repo.cursor = &models.AgentDeliveryCursor{SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: 1, StreamID: execution.DeliveryStreamID}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/delivery" {
			_ = json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
				HarnessGeneration: 1, StreamID: execution.DeliveryStreamID,
			})
			return
		}
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if err := conn.WriteJSON(event); err != nil {
			return
		}
		_, _, _ = conn.ReadMessage()
	})
	execution.agentctl = client
	disconnected := make(chan error, 1)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{OnStreamDisconnect: func(_ *AgentExecution, err error, _ uint64) {
		disconnected <- err
	}}, nil, nil)
	t.Cleanup(manager.Wait)
	t.Cleanup(client.Close)
	manager.delivery = repo
	manager.connectUpdatesStream(execution, nil)
	select {
	case err := <-disconnected:
		require.ErrorIs(t, err, ErrDurableAdoptionIdentityMismatch)
	case <-time.After(3 * time.Second):
		t.Fatal("failed event processor left the quiet transport connected")
	}
	require.False(t, client.HasAgentStream())
	require.Zero(t, repo.receipts.Load())
}
