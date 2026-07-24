// Package store persists authentication state: login identities (OIDC-ready),
// browser sessions, personal access tokens, and invites. Only SHA-256 digests
// of credentials are stored — never the secrets themselves.
package store

import "time"

// ProviderLocal is the built-in email+password identity provider. Additional
// providers (OIDC issuers) reuse the same table with their issuer URL as the
// provider and the subject claim as Subject.
const ProviderLocal = "local"

// LoginIdentity links a user to a credential provider.
type LoginIdentity struct {
	ID           string    `db:"id"`
	UserID       string    `db:"user_id"`
	Provider     string    `db:"provider"`
	Subject      string    `db:"subject"`
	PasswordHash string    `db:"password_hash"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// Session is a browser session backed by the kandev_session cookie.
type Session struct {
	ID         string    `db:"id"`
	UserID     string    `db:"user_id"`
	TokenHash  string    `db:"token_sha256"`
	CreatedAt  time.Time `db:"created_at"`
	ExpiresAt  time.Time `db:"expires_at"`
	LastSeenAt time.Time `db:"last_seen_at"`
	UserAgent  string    `db:"user_agent"`
	IP         string    `db:"ip"`
}

// APIToken is a personal access token for programmatic clients.
type APIToken struct {
	ID         string     `db:"id"`
	UserID     string     `db:"user_id"`
	Name       string     `db:"name"`
	Prefix     string     `db:"prefix"`
	TokenHash  string     `db:"token_sha256"`
	CreatedAt  time.Time  `db:"created_at"`
	LastUsedAt *time.Time `db:"last_used_at"`
	ExpiresAt  *time.Time `db:"expires_at"`
}

// Invite is a tokenized invitation URL minted by an admin.
type Invite struct {
	ID        string     `db:"id"`
	TokenHash string     `db:"token_sha256"`
	Email     string     `db:"email"`
	Role      string     `db:"role"`
	CreatedBy string     `db:"created_by"`
	CreatedAt time.Time  `db:"created_at"`
	ExpiresAt time.Time  `db:"expires_at"`
	UsedBy    *string    `db:"used_by"`
	UsedAt    *time.Time `db:"used_at"`
}
