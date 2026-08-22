package auth

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/store"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// Setup completes the first-run wizard: it promotes the pre-auth default-user
// row into the admin account (preserving all existing user settings), claims
// unowned workspaces/secrets for that admin, persists the enabled toggle, and
// mints the first session.
//
// Ordering is deliberate: backfills run first (harmless if repeated), the
// local identity insert is the commit point that flips ModeSetup → ModeEnabled,
// and a partial failure before it leaves the instance safely in setup mode.
func (s *Service) Setup(ctx context.Context, email, password, displayName, userAgent, ip string) (*usermodels.User, string, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.Mode() != ModeSetup {
		return nil, "", ErrSetupNotAvailable
	}
	email = normalizeEmail(email)
	if err := validateEmailPassword(email, password); err != nil {
		return nil, "", err
	}
	adminID := userstore.DefaultUserID
	for _, backfill := range s.backfills {
		if err := backfill(ctx, adminID); err != nil {
			return nil, "", err
		}
	}
	user, err := s.users.UpdateUserProfile(ctx, adminID, email, displayName, usermodels.RoleAdmin)
	if err != nil {
		return nil, "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, "", err
	}
	identity := &store.LoginIdentity{
		UserID:       adminID,
		Provider:     store.ProviderLocal,
		Subject:      email,
		PasswordHash: hash,
	}
	if err := s.store.CreateIdentity(ctx, identity); err != nil {
		// A concurrent setup already created the admin identity.
		return nil, "", ErrSetupNotAvailable
	}
	// An admin identity now exists, so refreshMode derives Enabled from the
	// (still-on) features.auth flag — no separate toggle to persist.
	if err := s.refreshMode(ctx); err != nil {
		return nil, "", err
	}
	token, err := s.createSession(ctx, adminID, userAgent, ip)
	if err != nil {
		return nil, "", err
	}
	if s.log != nil {
		s.log.Info("authentication setup completed", zap.String("admin_email", email))
	}
	// The setup wizard runs after the boot-time tenancy migration, so the
	// admin it creates has no organization and no operator tier yet. Without
	// this an instance that enables both features at once would come up with
	// organizations and nobody able to manage them.
	if s.adminCreated != nil {
		if err := s.adminCreated(ctx, adminID); err != nil && s.log != nil {
			s.log.Warn("failed to place the new admin in an organization", zap.Error(err))
		}
	}
	return user, token, nil
}
