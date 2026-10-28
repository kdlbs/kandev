package backendapp

import (
	"context"

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

// taskChangeRegistrationActorResolver re-resolves the accepted message author
// for every processing attempt. Authenticated processing never substitutes
// the single-user identity when the recorded actor is no longer active.
func taskChangeRegistrationActorResolver(svc *auth.Service) func(context.Context, string) (authn.Identity, bool) {
	return func(ctx context.Context, actorID string) (authn.Identity, bool) {
		if svc == nil {
			return authn.Identity{}, false
		}
		switch svc.Mode() {
		case auth.ModeDisabled:
			return httpmw.SyntheticIdentity(), true
		case auth.ModeEnabled:
			identity, ok := svc.IdentityForUser(ctx, actorID)
			return identity, ok && !identity.Synthetic && identity.UserID == actorID
		default:
			return authn.Identity{}, false
		}
	}
}
