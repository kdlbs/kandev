package gitcredentials

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
)

// resolveE2ELocalIdentity accepts the file:// and loopback-host remotes of the
// synthetic e2e-local provider. It answers only under the e2e mock profile;
// every other provider and profile keeps the HTTPS and SSH requirement.
func resolveE2ELocalIdentity(input RepositoryIdentityInput) (RepositoryIdentity, bool) {
	if os.Getenv(e2eMockEnv) != "true" || !strings.EqualFold(strings.TrimSpace(input.Provider), providerE2ELocal) {
		return RepositoryIdentity{}, false
	}
	parsed, err := url.Parse(strings.TrimSpace(input.RemoteURL))
	if err != nil || !isE2ELocalHost(parsed) {
		return RepositoryIdentity{}, false
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(parsed.Path, "/")), ".git")
	if name == "" || name == "." || name == "/" {
		return RepositoryIdentity{}, false
	}
	host := parsed.Host
	if host == "" {
		host = "localhost"
	}
	return RepositoryIdentity{Host: host, Path: "/local/" + name + ".git", Owner: "local", Repository: name}, true
}

func isE2ELocalHost(parsed *url.URL) bool {
	switch parsed.Scheme {
	case "file":
		return parsed.Host == ""
	case "http", "https":
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		return host == "localhost" || (ip != nil && ip.IsLoopback())
	default:
		return false
	}
}
