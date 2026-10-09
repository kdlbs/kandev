package config

import (
	"net"
	"net/http"
	"strconv"
	"testing"
)

func TestMCPReachableHostUsesMatchingLoopbackForWildcardListeners(t *testing.T) {
	for _, tc := range []struct {
		listenHost string
		want       string
	}{
		{listenHost: "", want: "127.0.0.1"},
		{listenHost: "0.0.0.0", want: "127.0.0.1"},
		{listenHost: "::", want: "::1"},
	} {
		t.Run(tc.listenHost, func(t *testing.T) {
			if got := MCPReachableHost(tc.listenHost); got != tc.want {
				t.Fatalf("MCPReachableHost(%q) = %q, want %q", tc.listenHost, got, tc.want)
			}
		})
	}
}

func TestNewInstanceConfigMCPURLsReachConfiguredListener(t *testing.T) {
	listener := listenOnNonLoopbackIPv4(t)
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse listener port: %v", err)
	}

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	instance := (&Config{AuthToken: "test-token", ListenHostOverride: host}).NewInstanceConfig(port, nil)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	for _, mcpServer := range instance.McpServers {
		response, err := client.Get(mcpServer.URL)
		if err != nil {
			t.Fatalf("request injected %s endpoint %q: %v", mcpServer.Type, mcpServer.URL, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("request injected %s endpoint %q status = %d, want %d", mcpServer.Type, mcpServer.URL, response.StatusCode, http.StatusNoContent)
		}
	}
}

func listenOnNonLoopbackIPv4(t *testing.T) net.Listener {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Skipf("list network interfaces: %v", err)
	}
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			listener, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
			if err == nil {
				return listener
			}
		}
	}
	t.Skip("no usable non-loopback IPv4 address is available")
	return nil
}
