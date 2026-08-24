package launcher

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
)

var (
	networkInterfacesFn     = net.Interfaces
	networkInterfaceAddrsFn = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }
)

type backendEndpointSet struct {
	bindHosts     []string
	healthTargets []string
	accessURLs    []string
	accessURL     string
}

func resolveBackendEndpoints(cfg *config.Config, port int) (backendEndpointSet, error) {
	server := config.ServerConfig{Host: os.Getenv("KANDEV_SERVER_HOST")}
	if cfg != nil {
		server = cfg.Server
	}
	binds, err := server.ResolvedBinds()
	if err != nil {
		return backendEndpointSet{}, err
	}

	loopbackTargets := make([]string, 0, len(binds))
	loopbackAccessURLs := make([]string, 0, len(binds))
	nonLoopbackTargets := make([]string, 0, len(binds))
	nonLoopbackAccessURLs := make([]string, 0, len(binds))
	seen := make(map[string]struct{}, len(binds))
	for _, bind := range binds {
		host := endpointHost(bind)
		healthURL := backendURLForHost(host, port) + "/health"
		accessURL := backendURLForHost(browserEndpointHost(bind, host), port)
		if _, ok := seen[healthURL]; ok {
			continue
		}
		seen[healthURL] = struct{}{}
		if config.IsLoopbackHost(host) {
			loopbackTargets = append(loopbackTargets, healthURL)
			loopbackAccessURLs = append(loopbackAccessURLs, accessURL)
			continue
		}
		nonLoopbackTargets = append(nonLoopbackTargets, healthURL)
		nonLoopbackAccessURLs = append(nonLoopbackAccessURLs, accessURL)
	}
	healthTargets := append([]string(nil), loopbackTargets...)
	healthTargets = append(healthTargets, nonLoopbackTargets...)
	accessURLs := append([]string(nil), loopbackAccessURLs...)
	accessURLs = append(accessURLs, nonLoopbackAccessURLs...)
	if len(healthTargets) == 0 {
		return backendEndpointSet{}, fmt.Errorf("server bind configuration produced no health targets")
	}
	return backendEndpointSet{
		bindHosts:     append([]string(nil), binds...),
		healthTargets: healthTargets,
		accessURLs:    accessURLs,
		accessURL:     accessURLs[0],
	}, nil
}

func browserEndpointHost(bind, probeHost string) string {
	ip := net.ParseIP(strings.TrimSuffix(strings.TrimSpace(bind), "."))
	if ip != nil && ip.IsUnspecified() && ip.To4() != nil {
		return "localhost"
	}
	return probeHost
}

func endpointHost(bind string) string {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(bind), "."))
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			if ip.To4() != nil {
				return "127.0.0.1"
			}
			return "::1"
		}
		return ip.String()
	}
	return host
}

func backendURLForHost(host string, port int) string {
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(port)))
}

func endpointSetForAccessURL(accessURL string) backendEndpointSet {
	accessURL = strings.TrimRight(strings.TrimSpace(accessURL), "/")
	return backendEndpointSet{
		bindHosts:     []string{"localhost"},
		healthTargets: []string{accessURL + "/health"},
		accessURLs:    []string{accessURL},
		accessURL:     accessURL,
	}
}

func (e backendEndpointSet) browserURLForHealthTarget(target string) string {
	for index, healthTarget := range e.healthTargets {
		if healthTarget != target {
			continue
		}
		if index < len(e.accessURLs) && e.accessURLs[index] != "" {
			return e.accessURLs[index]
		}
		break
	}
	return strings.TrimSuffix(target, "/health")
}

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
