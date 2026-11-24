package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/plugins"
	userservice "github.com/kandev/kandev/internal/user/service"
)

// pluginLinkedUserResolver resolves the Kandev user behind a plugin identity
// link: the identity that user acts with and the display name shown in
// attribution. An inactive user, or a user of an unusable organization,
// resolves to false. While authentication is disabled only the single local
// user exists.
func pluginLinkedUserResolver(authSvc *auth.Service, users *userservice.Service) func(context.Context, string) (plugins.LinkedHuman, bool) {
	return func(ctx context.Context, userID string) (plugins.LinkedHuman, bool) {
		var identity authn.Identity
		if authSvc == nil || authSvc.Mode() == auth.ModeDisabled {
			identity = authhttpmw.SyntheticIdentity()
			if userID != identity.UserID {
				return plugins.LinkedHuman{}, false
			}
		} else {
			resolved, ok := authSvc.IdentityForUser(ctx, userID)
			if !ok {
				return plugins.LinkedHuman{}, false
			}
			identity = resolved
		}
		human := plugins.LinkedHuman{Identity: identity}
		if users != nil {
			if user, err := users.GetCurrentUser(authn.WithIdentity(ctx, identity)); err == nil && user != nil {
				human.DisplayName = user.DisplayName
			}
		}
		return human, true
	}
}
