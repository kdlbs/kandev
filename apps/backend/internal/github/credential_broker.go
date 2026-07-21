package github

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultCredentialLeaseTTL       = 12 * time.Hour
	credentialLeaseBytes            = 32
	maxCredentialLeasesPerWorkspace = 10_000
	gitHubTokenUsername             = "x-access-token"
)

var (
	ErrCredentialLeaseInvalid = errors.New("GitHub credential lease is invalid")
	ErrCredentialLeaseExpired = errors.New("GitHub credential lease is expired")
	ErrCredentialLeaseRevoked = errors.New("GitHub credential lease was revoked")
	ErrCredentialLeaseLimit   = errors.New("GitHub credential lease limit reached")
	ErrCredentialScopeDenied  = errors.New("GitHub credential scope denied")
)

// BrokerScopeAuthorizer verifies task/workspace/repository ownership. It is
// called both when a lease is issued and each time it is redeemed.
type BrokerScopeAuthorizer interface {
	AuthorizeGitHubRepository(
		ctx context.Context,
		workspaceID, taskID, sessionID, repositoryID, owner, repo string,
	) error
}

type CredentialLeaseRequest struct {
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
	Owner        string
	Repo         string
	Host         string
	TTL          time.Duration
}

type CredentialLease struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BrokerCredentialRequest struct {
	Lease        string
	TaskID       string
	SessionID    string
	RepositoryID string
	Owner        string
	Repo         string
	Host         string
}

type BrokerCredential struct {
	Username  string        `json:"username"`
	Password  string        `json:"password"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`
	Principal AuthPrincipal `json:"principal"`
}

type credentialLeaseRecord struct {
	WorkspaceID             string
	TaskID                  string
	SessionID               string
	RepositoryID            string
	Owner                   string
	Repo                    string
	Host                    string
	CredentialGeneration    int64
	AppRegistrationID       string
	AppCredentialGeneration int64
	TTL                     time.Duration
	ExpiresAt               time.Time
}

// CredentialBroker exchanges opaque, task-scoped leases for renewable
// automation credentials. Only lease hashes and non-secret scope metadata are
// retained; raw leases and GitHub tokens are never persisted in the broker.
type CredentialBroker struct {
	connections workspaceConnectionReader
	resolver    *CredentialResolver
	authorizer  BrokerScopeAuthorizer

	mu     sync.Mutex
	leases map[[sha256.Size]byte]credentialLeaseRecord
	now    func() time.Time
}

func NewCredentialBroker(
	connections workspaceConnectionReader,
	resolver *CredentialResolver,
	authorizer BrokerScopeAuthorizer,
) *CredentialBroker {
	return &CredentialBroker{
		connections: connections,
		resolver:    resolver,
		authorizer:  authorizer,
		leases:      make(map[[sha256.Size]byte]credentialLeaseRecord),
		now:         time.Now,
	}
}

