package websocket

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// newTunnelClient returns a keep-alive-free client so tunnel connections do not
// outlive the test and trip this package's goroutine-leak check.
func newTunnelClient(t *testing.T) *http.Client {
	t.Helper()
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: wsTestTimeout}
}

// getThroughTunnel performs a GET against a tunnel port and returns status and body.
func getThroughTunnel(t *testing.T, client *http.Client, tunnelPort int, path string) (int, string) {
	t.Helper()
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", tunnelPort, path))
	if err != nil {
		t.Fatalf("GET through tunnel port %d: %v", tunnelPort, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read tunnel response: %v", err)
	}
	return resp.StatusCode, string(body)
}

func newTunnelManagerForTest(t *testing.T, log *logger.Logger) *TunnelManager {
	t.Helper()
	manager := NewTunnelManager(newProxyTestManager(t, log), log)
	t.Cleanup(manager.Shutdown)
	return manager
}

func tunnelList(t *testing.T, manager *TunnelManager, sessionID string) []TunnelInfo {
	t.Helper()
	listed, ok := manager.ListTunnels(sessionID).([]TunnelInfo)
	if !ok {
		t.Fatalf("ListTunnels() returned %T, want []TunnelInfo", manager.ListTunnels(sessionID))
	}
	return listed
}

// A tunnel serves the app at "/", so the port must be re-attached on the way
// out and the original Host preserved for the app's CORS/Origin checks.
func TestStartTunnelForwardsRootPathsToAgentctlPortProxy(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("app-response"))
	})

	lifecycleMgr := newProxyTestManager(t, log)
	addExecutionForURL(t, lifecycleMgr, "sess-tunnel", upstream.server.URL, "tunnel-token", log)
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(manager.Shutdown)

	tunnelPort, err := manager.StartTunnel("sess-tunnel", 5173, 0)
	if err != nil {
		t.Fatalf("StartTunnel: %v", err)
	}
	if tunnelPort <= 0 {
		t.Fatalf("StartTunnel() port = %d, want an OS-assigned port", tunnelPort)
	}

	status, body := getThroughTunnel(t, newTunnelClient(t), tunnelPort, "/assets/index.css")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if body != "app-response" {
		t.Fatalf("body = %q, want %q", body, "app-response")
	}

	seen := upstream.seen()
	if len(seen) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(seen))
	}
	if seen[0].path != "/api/v1/port-proxy/5173/assets/index.css" {
		t.Fatalf("upstream path = %q, want %q", seen[0].path, "/api/v1/port-proxy/5173/assets/index.css")
	}
	if seen[0].auth != "Bearer tunnel-token" {
		t.Fatalf("upstream Authorization = %q, want %q", seen[0].auth, "Bearer tunnel-token")
	}
	wantHost := fmt.Sprintf("127.0.0.1:%d", tunnelPort)
	if seen[0].host != wantHost {
		t.Fatalf("upstream Host = %q, want the tunnel host %q", seen[0].host, wantHost)
	}
}

func TestStartTunnelBindsRequestedPortAndIsIdempotent(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	lifecycleMgr := newProxyTestManager(t, log)
	addExecutionForURL(t, lifecycleMgr, "sess-fixed", upstream.server.URL, "", log)
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(manager.Shutdown)

	// Reserving a port means binding it, reading it, and releasing it, so another
	// process can always win the gap before StartTunnel re-binds. Retry rather
	// than let a loaded CI machine turn that into a flake; losing every attempt
	// is not plausible. Loopback-only, to match the tunnel listener and to keep
	// the window as narrow as possible.
	var wanted, got int
	var err error
	for attempt := 1; attempt <= tunnelPortBindAttempts; attempt++ {
		wanted = reserveLoopbackPort(t)
		got, err = manager.StartTunnel("sess-fixed", 3000, wanted)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "already in use") {
			t.Fatalf("StartTunnel: %v", err)
		}
		t.Logf("attempt %d: port %d was taken between reserve and bind, retrying", attempt, wanted)
	}
	if err != nil {
		t.Fatalf("StartTunnel never won the port race in %d attempts: %v", tunnelPortBindAttempts, err)
	}
	if got != wanted {
		t.Fatalf("StartTunnel() port = %d, want the requested %d", got, wanted)
	}

	again, err := manager.StartTunnel("sess-fixed", 3000, 0)
	if err != nil {
		t.Fatalf("second StartTunnel: %v", err)
	}
	if again != wanted {
		t.Fatalf("second StartTunnel() port = %d, want the existing %d", again, wanted)
	}
	if listed := tunnelList(t, manager, "sess-fixed"); len(listed) != 1 {
		t.Fatalf("tunnels = %d, want 1 (a duplicate start must not bind a second port)", len(listed))
	}
}

