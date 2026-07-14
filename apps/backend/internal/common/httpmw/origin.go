package httpmw

import (
	"net"
	"net/url"
	"strings"
)

// AllowedOrigin reports whether a browser Origin header value is trusted to
// reach this backend, given the Host of the incoming request. It is the single
// origin trust policy shared by the HTTP CORS middleware and the WebSocket
// upgraders so the two cannot silently diverge.
//
// Policy:
//   - Origin hostname equals the request hostname: allow. Ports are
//     deliberately not compared — the dev SPA connects cross-port over the
//     same hostname (apps/web/lib/config.ts), and content served from the
//     backend's own hostname implies a process on that host, which could
//     open a direct connection without an Origin header anyway.
//   - Origin and request host are both loopback: allow (dev servers and the
//     desktop shell talk to the backend across different loopback ports).
//   - Anything else: deny.
//
// Hostnames are compared exactly after parsing — a prefix check against
// "http://localhost" would also accept http://localhost.attacker.tld.
//
// The empty-Origin case is intentionally not handled here; callers decide
// (CORS answers with a wildcard, the WebSocket upgrader allows non-browser
// clients such as the CLI or curl).
func AllowedOrigin(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || !isWellFormedOrigin(parsed) {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	originHost := normalizeHost(parsed.Hostname())
	host := normalizeHost(requestHost)
	if originHost == "" || host == "" {
		return false
	}

	return originHost == host || (isLoopbackHost(originHost) && isLoopbackHost(host))
}

// isWellFormedOrigin rejects URL components that never appear in a
// browser-sent Origin header (userinfo, path, query, fragment, opaque data);
// url.Parse accepts them, but the strict policy treats them as malformed.
func isWellFormedOrigin(u *url.URL) bool {
	return u.Scheme != "" && u.Host != "" &&
		u.User == nil && u.Opaque == "" && u.Path == "" &&
		u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func normalizeHost(host string) string {
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
