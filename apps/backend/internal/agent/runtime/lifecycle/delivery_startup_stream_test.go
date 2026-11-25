package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const (
	startupStreamSessionID   = "session"
	startupStreamNativeID    = "native-session-1"
	startupStreamExecutionID = "execution"
	startupStreamIncarnation = "incarnation"
	startupStreamID          = startupStreamIncarnation + ":g1"
)

type startupStreamSQLRepository struct {
	*tasksqlite.Repository
	authorityRead chan struct{}
	authorityOnce sync.Once
}

func (r *startupStreamSQLRepository) GetCurrentHarnessSessionGeneration(
	ctx context.Context, sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	r.authorityOnce.Do(func() { close(r.authorityRead) })
	return r.Repository.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
}

// startupCommitBus models the orchestrator's asynchronous generation commit:
// publication records the native identity, and the dedicated commit runs only
// after the fixture releases its channel.
type startupCommitBus struct {
	MockEventBusWithTracking
	repository *tasksqlite.Repository
	generation *models.HarnessSessionGeneration
	native     chan ACPSessionCreatedPayload
	commitGate chan struct{}
	commitOnce sync.Once
	commitDone chan error
}

func (b *startupCommitBus) releaseCommit() {
	b.commitOnce.Do(func() { close(b.commitGate) })
}

func (b *startupCommitBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if err := b.MockEventBusWithTracking.Publish(ctx, subject, event); err != nil {
		return err
	}
	if subject != events.AgentACPSessionCreated {
		return nil
	}
	payload, ok := event.Data.(ACPSessionCreatedPayload)
	if !ok {
		return fmt.Errorf("unexpected ACP session payload %T", event.Data)
	}
	b.native <- payload
	go func() {
		<-b.commitGate
		_, err := b.repository.CommitHarnessSessionGeneration(context.Background(), b.generation, 0)
		b.commitDone <- err
	}()
	return nil
}

type startupStreamServer struct {
	server          *httptest.Server
	client          *agentctl.Client
	earlyEvent      agentctl.AgentEvent
	sessionNewSeen  chan struct{}
	sessionNewOnce  sync.Once
	releaseNative   chan struct{}
	releaseOnce     sync.Once
	ack             chan struct{}
	ackOnce         sync.Once
	deliveryEnabled atomic.Bool
	mu              sync.Mutex
	actions         []string
}

func newStartupStreamServer(t *testing.T) *startupStreamServer {
	t.Helper()
	fixture := &startupStreamServer{
		earlyEvent: agentctl.AgentEvent{
			Type:                      "session_config_options",
			SessionID:                 startupStreamSessionID,
			DeliveryStreamID:          startupStreamID,
			DeliveryIncarnationID:     startupStreamIncarnation,
			DeliveryHarnessGeneration: 1,
			DeliverySequence:          1,
		},
		sessionNewSeen: make(chan struct{}),
		releaseNative:  make(chan struct{}),
		ack:            make(chan struct{}, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agent/delivery", func(w http.ResponseWriter, r *http.Request) {
		if !fixture.deliveryEnabled.Load() {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
			StorageCapability: journal.StorageCapability{Version: journal.CurrentProtocolVersion, Durable: true},
			SessionID:         startupStreamSessionID,
			IncarnationID:     startupStreamIncarnation,
			HarnessGeneration: 1,
			StreamID:          startupStreamID,
			Stream: &journal.Stream{
				SessionID: startupStreamSessionID, IncarnationID: startupStreamIncarnation,
				HarnessGeneration: 1, StreamID: startupStreamID, HighWater: 1, FirstRetained: 1,
			},
		})
	})
	mux.HandleFunc("/api/v1/agent/delivery/stream/ack", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		fixture.ackOnce.Do(func() { fixture.ack <- struct{}{} })
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v1/agent/stream", fixture.serveAgentStream)
	mux.HandleFunc("/api/v1/workspace/stream", func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	fixture.server = httptest.NewServer(mux)
	address, err := url.Parse(fixture.server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(address.Port())
	require.NoError(t, err)
	fixture.client = agentctl.NewClient(address.Hostname(), port, newTestLogger())
	t.Cleanup(func() {
		fixture.release()
		fixture.client.Close()
		fixture.server.Close()
	})
	return fixture
}

func (s *startupStreamServer) release() {
	s.releaseOnce.Do(func() { close(s.releaseNative) })
}

func (s *startupStreamServer) serveAgentStream(w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var message ws.Message
		if json.Unmarshal(data, &message) != nil || message.Type != ws.MessageTypeRequest {
			continue
		}
		s.recordAction(message.Action)
		response := s.handleRequest(conn, message)
		if response == nil {
			continue
		}
		responseData, _ := json.Marshal(response)
		if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
			return
		}
	}
}

