package identity

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// CreatorUserID preserves synthetic and explicitly bound actor precedence.
func CreatorUserID(ctx context.Context) string {
	identity, known := authn.IdentityFromContext(ctx)
	if known && identity.Synthetic {
		return ""
	}
	if actor, bound := taskmodels.ExecutorActorFromContext(ctx); bound {
		return actor
	}
	if known {
		return identity.UserID
	}
	return ""
}
