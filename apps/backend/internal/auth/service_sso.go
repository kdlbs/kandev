package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/store"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

// Sentinel errors for the external (OIDC/SAML) login path.
var (
	// ErrSSONotEnabled is returned when an external login is attempted while
	// authentication is not fully enabled. SSO logs in against real accounts,
	// so it only operates in ModeEnabled — never disabled or setup.
	ErrSSONotEnabled = errors.New("sso login requires authentication to be enabled")
	// ErrSSOInvalidAssertion is returned for a malformed external identity:
	// an empty/local provider, an empty subject, or a new identity with no
	// email to link or provision against.
	ErrSSOInvalidAssertion = errors.New("invalid external identity assertion")
	// ErrSSOAdminLinkForbidden is returned when an external login would
	// auto-link to an existing admin account. External login must never
	// authenticate an admin it didn't already have a deliberate identity link
	// for — otherwise a plugin/IdP asserting an admin's (possibly unverified)
	// email would be an admin account takeover.
	ErrSSOAdminLinkForbidden = errors.New("external login cannot link to an admin account")
)

// ExternalIdentity is a validated identity assertion from an auth-capable
// plugin that has authenticated a user against an external IdP (OIDC/SAML).
// The plugin is trusted to have verified the token/signature and mapped IdP
// claims; kandev maps the assertion to a user and mints the session. Provider
// is a non-"local" provider key (e.g. "oidc:https://issuer.example"); Subject
// is the stable IdP subject claim. Email/DisplayName come from the IdP.
//
// SECURITY CONTRACT: Email is used to auto-link to (or provision) an account,
// so the plugin MUST only assert an email it has confirmed the IdP verified as
// owned by Subject. Asserting an unverified/user-settable email is an account
// takeover vector. As defense-in-depth kandev additionally refuses to auto-link
// to an admin account (ErrSSOAdminLinkForbidden), but it cannot itself tell a
// verified email from an unverified one — that guarantee is the plugin's.
type ExternalIdentity struct {
	Provider    string
	Subject     string
	Email       string
	DisplayName string
}

// AuthenticateExternal establishes a browser session for an external identity
// asserted by an auth-capable plugin. Resolution order:
//
//  1. an existing (provider, subject) identity → its user;
//  2. else an active user with the asserted email → link a new identity to it
//     (just-in-time account linking);
//  3. else just-in-time provision a new member user + identity.
//
// New users are always provisioned as members; external login never creates or
// elevates an admin (role changes stay with admin user management). A disabled
// account is rejected. The returned string is the raw session token to place
// in the kandev_session cookie — the caller (the plugin webhook relay) sets the
// cookie; the plugin never receives this token.
func (s *Service) AuthenticateExternal(ctx context.Context, ext ExternalIdentity, userAgent, ip string) (*usermodels.User, string, error) {
	if s.Mode() != ModeEnabled {
		return nil, "", ErrSSONotEnabled
	}
	provider := strings.TrimSpace(ext.Provider)
	subject := strings.TrimSpace(ext.Subject)
	if provider == "" || provider == store.ProviderLocal || subject == "" {
		return nil, "", ErrSSOInvalidAssertion
	}
	email := normalizeEmail(ext.Email)

	user, err := s.resolveExternalUser(ctx, provider, subject, email, ext.DisplayName)
	if err != nil {
		return nil, "", err
	}
	token, err := s.createSession(ctx, user.ID, userAgent, ip)
	if err != nil {
		return nil, "", err
	}
	if s.log != nil {
		s.log.Info("external identity authenticated",
			zap.String("provider", provider),
			zap.String("user_id", user.ID))
	}
	return user, token, nil
}

// resolveExternalUser maps a validated external identity to a kandev user,
// linking or provisioning as needed. See AuthenticateExternal for the order.
func (s *Service) resolveExternalUser(ctx context.Context, provider, subject, email, displayName string) (*usermodels.User, error) {
	identity, err := s.store.GetIdentityByProviderSubject(ctx, provider, subject)
	switch {
	case err == nil:
		// Known external identity: log in as its (still-active) user.
		return s.activeUser(ctx, identity.UserID)
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}

	// Unknown identity. Link it to an existing active account by email when we
	// can, otherwise just-in-time provision a new member account. A real store
	// error (transient DB failure, context cancellation) must abort rather than
	// be masked as "no such user" and fall through to provisioning — only a
	// genuine no-rows miss (sql.ErrNoRows, what user store's scanUser returns)
	// means the email is unused.
	if email == "" {
		return nil, ErrSSOInvalidAssertion
	}
	user, err := s.users.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		if user.Status != usermodels.StatusActive {
			return nil, ErrInvalidCredentials
		}
		// Defense-in-depth against takeover via an unverified/spoofed email
		// claim: never auto-link to an admin. New users provision as members,
		// so external login can only ever authenticate an admin through a link
		// established deliberately while they were a member (then promoted) —
		// never one minted from an email assertion here.
		if user.Role == usermodels.RoleAdmin {
			return nil, ErrSSOAdminLinkForbidden
		}
		if err := s.linkExternalIdentity(ctx, user.ID, provider, subject); err != nil {
			return nil, err
		}
		return user, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	return s.provisionExternalUser(ctx, provider, subject, email, displayName)
}

// linkExternalIdentity records a new external identity row for an existing
// user (JIT account linking). The (provider, subject) uniqueness guarantees a
// given IdP subject links to at most one user.
func (s *Service) linkExternalIdentity(ctx context.Context, userID, provider, subject string) error {
	return s.store.CreateIdentity(ctx, &store.LoginIdentity{
		UserID:   userID,
		Provider: provider,
		Subject:  subject,
	})
}

// provisionExternalUser creates a new member account and its external identity
// for a first-time SSO visitor. New accounts are always members.
//
// If the identity insert fails — a transient error, or a lost race on
// UNIQUE(provider, subject) when two callbacks for the same subject arrive
// concurrently — the just-created user is rolled back so no account is left
// without a login identity (and its email is not permanently reserved by an
// unusable row). On a lost race the winning identity now exists, so we
// authenticate as its user instead of erroring. (These two stores don't share a
// transaction; compensating cleanup is the pragmatic equivalent here.)
func (s *Service) provisionExternalUser(ctx context.Context, provider, subject, email, displayName string) (*usermodels.User, error) {
	user := &usermodels.User{
		ID:          uuid.New().String(),
		Email:       email,
		DisplayName: displayName,
		Role:        usermodels.RoleMember,
		Status:      usermodels.StatusActive,
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	if err := s.linkExternalIdentity(ctx, user.ID, provider, subject); err != nil {
		_ = s.users.DeleteUser(ctx, user.ID)
		if winner, lookupErr := s.store.GetIdentityByProviderSubject(ctx, provider, subject); lookupErr == nil {
			return s.activeUser(ctx, winner.UserID)
		}
		return nil, err
	}
	return user, nil
}
