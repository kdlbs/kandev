package sandbox

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/scope"
)

func (s *Service) operator(ctx context.Context) (authn.Identity, error) {
	if !s.enabled {
		return authn.Identity{}, ErrDisabled
	}
	id, ok := authn.IdentityFromContext(ctx)
	_, agent := scope.PrincipalFromContext(ctx)
	if !ok || id.Synthetic || id.Instance || id.UserID == "" || id.SessionID == "" || id.TokenID != "" || agent || origin.IsTrustedExternalTransport(ctx) || origin.IsTrustedInternalCall(ctx) {
		return authn.Identity{}, ErrAuthority
	}
	if !authz.OrgScopes(authz.NormalizeOrgRole(string(id.Role))).Has(authz.ScopeOrgConfigManage) {
		return authn.Identity{}, ErrAuthority
	}
	return id, nil
}

// SetWorkspaceAuthorizer binds the existing workspace authorization service at startup.
func (s *Service) SetWorkspaceAuthorizer(authorize func(context.Context, string) error) {
	s.authorizeWorkspace = authorize
}

func (s *Service) SetSessionAuthorizer(authorize func(context.Context, string) error) {
	s.authorizeSession = authorize
}

func (s *Service) sessionAccess(ctx context.Context, sessionID string) error {
	if sessionID == "" || s.authorizeSession == nil || s.authorizeSession(ctx, sessionID) != nil {
		return ErrNotFound
	}
	return nil
}

func (s *Service) workspaceAccess(ctx context.Context, workspaceID string) error {
	if workspaceID == "" {
		return nil
	}
	if s.authorizeWorkspace == nil {
		return ErrAuthority
	}
	return s.authorizeWorkspace(ctx, workspaceID)
}