// StartTunnel reserves its cache key before resolving; a failed resolve must
// release it, otherwise the session/port pair is permanently unusable.
func TestStartTunnelReleasesReservationWhenResolutionFails(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	lifecycleMgr := newProxyTestManager(t, log)
	clientless := &lifecycle.AgentExecution{ID: "exec-noclient", SessionID: "sess-noclient"}
	if err := lifecycleMgr.ExecutionStoreForTesting().Add(clientless); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(manager.Shutdown)

	tests := []struct {
		name      string
		sessionID string
		wantError string
	}{
		{name: "no execution", sessionID: "sess-missing", wantError: "session not found or no active execution"},
		{name: "no agentctl client", sessionID: "sess-noclient", wantError: "agentctl client not available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, err := manager.StartTunnel(tt.sessionID, 3000, 0)
			if err == nil {
				t.Fatal("StartTunnel() error = nil, want a failure")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantError)
			}
			if port != 0 {
				t.Fatalf("StartTunnel() port = %d, want 0 on failure", port)
			}
			manager.mu.Lock()
			_, stillRunning := manager.tunnels[tt.sessionID+":3000"]
			_, stillReserved := manager.pending[tt.sessionID+":3000"]
			manager.mu.Unlock()
			if stillRunning {
				t.Fatal("a failed start left an entry in the tunnel map")
			}
			if stillReserved {
				t.Fatal("the reserved cache key was not released after the failure")
			}
		})
	}
}

func TestStopTunnelClosesTheListenerAndForgetsTheEntry(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	lifecycleMgr := newProxyTestManager(t, log)
	addExecutionForURL(t, lifecycleMgr, "sess-stop", upstream.server.URL, "", log)
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(manager.Shutdown)

	tunnelPort, err := manager.StartTunnel("sess-stop", 3000, 0)
	if err != nil {
		t.Fatalf("StartTunnel: %v", err)
	}
	if status, _ := getThroughTunnel(t, newTunnelClient(t), tunnelPort, "/"); status != http.StatusOK {
		t.Fatalf("status before stop = %d, want %d", status, http.StatusOK)
	}

	if err := manager.StopTunnel("sess-stop", 3000); err != nil {
		t.Fatalf("StopTunnel: %v", err)
	}
	if listed := tunnelList(t, manager, "sess-stop"); len(listed) != 0 {
		t.Fatalf("tunnels after stop = %v, want none", listed)
	}
	assertPortClosed(t, tunnelPort)

	err = manager.StopTunnel("sess-stop", 3000)
	if err == nil {
		t.Fatal("second StopTunnel() error = nil, want a not-found error")
	}
	if !strings.Contains(err.Error(), "no tunnel found for session sess-stop port 3000") {
		t.Fatalf("error = %v", err)
	}
}

func TestListTunnelsIsScopedToTheSession(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	manager := newTunnelManagerForTest(t, log)

	manager.mu.Lock()
	manager.tunnels["sess-a:3000"] = &tunnelEntry{port: 3000, tunnelPort: 40001}
	manager.tunnels["sess-a:4000"] = &tunnelEntry{port: 4000, tunnelPort: 40002}
	manager.tunnels["sess-b:3000"] = &tunnelEntry{port: 3000, tunnelPort: 40003}
	manager.mu.Unlock()
	t.Cleanup(func() {
		manager.mu.Lock()
		manager.tunnels = make(map[string]*tunnelEntry)
		manager.mu.Unlock()
	})

	listed := tunnelList(t, manager, "sess-a")
	if len(listed) != 2 {
		t.Fatalf("tunnels for sess-a = %d, want 2", len(listed))
	}
	for _, info := range listed {
		if info.TunnelPort == 40003 {
			t.Fatalf("sess-b tunnel leaked into sess-a's list: %+v", listed)
		}
	}
	if other := tunnelList(t, manager, "sess-unknown"); len(other) != 0 {
		t.Fatalf("tunnels for an unknown session = %v, want none", other)
	}
}

