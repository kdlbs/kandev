package websocket

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

// tunnelBackedClient returns an agentctl client whose lease is host_tunnel and
// whose every stream is a fresh connection to upstream.
func tunnelBackedClient(t *testing.T, log *logger.Logger, upstream *httptest.Server, dials *atomic.Int32) *agentctlclient.Client {
	t.Helper()
	address := upstream.Listener.Addr().String()
	resolver := func(context.Context) (*agentctlclient.ConnectionLease, error) {
		return &agentctlclient.ConnectionLease{
			Transport: agentctlclient.LeaseTransportHostTunnel, Generation: "g1", ExpiresAt: time.Now().Add(time.Hour),
			Dial: func(ctx context.Context) (net.Conn, error) {
				dials.Add(1)
				return (&net.Dialer{}).DialContext(ctx, "tcp", address)
			},
		}, nil
	}
	client, err := agentctlclient.NewEndpointClient(context.Background(), resolver, log)
	if err != nil {
		t.Fatalf("NewEndpointClient() error = %v", err)
	}
	return client
}

func TestEditorProxyCarriesTrafficThroughTheHostTunnel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "editor:"+r.URL.Path)
	}))
	defer upstream.Close()
	var dials atomic.Int32
	log, _ := observedTerminalLogger(t)
	client := tunnelBackedClient(t, log, upstream, &dials)
	target, err := url.Parse(client.BaseURL())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewVscodeProxyHandler(nil, log)
	gateway := httptest.NewServer(handler.createProxy("session-1", target, client.ProxyTransport()))
	defer gateway.Close()

	response, err := http.Get(gateway.URL + "/folder")
	if err != nil {
		t.Fatalf("GET through the proxy error = %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "editor:/folder" {
		t.Fatalf("proxied body = %q, want the upstream response", body)
	}
	if dials.Load() == 0 {
		t.Fatal("the proxy must reach the environment through the tunnel dialer")
	}
}
