package auth

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

// StatePayload is the auth snapshot shared by GET /api/v1/auth/me and the
// boot payload's initialState["auth"]. The frontend auth store slice hydrates
// from exactly this shape.
type StatePayload struct {
	// Mode is "disabled" | "setup" | "enabled".
	Mode string `json:"mode"`
	// Authenticated is true when the request carried a valid session or PAT.
	// Always true in disabled mode (synthetic single-user identity).
	Authenticated bool             `json:"authenticated"`
	User          *usermodels.User `json:"user,omitempty"`
}

// StateFor builds the StatePayload for a request identity (nil identity =
// unauthenticated visitor).
func (s *Service) StateFor(ctx context.Context, identity *authn.Identity) StatePayload {
	payload := StatePayload{
		Mode: string(s.Mode()),
	}
	if identity == nil {
		return payload
	}
	payload.Authenticated = true
	if identity.Synthetic {
		// Disabled mode: report the implicit single user without exposing the
		// placeholder email through the payload.
		payload.User = &usermodels.User{
			ID:     identity.UserID,
			Role:   usermodels.RoleAdmin,
			Status: usermodels.StatusActive,
		}
		return payload
	}
	if user, err := s.users.GetUser(ctx, identity.UserID); err == nil {
		payload.User = user
	}
	return payload
}