func (b *CredentialBroker) Issue(ctx context.Context, req CredentialLeaseRequest) (*CredentialLease, error) {
	if err := validateCredentialLeaseScope(
		req.WorkspaceID, req.TaskID, req.SessionID, req.RepositoryID, req.Owner, req.Repo, req.Host,
	); err != nil {
		return nil, err
	}
	if b == nil || b.connections == nil || b.resolver == nil || b.authorizer == nil {
		return nil, ErrGitHubNotConfigured
	}
	if err := b.authorizer.AuthorizeGitHubRepository(
		ctx, req.WorkspaceID, req.TaskID, req.SessionID, req.RepositoryID, req.Owner, req.Repo,
	); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCredentialScopeDenied, err)
	}
	connection, err := b.connections.GetWorkspaceConnection(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load GitHub workspace connection: %w", err)
	}
	if connection == nil {
		return nil, ErrGitHubNotConfigured
	}
	if connection.Status != ConnectionStatusActive {
		return nil, ErrGitHubConnectionInvalid
	}
	appCredentialGeneration := int64(0)
	if connection.Source == ConnectionSourceGitHubAppInstallation {
		if strings.TrimSpace(connection.AppRegistrationID) == "" {
			return nil, ErrGitHubNotConfigured
		}
		appCredentialGeneration, err = b.resolver.appCredentialGeneration(connection.AppRegistrationID)
		if err != nil {
			return nil, err
		}
	}
	ttl := req.TTL
	if ttl <= 0 || ttl > defaultCredentialLeaseTTL {
		ttl = defaultCredentialLeaseTTL
	}
	raw := make([]byte, credentialLeaseBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("create GitHub credential lease: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	now := b.now()
	expiresAt := now.UTC().Add(ttl)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.sweepExpiredLocked(now)
	if len(b.leases) >= maxCredentialLeasesPerWorkspace &&
		b.workspaceLeaseCountLocked(req.WorkspaceID) >= maxCredentialLeasesPerWorkspace {
		return nil, fmt.Errorf("%w for workspace %s", ErrCredentialLeaseLimit, req.WorkspaceID)
	}
	b.leases[hash] = credentialLeaseRecord{
		WorkspaceID:             req.WorkspaceID,
		TaskID:                  req.TaskID,
		SessionID:               req.SessionID,
		RepositoryID:            req.RepositoryID,
		Owner:                   strings.ToLower(req.Owner),
		Repo:                    strings.ToLower(req.Repo),
		Host:                    strings.ToLower(req.Host),
		CredentialGeneration:    connection.CredentialGeneration,
		AppRegistrationID:       connection.AppRegistrationID,
		AppCredentialGeneration: appCredentialGeneration,
		TTL:                     ttl,
		ExpiresAt:               expiresAt,
	}
	return &CredentialLease{Token: token, ExpiresAt: expiresAt}, nil
}

func (b *CredentialBroker) Resolve(
	ctx context.Context,
	req BrokerCredentialRequest,
) (*BrokerCredential, error) {
	if b == nil || b.connections == nil || b.resolver == nil || b.authorizer == nil {
		return nil, ErrGitHubNotConfigured
	}
	if strings.TrimSpace(req.Lease) == "" {
		return nil, ErrCredentialLeaseInvalid
	}
	record, err := b.loadLease(req)
	if err != nil {
		return nil, err
	}
	if err := b.authorizeLease(ctx, record); err != nil {
		return nil, err
	}
	credential, err := b.resolveLeaseCredential(ctx, record)
	if err != nil {
		return nil, err
	}
	if err := b.validateLeaseConnection(ctx, record); err != nil {
		return nil, err
	}
	if !b.renewLease(req.Lease) {
		return nil, ErrCredentialLeaseRevoked
	}
	return credential, nil
}

func (b *CredentialBroker) loadLease(req BrokerCredentialRequest) (credentialLeaseRecord, error) {
	hash := sha256.Sum256([]byte(req.Lease))
	now := b.now()
	b.mu.Lock()
	record, ok := b.leases[hash]
	b.sweepExpiredLocked(now)
	b.mu.Unlock()
	if !ok {
		return credentialLeaseRecord{}, ErrCredentialLeaseInvalid
	}
	if !record.ExpiresAt.After(now) {
		return credentialLeaseRecord{}, ErrCredentialLeaseExpired
	}
	if !credentialLeaseMatches(record, req) {
		return credentialLeaseRecord{}, ErrCredentialScopeDenied
	}
	return record, nil
}

func (b *CredentialBroker) authorizeLease(ctx context.Context, record credentialLeaseRecord) error {
	if err := b.authorizer.AuthorizeGitHubRepository(
		ctx, record.WorkspaceID, record.TaskID, record.SessionID,
		record.RepositoryID, record.Owner, record.Repo,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrCredentialScopeDenied, err)
	}
	return b.validateLeaseConnection(ctx, record)
}

func (b *CredentialBroker) validateLeaseConnection(
	ctx context.Context,
	record credentialLeaseRecord,
) error {
	connection, err := b.connections.GetWorkspaceConnection(ctx, record.WorkspaceID)
	if err != nil {
		return fmt.Errorf("load GitHub workspace connection: %w", err)
	}
	if connection == nil || connection.Status != ConnectionStatusActive ||
		connection.CredentialGeneration != record.CredentialGeneration ||
		connection.AppRegistrationID != record.AppRegistrationID {
		return ErrCredentialLeaseRevoked
	}
	if record.AppRegistrationID != "" {
		generation, generationErr := b.resolver.appCredentialGeneration(record.AppRegistrationID)
		if generationErr != nil || generation != record.AppCredentialGeneration {
			return ErrCredentialLeaseRevoked
		}
	}
	return nil
}

