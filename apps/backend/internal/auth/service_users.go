package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/auth/store"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

// ListUsers returns all users for the admin UI.
func (s *Service) ListUsers(ctx context.Context) ([]*usermodels.User, error) {
	return s.users.ListUsers(ctx)
}

// AdminCreateUser creates a user with a password directly (no invite).
func (s *Service) AdminCreateUser(ctx context.Context, email, password, displayName, role string) (*usermodels.User, error) {
	email = normalizeEmail(email)
	if err := validateEmailPassword(email, password); err != nil {
		return nil, err
	}
	role = usermodels.NormalizeRole(role)
	if _, err := s.users.GetUserByEmail(ctx, email); err == nil {
		return nil, ErrEmailTaken
	}
	user := &usermodels.User{
		ID:          uuid.New().String(),
		Email:       email,
		DisplayName: displayName,
		Role:        role,
		Status:      usermodels.StatusActive,
		// A new account joins the creating admin's organization. There is no
		// org parameter on this path on purpose: an admin can only ever create
		// users in their own tenant, so the org comes from the caller's
		// identity rather than from the request.
		OrgID: callerOrgID(ctx),
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, ErrEmailTaken
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	identity := &store.LoginIdentity{
		UserID:       user.ID,
		Provider:     store.ProviderLocal,
		Subject:      email,
		PasswordHash: hash,
	}
	if err := s.store.CreateIdentity(ctx, identity); err != nil {
		return nil, err
	}
	return user, nil
}

// AdminSetRoleStatus updates a user's role and/or status with a last-admin
// guard. Disabling a user revokes all of their sessions and API tokens.
func (s *Service) AdminSetRoleStatus(ctx context.Context, targetID, role, status string) (*usermodels.User, error) {
	target, err := s.users.GetUser(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		role = target.Role
	}
	if status == "" {
		status = target.Status
	}
	role = usermodels.NormalizeRole(role)
	if status != usermodels.StatusDisabled {
		status = usermodels.StatusActive
	}
	losesAdmin := target.Role == usermodels.RoleAdmin && target.Status == usermodels.StatusActive &&
		(role != usermodels.RoleAdmin || status != usermodels.StatusActive)
	if losesAdmin {
		if err := s.ensureAnotherAdmin(ctx, targetID); err != nil {
			return nil, err
		}
	}
	updated, err := s.users.UpdateUserRoleStatus(ctx, targetID, role, status)
	if err != nil {
		return nil, err
	}
	if status == usermodels.StatusDisabled {
		if err := s.store.DeleteSessionsByUser(ctx, targetID, ""); err != nil {
			return nil, err
		}
		if err := s.store.DeleteAPITokensByUser(ctx, targetID); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

// MintToken creates a personal access token. Returns the raw token once.
func (s *Service) MintToken(ctx context.Context, userID, name string, ttlHours int) (*store.APIToken, string, error) {
	if name == "" {
		return nil, "", ErrValidation
	}
	token, prefix, hash, err := GeneratePAT()
	if err != nil {
		return nil, "", err
	}
	record := &store.APIToken{
		UserID:    userID,
		Name:      name,
		Prefix:    prefix,
		TokenHash: hash,
	}
	if ttlHours > 0 {
		expires := time.Now().UTC().Add(time.Duration(ttlHours) * time.Hour)
		record.ExpiresAt = &expires
	}
	if err := s.store.CreateAPIToken(ctx, record); err != nil {
		return nil, "", err
	}
	return record, token, nil
}

// ListTokens returns the user's PATs.
func (s *Service) ListTokens(ctx context.Context, userID string) ([]*store.APIToken, error) {
	return s.store.ListAPITokensByUser(ctx, userID)
}

// RevokeToken deletes one of the user's PATs.
func (s *Service) RevokeToken(ctx context.Context, tokenID, userID string) error {
	return s.store.DeleteAPIToken(ctx, tokenID, userID)
}

func (s *Service) ensureAnotherAdmin(ctx context.Context, excludingID string) error {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if user.ID == excludingID {
			continue
		}
		if user.Role == usermodels.RoleAdmin && user.Status == usermodels.StatusActive {
			return nil
		}
	}
	return ErrLastAdmin
}

// CreateUserInOrg provisions an account in a named organization.
//
// It exists for exactly one caller: the operator creating an organization's
// first administrator. Every other creation path takes the org from the
// requesting admin's identity, because an admin may only ever create accounts
// in their own tenant.
func (s *Service) CreateUserInOrg(
	ctx context.Context, orgID, email, password, displayName string,
) error {
	email = normalizeEmail(email)
	if err := validateEmailPassword(email, password); err != nil {
		return err
	}
	if _, err := s.users.GetUserByEmail(ctx, email); err == nil {
		return ErrEmailTaken
	}
	user := &usermodels.User{
		ID:          uuid.New().String(),
		Email:       email,
		DisplayName: displayName,
		Role:        usermodels.RoleAdmin,
		Status:      usermodels.StatusActive,
		OrgID:       orgID,
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return ErrEmailTaken
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.store.CreateIdentity(ctx, &store.LoginIdentity{
		UserID:       user.ID,
		Provider:     store.ProviderLocal,
		Subject:      email,
		PasswordHash: hash,
	})
}
