package auth

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/store"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

// Login verifies email+password and mints a browser session. The returned
// string is the raw session token to place in the cookie.
func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (*usermodels.User, string, error) {
	email = normalizeEmail(email)
	limiterKey := ip + "|" + email
	if !s.limiter.Allow(limiterKey) {
		return nil, "", ErrRateLimited
	}
	user, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		// Equalize timing with the real verification path.
		_ = VerifyPassword(password, s.dummyHash)
		return nil, "", ErrInvalidCredentials
	}
	if user.Status != usermodels.StatusActive {
		_ = VerifyPassword(password, s.dummyHash)
		return nil, "", ErrInvalidCredentials
	}
	identity, err := s.store.GetLocalIdentity(ctx, user.ID)
	if err != nil {
		_ = VerifyPassword(password, s.dummyHash)
		return nil, "", ErrInvalidCredentials
	}
	if err := VerifyPassword(password, identity.PasswordHash); err != nil {
		return nil, "", ErrInvalidCredentials
	}
	s.limiter.Reset(limiterKey)
	// A suspended organization refuses login outright rather than handing out
	// a session that fails on the very next request. The message is distinct
	// from ErrInvalidCredentials on purpose: the credentials were correct, and
	// telling the user their password is wrong would send them to reset it.
	if !s.orgUsable(ctx, user.OrgID) {
		return nil, "", ErrOrgUnavailable
	}
	token, err := s.createSession(ctx, user.ID, userAgent, ip)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// Logout deletes the session backing the current cookie.
func (s *Service) Logout(ctx context.Context, sessionID, userID string) error {
	return s.store.DeleteSession(ctx, sessionID, userID)
}

// ChangePassword verifies the current password, stores the new hash, and
// revokes every other session of the user.
func (s *Service) ChangePassword(ctx context.Context, userID, currentSessionID, current, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return ErrValidation
	}
	identity, err := s.store.GetLocalIdentity(ctx, userID)
	if err != nil {
		return ErrInvalidCredentials
	}
	if err := VerifyPassword(current, identity.PasswordHash); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.store.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return err
	}
	return s.store.DeleteSessionsByUser(ctx, userID, currentSessionID)
}

// ResolveSessionToken authenticates a raw session-cookie value. The passed
// ip is the request's resolved client IP: on the throttled touch path it
// refreshes the stored session IP, but only when non-empty and differing
// from the recorded value (an empty ip never overwrites; same-IP requests
// only touch timestamps).
func (s *Service) ResolveSessionToken(ctx context.Context, token, ip string) (authn.Identity, bool) {
	if token == "" {
		return authn.Identity{}, false
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, HashToken(token))
	if err != nil {
		return authn.Identity{}, false
	}
	now := time.Now().UTC()
	if sess.ExpiresAt.Before(now) {
		_ = s.store.DeleteSession(ctx, sess.ID, sess.UserID)
		return authn.Identity{}, false
	}
	user, err := s.activeUser(ctx, sess.UserID)
	if err != nil {
		return authn.Identity{}, false
	}
	if now.Sub(sess.LastSeenAt) > sessionTouchInterval {
		refreshIP := ""
		if ip != "" && ip != sess.IP {
			refreshIP = ip
		}
		_ = s.store.TouchSession(ctx, sess.ID, now, now.Add(s.SessionTTL()), refreshIP)
	}
	if !s.orgUsable(ctx, user.OrgID) {
		return authn.Identity{}, false
	}
	return identityOf(user, sess.ID, ""), true
}

// ResolveBearer authenticates an Authorization bearer credential when it is a
// kandev personal access token. Non-PAT bearers (e.g. office agent JWTs)
// return false so the caller can defer to their own validators.
func (s *Service) ResolveBearer(ctx context.Context, token string) (authn.Identity, bool) {
	if !IsPATFormat(token) {
		return authn.Identity{}, false
	}
	record, err := s.store.GetAPITokenByHash(ctx, HashToken(token))
	if err != nil {
		return authn.Identity{}, false
	}
	now := time.Now().UTC()
	if record.ExpiresAt != nil && record.ExpiresAt.Before(now) {
		return authn.Identity{}, false
	}
	user, err := s.activeUser(ctx, record.UserID)
	if err != nil {
		return authn.Identity{}, false
	}
	if record.LastUsedAt == nil || now.Sub(*record.LastUsedAt) > sessionTouchInterval {
		_ = s.store.TouchAPIToken(ctx, record.ID, now)
	}
	if !s.orgUsable(ctx, user.OrgID) {
		return authn.Identity{}, false
	}
	return identityOf(user, "", record.ID), true
}

// IdentityForUser resolves a stored user ID to the identity that user carries
// on an authenticated request, without needing one of their credentials.
//
// It exists for callers that already know *whose* work they are performing but
// have no request to authenticate: in-session agent MCP calls arrive over the
// agent's own WebSocket stream, so the owning user is derived from the stream's
// task rather than from a cookie or PAT (see internal/mcp/scope). The returned
// identity is real — never synthetic — so per-user scoping applies to it.
//
// Returns false while authentication is disabled (callers keep the pre-auth
// unscoped behavior) and when the account is missing or no longer active.
func (s *Service) IdentityForUser(ctx context.Context, userID string) (authn.Identity, bool) {
	if s == nil || userID == "" || s.Mode() == ModeDisabled {
		return authn.Identity{}, false
	}
	user, err := s.activeUser(ctx, userID)
	if err != nil {
		return authn.Identity{}, false
	}
	if !s.orgUsable(ctx, user.OrgID) {
		return authn.Identity{}, false
	}
	return identityOf(user, "", ""), true
}

// ListSessions returns the user's sessions for the account page.
func (s *Service) ListSessions(ctx context.Context, userID string) ([]*store.Session, error) {
	return s.store.ListSessionsByUser(ctx, userID)
}

// RevokeSession deletes one of the user's sessions.
func (s *Service) RevokeSession(ctx context.Context, sessionID, userID string) error {
	return s.store.DeleteSession(ctx, sessionID, userID)
}

func (s *Service) activeUser(ctx context.Context, userID string) (*usermodels.User, error) {
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != usermodels.StatusActive {
		return nil, errors.New("user disabled")
	}
	return user, nil
}

func (s *Service) createSession(ctx context.Context, userID, userAgent, ip string) (string, error) {
	token, hash, err := GenerateSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	session := &store.Session{
		UserID:     userID,
		TokenHash:  hash,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.SessionTTL()),
		LastSeenAt: now,
		UserAgent:  truncateUserAgent(userAgent),
		IP:         ip,
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return "", err
	}
	return token, nil
}
