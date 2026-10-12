package netprobe

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

const addressLookupTimeout = 2 * time.Second

// ProbeAddresses returns the concrete addresses an agentctl listener can use
// for host. Wildcard hosts expand to local interface addresses so callers can
// probe without opening a temporary all-interfaces listener.
func ProbeAddresses(host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return localInterfaceAddresses(0)
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if !addr.IsUnspecified() {
			return []netip.Addr{addr}, nil
		}
		family := 0
		if addr.Is4() {
			family = 4
		}
		// An IPv6 wildcard listener can also accept IPv4 on dual-stack hosts.
		return localInterfaceAddresses(family)
	}

	ctx, cancel := context.WithTimeout(context.Background(), addressLookupTimeout)
	defer cancel()
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve listen host %q: %w", host, err)
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	for _, addr := range resolved {
		if addr.Is4In6() {
			addr = addr.Unmap()
		}
		if !addr.IsUnspecified() && !addr.IsMulticast() {
			addresses = append(addresses, addr)
		}
	}
	return uniqueAddresses(addresses), nil
}

// PortAvailableAtHost checks the address set used by a listener bound to host.
// It uses bounded connect probes before specific-address bind probes. No probe
// binds a wildcard address, including when host itself is unspecified.
func PortAvailableAtHost(host string, port int) bool {
	addresses, err := ProbeAddresses(host)
	if err != nil {
		return false
	}
	return PortAvailableAtAddresses(addresses, port)
}

// PortAvailableAtAddresses checks a concrete address set. The set is normally
// produced by ProbeAddresses and is also reused while selecting a fallback
// port so hostname resolution does not change between probe stages.
func PortAvailableAtAddresses(addresses []netip.Addr, port int) bool {
	if len(addresses) == 0 || port < 1 || port > 65535 {
		return false
	}
	portText := strconv.Itoa(port)
	for _, addr := range addresses {
		if hasListenerAt(addr, portText) {
			return false
		}
	}
	for _, addr := range addresses {
		listener, err := net.Listen(networkForAddress(addr), net.JoinHostPort(addr.String(), portText))
		if err != nil {
			return false
		}
		_ = listener.Close()
	}
	return true
}

func hasListenerAt(addr netip.Addr, port string) bool {
	conn, err := net.DialTimeout(networkForAddress(addr), net.JoinHostPort(addr.String(), port), ConnectProbeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func networkForAddress(addr netip.Addr) string {
	if addr.Is4() {
		return "tcp4"
	}
	return "tcp6"
}

func localInterfaceAddresses(family int) ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces for port probe: %w", err)
	}
	addresses := make([]netip.Addr, 0)
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses = append(addresses, probeAddressesForInterface(iface, family)...)
	}
	if len(addresses) == 0 {
		switch family {
		case 4:
			addresses = append(addresses, netip.MustParseAddr("127.0.0.1"))
		case 6:
			addresses = append(addresses, netip.MustParseAddr("::1"))
		default:
			addresses = append(addresses, netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1"))
		}
	}
	return uniqueAddresses(addresses), nil
}

func probeAddressesForInterface(iface net.Interface, family int) []netip.Addr {
	ifaceAddresses, err := iface.Addrs()
	if err != nil {
		return nil
	}
	addresses := make([]netip.Addr, 0, len(ifaceAddresses))
	for _, ifaceAddress := range ifaceAddresses {
		prefix, err := netip.ParsePrefix(ifaceAddress.String())
		if err != nil {
			continue
		}
		addr := prefix.Addr()
		if addr.Is4In6() {
			addr = addr.Unmap()
		}
		if family == 4 && !addr.Is4() || family == 6 && !addr.Is6() {
			continue
		}
		if addr.Is6() && addr.IsLinkLocalUnicast() {
			addr = addr.WithZone(iface.Name)
		}
		if addr.IsUnspecified() || addr.IsMulticast() {
			continue
		}
		addresses = append(addresses, addr)
	}
	return addresses
}

func uniqueAddresses(addresses []netip.Addr) []netip.Addr {
	seen := make(map[netip.Addr]struct{}, len(addresses))
	unique := make([]netip.Addr, 0, len(addresses))
	for _, addr := range addresses {
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		unique = append(unique, addr)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].Compare(unique[j]) < 0 })
	return unique
}
