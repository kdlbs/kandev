package github

import (
	"errors"
	"time"
)

const DefaultUserID = "default-user"

// CredentialPurpose makes caller intent explicit so background automation
// cannot accidentally receive a user's personal credential.
type CredentialPurpose string

const (
	CredentialPurposeAutomation    CredentialPurpose = "automation"
	CredentialPurposePersonalRead  CredentialPurpose = "personal_read"
	CredentialPurposePersonalWrite CredentialPurpose = "personal_write"
	CredentialPurposeGitTransport  CredentialPurpose = "git_transport"
)

type AuthPrincipalKind string

const (
	AuthPrincipalHuman AuthPrincipalKind = "human"
	AuthPrincipalApp   AuthPrincipalKind = "app"
)

// AuthPrincipal describes the identity GitHub will attribute an operation to.
// It deliberately contains no credential material.
type AuthPrincipal struct {
	Kind           AuthPrincipalKind `json:"kind"`
	Source         ConnectionSource  `json:"source"`
	Login          string            `json:"login,omitempty"`
	InstallationID int64             `json:"installation_id,omitempty"`
	WorkspaceID    string            `json:"workspace_id"`
	UserID         string            `json:"user_id,omitempty"`
}

type ResolveCredentialRequest struct {
	WorkspaceID string
	UserID      string
	Purpose     CredentialPurpose
	RepoOwner   string
	RepoName    string
}

type ResolvedCredential struct {
	Client               Client
	Principal            AuthPrincipal
	Capabilities         map[GitHubAppCapability]bool
	CredentialGeneration int64
	ExpiresAt            time.Time
	RateTracker          *RateTracker
	credential           string
}

var (
	ErrGitHubWorkspaceRequired = errors.New("GitHub workspace is required")
	ErrGitHubNotConfigured     = errors.New("GitHub is not configured for this workspace")
	ErrGitHubConnectionInvalid = errors.New("GitHub connection is not active")
	ErrGitHubPersonalRequired  = errors.New("personal GitHub connection is required")
	ErrGitHubCapabilityDenied  = errors.New("GitHub connection lacks a required capability")
)
