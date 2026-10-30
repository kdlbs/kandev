package gitcredentials

import "github.com/kandev/kandev/internal/repoclone"

// resolveE2ELocalIdentity accepts the file:// and loopback-host remotes of the
// synthetic e2e-local provider; repoclone owns the e2e-profile gate.
func resolveE2ELocalIdentity(input RepositoryIdentityInput) (RepositoryIdentity, bool) {
	name, ok := repoclone.E2ELocalRemote(input.Provider, input.RemoteURL)
	if !ok {
		return RepositoryIdentity{}, false
	}
	return RepositoryIdentity{Host: "localhost", Path: "/local/" + name + ".git", Owner: "local", Repository: name}, true
}