func (b *CredentialBroker) resolveLeaseCredential(
	ctx context.Context,
	record credentialLeaseRecord,
) (*BrokerCredential, error) {
	resolved, err := b.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: record.WorkspaceID,
		Purpose:     CredentialPurposeGitTransport,
		RepoOwner:   record.Owner,
		RepoName:    record.Repo,
	})
	if err != nil {
		return nil, err
	}
	if resolved.CredentialGeneration != record.CredentialGeneration ||
		resolved.AppRegistrationID != record.AppRegistrationID ||
		resolved.AppCredentialGeneration != record.AppCredentialGeneration || resolved.credential == "" {
		return nil, ErrCredentialLeaseRevoked
	}
	// Credential-helper get requests do not distinguish fetch from push. Read
	// is the minimum transport capability; GitHub enforces write permission if
	// the returned installation token is subsequently used for a push.
	if resolved.Principal.Kind == AuthPrincipalApp && !resolved.Capabilities[CapabilityGitRead] {
		return nil, fmt.Errorf("%w: %s", ErrGitHubCapabilityDenied, CapabilityGitRead)
	}
	return &BrokerCredential{
		Username:  gitHubTokenUsername,
		Password:  resolved.credential,
		ExpiresAt: resolved.ExpiresAt,
		Principal: resolved.Principal,
	}, nil
}

func (b *CredentialBroker) renewLease(rawLease string) bool {
	hash := sha256.Sum256([]byte(rawLease))
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	record, ok := b.leases[hash]
	if !ok || !record.ExpiresAt.After(now) {
		return false
	}
	ttl := record.TTL
	if ttl <= 0 || ttl > defaultCredentialLeaseTTL {
		ttl = defaultCredentialLeaseTTL
	}
	record.ExpiresAt = now.UTC().Add(ttl)
	b.leases[hash] = record
	return true
}

func (b *CredentialBroker) sweepExpiredLocked(now time.Time) {
	for hash, record := range b.leases {
		if !record.ExpiresAt.After(now) {
			delete(b.leases, hash)
		}
	}
}

func (b *CredentialBroker) workspaceLeaseCountLocked(workspaceID string) int {
	count := 0
	for _, record := range b.leases {
		if record.WorkspaceID == workspaceID {
			count++
		}
	}
	return count
}

func (b *CredentialBroker) RevokeTask(taskID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for hash, record := range b.leases {
		if record.TaskID == taskID {
			delete(b.leases, hash)
		}
	}
}

func (b *CredentialBroker) RevokeSession(sessionID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for hash, record := range b.leases {
		if record.SessionID == sessionID {
			delete(b.leases, hash)
		}
	}
}

func (b *CredentialBroker) RevokeWorkspace(workspaceID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for hash, record := range b.leases {
		if record.WorkspaceID == workspaceID {
			delete(b.leases, hash)
		}
	}
}

func validateCredentialLeaseScope(workspaceID, taskID, sessionID, repositoryID, owner, repo, host string) error {
	for name, value := range map[string]string{
		"workspace":  workspaceID,
		"task":       taskID,
		"session":    sessionID,
		"repository": repositoryID,
		"owner":      owner,
		"repo":       repo,
		"host":       host,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s is required", ErrCredentialLeaseInvalid, name)
		}
	}
	if !strings.EqualFold(host, defaultGitHubHost) {
		return fmt.Errorf("%w: unsupported host", ErrCredentialScopeDenied)
	}
	return nil
}

func credentialLeaseMatches(record credentialLeaseRecord, req BrokerCredentialRequest) bool {
	return record.TaskID == req.TaskID &&
		record.SessionID == req.SessionID &&
		record.RepositoryID == req.RepositoryID &&
		strings.EqualFold(record.Owner, req.Owner) &&
		strings.EqualFold(record.Repo, req.Repo) &&
		strings.EqualFold(record.Host, req.Host)
}