func (s *startupStreamServer) handleRequest(conn *websocket.Conn, message ws.Message) *ws.Message {
	switch message.Action {
	case "agent.initialize":
		response, _ := ws.NewResponse(message.ID, message.Action, map[string]any{
			"success": true,
			"agent_info": map[string]string{
				"name": "test-agent", "version": "1.0.0",
			},
			"durable_delivery": map[string]any{
				"version": journal.CurrentVersion, "durable": true,
			},
		})
		return response
	case "agent.session.new":
		// Deliver the retained setup event before the native identity exists.
		// The retained descriptor becomes readable only after this request so
		// the startup preflight keeps using the absent-cursor bootstrap path.
		s.deliveryEnabled.Store(true)
		eventData, _ := json.Marshal(s.earlyEvent)
		if err := conn.WriteMessage(websocket.TextMessage, eventData); err != nil {
			return nil
		}
		s.sessionNewOnce.Do(func() { close(s.sessionNewSeen) })
		<-s.releaseNative
		response, _ := ws.NewResponse(message.ID, message.Action, map[string]any{
			"success": true, "session_id": startupStreamNativeID,
		})
		return response
	case "agent.session.set_mode":
		response, _ := ws.NewResponse(message.ID, message.Action, map[string]any{
			"requested": "", "effective": "", "confirmed": false,
		})
		return response
	default:
		response, _ := ws.NewResponse(message.ID, message.Action, map[string]any{"success": true})
		return response
	}
}

func (s *startupStreamServer) recordAction(action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, action)
}

func (s *startupStreamServer) actionLog() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.actions...)
}

func requireStartupDeliveryRows(t *testing.T, database *sqlx.DB, inbox, cursors, effects int) {
	t.Helper()
	for query, want := range map[string]int{
		"SELECT COUNT(*) FROM agent_delivery_inbox":   inbox,
		"SELECT COUNT(*) FROM agent_delivery_cursors": cursors,
		"SELECT COUNT(*) FROM agent_delivery_effects": effects,
	} {
		var got int
		require.NoError(t, database.Get(&got, query))
		require.Equal(t, want, got, query)
	}
}

