package service

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
)

func authenticatedWorkspaceOwnerUserID(ctx context.Context) string {
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		return identity.UserID
	}
	return ""
}