func TestInvalidateSessionStopsOnlyThatSessionsTunnels(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	lifecycleMgr := newProxyTestManager(t, log)
	addExecutionForURL(t, lifecycleMgr, "sess-gone", upstream.server.URL, "", log)
	addExecutionForURL(t, lifecycleMgr, "sess-kept", upstream.server.URL, "", log)
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(manager.Shutdown)

	gonePort, err := manager.StartTunnel("sess-gone", 3000, 0)
	if err != nil {
		t.Fatalf("StartTunnel(sess-gone): %v", err)
	}
	keptPort, err := manager.StartTunnel("sess-kept", 3000, 0)
	if err != nil {
		t.Fatalf("StartTunnel(sess-kept): %v", err)
	}

	manager.InvalidateSession("sess-gone")

	if listed := tunnelList(t, manager, "sess-gone"); len(listed) != 0 {
		t.Fatalf("invalidated session still lists %v", listed)
	}
	assertPortClosed(t, gonePort)

	if listed := tunnelList(t, manager, "sess-kept"); len(listed) != 1 {
		t.Fatalf("surviving session lists %v, want 1 tunnel", listed)
	}
	if status, _ := getThroughTunnel(t, newTunnelClient(t), keptPort, "/"); status != http.StatusOK {
		t.Fatalf("surviving tunnel status = %d, want %d", status, http.StatusOK)
	}
}

func TestShutdownStopsEveryTunnel(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	lifecycleMgr := newProxyTestManager(t, log)
	addExecutionForURL(t, lifecycleMgr, "sess-1", upstream.server.URL, "", log)
	addExecutionForURL(t, lifecycleMgr, "sess-2", upstream.server.URL, "", log)
	manager := NewTunnelManager(lifecycleMgr, log)
	// Registered before the StartTunnel calls below: an early t.Fatalf there
	// would otherwise strand the listeners this test already opened.
	t.Cleanup(manager.Shutdown)

	first, err := manager.StartTunnel("sess-1", 3000, 0)
	if err != nil {
		t.Fatalf("StartTunnel(sess-1): %v", err)
	}
	second, err := manager.StartTunnel("sess-2", 4000, 0)
	if err != nil {
		t.Fatalf("StartTunnel(sess-2): %v", err)
	}

	manager.Shutdown()

	manager.mu.Lock()
	remaining := len(manager.tunnels)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("tunnels after Shutdown = %d, want 0", remaining)
	}
	assertPortClosed(t, first)
	assertPortClosed(t, second)

	// Shutdown must be idempotent: the registered cleanup calls it a second time.
	manager.Shutdown()
}

func TestTunnelProxyReportsBadGatewayWhenAgentctlIsDown(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	lifecycleMgr := newProxyTestManager(t, log)
	addExecutionForURL(t, lifecycleMgr, "sess-dead", deadURL, "", log)
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(manager.Shutdown)

	tunnelPort, err := manager.StartTunnel("sess-dead", 3000, 0)
	if err != nil {
		t.Fatalf("StartTunnel: %v", err)
	}

	status, body := getThroughTunnel(t, newTunnelClient(t), tunnelPort, "/")
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", status, http.StatusBadGateway)
	}
	if !strings.Contains(body, "tunnel proxy error") {
		t.Fatalf("body = %q, want the tunnel proxy error", body)
	}
}

// tunnelPortBindAttempts bounds the reserve/re-bind retry below.
const tunnelPortBindAttempts = 5

// reserveLoopbackPort returns a loopback port that was free a moment ago.
func reserveLoopbackPort(t *testing.T) int {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a free port: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatalf("release probe listener: %v", err)
	}
	return port
}

// assertPortClosed fails unless nothing is listening on the given local port.
func assertPortClosed(t *testing.T, port int) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("port %d still accepts connections, want it closed", port)
	}
}
