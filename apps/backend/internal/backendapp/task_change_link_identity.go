package backendapp

import (
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/httpmw"
)

// taskChangeLinkIdentityResolver supplies the host's single-user identity only
// while authentication is explicitly disabled. An unavailable service denies it.
func taskChangeLinkIdentityResolver(svc *auth.Service) func() (authn.Identity, bool) {
	return func() (authn.Identity, bool) {
		if svc == nil || svc.Mode() != auth.ModeDisabled {
			return authn.Identity{}, false
		}
		return httpmw.SyntheticIdentity(), true
	}
}
