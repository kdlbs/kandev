package lifecycle

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	sprites "github.com/superfly/sprites-go"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPendingSpritesStopStopsAgentAndPreservesSandbox(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "forced"}[force], func(t *testing.T) {
			fixture := newPendingSpritesStopFixture(t, false)
			require.NoError(t, fixture.manager.StopAgentWithReason(context.Background(), "execution-pending-sprites", "user stop", force))
			require.EqualValues(t, 1, fixture.stopCalls.Load(), "pending Stop must call the authenticated native agent")
			require.Zero(t, fixture.spriteDeletes.Load(), "a user Stop must retain the saved Sprite")
			require.NoError(t, fixture.manager.recoveryGuard.CheckLaunchAllowed("session-pending-sprites"), "successful Stop releases the pending recovery guard")
			require.Nil(t, fixture.manager.pendingRemoteRecoveryForSession("session-pending-sprites"))
		})
	}
}

func TestPendingSpritesStopFailureRetainsGuardAndRetries(t *testing.T) {
	fixture := newPendingSpritesStopFixture(t, true)
	err := fixture.manager.StopAgentWithReason(context.Background(), "execution-pending-sprites", "user stop", false)
	require.Error(t, err, "a failed native Stop must leave remote cleanup retryable")
	require.EqualValues(t, 1, fixture.stopCalls.Load())
	require.Error(t, fixture.manager.recoveryGuard.CheckLaunchAllowed("session-pending-sprites"), "failed Stop must keep the pending launch guard")
	require.NotNil(t, fixture.manager.pendingRemoteRecoveryForSession("session-pending-sprites"))

	fixture.failStop.Store(false)
	require.NoError(t, fixture.manager.StopAgentWithReason(context.Background(), "execution-pending-sprites", "user stop", false))
	require.EqualValues(t, 2, fixture.stopCalls.Load(), "retry must issue native Stop on a fresh exact proxy")
	require.Zero(t, fixture.spriteDeletes.Load(), "plain Stop and retry must preserve the saved Sprite")
	require.NoError(t, fixture.manager.recoveryGuard.CheckLaunchAllowed("session-pending-sprites"), "successful retry releases the pending recovery guard")
	require.Nil(t, fixture.manager.pendingRemoteRecoveryForSession("session-pending-sprites"))
}

type pendingSpritesStopFixture struct {
	manager       *Manager
	backend       *SpritesExecutor
	stopCalls     atomic.Int32
	spriteDeletes atomic.Int32
	failStop      atomic.Bool
}

func newPendingSpritesStopFixture(t *testing.T, failStop bool) *pendingSpritesStopFixture {
	t.Helper()
	fixture := &pendingSpritesStopFixture{}
	fixture.failStop.Store(failStop)
	const spritePort = 49271

	agentListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	agentHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agent/session":
			_ = json.NewEncoder(w).Encode(agentctl.AgentSessionAssociation{
				InstanceID: "execution-pending-sprites", SessionID: "session-pending-sprites",
				IncarnationID: "incarnation-sprites", HarnessGeneration: 1,
				NativeSessionID: "native-session-sprites", AgentStatus: "running",
			})
		case "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         "session-pending-sprites", IncarnationID: "incarnation-sprites",
				HarnessGeneration: 1, StreamID: "stream-sprites",
			})
		case "/api/v1/instances/execution-pending-sprites":
			_ = json.NewEncoder(w).Encode(agentctl.InstanceInfo{
				ID: "execution-pending-sprites", Port: spritePort, Status: "running",
				WorkspacePath: "/workspace", SessionID: "session-pending-sprites", TaskID: "task-pending-sprites",
			})
		case "/api/v1/stop":
			fixture.stopCalls.Add(1)
			if fixture.failStop.Load() {
				http.Error(w, "stop unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			http.NotFound(w, r)
		}
	})
	agentServer := &http.Server{Handler: agentHandler}
	go func() { _ = agentServer.Serve(agentListener) }()
	t.Cleanup(func() {
		_ = agentServer.Close()
		_ = agentListener.Close()
	})

	const spriteName = "kandev-pending-stop"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			fixture.spriteDeletes.Add(1)
			http.Error(w, "unexpected Sprite deletion", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sprite-api-token" {
			http.Error(w, "missing Sprite API authentication", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/sprites/" + spriteName:
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "sprite-pending-stop", "name": spriteName, "status": "running"})
		case "/v1/sprites/" + spriteName + "/proxy":
			proxyAgentctlWebSocket(w, r, upgrader, agentListener.Addr().String(), spritesAgentctlPort, spritePort)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(apiServer.Close)

	backend := NewSpritesExecutor(
		&mockSecretStore{store: map[string]string{"sprite-api-secret": "sprite-api-token"}}, nil, nil,
		spritesAgentctlPort, newTestLogger(),
	)
	backend.setSpritesClientFactory(func(token string) *sprites.Client {
		if token != "sprite-api-token" {
			t.Errorf("Sprite API token = %q", token)
		}
		return sprites.New(token, sprites.WithBaseURL(apiServer.URL), sprites.WithDisableControl())
	})
	fixture.backend = backend
	registry := NewExecutorRegistry(newTestLogger())
	registry.Register(backend)
	manager := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", newTestLogger())
	cleanupManagerStopCh(t, manager)
	fixture.manager = manager
	fixture.manager.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{})
	row := &models.ExecutorRunning{
		ID: "session-pending-sprites", SessionID: "session-pending-sprites", TaskID: "task-pending-sprites",
		AgentExecutionID: "execution-pending-sprites", Runtime: agentruntime.RuntimeSprites,
		Metadata: map[string]interface{}{
			MetadataKeySpriteName:             spriteName,
			"env_secret_id_SPRITES_API_TOKEN": "sprite-api-secret",
		},
	}
	task08SetSnapshots(manager, []*models.ExecutorRunning{row})
	manager.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	t.Cleanup(func() { _ = backend.Close() })
	return fixture
}

func proxyAgentctlWebSocket(
	w http.ResponseWriter,
	r *http.Request,
	upgrader websocket.Upgrader,
	agentctlAddress string,
	wantPorts ...int,
) {
	websocketConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = websocketConn.Close() }()
	var init sprites.ProxyInitMessage
	if err := websocketConn.ReadJSON(&init); err != nil {
		return
	}
	portAccepted := false
	for _, wantPort := range wantPorts {
		if init.Port == wantPort {
			portAccepted = true
			break
		}
	}
	if !portAccepted {
		return
	}
	if err := websocketConn.WriteJSON(sprites.ProxyResponseMessage{Status: "connected", Target: "localhost"}); err != nil {
		return
	}
	backendConn, err := net.Dial("tcp", agentctlAddress)
	if err != nil {
		return
	}
	defer func() { _ = backendConn.Close() }()
	go func() {
		defer func() { _ = backendConn.Close() }()
		for {
			messageType, data, readErr := websocketConn.ReadMessage()
			if readErr != nil || messageType != websocket.BinaryMessage {
				return
			}
			if _, writeErr := backendConn.Write(data); writeErr != nil {
				return
			}
		}
	}()
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := backendConn.Read(buffer)
		if count > 0 {
			if writeErr := websocketConn.WriteMessage(websocket.BinaryMessage, buffer[:count]); writeErr != nil {
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return
			}
			return
		}
	}
}
