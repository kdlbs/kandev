package netprobe

import (
	"net"
	"net/netip"
	"testing"
)

// TestPortAvailableReportsAWildcardListenerAsOccupied is the case a
// bind-only check gets wrong. A control server binds the wildcard address,
// and on macOS/BSD a fresh 127.0.0.1 bind against an active wildcard
// listener succeeds, so a launcher that only tries to bind concludes the
// port is free and starts a second server on a port some durable record
// already names.
func TestPortAvailableReportsAWildcardListenerAsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("wildcard listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	if PortAvailable(port) {
		t.Fatalf("port %d reported free while a wildcard listener holds it", port)
	}
	if !HasListener(port) {
		t.Fatalf("port %d reported as having no listener while one is bound", port)
	}
}

// TestPortAvailableReportsALoopbackListenerAsOccupied covers the narrower
// bind: a server listening only on IPv4 loopback is still occupying the
// port for our purposes.
func TestPortAvailableReportsALoopbackListenerAsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("loopback listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	if PortAvailable(port) {
		t.Fatalf("port %d reported free while a loopback listener holds it", port)
	}
}

// TestPortAvailableReportsAClosedPortAsFree is the open side: the probe must
// not be so conservative that it never finds a usable port.
func TestPortAvailableReportsAClosedPortAsFree(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("loopback listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	if !PortAvailable(port) {
		t.Fatalf("port %d reported occupied after its listener closed", port)
	}
	if HasListener(port) {
		t.Fatalf("port %d reported a listener after its listener closed", port)
	}
}

// TestHasListenerProbesBothLoopbackFamilies pins the dual-stack half of the
// contract: a server holding only the IPv6 wildcard socket is still found.
// Skipped where the host has no usable IPv6 loopback.
func TestHasListenerProbesBothLoopbackFamilies(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("no usable IPv6 loopback on this host: %v", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	if !HasListener(port) {
		t.Fatalf("port %d reported as having no listener while an IPv6 listener holds it", port)
	}
	if PortAvailable(port) {
		t.Fatalf("port %d reported free while an IPv6 listener holds it", port)
	}
}

func TestPortAvailableAtHostReportsNonLoopbackListener(t *testing.T) {
	listener, host := listenOnNonLoopbackIPv4(t)
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	if PortAvailableAtHost(host, port) {
		t.Fatalf("port %d reported free while %s holds it", port, host)
	}
}

func TestProbeAddressesForUnspecifiedHostUsesSpecificInterfaces(t *testing.T) {
	addresses, err := ProbeAddresses("0.0.0.0")
	if err != nil {
		t.Fatalf("ProbeAddresses: %v", err)
	}
	if len(addresses) == 0 {
		t.Fatal("ProbeAddresses returned no interface addresses")
	}
	for _, address := range addresses {
		if address.IsUnspecified() {
			t.Fatalf("probe address %s is unspecified; temporary wildcard listeners are not allowed", address)
		}
	}
}

func listenOnNonLoopbackIPv4(t *testing.T) (net.Listener, string) {
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
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil || !prefix.Addr().Is4() || prefix.Addr().IsLoopback() {
				continue
			}
			host := prefix.Addr().String()
			listener, err := net.Listen("tcp4", net.JoinHostPort(host, "0"))
			if err == nil {
				return listener, host
			}
		}
	}
	t.Skip("no usable non-loopback IPv4 address is available")
	return nil, ""
}
