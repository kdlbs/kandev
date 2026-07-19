package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const credentialCacheRefreshMargin = 5 * time.Minute

var errCredentialResolutionInvalidated = errors.New("GitHub credential resolution invalidated")

type workspaceConnectionReader interface {
	GetWorkspaceConnection(ctx context.Context, workspaceID string) (*WorkspaceConnection, error)
	GetUserConnection(ctx context.Context, workspaceID, userID string) (*UserConnection, error)
}

type authSecretReader interface {
	Reveal(ctx context.Context, id string) (string, error)
}

type installationCredentialProvider interface {
	ResolveInstallation(
		ctx context.Context,
		connection *WorkspaceConnection,
		req ResolveCredentialRequest,
	) (*ResolvedCredential, error)
}

type userCredentialProvider interface {
	ResolveUser(
		ctx context.Context,
		connection *UserConnection,
		req ResolveCredentialRequest,
	) (*ResolvedCredential, error)
}

type automationCredentialProvider interface {
	ResolveAutomation(
		ctx context.Context,
		connection *WorkspaceConnection,
		req ResolveCredentialRequest,
	) (*ResolvedCredential, error)
}

type legacyCredentialFactory func(ctx context.Context) (Client, string, error)
type ghAccountTokenResolver func(ctx context.Context, host, login string) (string, error)

type credentialCacheKey struct {
	workspaceID string
	source      ConnectionSource
	generation  int64
	purpose     CredentialPurpose
	repoOwner   string
	repoName    string
}

type credentialCacheEpoch struct {
	all       uint64
	workspace uint64
}

// CredentialResolver owns credential selection for all GitHub operations.
// Its cache is keyed by workspace and generation so replacement or revocation
// cannot reuse a client created for another principal.
type CredentialResolver struct {
	connections workspaceConnectionReader
	secrets     authSecretReader
	app         installationCredentialProvider
	users       userCredentialProvider
	automation  automationCredentialProvider
	legacy      legacyCredentialFactory
	ghToken     ghAccountTokenResolver
	now         func() time.Time

	mu              sync.Mutex
	cache           map[credentialCacheKey]*ResolvedCredential
	allEpoch        uint64
	workspaceEpochs map[string]uint64
}

func NewCredentialResolver(connections workspaceConnectionReader, secrets authSecretReader) *CredentialResolver {
	return &CredentialResolver{
		connections: connections,
		secrets:     secrets,
		legacy: func(ctx context.Context) (Client, string, error) {
			return nil, AuthMethodNone, ErrGitHubNotConfigured
		},
		ghToken:         ResolveGHAccountToken,
		now:             time.Now,
		cache:           make(map[credentialCacheKey]*ResolvedCredential),
		workspaceEpochs: make(map[string]uint64),
	}
}

func (r *CredentialResolver) SetLegacyFactory(factory legacyCredentialFactory) {
	if factory != nil {
		r.legacy = factory
	}
}

func (r *CredentialResolver) SetInstallationProvider(provider installationCredentialProvider) {
	r.app = provider
}

func (r *CredentialResolver) SetUserProvider(provider userCredentialProvider) {
	r.users = provider
}

func (r *CredentialResolver) SetAutomationProvider(provider automationCredentialProvider) {
	r.automation = provider
}

func (r *CredentialResolver) Resolve(ctx context.Context, req ResolveCredentialRequest) (*ResolvedCredential, error) {
	if req.WorkspaceID == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if !validCredentialPurpose(req.Purpose) {
		return nil, fmt.Errorf("invalid GitHub credential purpose %q", req.Purpose)
	}
	for {
		epoch := r.epoch(req.WorkspaceID)
		resolved, err := r.resolveAtEpoch(ctx, req, epoch)
		if !r.epochCurrent(req.WorkspaceID, epoch) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			continue
		}
		if errors.Is(err, errCredentialResolutionInvalidated) {
			continue
		}
		return resolved, err
	}
}

func (r *CredentialResolver) resolveAtEpoch(
	ctx context.Context,
	req ResolveCredentialRequest,
	epoch credentialCacheEpoch,
) (*ResolvedCredential, error) {
	connection, err := r.connections.GetWorkspaceConnection(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load GitHub workspace connection: %w", err)
	}
	if connection == nil {
		return nil, ErrGitHubNotConfigured
	}

	if req.Purpose == CredentialPurposePersonalRead || req.Purpose == CredentialPurposePersonalWrite {
		if personal, resolveErr := r.resolvePersonal(ctx, req, connection); personal != nil || resolveErr != nil {
			return personal, resolveErr
		}
	}
	return r.resolveAutomation(ctx, req, connection, epoch)
}

func validCredentialPurpose(purpose CredentialPurpose) bool {
	switch purpose {
	case CredentialPurposeAutomation, CredentialPurposePersonalRead,
		CredentialPurposePersonalWrite, CredentialPurposeGitTransport:
		return true
	default:
		return false
	}
}

