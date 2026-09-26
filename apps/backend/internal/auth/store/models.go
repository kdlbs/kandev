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
// json tags drive the API wire shape (these structs are serialized directly
// by the auth handlers); credential digests are marked `json:"-"` so they are
// never sent to clients.
type LoginIdentity struct {
	ID           string    `db:"id" json:"id"`
	UserID       string    `db:"user_id" json:"user_id"`
	Provider     string    `db:"provider" json:"provider"`
	Subject      string    `db:"subject" json:"subject"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// Session is a browser session backed by the session cookie (base name
// kandev_session; effective name derived from the request host).
type Session struct {
	ID         string    `db:"id" json:"id"`
	UserID     string    `db:"user_id" json:"user_id"`
	TokenHash  string    `db:"token_sha256" json:"-"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	ExpiresAt  time.Time `db:"expires_at" json:"expires_at"`
	LastSeenAt time.Time `db:"last_seen_at" json:"last_seen_at"`
	UserAgent  string    `db:"user_agent" json:"user_agent"`
	IP         string    `db:"ip" json:"ip"`
}

// APIToken is a personal access token for programmatic clients.
type APIToken struct {
	ID         string     `db:"id" json:"id"`
	UserID     string     `db:"user_id" json:"user_id"`
	Name       string     `db:"name" json:"name"`
	Prefix     string     `db:"prefix" json:"prefix"`
	TokenHash  string     `db:"token_sha256" json:"-"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	LastUsedAt *time.Time `db:"last_used_at" json:"last_used_at"`
	ExpiresAt  *time.Time `db:"expires_at" json:"expires_at"`
}

// Invite is a tokenized invitation URL minted by an admin.
type Invite struct {
	ID        string `db:"id" json:"id"`
	TokenHash string `db:"token_sha256" json:"-"`
	Email     string `db:"email" json:"email"`
	Role      string `db:"role" json:"role"`
	// OrgID is the organization the accepted account will join.
	OrgID     string     `db:"org_id" json:"org_id,omitempty"`
	CreatedBy string     `db:"created_by" json:"created_by"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	ExpiresAt time.Time  `db:"expires_at" json:"expires_at"`
	UsedBy    *string    `db:"used_by" json:"used_by"`
	UsedAt    *time.Time `db:"used_at" json:"used_at"`
}
