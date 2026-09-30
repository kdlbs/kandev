package launcher

import (
	"net"
	"testing"
)

// TestFreePortProbeAddrFollowsChildListenHost pins where the fallback-port
// probe binds for each shape of agent.standaloneHost. It only computes
// addresses; nothing listens.
func TestFreePortProbeAddrFollowsChildListenHost(t *testing.T) {
	cases := []struct {
		name string
		host string
		want string
	}{
		{name: "default IPv4 loopback", host: "127.0.0.1", want: "127.0.0.1:0"},
		{name: "bracketed IPv6 loopback", host: "[::1]", want: "[::1]:0"},
		{name: "bare IPv6 loopback", host: "::1", want: "[::1]:0"},
		{name: "operator-selected IPv4 address", host: "192.0.2.10", want: "192.0.2.10:0"},
		{name: "operator-selected IPv6 address", host: "[2001:db8::10]", want: "[2001:db8::10]:0"},
		{name: "localhost", host: "localhost", want: "127.0.0.1:0"},
		{name: "empty host", host: "", want: "127.0.0.1:0"},
		{name: "other host name", host: "agent-host.example", want: "127.0.0.1:0"},
		{name: "IPv4 unspecified address", host: "0.0.0.0", want: "127.0.0.1:0"},
		{name: "bare IPv6 unspecified address", host: "::", want: "127.0.0.1:0"},
		{name: "bracketed IPv6 unspecified address", host: "[::]", want: "127.0.0.1:0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := freePortProbeAddr(tc.host); got != tc.want {
				t.Fatalf("freePortProbeAddr(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}

// TestFindFreePortOnLoopbackHosts runs the probe for the IPv4 and IPv6
// loopback literals. It binds loopback addresses only.
func TestFindFreePortOnLoopbackHosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		t.Run(host, func(t *testing.T) {
			if host == "[::1]" {
				skipWithoutIPv6Loopback(t)
			}
			got, err := findFreePort(host)
			if err != nil {
				t.Fatalf("findFreePort(%q): %v", host, err)
			}
			if got <= 0 || got > 65535 {
				t.Fatalf("findFreePort(%q) = %d, want a TCP port number", host, got)
			}
		})
	}
}

func skipWithoutIPv6Loopback(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is not available: %v", err)
	}
	_ = ln.Close()
}