func (r *CredentialResolver) resolvePersonal(
	ctx context.Context,
	req ResolveCredentialRequest,
	automation *WorkspaceConnection,
) (*ResolvedCredential, error) {
	personal, err := r.resolveConfiguredPersonal(ctx, req)
	if personal != nil {
		return personal, err
	}
	if err != nil && (req.Purpose != CredentialPurposePersonalWrite || !errors.Is(err, ErrGitHubConnectionInvalid)) {
		return nil, err
	}

	// A human PAT/CLI automation identity can supply authenticated-viewer
	// semantics. An App installation cannot impersonate a person.
	if automation.Source == ConnectionSourcePAT || automation.Source == ConnectionSourceGHCLI || automation.Source == ConnectionSourceLegacyShared {
		return nil, nil
	}
	if req.Purpose == CredentialPurposePersonalWrite {
		// Manual mutations may intentionally fall back to the App. The returned
		// principal lets the caller disclose that attribution.
		return nil, nil
	}
	return nil, ErrGitHubPersonalRequired
}

func (r *CredentialResolver) resolveConfiguredPersonal(
	ctx context.Context,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if req.UserID == "" || r.users == nil {
		return nil, nil
	}
	connection, err := r.connections.GetUserConnection(ctx, req.WorkspaceID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("load personal GitHub connection: %w", err)
	}
	if connection == nil {
		return nil, nil
	}
	if connection.Status != ConnectionStatusActive {
		return nil, ErrGitHubConnectionInvalid
	}
	resolved, err := r.users.ResolveUser(ctx, connection, req)
	if err != nil {
		return nil, err
	}
	resolved.Principal.WorkspaceID = req.WorkspaceID
	resolved.Principal.UserID = req.UserID
	resolved.CredentialGeneration = connection.CredentialGeneration
	return resolved, nil
}

func (r *CredentialResolver) resolveAutomation(
	ctx context.Context,
	req ResolveCredentialRequest,
	connection *WorkspaceConnection,
	epoch credentialCacheEpoch,
) (*ResolvedCredential, error) {
	if connection.Status != ConnectionStatusActive {
		return nil, ErrGitHubConnectionInvalid
	}
	key := credentialCacheKey{
		workspaceID: req.WorkspaceID,
		source:      connection.Source,
		generation:  connection.CredentialGeneration,
		purpose:     req.Purpose,
		repoOwner:   strings.ToLower(strings.TrimSpace(req.RepoOwner)),
		repoName:    strings.ToLower(strings.TrimSpace(req.RepoName)),
	}
	if cached := r.cached(key); cached != nil {
		return cached, nil
	}

	resolved, err := r.resolveAutomationSource(ctx, connection, req)
	if err != nil {
		return nil, err
	}
	if resolved == nil || resolved.Client == nil {
		return nil, ErrGitHubNotConfigured
	}
	resolved.Principal.WorkspaceID = req.WorkspaceID
	resolved.CredentialGeneration = connection.CredentialGeneration
	if !resolved.ExpiresAt.IsZero() && !resolved.ExpiresAt.After(r.now()) {
		return nil, ErrInstallationTokenExpired
	}
	if !r.storeCached(key, resolved, epoch) {
		return nil, errCredentialResolutionInvalidated
	}
	return resolved, nil
}

func (r *CredentialResolver) resolveAutomationSource(
	ctx context.Context,
	connection *WorkspaceConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if r.automation != nil {
		return r.automation.ResolveAutomation(ctx, connection, req)
	}
	switch connection.Source {
	case ConnectionSourcePAT:
		return r.resolvePAT(ctx, connection)
	case ConnectionSourceGHCLI:
		return r.resolveGHCLI(ctx, connection)
	case ConnectionSourceGitHubAppInstallation:
		if r.app == nil {
			return nil, ErrGitHubNotConfigured
		}
		return r.app.ResolveInstallation(ctx, connection, req)
	case ConnectionSourceLegacyShared:
		return r.resolveLegacy(ctx, connection)
	default:
		return nil, fmt.Errorf("unsupported GitHub connection source %q", connection.Source)
	}
}

func (r *CredentialResolver) resolvePAT(ctx context.Context, connection *WorkspaceConnection) (*ResolvedCredential, error) {
	if r.secrets == nil {
		return nil, ErrGitHubNotConfigured
	}
	token, err := r.secrets.Reveal(ctx, WorkspacePATSecretKey(connection.WorkspaceID))
	if err != nil {
		return nil, fmt.Errorf("reveal workspace GitHub PAT: %w", err)
	}
	tracker := NewRateTracker(nil, nil)
	client := NewTokenClient(token, TokenPrincipal{
		Kind:        TokenCredentialPAT,
		PrincipalID: "login:" + connection.Login,
		Login:       connection.Login,
	}).WithRateTracker(tracker)
	return &ResolvedCredential{
		Client:       client,
		Capabilities: allTokenCapabilities(),
		credential:   token,
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourcePAT,
			Login:  connection.Login,
		},
		RateTracker: tracker,
	}, nil
}

