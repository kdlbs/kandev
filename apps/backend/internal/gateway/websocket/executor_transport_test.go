package websocket

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
)

type pluginExecutorProxyObservation struct {
	path          string
	authorization string
	instanceID    string
	providerHTTP  string
	providerWS    string
	protocols     string
}

type pluginExecutorProxyTransport struct {
	generation atomic.Value
}

func newPluginExecutorProxyTransport(generation string) *pluginExecutorProxyTransport {
	transport := &pluginExecutorProxyTransport{}
	transport.generation.Store(generation)
	return transport
}

func (t *pluginExecutorProxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	out := request.Clone(request.Context())
	out.Header = request.Header.Clone()
	out.Header.Set("Authorization", "Bearer host-secret")
	out.Header.Set("X-Instance-ID", "execution-remote")
	out.Header.Set("X-Provider-HTTP", t.generation.Load().(string))
	if strings.EqualFold(out.Header.Get("Upgrade"), "websocket") {
		out.Header.Set("X-Provider-WS", t.generation.Load().(string))
		protocols := out.Header.Get("Sec-WebSocket-Protocol")
		if protocols == "" {
			out.Header.Set("Sec-WebSocket-Protocol", "provider.route")
		} else {
			out.Header.Set("Sec-WebSocket-Protocol", protocols+", provider.route")
		}
	}
	return http.DefaultTransport.RoundTrip(out)
}

func TestPluginExecutorProxyHTTPAndUpgrade(t *testing.T) {
	observed := make(chan pluginExecutorProxyObservation, 8)
	upstream, target := newPluginExecutorProxyUpstream(t, observed)
	defer upstream.Close()
	transport := newPluginExecutorProxyTransport("provider-token")

	parsedTarget, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	vscode := &VscodeProxyHandler{logger: testLogger(), proxies: make(map[string]*proxyEntry)}
	editorProxy := vscode.createProxy("session-1", parsedTarget, transport)
	editorServer := httptest.NewServer(editorProxy)
	defer editorServer.Close()
	requests := []pluginExecutorProxyObservation{
		assertPluginExecutorProxyHTTP(t, editorServer.URL+"/api/v1/vscode/proxy/index.html", "/api/v1/vscode/proxy/index.html", observed),
		assertPluginExecutorProxyUpgrade(t, editorServer.URL+"/api/v1/vscode/proxy/socket", "/api/v1/vscode/proxy/socket", observed),
	}

	portProxy := &PortProxyHandler{logger: testLogger(), proxies: make(map[string]*portProxyEntry)}
	preview := portProxy.createProxy("session-1:3000", parsedTarget, transport, "/port-proxy/session-1/3000")
	previewServer := httptest.NewServer(preview)
	defer previewServer.Close()
	requests = append(requests,
		assertPluginExecutorProxyHTTP(t, previewServer.URL+"/api/v1/port-proxy/3000/index.html", "/api/v1/port-proxy/3000/index.html", observed),
		assertPluginExecutorProxyUpgrade(t, previewServer.URL+"/api/v1/port-proxy/3000/socket", "/api/v1/port-proxy/3000/socket", observed),
	)

	for _, request := range requests {
		if request.authorization != "Bearer host-secret" || request.instanceID != "execution-remote" {
			t.Errorf("host authentication on %s = %q / %q", request.path, request.authorization, request.instanceID)
		}
		if request.providerHTTP != "provider-token" {
			t.Errorf("provider HTTP lease on %s = %q", request.path, request.providerHTTP)
		}
		if strings.Contains(request.path, "socket") {
			if request.providerWS != "provider-token" || !strings.Contains(request.protocols, "provider.route") {
				t.Errorf("provider WebSocket lease on %s = %q / %q", request.path, request.providerWS, request.protocols)
			}
		}
	}
}

func TestPluginExecutorProxyGeneration(t *testing.T) {
	observed := make(chan pluginExecutorProxyObservation, 4)
	upstream, target := newPluginExecutorProxyUpstream(t, observed)
	defer upstream.Close()
	transport := newPluginExecutorProxyTransport("generation-one")
	parsedTarget, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	handler := &VscodeProxyHandler{logger: testLogger(), proxies: make(map[string]*proxyEntry)}
	cachedProxy := handler.createProxy("session-1", parsedTarget, transport)
	handler.proxies["session-1"] = &proxyEntry{proxy: cachedProxy, target: target}
	proxyServer := httptest.NewServer(cachedProxy)
	defer proxyServer.Close()

	first := assertPluginExecutorProxyHTTP(t, proxyServer.URL+"/api/v1/vscode/proxy/state", "/api/v1/vscode/proxy/state", observed)
	transport.generation.Store("generation-two")
	second := assertPluginExecutorProxyHTTP(t, proxyServer.URL+"/api/v1/vscode/proxy/state", "/api/v1/vscode/proxy/state", observed)

	if first.providerHTTP != "generation-one" || second.providerHTTP != "generation-two" {
		t.Fatalf("cached proxy generations = %q then %q, want generation-one then generation-two", first.providerHTTP, second.providerHTTP)
	}
	if handler.proxies["session-1"].proxy != cachedProxy {
		t.Fatal("proxy cache changed during lease generation refresh")
	}
}

func newPluginExecutorProxyUpstream(t *testing.T, observed chan<- pluginExecutorProxyObservation) (*httptest.Server, string) {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin:  func(*http.Request) bool { return true },
		Subprotocols: []string{"app.v1"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- pluginExecutorProxyObservation{
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			instanceID:    r.Header.Get("X-Instance-ID"),
			providerHTTP:  r.Header.Get("X-Provider-HTTP"),
			providerWS:    r.Header.Get("X-Provider-WS"),
			protocols:     r.Header.Get("Sec-WebSocket-Protocol"),
		}
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	return server, server.URL
}

func assertPluginExecutorProxyHTTP(t *testing.T, endpoint, wantPath string, observed <-chan pluginExecutorProxyObservation) pluginExecutorProxyObservation {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("proxy HTTP GET: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatalf("read proxy HTTP response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK || string(body) != wantPath {
		t.Fatalf("proxy response = %d %q, want 200 %q", response.StatusCode, body, wantPath)
	}
	request := <-observed
	if request.path != wantPath {
		t.Fatalf("upstream path = %q, want %q", request.path, wantPath)
	}
	return request
}

func assertPluginExecutorProxyUpgrade(t *testing.T, endpoint, wantPath string, observed <-chan pluginExecutorProxyObservation) pluginExecutorProxyObservation {
	t.Helper()
	dialer := *websocket.DefaultDialer
	dialer.Subprotocols = []string{"app.v1"}
	connection, response, err := dialer.Dial("ws"+strings.TrimPrefix(endpoint, "http"), nil)
	if err != nil {
		if response != nil {
			t.Fatalf("proxy WebSocket upgrade: %v (status %d)", err, response.StatusCode)
		}
		t.Fatalf("proxy WebSocket upgrade: %v", err)
	}
	defer func() { _ = connection.Close() }()
	if connection.Subprotocol() != "app.v1" {
		t.Fatalf("browser subprotocol = %q, want app.v1", connection.Subprotocol())
	}
	request := <-observed
	if request.path != wantPath {
		t.Fatalf("upstream WebSocket path = %q, want %q", request.path, wantPath)
	}
	return request
}