func TestDeliveryStartupStreamOrdering(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	repository, database := runOwnerTestRepository(t)
	require.NoError(t, repository.CreateTask(ctx, &models.Task{ID: "task", Title: "Startup ordering"}))
	require.NoError(t, repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: startupStreamSessionID, TaskID: "task", QueueIncarnationID: startupStreamIncarnation,
	}))
	require.NoError(t, repository.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: startupStreamSessionID, TaskID: "task", AgentExecutionID: startupStreamExecutionID,
		ResumeToken: startupStreamNativeID, Status: models.ExecutorRunningStatusRunning,
	}))
	delivery := &startupStreamSQLRepository{Repository: repository, authorityRead: make(chan struct{})}
	commitBus := &startupCommitBus{
		repository: repository,
		generation: &models.HarnessSessionGeneration{
			SessionID: startupStreamSessionID, IncarnationID: startupStreamIncarnation,
			Generation: 1, NativeSessionID: startupStreamNativeID, CreationReason: "native_started",
		},
		native:     make(chan ACPSessionCreatedPayload, 1),
		commitGate: make(chan struct{}),
		commitDone: make(chan error, 1),
	}
	t.Cleanup(commitBus.releaseCommit)
	server := newStartupStreamServer(t)
	callback := make(chan agentctl.AgentEvent, 4)
	disconnected := make(chan error, 1)
	streams := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEventWithGeneration: func(_ *AgentExecution, event agentctl.AgentEvent, _ uint64) {
			callback <- event
		},
		OnStreamDisconnect: func(_ *AgentExecution, err error, _ uint64) {
			select {
			case disconnected <- err:
			default:
			}
		},
	}, nil, nil)
	streams.setAgentDeliveryRepository(delivery)
	t.Cleanup(streams.Wait)
	sessions := NewSessionManager(newSessionTestLogger(), nil)
	sessions.SetDependencies(NewEventPublisher(commitBus, newSessionTestLogger()), streams, nil, nil)

	execution := &AgentExecution{
		ID:                        startupStreamExecutionID,
		TaskID:                    "task",
		SessionID:                 startupStreamSessionID,
		WorkspacePath:             t.TempDir(),
		DeliveryStreamID:          startupStreamID,
		DeliveryIncarnationID:     startupStreamIncarnation,
		DeliveryHarnessGeneration: 1,
		agentctl:                  server.client,
	}
	agentConfig := &testAgent{
		id:      "test-agent",
		enabled: true,
		runtimeConfig: &agents.RuntimeConfig{
			Cmd:           agents.NewCommand("test-agent"),
			Protocol:      agent.ProtocolACP,
			SessionConfig: agents.SessionConfig{},
		},
	}
	readyCalls := 0
	initialized := make(chan error, 1)
	go func() {
		initialized <- sessions.InitializeAndPromptWithLayers(
			ctx, execution, agentConfig, "", nil, nil,
			func(string) error { readyCalls++; return nil },
			"", "", nil, "", "", nil, StartModelPolicy{},
		)
	}()

	select {
	case <-server.sessionNewSeen:
	case <-ctx.Done():
		t.Fatal("native session request never reached the peer")
	}
	requireStartupDeliveryRows(t, database, 0, 0, 0)

	server.release()
	select {
	case payload := <-commitBus.native:
		require.Equal(t, startupStreamNativeID, payload.ACPSessionID)
	case <-ctx.Done():
		t.Fatal("native session identity was never published")
	}
	select {
	case err := <-initialized:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("initialization did not preserve the delayed native response")
	}
	require.Equal(t, startupStreamNativeID, execution.ACPSessionID)
	require.Equal(t, 1, readyCalls)
	require.NotContains(t, server.actionLog(), "agent.prompt")
	require.NotContains(t, server.actionLog(), "agent.session.load")

	select {
	case <-delivery.authorityRead:
	case <-ctx.Done():
		t.Fatal("setup event never reached the post-native authority wait")
	}
	requireStartupDeliveryRows(t, database, 0, 0, 0)
	select {
	case <-server.ack:
		t.Fatal("ACK arrived before the generation commit")
	default:
	}
	select {
	case event := <-callback:
		t.Fatalf("callback ran before the generation commit: %s", event.Type)
	default:
	}

	commitBus.releaseCommit()
	select {
	case err := <-commitBus.commitDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("generation commit did not run")
	}
	select {
	case <-server.ack:
	case err := <-disconnected:
		t.Fatalf("stream failed after the generation commit: %v", err)
	case <-ctx.Done():
		t.Fatal("setup event was not acknowledged after the generation commit")
	}
	select {
	case event := <-callback:
		require.Equal(t, "session_config_options", event.Type)
	case <-ctx.Done():
		t.Fatal("setup event callback did not run after the generation commit")
	}
	select {
	case event := <-callback:
		t.Fatalf("unexpected extra agent event %q", event.Type)
	default:
	}
	requireStartupDeliveryRows(t, database, 1, 1, 1)
	cursor, err := repository.GetAgentDeliveryCursor(ctx, startupStreamID)
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ReceivedSequence)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	for _, action := range server.actionLog() {
		require.NotEqual(t, "agent.prompt", action)
	}
}
