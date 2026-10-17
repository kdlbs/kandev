package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newGatewayForTest(t *testing.T) *Gateway {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	gateway, err := Provide(log)
	if err != nil {
		t.Fatalf("Provide: %v", err)
	}
	return gateway
}

// registeredRoutes returns "METHOD PATH" strings for everything on the engine.
func registeredRoutes(engine *gin.Engine) []string {
	routes := make([]string, 0, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes = append(routes, route.Method+" "+route.Path)
	}
	sort.Strings(routes)
	return routes
}

func hasRoute(routes []string, want string) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}

// A gateway with no optional handlers wired must expose only /ws — the terminal,
// LSP and proxy surfaces are opt-in and must not appear by accident.
func TestSetupRoutesRegistersOnlyWiredSurfaces(t *testing.T) {
	gateway := newGatewayForTest(t)
	gin.SetMode(gin.TestMode)

	bare := gin.New()
	gateway.SetupRoutes(bare)
	bareRoutes := registeredRoutes(bare)
	if !hasRoute(bareRoutes, "GET /ws") {
		t.Fatalf("routes = %v, want GET /ws", bareRoutes)
	}
	for _, unwanted := range []string{
		"GET /terminal/*target",
		"GET /lsp/tasks/:taskId/:language/attach",
		"GET /vscode/:sessionId/*path",
		"GET /port-proxy/:sessionId/:port/*path",
	} {
		if hasRoute(bareRoutes, unwanted) {
			t.Fatalf("routes = %v, want no %q before the handler is wired", bareRoutes, unwanted)
		}
	}

	gateway.SetLifecycleManager(nil, nil, nil)
	gateway.SetLSPHandler(nil)
	gateway.SetVscodeProxy(nil)
	gateway.SetPortProxy(nil)
	gateway.SetPortTunnel(nil)

	if gateway.TerminalHandler == nil || gateway.LSPHandler == nil ||
		gateway.VscodeProxyHandler == nil || gateway.PortProxyHandler == nil ||
		gateway.TunnelManager == nil {
		t.Fatalf("gateway handlers were not all constructed: %+v", gateway)
	}

	wired := gin.New()
	gateway.SetupRoutes(wired)
	wiredRoutes := registeredRoutes(wired)
	for _, want := range []string{
		"GET /ws",
		"GET /terminal/*target",
		"GET /lsp/tasks/:taskId/:language/attach",
		"GET /vscode/:sessionId/*path",
		"GET /port-proxy/:sessionId/:port",
		"GET /port-proxy/:sessionId/:port/*path",
	} {
		if !hasRoute(wiredRoutes, want) {
			t.Fatalf("routes = %v, want %q", wiredRoutes, want)
		}
	}
}

func TestGatewayHealthActionIsDispatchable(t *testing.T) {
	gateway := newGatewayForTest(t)

	dispatcher := gateway.Hub.GetDispatcher()
	if dispatcher != gateway.Dispatcher {
		t.Fatal("Hub.GetDispatcher() did not return the gateway's dispatcher")
	}

	resp, err := dispatcher.Dispatch(
		context.Background(),
		&ws.Message{ID: "health-1", Type: ws.MessageTypeRequest, Action: ws.ActionHealthCheck},
	)
	if err != nil {
		t.Fatalf("Dispatch(health) error = %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("decode health payload: %v", err)
	}
	if payload["status"] != "ok" || payload["service"] != "kandev" {
		t.Fatalf("health payload = %v, want status=ok service=kandev", payload)
	}
}

func TestBroadcastToTaskReachesOnlySubscribers(t *testing.T) {
	h := newTestHub(t)
	subscribed := newTestClient("c-subscribed")
	subscribed.hub = h
	other := newTestClient("c-other")
	other.hub = h

	h.SubscribeToTask(subscribed, "task-1")
	h.SubscribeToTask(other, "task-2")

	msg, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}
	h.BroadcastToTask("task-1", msg)

	got := nextClientFrame(t, subscribed)
	if got.Action != "task.updated" {
		t.Fatalf("subscriber received action %q, want %q", got.Action, "task.updated")
	}
	assertNoMoreFrames(t, other)
}

func TestGetSessionModeReflectsSubscriptionAndFocus(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-mode")
	c.hub = h

	if got := h.GetSessionMode("sess-1"); got != SessionModePaused {
		t.Fatalf("mode with no interest = %q, want %q", got, SessionModePaused)
	}

	h.SubscribeToSession(c, "sess-1")
	if got := h.GetSessionMode("sess-1"); got != SessionModeSlow {
		t.Fatalf("mode when subscribed = %q, want %q", got, SessionModeSlow)
	}

	h.FocusSession(c, "sess-1")
	if got := h.GetSessionMode("sess-1"); got != SessionModeFast {
		t.Fatalf("mode when focused = %q, want %q", got, SessionModeFast)
	}
}

func TestRemoveTunnelDropsASingleCacheKey(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	manager := newTunnelManagerForTest(t, log)

	manager.mu.Lock()
	manager.tunnels["sess-a:3000"] = &tunnelEntry{port: 3000, tunnelPort: 40001}
	manager.tunnels["sess-a:4000"] = &tunnelEntry{port: 4000, tunnelPort: 40002}
	manager.mu.Unlock()
	t.Cleanup(func() {
		manager.mu.Lock()
		manager.tunnels = make(map[string]*tunnelEntry)
		manager.mu.Unlock()
	})

	manager.removeTunnel("sess-a:3000")

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, ok := manager.tunnels["sess-a:3000"]; ok {
		t.Fatal("removeTunnel did not drop its cache key")
	}
	if _, ok := manager.tunnels["sess-a:4000"]; !ok {
		t.Fatal("removeTunnel dropped an unrelated cache key")
	}
}

func TestShouldUseWorkspaceShellIsFalseWithoutAContainerExecution(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)

	if handler.shouldUseWorkspaceShell(context.Background(), "session-unknown") {
		t.Fatal("shouldUseWorkspaceShell() = true for a session with no execution")
	}
}

// The /ws route is guarded by requireConnectionAuth, which must not be skipped
// just because the request is a WebSocket upgrade.
func TestSetupRoutesGuardsWebSocketRouteWithConnectionAuth(t *testing.T) {
	gateway := newGatewayForTest(t)
	gateway.SetAuthPolicy(AuthPolicy{Enforced: func() bool { return true }})
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	gateway.SetupRoutes(engine)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
