package launcher

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

var (
	networkInterfacesFn     = net.Interfaces
	networkInterfaceAddrsFn = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }
)

func listHostNetworkAddresses() []string {
	interfaces, err := networkInterfacesFn()
	if err != nil {
		return nil
	}

	var addresses []net.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := networkInterfaceAddrsFn(iface)
		if err != nil {
			continue
		}
		addresses = append(addresses, addrs...)
	}
	return networkAddressesFromAddrs(addresses)
}

func networkAddressesFromAddrs(addresses []net.Addr) []string {
	var ipv4, ipv6 []string
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		ip := networkAddrIP(address)
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		host := ip.String()
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		if ip.To4() != nil {
			ipv4 = append(ipv4, host)
		} else {
			ipv6 = append(ipv6, host)
		}
	}
	return append(ipv4, ipv6...)
}

func networkAddrIP(address net.Addr) net.IP {
	switch value := address.(type) {
	case *net.IPNet:
		return value.IP
	case *net.IPAddr:
		return value.IP
	default:
		return nil
	}
}

func networkURLsForPort(port int, hosts []string) []string {
	urls := make([]string, 0, len(hosts))
	for _, host := range hosts {
		urls = append(urls, fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(port))))
	}
	return urls
}

func networkAddressesForBindHost(addresses []string, bindHost string) []string {
	if strings.TrimSpace(bindHost) == "" {
		return addresses
	}

	allowed := make(map[string]struct{})
	for _, rawHost := range strings.Split(bindHost, ",") {
		host := strings.TrimSpace(rawHost)
		if host == "" {
			continue
		}
		ip := net.ParseIP(host)
		if ip == nil {
			continue
		}
		if ip.IsUnspecified() {
			return addresses
		}
		if !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			allowed[ip.String()] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(allowed))
	for _, address := range addresses {
		if _, ok := allowed[address]; ok {
			filtered = append(filtered, address)
		}
	}
	return filtered
}
