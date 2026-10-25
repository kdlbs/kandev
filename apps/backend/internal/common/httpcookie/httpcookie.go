// Package httpcookie derives port-scoped cookie names from a request host.
//
// Browsers match cookies by host, domain, and path — port is not part of
// cookie scope — so multiple kandev instances on one host (same IP,
// different ports) share one cookie jar. Suffixing cookie names with the
// request's port lets each instance read and write only its own cookies
// (see docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md).
package httpcookie

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// PortSuffix returns "_<port>" derived from the request host, or "" when no
// usable port is present. X-Forwarded-Host (first value, trimmed) wins over
// Host so a proxy that rewrites only the port can carry the browser's
// host:port. The HTTP boundary (httpmw.StripUntrustedForwardedHost,
// registered in backendapp.buildHTTPServer) removes X-Forwarded-Host from
// untrusted peers, so a value present here is proxy-authored. A suffix
// requires a decimal port in 1..65535: nonnumeric service names and
// out-of-range values yield no suffix. The scheme's default port yields NO
// suffix: 443 for TLS (direct or X-Forwarded-Proto: https) and 80 otherwise.
// This mirrors the SPA's API-origin derivation, where URL.port reports "" for
// the scheme-default port only — so http://host:443 stays "_443" on both
// sides instead of splitting one instance's cookies across two names. A nil
// request yields "".
func PortSuffix(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = firstValue(xfh)
	}
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		return ""
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return ""
	}
	if n == defaultPortForRequest(r) {
		return ""
	}
	return "_" + port
}

// defaultPortForRequest returns the scheme's default port: 443 when the
// request arrived over TLS (directly or via X-Forwarded-Proto), 80 otherwise.
// Mirrors the Secure-cookie flag logic (requestIsTLS) so the two scheme
// decisions cannot diverge.
func defaultPortForRequest(r *http.Request) int {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return 443
	}
	return 80
}

// ScopedName returns base suffixed with the request's port suffix, or "" when
// base is empty (a suffix is meaningless without a base name).
func ScopedName(r *http.Request, base string) string {
	if base == "" {
		return ""
	}
	return base + PortSuffix(r)
}

// firstValue returns the first comma-separated value, trimmed.
func firstValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return strings.TrimSpace(v)
}
