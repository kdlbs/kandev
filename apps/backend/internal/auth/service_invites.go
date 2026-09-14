package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/auth/store"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

const (
	defaultInviteTTL = 7 * 24 * time.Hour
	maxInviteTTL     = 90 * 24 * time.Hour
)

// CreateInvite mints an invite link token. Email is optional; when set, the
// accept step must use the same address. Returns the raw token exactly once.
func (s *Service) CreateInvite(ctx context.Context, createdBy, email, role string, ttlHours int) (*store.Invite, string, error) {
	role = usermodels.NormalizeRole(role)
	ttl := defaultInviteTTL
	if ttlHours > 0 {
		ttl = time.Duration(ttlHours) * time.Hour
		if ttl > maxInviteTTL {
			ttl = maxInviteTTL
		}
	}
	token, hash, err := GenerateSessionToken()
	if err != nil {
		return nil, "", err
	}
	invite := &store.Invite{
		TokenHash: hash,
		Email:     normalizeEmail(email),
		Role:      role,
		// The invite joins the minting admin's organization, so accepting it
		// can never place an account in a tenant the inviter cannot see.
		OrgID:     callerOrgID(ctx),
		CreatedBy: createdBy,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	if err := s.store.CreateInvite(ctx, invite); err != nil {
		return nil, "", err
	}
	return invite, token, nil
}

// AcceptInvite consumes a valid invite, creates the user + local identity,
// and mints their first session.
func (s *Service) AcceptInvite(ctx context.Context, token, email, password, displayName, userAgent, ip string) (*usermodels.User, string, error) {
	if s.Mode() != ModeEnabled {
		return nil, "", ErrInviteInvalid
	}
	email = normalizeEmail(email)
	if err := validateEmailPassword(email, password); err != nil {
		return nil, "", err
	}
	invite, err := s.validInvite(ctx, token, email)
	if err != nil {
		return nil, "", err
	}
	if _, err := s.users.GetUserByEmail(ctx, email); err == nil {
		return nil, "", ErrEmailTaken
	}
	// Consume the invite FIRST — MarkInviteUsed is atomic (WHERE used_by IS
	// NULL). This is the serialization point: two concurrent accepts of the
	// same (empty-email) invite cannot both proceed to create a user, closing
	// the double-provisioning race.
	userID := uuid.New().String()
	if err := s.store.MarkInviteUsed(ctx, invite.ID, userID, time.Now().UTC()); err != nil {
		return nil, "", ErrInviteInvalid
	}
	user := &usermodels.User{
		ID:          userID,
		Email:       email,
		DisplayName: displayName,
		Role:        invite.Role,
		Status:      usermodels.StatusActive,
		// The organization comes from the invite, which recorded the minting
		// admin's org. Accepting an invite is an anonymous request, so this is
		// the only trustworthy source for it.
		OrgID: invite.OrgID,
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, "", ErrEmailTaken
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, "", err
	}
	identity := &store.LoginIdentity{
		UserID:       user.ID,
		Provider:     store.ProviderLocal,
		Subject:      email,
		PasswordHash: hash,
	}
	if err := s.store.CreateIdentity(ctx, identity); err != nil {
		return nil, "", err
	}
	sessionToken, err := s.createSession(ctx, user.ID, userAgent, ip)
	if err != nil {
		return nil, "", err
	}
	return user, sessionToken, nil
}

// InspectInvite validates a raw invite token for the accept page (prefill).
func (s *Service) InspectInvite(ctx context.Context, token string) (*store.Invite, error) {
	return s.validInvite(ctx, token, "")
}

// ListInvites returns all invites for the admin UI.
func (s *Service) ListInvites(ctx context.Context) ([]*store.Invite, error) {
	return s.store.ListInvites(ctx)
}

// DeleteInvite revokes an invite link.
func (s *Service) DeleteInvite(ctx context.Context, id string) error {
	return s.store.DeleteInvite(ctx, id)
}

func (s *Service) validInvite(ctx context.Context, token, email string) (*store.Invite, error) {
	if token == "" {
		return nil, ErrInviteInvalid
	}
	invite, err := s.store.GetInviteByTokenHash(ctx, HashToken(token))
	if err != nil {
		return nil, ErrInviteInvalid
	}
	if invite.UsedBy != nil || invite.ExpiresAt.Before(time.Now().UTC()) {
		return nil, ErrInviteInvalid
	}
	if email != "" && invite.Email != "" && invite.Email != email {
		return nil, ErrInviteInvalid
	}
	return invite, nil
}
