package service

import (
	"net"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	e2eMockEnv       = "KANDEV_E2E_MOCK"
	providerE2ELocal = "e2e-local"
	e2eLocalOwner    = "local"
)

// parseE2ELocalRemote recognizes a file:// or loopback-host remote. It only
// answers under the e2e mock profile; production keeps the provider allowlist.
func parseE2ELocalRemote(raw string) (owner, name string, ok bool) {
	if os.Getenv(e2eMockEnv) != "true" {
		return "", "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !isE2ELocalRemoteHost(parsed) {
		return "", "", false
	}
	name = strings.TrimSuffix(path.Base(strings.TrimRight(parsed.Path, "/")), ".git")
	if name == "" || name == "." || name == "/" {
		return "", "", false
	}
	return e2eLocalOwner, name, true
}

func isE2ELocalRemoteHost(parsed *url.URL) bool {
	switch parsed.Scheme {
	case "file":
		return parsed.Host == ""
	case protocolHTTP, protocolHTTPS:
		host := parsed.Hostname()
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	default:
		return false
	}
}

func isE2ELocalRemote(raw string) bool {
	_, _, ok := parseE2ELocalRemote(raw)
	return ok
}
