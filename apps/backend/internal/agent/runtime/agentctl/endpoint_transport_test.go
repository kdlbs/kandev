package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPluginExecutorLeaseRefresh(t *testing.T) {
	now := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	var current atomic.Int64
	current.Store(now.UnixNano())
	var calls atomic.Int32
	var generations atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	resolver := func(ctx context.Context) (*ConnectionLease, error) {
		call := calls.Add(1)
		if call == 1 {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &ConnectionLease{
			BaseURL:     "https://executor.example",
			HTTPHeaders: http.Header{"X-Provider-Token": {"rotated"}},
			Generation:  strconv.Itoa(int(call)),
			ExpiresAt:   time.Unix(0, current.Load()).Add(2 * time.Minute),
		}, nil
	}
	manager := newConnectionLeaseManager(resolver, func() time.Time { return time.Unix(0, current.Load()) }, func() { generations.Add(1) })

	const callers = 24
	start := make(chan struct{})
	results := make(chan *ConnectionLease, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lease, err := manager.resolve(context.Background())
			if err != nil {
				errs <- err
				return
			}
			results <- lease
		}()
	}
	close(start)
	<-entered
	close(release)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent resolve: %v", err)
	}
	for lease := range results {
		if lease.Generation != "1" {
			t.Fatalf("concurrent generation = %q, want 1", lease.Generation)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls = %d, want one shared refresh", got)
	}

	// A lease refreshes before the 30-second safety window. A new generation
	// must become visible to subsequent HTTP and WebSocket requests.
	current.Store(now.Add(91 * time.Second).UnixNano())
	lease, err := manager.resolve(context.Background())
	if err != nil {
		t.Fatalf("refresh near expiry: %v", err)
	}
	if lease.Generation != "2" || calls.Load() != 2 {
		t.Fatalf("refreshed generation/calls = %q/%d, want 2/2", lease.Generation, calls.Load())
	}
	if got := generations.Load(); got != 2 {
		t.Fatalf("generation invalidations = %d, want one per lease generation", got)
	}

	// Cancellation of one waiter does not invalidate a still-valid cached lease.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.resolve(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resolve error = %v, want context.Canceled", err)
	}
}

func TestPluginExecutorEndpointRejection(t *testing.T) {
	now := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	valid := func() ConnectionLease {
		return ConnectionLease{
			BaseURL:     "https://executor.example",
			HTTPHeaders: http.Header{"X-Provider-Token": {"short-lived"}},
			ExpiresAt:   now.Add(time.Minute),
			Generation:  "g1",
		}
	}
	cases := []struct {
		name   string
		mutate func(*ConnectionLease)
	}{
		{name: "http endpoint", mutate: func(lease *ConnectionLease) { lease.BaseURL = "http://executor.example" }},
		{name: "userinfo", mutate: func(lease *ConnectionLease) { lease.BaseURL = "https://user:pass@executor.example" }},
		{name: "fragment", mutate: func(lease *ConnectionLease) { lease.BaseURL = "https://executor.example/#secret" }},
		{name: "credential query", mutate: func(lease *ConnectionLease) { lease.BaseURL = "https://executor.example/?access_token=secret" }},
		{name: "loopback", mutate: func(lease *ConnectionLease) { lease.BaseURL = "https://127.0.0.1" }},
		{name: "expired", mutate: func(lease *ConnectionLease) { lease.ExpiresAt = now.Add(-time.Second) }},
		{name: "authorization override", mutate: func(lease *ConnectionLease) { lease.HTTPHeaders.Set("authorization", "Bearer attacker") }},
		{name: "instance override", mutate: func(lease *ConnectionLease) { lease.HTTPHeaders.Set("X-Instance-ID", "stale") }},
		{name: "cookie override", mutate: func(lease *ConnectionLease) { lease.HTTPHeaders.Set("Cookie", "session=provider") }},
		{name: "hop-by-hop override", mutate: func(lease *ConnectionLease) { lease.HTTPHeaders.Set("Connection", "close") }},
		{name: "short lease", mutate: func(lease *ConnectionLease) { lease.ExpiresAt = now.Add(29 * time.Second) }},
		{name: "missing generation", mutate: func(lease *ConnectionLease) { lease.Generation = "" }},
		{name: "oversized headers", mutate: func(lease *ConnectionLease) {
			lease.HTTPHeaders.Set("X-Provider-Token", strings.Repeat("x", connectionLeaseHeaderLimit))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lease := valid()
			tc.mutate(&lease)
			if _, err := validateConnectionLease(lease, now); err == nil {
				t.Fatal("validateConnectionLease() accepted invalid lease")
			}
		})
	}
	if err := validateResolvedAddresses([]net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}); err == nil {
		t.Fatal("link-local address was accepted")
	}
	if err := validateResolvedAddresses([]net.IPAddr{{IP: net.ParseIP("203.0.113.17")}}); err != nil {
		t.Fatalf("documentation address should be accepted for controlled transport: %v", err)
	}
}

