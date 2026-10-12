package launcher

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/netprobe"
)

// TestFreePortProbeAddrsFollowChildListenHost pins the concrete addresses used
// by fallback probes. Wildcard hosts expand to interface addresses instead of
// opening a temporary wildcard listener.
func TestFreePortProbeAddrsFollowChildListenHost(t *testing.T) {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := freePortProbeAddrs(tc.host)
			if err != nil {
				t.Fatalf("freePortProbeAddrs(%q): %v", tc.host, err)
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("freePortProbeAddrs(%q) = %q, want [%q]", tc.host, got, tc.want)
			}
		})
	}

	t.Run("localhost resolves to its child bind addresses", func(t *testing.T) {
		got, err := freePortProbeAddrs("localhost")
		if err != nil {
			t.Fatalf("freePortProbeAddrs(localhost): %v", err)
		}
		resolved, err := netprobe.ProbeAddresses("localhost")
		if err != nil {
			t.Fatalf("ProbeAddresses(localhost): %v", err)
		}
		if len(got) != len(resolved) {
			t.Fatalf("localhost probes = %q, want one per resolved address %v", got, resolved)
		}
	})

	for _, host := range []string{"", "0.0.0.0", "::", "[::]"} {
		t.Run("wildcard "+host, func(t *testing.T) {
			got, err := freePortProbeAddrs(host)
			if err != nil {
				t.Fatalf("freePortProbeAddrs(%q): %v", host, err)
			}
			if len(got) == 0 {
				t.Fatalf("freePortProbeAddrs(%q) returned no interface addresses", host)
			}
			for _, probe := range got {
				address, _, err := net.SplitHostPort(probe)
				if err != nil {
					t.Fatalf("invalid probe address %q: %v", probe, err)
				}
				ip, err := netip.ParseAddr(strings.Trim(address, "[]"))
				if err != nil || ip.IsUnspecified() {
					t.Fatalf("wildcard probe %q is not a concrete address", probe)
				}
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

func TestFindFreePortOnWildcardHostIsAvailableAtEveryInterface(t *testing.T) {
	addresses, err := netprobe.ProbeAddresses("0.0.0.0")
	if err != nil {
		t.Fatalf("ProbeAddresses: %v", err)
	}
	port, err := findFreePort("0.0.0.0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	if !netprobe.PortAvailableAtAddresses(addresses, port) {
		t.Fatalf("fallback port %d is not available on every probed interface %v", port, addresses)
	}
}

func TestEnsurePortAvailableChecksConfiguredListenHost(t *testing.T) {
	listener, host := listenOnNonLoopbackIPv4ForLauncher(t)
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	launcher := &Launcher{host: host, port: port, logger: newUnexpectedExitTestLogger(t)}
	if err := launcher.ensurePortAvailable(); err != nil {
		t.Fatalf("ensurePortAvailable: %v", err)
	}
	if launcher.port == port {
		t.Fatalf("preferred port %d stayed selected while occupied on %s", port, host)
	}

	selected, err := net.Listen("tcp4", net.JoinHostPort(host, strconv.Itoa(launcher.port)))
	if err != nil {
		t.Fatalf("selected port %d is not available on %s: %v", launcher.port, host, err)
	}
	_ = selected.Close()
}

func listenOnNonLoopbackIPv4ForLauncher(t *testing.T) (net.Listener, string) {
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
			host := ip.String()
			listener, err := net.Listen("tcp4", net.JoinHostPort(host, "0"))
			if err == nil {
				return listener, host
			}
		}
	}
	t.Skip("no usable non-loopback IPv4 address is available")
	return nil, ""
}

func skipWithoutIPv6Loopback(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is not available: %v", err)
	}
	_ = ln.Close()
}
