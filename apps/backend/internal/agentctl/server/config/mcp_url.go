package config

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// MCPReachableHost returns a host that a process in agentctl's execution
// environment can use to reach an instance listener bound to listenHost.
// An empty or unspecified host means agentctl listens on all interfaces, so
// loopback is the narrowest reachable address to advertise.
func MCPReachableHost(listenHost string) string {
	host := strings.TrimSpace(strings.Trim(listenHost, "[]"))
	if host == "" {
		return "127.0.0.1"
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.IsUnspecified() {
			if addr.Is6() {
				return "::1"
			}
			return "127.0.0.1"
		}
		return addr.String()
	}
	return host
}

// MCPServerURL builds an HTTP MCP endpoint for an instance server.
func MCPServerURL(host string, port int, path string) string {
	if strings.TrimSpace(host) == "" {
		// Preserve the legacy endpoint for InstanceConfig values that predate
		// MCPHost. New instance configs always pass their reachable host.
		host = "localhost"
	} else {
		host = MCPReachableHost(host)
	}
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	if strings.Contains(host, "%") {
		hostPort = strings.Replace(hostPort, "%", "%25", 1)
	}
	return (&url.URL{Scheme: "http", Host: hostPort, Path: path}).String()
}