func TestPluginExecutorRevokedLeaseFailsClosed(t *testing.T) {
	now := time.Now()
	var current atomic.Int64
	current.Store(now.UnixNano())
	var calls atomic.Int32
	resolver := func(context.Context) (*ConnectionLease, error) {
		if calls.Add(1) > 1 {
			return nil, errors.New("provider revoked the connection lease")
		}
		return &ConnectionLease{
			BaseURL:    "https://executor.example",
			ExpiresAt:  now.Add(time.Minute),
			Generation: "first",
		}, nil
	}
	manager := newConnectionLeaseManager(resolver, func() time.Time { return time.Unix(0, current.Load()) }, nil)
	if _, err := manager.resolve(context.Background()); err != nil {
		t.Fatalf("initial lease: %v", err)
	}
	current.Store(now.Add(time.Minute + time.Second).UnixNano())
	if _, err := manager.resolve(context.Background()); err == nil {
		t.Fatal("expired lease was reused after provider revoked it")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("resolver calls = %d, want 2", got)
	}
}

func TestPluginExecutorEndpointBootstrapHandshake(t *testing.T) {
	var handshakeAuthorization atomic.Value
	var healthAuthorization atomic.Value
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server, dependencies, endpoint := newPluginExecutorTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/handshake":
			handshakeAuthorization.Store(r.Header.Get("Authorization"))
			var request struct {
				Nonce string `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Nonce != "one-time-nonce" {
				http.Error(w, "invalid nonce", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"issued-agentctl-token"}`))
		case "/health":
			healthAuthorization.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		case "/ws":
			if r.Header.Get("Authorization") != "Bearer issued-agentctl-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			_ = conn.Close()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := time.Now()
	resolver := func(context.Context) (*ConnectionLease, error) {
		return &ConnectionLease{
			BaseURL: endpoint, HTTPHeaders: http.Header{"X-Provider-Token": {"short-lived"}},
			ExpiresAt: now.Add(2 * time.Minute), Generation: "g1",
		}, nil
	}
	client, err := newEndpointClient(context.Background(), resolver, newTestLogger(), dependencies, func() time.Time { return now }, WithExecutionID("execution-1"))
	if err != nil {
		t.Fatalf("newEndpointClient(): %v", err)
	}
	defer client.Close()
	if _, err := client.BootstrapHandshake(context.Background(), "one-time-nonce"); err != nil {
		t.Fatalf("BootstrapHandshake(): %v", err)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health(): %v", err)
	}
	conn, response, err := client.dialWebSocket(context.Background(), "/ws", nil)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatalf("dialWebSocket() after bootstrap handshake: %v", err)
	}
	_ = conn.Close()
	if got := client.AuthToken(); got != "issued-agentctl-token" {
		t.Fatalf("AuthToken() = %q, want issued token", got)
	}
	if got := handshakeAuthorization.Load(); got != "" {
		t.Fatalf("handshake authorization = %v, want empty", got)
	}
	if got := healthAuthorization.Load(); got != "Bearer issued-agentctl-token" {
		t.Fatalf("health authorization = %v, want issued token", got)
	}
}