func (r *CredentialResolver) resolveGHCLI(ctx context.Context, connection *WorkspaceConnection) (*ResolvedCredential, error) {
	token, err := r.ghToken(ctx, connection.GitHubHost, connection.Login)
	if err != nil {
		return nil, err
	}
	tracker := NewRateTracker(nil, nil)
	client := NewTokenClient(token, TokenPrincipal{
		Kind:        TokenCredentialCLI,
		PrincipalID: "login:" + connection.Login,
		Login:       connection.Login,
	}).WithRateTracker(tracker)
	return &ResolvedCredential{
		Client:       client,
		Capabilities: allTokenCapabilities(),
		credential:   token,
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourceGHCLI,
			Login:  connection.Login,
		},
		RateTracker: tracker,
	}, nil
}

func (r *CredentialResolver) resolveLegacy(ctx context.Context, connection *WorkspaceConnection) (*ResolvedCredential, error) {
	client, method, err := r.legacy(ctx)
	if err != nil {
		return nil, err
	}
	if client == nil || method == AuthMethodNone {
		return nil, ErrGitHubNotConfigured
	}
	login, _ := client.GetAuthenticatedUser(ctx)
	return &ResolvedCredential{
		Client:       client,
		Capabilities: allTokenCapabilities(),
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourceLegacyShared,
			Login:  login,
		},
		RateTracker: NewRateTracker(nil, nil),
	}, nil
}

func (r *CredentialResolver) cached(key credentialCacheKey) *ResolvedCredential {
	r.mu.Lock()
	defer r.mu.Unlock()
	resolved := r.cache[key]
	if resolved == nil {
		return nil
	}
	if !resolved.ExpiresAt.IsZero() && !resolved.ExpiresAt.After(r.now().Add(credentialCacheRefreshMargin)) {
		delete(r.cache, key)
		return nil
	}
	return cloneResolvedCredential(resolved)
}

func (r *CredentialResolver) storeCached(
	key credentialCacheKey,
	resolved *ResolvedCredential,
	epoch credentialCacheEpoch,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.epochCurrentLocked(key.workspaceID, epoch) {
		return false
	}
	for existing := range r.cache {
		if existing.workspaceID == key.workspaceID && existing.generation < key.generation {
			delete(r.cache, existing)
		}
	}
	r.cache[key] = cloneResolvedCredential(resolved)
	return true
}

func cloneResolvedCredential(resolved *ResolvedCredential) *ResolvedCredential {
	if resolved == nil {
		return nil
	}
	cloned := *resolved
	if resolved.Capabilities != nil {
		cloned.Capabilities = make(map[GitHubAppCapability]bool, len(resolved.Capabilities))
		for capability, allowed := range resolved.Capabilities {
			cloned.Capabilities[capability] = allowed
		}
	}
	return &cloned
}

var allGitHubCapabilities = []GitHubAppCapability{
	CapabilityRepositoryRead,
	CapabilityGitRead,
	CapabilityGitWrite,
	CapabilityPullRequestRead,
	CapabilityPullRequestWrite,
	CapabilityIssueRead,
	CapabilityIssueWrite,
	CapabilityChecksRead,
	CapabilityStatusesRead,
	CapabilityActionsRead,
	CapabilityBranchProtectionRead,
	CapabilityMembersRead,
	CapabilityWorkflowsWrite,
}

func allTokenCapabilities() map[GitHubAppCapability]bool {
	capabilities := make(map[GitHubAppCapability]bool, len(allGitHubCapabilities))
	for _, capability := range allGitHubCapabilities {
		capabilities[capability] = true
	}
	return capabilities
}

func (r *CredentialResolver) InvalidateWorkspace(workspaceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceEpochs[workspaceID]++
	for key := range r.cache {
		if key.workspaceID == workspaceID {
			delete(r.cache, key)
		}
	}
}

func (r *CredentialResolver) InvalidateAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allEpoch++
	clear(r.cache)
}

func (r *CredentialResolver) epoch(workspaceID string) credentialCacheEpoch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return credentialCacheEpoch{all: r.allEpoch, workspace: r.workspaceEpochs[workspaceID]}
}

func (r *CredentialResolver) epochCurrent(workspaceID string, expected credentialCacheEpoch) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.epochCurrentLocked(workspaceID, expected)
}

func (r *CredentialResolver) epochCurrentLocked(workspaceID string, expected credentialCacheEpoch) bool {
	return r.allEpoch == expected.all && r.workspaceEpochs[workspaceID] == expected.workspace
}
