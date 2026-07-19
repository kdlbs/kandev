package github

import (
	"fmt"
	"net/http"
	"time"
)

// TokenCredentialKind identifies the authority represented by a token. It is
// intentionally separate from the transport so callers retain attribution and
// routing information after a REST/GraphQL client is constructed.
type TokenCredentialKind string

const (
	TokenCredentialPAT          TokenCredentialKind = "pat"
	TokenCredentialCLI          TokenCredentialKind = "gh_cli"
	TokenCredentialInstallation TokenCredentialKind = "github_app_installation"
	TokenCredentialUser         TokenCredentialKind = "github_app_user"
)

// TokenPrincipal describes the GitHub actor behind a bearer token.
type TokenPrincipal struct {
	Kind           TokenCredentialKind `json:"kind"`
	PrincipalID    string              `json:"principal_id"`
	Login          string              `json:"login,omitempty"`
	InstallationID int64               `json:"installation_id,omitempty"`
}

// TokenClient implements GitHub REST and GraphQL operations for any bearer
// token source. The token itself remains private while principal metadata is
// available for authorization, rate-limit, cache, and attribution keys.
type TokenClient struct {
	token       string
	httpClient  *http.Client
	username    string // cached after first GetAuthenticatedUser call
	rateTracker *RateTracker
	principal   TokenPrincipal
}

// PATClient remains an alias so existing callers retain source compatibility.
type PATClient = TokenClient

// NewTokenClient constructs a GitHub client for a token and explicit actor.
func NewTokenClient(token string, principal TokenPrincipal) *TokenClient {
	return &TokenClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		principal: principal,
	}
}

// NewPATClient creates a PAT-based GitHub client.
func NewPATClient(token string) *PATClient {
	return NewTokenClient(token, TokenPrincipal{Kind: TokenCredentialPAT})
}

// NewAppUserTokenClient creates a client for a GitHub App user access token.
func NewAppUserTokenClient(token string, githubUserID int64, login string) *TokenClient {
	return NewTokenClient(token, TokenPrincipal{
		Kind:        TokenCredentialUser,
		PrincipalID: fmt.Sprintf("user:%d", githubUserID),
		Login:       login,
	})
}

// Principal returns the actor metadata attached at construction.
func (c *TokenClient) Principal() TokenPrincipal {
	return c.principal
}
