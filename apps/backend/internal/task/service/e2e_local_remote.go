package service

import "github.com/kandev/kandev/internal/repoclone"

const (
	providerE2ELocal = repoclone.ProviderE2ELocal
	e2eLocalOwner    = "local"
)

// parseE2ELocalRemote recognizes a file:// or loopback-host remote through the
// same predicate the clone path uses. It only answers under the e2e mock
// profile; production keeps the provider allowlist.
func parseE2ELocalRemote(raw string) (owner, name string, ok bool) {
	name, ok = repoclone.ParseE2ELocalRemote(raw)
	if !ok {
		return "", "", false
	}
	return e2eLocalOwner, name, true
}
