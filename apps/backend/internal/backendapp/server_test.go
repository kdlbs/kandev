package backendapp

import (
	"net"
	"testing"
)

func TestServerListenAddr(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "blank host keeps port-only address", host: "", port: 38429, want: ":38429"},
		{name: "wildcard host", host: "0.0.0.0", port: 38429, want: "0.0.0.0:38429"},
		{name: "loopback host", host: "127.0.0.1", port: 38429, want: "127.0.0.1:38429"},
		{name: "ipv6 host", host: "::1", port: 38429, want: "[::1]:38429"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverListenAddr(tt.host, tt.port); got != tt.want {
				t.Fatalf("serverListenAddr(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

func TestDesktopHealthTokenTrimsEnv(t *testing.T) {
	t.Setenv(desktopHealthTokenEnv, "  token-value  ")

	if got := desktopHealthToken(); got != "token-value" {
		t.Fatalf("desktopHealthToken() = %q, want token-value", got)
	}
}

func TestServerProbeAddr(t *testing.T) {
	tests := []struct {
		name       string
		listenAddr string
		want       string
	}{
		{name: "port-only address probes loopback", listenAddr: ":38429", want: "127.0.0.1:38429"},
		{name: "wildcard ipv4 probes loopback", listenAddr: "0.0.0.0:38429", want: "127.0.0.1:38429"},
		{name: "wildcard ipv6 probes ipv6 loopback", listenAddr: "[::]:38429", want: "[::1]:38429"},
		{name: "loopback address probes itself", listenAddr: "127.0.0.1:38429", want: "127.0.0.1:38429"},
		{name: "ipv6 loopback probes itself", listenAddr: "[::1]:38429", want: "[::1]:38429"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverProbeAddr(tt.listenAddr); got != tt.want {
				t.Fatalf("serverProbeAddr(%q) = %q, want %q", tt.listenAddr, got, tt.want)
			}
		})
	}
}

func listenOnFreePort(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free port: %v", err)
	}
	return ln
}

func listenerPort(t *testing.T, ln net.Listener) int {
	t.Helper()
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected TCP listener, got %T", ln.Addr())
	}
	return tcpAddr.Port
}
