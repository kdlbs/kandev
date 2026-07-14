package websocket

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// checkWebSocketOrigin validates the Origin header on WebSocket upgrade
// requests to prevent cross-site WebSocket hijacking: without it, any web
// page the user visits could open a socket to the local backend and drive
// session/shell actions on the host.
//
// Policy (mirrors corsMiddleware in internal/backendapp):
//   - no Origin header: allow (non-browser clients such as the CLI or curl)
//   - Origin hostname equals the request Host (ports ignored): allow
//   - Origin and request Host are both loopback: allow (dev servers and the
//     desktop shell talk to the backend across different loopback ports)
//   - anything else: deny
//
// Hostnames are compared exactly after parsing — a prefix check against
// "http://localhost" would also accept http://localhost.attacker.tld.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	originHost := normalizeOriginHost(parsed.Hostname())
	requestHost := normalizeOriginHost(r.Host)
	if originHost == "" || requestHost == "" {
		return false
	}

	return originHost == requestHost || (isLoopbackHost(originHost) && isLoopbackHost(requestHost))
}

func normalizeOriginHost(host string) string {
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
