package launcher

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
)

func TestLogStartupPrintsNetworkAddress(t *testing.T) {
	t.Setenv("KANDEV_SERVER_HOST", "")
	oldNetworkInterfaces := networkInterfacesFn
	oldNetworkInterfaceAddrs := networkInterfaceAddrsFn
	t.Cleanup(func() {
		networkInterfacesFn = oldNetworkInterfaces
		networkInterfaceAddrsFn = oldNetworkInterfaceAddrs
	})
	networkInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}, {Name: "tailscale0"}}, nil
	}
	networkInterfaceAddrsFn = func(iface net.Interface) ([]net.Addr, error) {
		switch iface.Name {
		case "eth0":
			return []net.Addr{testNetworkAddr(t, "192.168.1.34/22")}, nil
		case "tailscale0":
			return []net.Addr{testNetworkAddr(t, "100.94.173.104/32")}, nil
		default:
			return nil, nil
		}
	}

	output := captureLauncherStdout(t, func() {
		logStartup("test", portConfig{
			BackendPort: 38429,
			BackendURL:  "http://localhost:38429",
		}, "", "")
	})

	for _, want := range []string{
		"[kandev]   network: http://192.168.1.34:38429",
		"[kandev]   network: http://100.94.173.104:38429",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("startup output = %q, want %q", output, want)
		}
	}
}

func TestListHostNetworkAddressesFiltersAndOrders(t *testing.T) {
	addresses := []net.Addr{
		testNetworkAddr(t, "127.0.0.1/8"),
		testNetworkAddr(t, "169.254.10.5/16"),
		testNetworkAddr(t, "2001:db8::1/64"),
		testNetworkAddr(t, "192.168.1.34/22"),
		testNetworkAddr(t, "100.94.173.104/32"),
		testNetworkAddr(t, "192.168.1.34/22"),
		testNetworkAddr(t, "fe80::1/64"),
		testNetworkAddr(t, "::1/128"),
	}

	got := networkAddressesFromAddrs(addresses)
	want := []string{"192.168.1.34", "100.94.173.104", "2001:db8::1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("networkAddressesFromAddrs() = %v, want %v", got, want)
	}
}

func TestNetworkURLsForPortFormatsIPv6(t *testing.T) {
	hosts := []string{"192.168.1.34", "100.94.173.104", "2001:db8::1"}
	got := networkURLsForPort(38429, hosts)
	want := []string{
		"http://192.168.1.34:38429",
		"http://100.94.173.104:38429",
		"http://[2001:db8::1]:38429",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("networkURLsForPort() = %v, want %v", got, want)
	}
}

func TestNetworkAddressesForBindHost(t *testing.T) {
	addresses := []string{"192.168.1.34", "100.94.173.104", "2001:db8::1"}
	tests := []struct {
		name     string
		bindHost string
		want     []string
	}{
		{name: "wildcard default", bindHost: "", want: addresses},
		{name: "ipv4 wildcard", bindHost: "0.0.0.0", want: addresses},
		{name: "ipv6 wildcard", bindHost: "::", want: addresses},
		{name: "loopback", bindHost: "127.0.0.1"},
		{name: "specific address", bindHost: "192.168.1.34", want: []string{"192.168.1.34"}},
		{name: "multiple addresses", bindHost: "127.0.0.1,100.94.173.104", want: []string{"100.94.173.104"}},
		{name: "hostname loopback", bindHost: "localhost"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := networkAddressesForBindHost(addresses, test.bindHost)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("networkAddressesForBindHost(%q) = %v, want %v", test.bindHost, got, test.want)
			}
		})
	}
}

func TestLogStartupKeepsLocalhostWhenNetworkDiscoveryFails(t *testing.T) {
	t.Setenv("KANDEV_SERVER_HOST", "")
	oldNetworkInterfaces := networkInterfacesFn
	oldNetworkInterfaceAddrs := networkInterfaceAddrsFn
	t.Cleanup(func() {
		networkInterfacesFn = oldNetworkInterfaces
		networkInterfaceAddrsFn = oldNetworkInterfaceAddrs
	})
	networkInterfacesFn = func() ([]net.Interface, error) {
		return nil, errors.New("test interface enumeration failure")
	}

	output := captureLauncherStdout(t, func() {
		logStartup("test", portConfig{
			BackendPort: 38429,
			BackendURL:  "http://localhost:38429",
		}, "", "")
	})
	if !strings.Contains(output, "[kandev] url: http://localhost:38429") {
		t.Fatalf("startup output = %q, missing localhost URL", output)
	}
	if strings.Contains(output, "network:") {
		t.Fatalf("startup output = %q, want no network lines", output)
	}
}

func TestLogStartupSuppressesNetworkAddressesForLoopbackBind(t *testing.T) {
	oldNetworkInterfaces := networkInterfacesFn
	oldNetworkInterfaceAddrs := networkInterfaceAddrsFn
	t.Cleanup(func() {
		networkInterfacesFn = oldNetworkInterfaces
		networkInterfaceAddrsFn = oldNetworkInterfaceAddrs
	})
	t.Setenv("KANDEV_SERVER_HOST", "127.0.0.1")
	networkInterfacesFn = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "eth0"}}, nil
	}
	networkInterfaceAddrsFn = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{testNetworkAddr(t, "192.168.1.34/22")}, nil
	}

	output := captureLauncherStdout(t, func() {
		logStartup("test", portConfig{
			BackendPort: 38429,
			BackendURL:  "http://localhost:38429",
		}, "", "")
	})
	if strings.Contains(output, "network:") {
		t.Fatalf("startup output = %q, want no network lines for loopback bind", output)
	}
}

func testNetworkAddr(t *testing.T, cidr string) net.Addr {
	t.Helper()
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("net.ParseCIDR(%q) = %v", cidr, err)
	}
	return &net.IPNet{IP: ip, Mask: network.Mask}
}

func captureLauncherStdout(t *testing.T, fn func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() = %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = oldStdout }()

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	var output bytes.Buffer
	if _, err := io.Copy(&output, read); err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return output.String()
}