func TestPluginExecutorLeasedControlHandshakeAndInstanceCreation(t *testing.T) {
	type observedCall struct {
		path          string
		authorization string
		lease         string
	}
	var calls []observedCall
	var redirects atomic.Int32
	server, dependencies, endpoint := newPluginExecutorTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect-target" {
			redirects.Add(1)
			return
		}
		calls = append(calls, observedCall{path: r.URL.Path, authorization: r.Header.Get("Authorization"), lease: r.Header.Get("X-Provider-Lease")})
		switch r.URL.Path {
		case "/auth/handshake":
			if r.Header.Get("Authorization") != "" {
				http.Error(w, "handshake must not have a token", http.StatusUnauthorized)
				return
			}
			var request struct {
				Nonce string `json:"nonce"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Nonce != "control-nonce" {
				http.Error(w, "invalid nonce", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"issued-control-token"}`))
		case "/api/v1/instances":
			var request struct {
				ID        string `json:"id"`
				SessionID string `json:"session_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			if request.ID == "redirect" {
				http.Redirect(w, r, "https://redirect.example/credential-check", http.StatusFound)
				return
			}
			if r.Header.Get("Authorization") != "Bearer issued-control-token" || request.ID != "execution-control-1" || request.SessionID != "session-control-1" {
				http.Error(w, "invalid control request", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"execution-control-1","port":41234}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := time.Now()
	var current atomic.Int64
	current.Store(now.UnixNano())
	var leases atomic.Int32
	resolver := func(context.Context) (*ConnectionLease, error) {
		generation := leases.Add(1)
		return &ConnectionLease{
			BaseURL: endpoint, HTTPHeaders: http.Header{"X-Provider-Lease": {fmt.Sprintf("lease-%d", generation)}},
			ExpiresAt: time.Unix(0, current.Load()).Add(2 * time.Minute), Generation: strconv.Itoa(int(generation)),
		}, nil
	}
	control, err := newEndpointControlClient(
		context.Background(), resolver, newTestLogger(), dependencies,
		func() time.Time { return time.Unix(0, current.Load()) },
	)
	if err != nil {
		t.Fatalf("newEndpointControlClient(): %v", err)
	}

	token, err := control.Handshake(context.Background(), "control-nonce")
	if err != nil || token != "issued-control-token" || control.AuthToken() != token {
		t.Fatalf("Handshake() = %q, %v; stored token = %q", token, err, control.AuthToken())
	}
	created, err := control.CreateInstance(context.Background(), &CreateInstanceRequest{ID: "execution-control-1", SessionID: "session-control-1"})
	if err != nil || created == nil || created.Port != 41234 {
		t.Fatalf("CreateInstance() = %#v, %v", created, err)
	}
	if len(calls) != 2 || calls[0].authorization != "" || calls[1].authorization != "Bearer issued-control-token" ||
		calls[0].lease != "lease-1" || calls[1].lease != "lease-1" {
		t.Fatalf("control requests = %+v, want handshake then authenticated create with the provider lease", calls)
	}

	current.Store(now.Add(91 * time.Second).UnixNano())
	if _, err := control.CreateInstance(context.Background(), &CreateInstanceRequest{ID: "execution-control-1", SessionID: "session-control-1"}); err != nil {
		t.Fatalf("CreateInstance() after lease expiry: %v", err)
	}
	if leases.Load() != 2 || calls[len(calls)-1].lease != "lease-2" || calls[len(calls)-1].authorization != "Bearer issued-control-token" {
		t.Fatalf("refreshed control request = %+v, lease calls=%d", calls[len(calls)-1], leases.Load())
	}
	if _, err := control.CreateInstance(context.Background(), &CreateInstanceRequest{ID: "redirect"}); !errors.Is(err, errCrossOriginRedirect) {
		t.Fatalf("cross-origin redirect error = %v, want errCrossOriginRedirect", err)
	}
	if redirects.Load() != 0 {
		t.Fatalf("cross-origin redirect reached target %d times", redirects.Load())
	}
}

func TestPluginExecutorTransportMatrix(t *testing.T) {
	type observedRequest struct {
		path          string
		authorization string
		instanceID    string
		httpToken     string
		wsToken       string
		protocols     string
	}
	observed := make(chan observedRequest, 16)
	var redirectCalls atomic.Int32
	upgrader := websocket.Upgrader{
		CheckOrigin:  func(*http.Request) bool { return true },
		Subprotocols: []string{"provider.route"},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- observedRequest{
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			instanceID:    r.Header.Get("X-Instance-ID"),
			httpToken:     r.Header.Get("X-Provider-HTTP"),
			wsToken:       r.Header.Get("X-Provider-WS"),
			protocols:     r.Header.Get("Sec-WebSocket-Protocol"),
		}
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		if r.URL.Path == "/redirect" {
			redirectCalls.Add(1)
			http.Redirect(w, r, "https://redirect.example/credential-check", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/api/v1/status" {
			_, _ = w.Write([]byte("{\"agent_status\":\"running\"}"))
		}
	})
	server, dependencies, endpoint := newPluginExecutorTestServer(t, handler)
	defer server.Close()

	now := time.Now()
	var leaseCalls atomic.Int32
	resolver := func(context.Context) (*ConnectionLease, error) {
		generation := leaseCalls.Add(1)
		return &ConnectionLease{
			BaseURL: endpoint,
			HTTPHeaders: http.Header{
				"X-Provider-HTTP": {"provider-http"},
			},
			WebSocketHeaders: http.Header{
				"X-Provider-WS": {"provider-ws"},
			},
			WebSocketSubprotocols: []string{"provider.route"},
			ExpiresAt:             now.Add(2 * time.Minute),
			Generation:            strconv.Itoa(int(generation)),
		}, nil
	}
	client, err := newEndpointClient(context.Background(), resolver, newTestLogger(), dependencies, func() time.Time { return now },
		WithAuthToken("host-secret"),
		WithExecutionID("execution-remote"),
	)
	if err != nil {
		t.Fatalf("newEndpointClient: %v", err)
	}
	defer client.Close()

	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if _, err := client.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if err := client.StartShellTerminal(context.Background(), "terminal-1", 80, 24); err != nil {
		t.Fatalf("StartShellTerminal: %v", err)
	}
	terminal, err := client.StreamShellTerminal(context.Background(), "terminal-1")
	if err != nil {
		t.Fatalf("StreamShellTerminal: %v", err)
	}
	_ = terminal.Close()
	lsp, _, err := client.DialLSP(context.Background(), "go", false)
	if err != nil {
		t.Fatalf("DialLSP: %v", err)
	}
	_ = lsp.Close()
	if err := client.StreamUpdates(context.Background(), func(AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("StreamUpdates: %v", err)
	}
	client.CloseUpdatesStream()
	stream, err := client.StreamWorkspace(context.Background(), WorkspaceStreamCallbacks{})
	if err != nil {
		t.Fatalf("StreamWorkspace: %v", err)
	}
	client.CloseWorkspaceStream()
	stream.Wait()

	expectedPaths := map[string]bool{
		"/health":                      false,
		"/api/v1/status":               false,
		"/api/v1/shell/terminal/start": false,
		"/api/v1/shell/terminal/terminal-1/stream": false,
		"/api/v1/lsp/stream":                       false,
		"/api/v1/agent/stream":                     false,
		"/api/v1/workspace/stream":                 false,
	}
	for range len(expectedPaths) {
		select {
		case request := <-observed:
			if _, ok := expectedPaths[request.path]; !ok {
				t.Errorf("unexpected upstream path %q", request.path)
				continue
			}
			expectedPaths[request.path] = true
			if request.authorization != "Bearer host-secret" || request.instanceID != "execution-remote" {
				t.Errorf("host headers on %s = %q / %q", request.path, request.authorization, request.instanceID)
			}
			if strings.Contains(request.path, "stream") {
				if request.wsToken != "provider-ws" || !strings.Contains(request.protocols, "provider.route") {
					t.Errorf("WebSocket lease on %s = header %q, protocols %q", request.path, request.wsToken, request.protocols)
				}
			} else if request.httpToken != "provider-http" {
				t.Errorf("HTTP provider header on %s = %q", request.path, request.httpToken)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for endpoint transport matrix request")
		}
	}
	for path, seen := range expectedPaths {
		if !seen {
			t.Errorf("endpoint transport did not request %s", path)
		}
	}

	if _, err := client.httpClient.Get(client.BaseURL() + "/redirect"); !errors.Is(err, errCrossOriginRedirect) {
		t.Errorf("cross-origin redirect error = %v, want errCrossOriginRedirect", err)
	}
	mutation, err := http.NewRequest(http.MethodPost, client.BaseURL()+"/redirect", strings.NewReader("change"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.httpClient.Do(mutation); !errors.Is(err, errCrossOriginRedirect) {
		t.Errorf("cross-origin mutation redirect error = %v, want errCrossOriginRedirect", err)
	}
	if got := redirectCalls.Load(); got != 2 {
		t.Errorf("provider received %d redirect requests, want one per original request and no replay", got)
	}
}

func newPluginExecutorTestServer(t *testing.T, handler http.Handler) (*httptest.Server, endpointTransportDependencies, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		DNSNames:     []string{"executor.example"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	rootCAs := x509.NewCertPool()
	rootCertificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	rootCAs.AddCert(rootCertificate)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := endpointTransportDependencies{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("203.0.113.17")}}, nil
		},
		dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		tlsConfig: &tls.Config{RootCAs: rootCAs},
	}
	endpoint := "https://executor.example:" + serverURL.Port()
	return server, dependencies, endpoint
}
