package repoclone

import (
	"net"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	e2eMockEnv = "KANDEV_E2E_MOCK"
	// ProviderE2ELocal names the synthetic provider of local rig repositories.
	ProviderE2ELocal = "e2e-local"
)

// E2ELocalRemote recognizes a file:// or loopback-host remote of the synthetic
// e2e-local provider and returns its repository name. It answers only under the
// e2e mock profile; every other provider and profile keeps the HTTPS-only,
// credentialed clone path.
func E2ELocalRemote(provider, raw string) (name string, ok bool) {
	if !IsE2ELocalProvider(provider) {
		return "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !isE2ELocalHost(parsed) {
		return "", false
	}
	name = strings.TrimSuffix(path.Base(strings.TrimRight(parsed.Path, "/")), ".git")
	if name == "" || name == "." || name == "/" {
		return "", false
	}
	return name, true
}

// IsE2ELocalProvider reports whether provider is the synthetic e2e-local one
// and the process runs under the e2e mock profile.
func IsE2ELocalProvider(provider string) bool {
	return os.Getenv(e2eMockEnv) == "true" && strings.EqualFold(strings.TrimSpace(provider), ProviderE2ELocal)
}

func isE2ELocalHost(parsed *url.URL) bool {
	switch parsed.Scheme {
	case "file":
		return parsed.Host == ""
	case "http", ProtocolHTTPS:
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		return host == "localhost" || (ip != nil && ip.IsLoopback())
	default:
		return false
	}
}
