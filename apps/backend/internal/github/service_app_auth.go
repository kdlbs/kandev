package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type GitHubAppRuntimeConfig struct {
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	Slug          string
	PublicBaseURL string
}

func (s *Service) ConfigureGitHubAppAuth(config GitHubAppRuntimeConfig) error {
	if s == nil || s.store == nil || s.connectionSecrets == nil || s.appClient == nil {
		return errors.New("GitHub App authentication dependencies are not configured")
	}
	baseURL := strings.TrimRight(config.PublicBaseURL, "/")
	if baseURL == "" || config.ClientID == "" || config.ClientSecret == "" ||
		config.WebhookSecret == "" || config.Slug == "" {
		return errors.New("GitHub App runtime configuration is incomplete")
	}
	flows := NewOAuthFlowManager(s.store)
	oauth := NewGitHubOAuthClient(config.ClientID, config.ClientSecret)
	personalRepo := s.personalConnections
	if personalRepo == nil {
		personalRepo = NewStorePersonalConnectionRepository(s.store, s.connectionSecrets)
		s.personalConnections = personalRepo
	}
	connectionStore := &serviceAppConnectionStore{service: s}
	personal := NewPersonalAuthService(PersonalAuthConfig{
		ClientID:    config.ClientID,
		CallbackURL: baseURL + "/api/v1/github/personal-connection/callback",
	}, flows, personalRepo, oauth)
	personal.SetWorkspaceMutationLock(s.workspaceConnectionMutationLock)

	s.mu.Lock()
	s.appAvailable = true
	s.appInstallationAuth = NewAppInstallationService(AppInstallationConfig{
		Slug:        config.Slug,
		CallbackURL: baseURL + "/api/v1/github/app/install/callback",
	}, flows, connectionStore, s.appClient, oauth)
	s.personalAuth = personal
	s.webhookAuth = NewGitHubWebhookService(
		config.WebhookSecret,
		connectionStore,
		&installationRepositorySettingsUpdater{service: s},
		personalRepo,
		GitHubWebhookReconciliation{Installations: s.appClient, Personal: personal},
	)
	s.mu.Unlock()
	if s.resolver != nil {
		s.resolver.SetUserProvider(&personalAuthCredentialProvider{service: personal})
	}
	return nil
}

func (s *Service) SetCredentialBroker(broker *CredentialBroker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentialBroker = broker
}

func (s *Service) CredentialBrokerReady() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.credentialBroker != nil
}

func (s *Service) ConfigureCredentialBroker(authorizer BrokerScopeAuthorizer) error {
	if s == nil || s.store == nil || s.resolver == nil || authorizer == nil {
		return errors.New("GitHub credential broker dependencies are not configured")
	}
	s.SetCredentialBroker(NewCredentialBroker(s.store, s.resolver, authorizer))
	return nil
}

func (s *Service) IssueGitHubCredentialLease(
	ctx context.Context,
	request CredentialLeaseRequest,
) (*CredentialLease, error) {
	s.mu.Lock()
	broker := s.credentialBroker
	s.mu.Unlock()
	if broker == nil {
		return nil, ErrGitHubNotConfigured
	}
	return broker.Issue(ctx, request)
}

func (s *Service) ResolveGitHubCredential(
	ctx context.Context,
	request BrokerCredentialRequest,
) (*BrokerCredential, error) {
	s.mu.Lock()
	broker := s.credentialBroker
	s.mu.Unlock()
	if broker == nil {
		return nil, ErrGitHubNotConfigured
	}
	return broker.Resolve(ctx, request)
}

type WorkspaceAutomationStatus struct {
	*WorkspaceConnection
	Actor               *AuthPrincipal               `json:"actor,omitempty"`
	Capabilities        map[GitHubAppCapability]bool `json:"capabilities,omitempty"`
	MissingCapabilities []GitHubAppCapability        `json:"missing_capabilities,omitempty"`
	RateLimit           *GitHubRateLimitInfo         `json:"rate_limit,omitempty"`
	LegacyMigration     bool                         `json:"legacy_migration"`
}

type WorkspaceAuthStatus struct {
	WorkspaceID                  string                     `json:"workspace_id"`
	Automation                   *WorkspaceAutomationStatus `json:"automation,omitempty"`
	Personal                     *UserConnection            `json:"personal,omitempty"`
	EffectivePersonalActor       *AuthPrincipal             `json:"effective_personal_actor,omitempty"`
	EffectiveManualMutationActor *AuthPrincipal             `json:"effective_manual_mutation_actor,omitempty"`
	GitHubAppAvailable           bool                       `json:"github_app_available"`
	AppAvailable                 bool                       `json:"app_available"`
	Authenticated                bool                       `json:"authenticated"`
	Username                     string                     `json:"username"`
	AuthMethod                   string                     `json:"auth_method"`
	TokenConfigured              bool                       `json:"token_configured"`
	RequiredScopes               []string                   `json:"required_scopes"`
	RateLimit                    *GitHubRateLimitInfo       `json:"rate_limit,omitempty"`
}

func (s *Service) GetWorkspaceAuthStatus(
	ctx context.Context,
	workspaceID, userID string,
) (*WorkspaceAuthStatus, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if s == nil || s.store == nil {
		return nil, ErrGitHubNotConfigured
	}
	connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var personal *UserConnection
	if userID != "" {
		personal, err = s.store.GetUserConnection(ctx, workspaceID, userID)
		if err != nil {
			return nil, err
		}
	}
	status := &WorkspaceAuthStatus{
		WorkspaceID: workspaceID, Personal: personal, RequiredScopes: RequiredGitHubScopes,
		AuthMethod: AuthMethodNone,
	}
	s.mu.Lock()
	status.GitHubAppAvailable = s.appAvailable
	status.AppAvailable = s.appAvailable
	s.mu.Unlock()
	if connection == nil {
		return status, nil
	}
	status.AuthMethod = string(connection.Source)
	status.TokenConfigured = connection.Source == ConnectionSourcePAT
	status.Automation = &WorkspaceAutomationStatus{
		WorkspaceConnection: connection,
		LegacyMigration:     connection.Source == ConnectionSourceLegacyShared,
	}
	if connection.Status != ConnectionStatusActive || s.resolver == nil {
		return status, nil
	}
	automation, resolveErr := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID, Purpose: CredentialPurposeAutomation,
	})
	if resolveErr != nil || automation == nil {
		return status, nil
	}
	status.Authenticated = true
	status.Username = automation.Principal.Login
	status.Automation.Actor = principalPointer(automation.Principal)
	status.Automation.Capabilities = automation.Capabilities
	status.Automation.MissingCapabilities = missingGitHubCapabilities(automation.Capabilities)
	status.Automation.RateLimit = rateLimitInfoForTracker(automation.RateTracker)
	status.RateLimit = status.Automation.RateLimit

	personalActor, _ := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID, UserID: userID, Purpose: CredentialPurposePersonalRead,
	})
	if personalActor != nil {
		status.EffectivePersonalActor = principalPointer(personalActor.Principal)
	}
	mutationActor, _ := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID, UserID: userID, Purpose: CredentialPurposePersonalWrite,
	})
	if mutationActor != nil {
		status.EffectiveManualMutationActor = principalPointer(mutationActor.Principal)
	}
	return status, nil
}

func principalPointer(principal AuthPrincipal) *AuthPrincipal {
	copy := principal
	return &copy
}

func missingGitHubCapabilities(capabilities map[GitHubAppCapability]bool) []GitHubAppCapability {
	missing := make([]GitHubAppCapability, 0)
	for _, capability := range allGitHubCapabilities {
		if !capabilities[capability] {
			missing = append(missing, capability)
		}
	}
	return missing
}

func (s *Service) StartAppInstallation(
	ctx context.Context,
	workspaceID, userID string,
) (AppInstallationStart, error) {
	s.mu.Lock()
	service := s.appInstallationAuth
	s.mu.Unlock()
	if service == nil {
		return AppInstallationStart{}, ErrGitHubNotConfigured
	}
	return service.Start(ctx, workspaceID, userID)
}

func (s *Service) CompleteAppInstallation(
	ctx context.Context,
	callback AppInstallationCallback,
) (AppInstallationResult, error) {
	s.mu.Lock()
	service := s.appInstallationAuth
	s.mu.Unlock()
	if service == nil {
		return AppInstallationResult{}, ErrGitHubNotConfigured
	}
	return service.Complete(ctx, callback)
}

func (s *Service) StartPersonalAuth(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalAuthStart, error) {
	s.mu.Lock()
	service := s.personalAuth
	s.mu.Unlock()
	if service == nil {
		return PersonalAuthStart{}, ErrGitHubNotConfigured
	}
	return service.Start(ctx, workspaceID, userID)
}

func (s *Service) CompletePersonalAuth(
	ctx context.Context,
	callback PersonalAuthCallback,
) (PersonalAuthResult, error) {
	s.mu.Lock()
	service := s.personalAuth
	s.mu.Unlock()
	if service == nil {
		return PersonalAuthResult{}, ErrGitHubNotConfigured
	}
	return service.Complete(ctx, callback)
}

func (s *Service) DisconnectPersonalAuth(ctx context.Context, workspaceID, userID string) error {
	s.mu.Lock()
	service := s.personalAuth
	s.mu.Unlock()
	if service == nil {
		return nil
	}
	return service.Revoke(ctx, workspaceID, userID)
}

func (s *Service) HandleAppWebhook(
	ctx context.Context,
	request GitHubWebhookRequest,
) (GitHubWebhookResult, error) {
	s.mu.Lock()
	service := s.webhookAuth
	s.mu.Unlock()
	if service == nil {
		return GitHubWebhookResult{}, ErrGitHubNotConfigured
	}
	return service.Handle(ctx, request)
}

type serviceAppConnectionStore struct {
	service *Service
}

func (r *serviceAppConnectionStore) GetWorkspaceConnection(
	ctx context.Context,
	workspaceID string,
) (*WorkspaceConnection, error) {
	return r.service.store.GetWorkspaceConnection(ctx, workspaceID)
}

func (r *serviceAppConnectionStore) UpsertWorkspaceConnection(
	ctx context.Context,
	connection *WorkspaceConnection,
) error {
	lock := r.service.workspaceConnectionMutationLock(connection.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()

	existing, err := r.service.store.GetWorkspaceConnection(ctx, connection.WorkspaceID)
	if err != nil {
		return err
	}
	connection.CredentialGeneration = nextCredentialGeneration(existing)
	return r.upsertWorkspaceConnectionLocked(ctx, existing, connection)
}

func (r *serviceAppConnectionStore) ReplaceWorkspaceConnection(
	ctx context.Context,
	connection *WorkspaceConnection,
	expected WorkspaceConnectionExpectation,
) error {
	lock := r.service.workspaceConnectionMutationLock(connection.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()

	existing, err := r.service.store.GetWorkspaceConnection(ctx, connection.WorkspaceID)
	if err != nil {
		return err
	}
	if !matchesWorkspaceConnectionExpectation(existing, expected) ||
		connection.CredentialGeneration != expected.CredentialGeneration+1 {
		return ErrOAuthFlowStale
	}
	return r.upsertWorkspaceConnectionLocked(ctx, existing, connection)
}

func (r *serviceAppConnectionStore) upsertWorkspaceConnectionLocked(
	ctx context.Context,
	existing, connection *WorkspaceConnection,
) error {
	var err error
	if existing != nil && !existing.CreatedAt.IsZero() {
		connection.CreatedAt = existing.CreatedAt
	}
	var previousPAT string
	var hadPreviousPAT bool
	if existing != nil && existing.Source == ConnectionSourcePAT {
		previousPAT, hadPreviousPAT, err = revealOptionalSecret(
			ctx,
			r.service.connectionSecrets,
			WorkspacePATSecretKey(connection.WorkspaceID),
		)
		if err != nil {
			return err
		}
	}
	if err := r.service.store.UpsertWorkspaceConnection(ctx, connection); err != nil {
		return err
	}
	if err := r.service.revokePersonalForAutomationTransition(ctx, existing, connection); err != nil {
		return errors.Join(
			fmt.Errorf("revoke personal GitHub connections: %w", err),
			restoreWorkspaceConnection(ctx, r.service.store, existing, connection.WorkspaceID),
		)
	}
	if existing != nil && existing.Source == ConnectionSourcePAT && hadPreviousPAT {
		if err := r.service.connectionSecrets.Delete(ctx, WorkspacePATSecretKey(connection.WorkspaceID)); err != nil {
			return errors.Join(
				err,
				restoreWorkspaceConnection(ctx, r.service.store, existing, connection.WorkspaceID),
				restoreOptionalSecret(
					ctx,
					r.service.connectionSecrets,
					WorkspacePATSecretKey(connection.WorkspaceID),
					workspacePATSecretName,
					previousPAT,
					hadPreviousPAT,
				),
			)
		}
	}
	if existing != nil && existing.InstallationID != nil {
		r.service.InvalidateInstallationCredentials(*existing.InstallationID)
	}
	r.service.invalidateWorkspaceCredential(connection.WorkspaceID)
	return nil
}

func restoreOptionalSecret(
	ctx context.Context,
	secrets ConnectionSecretStore,
	key, name, value string,
	existed bool,
) error {
	if existed {
		return secrets.Set(ctx, key, name, value)
	}
	return deleteOptionalSecret(ctx, secrets, key)
}

func (r *serviceAppConnectionStore) ListWorkspaceConnectionsByInstallation(
	ctx context.Context,
	installationID int64,
) ([]*WorkspaceConnection, error) {
	return r.service.store.ListWorkspaceConnectionsByInstallation(ctx, installationID)
}

func (r *serviceAppConnectionStore) TransitionWorkspaceInstallationConnection(
	ctx context.Context,
	expected, next *WorkspaceConnection,
) (bool, error) {
	lock := r.service.workspaceConnectionMutationLock(expected.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()

	updated, err := r.service.store.TransitionWorkspaceInstallationConnection(ctx, expected, next)
	if err != nil || !updated {
		return updated, err
	}
	if expected.InstallationID != nil {
		r.service.InvalidateInstallationCredentials(*expected.InstallationID)
	}
	r.service.invalidateWorkspaceCredential(expected.WorkspaceID)
	return true, nil
}

func (r *serviceAppConnectionStore) ListUserConnectionsByGitHubUser(
	ctx context.Context,
	githubUserID int64,
) ([]*UserConnection, error) {
	return r.service.store.ListUserConnectionsByGitHubUser(ctx, githubUserID)
}

func (r *serviceAppConnectionStore) ClaimWebhookDelivery(
	ctx context.Context,
	delivery *WebhookDelivery,
	staleBefore time.Time,
) (WebhookDeliveryClaim, error) {
	return r.service.store.ClaimWebhookDelivery(ctx, delivery, staleBefore)
}

func (r *serviceAppConnectionStore) CompleteWebhookDelivery(
	ctx context.Context,
	deliveryID string,
	status WebhookDeliveryStatus,
	result string,
	processedAt time.Time,
) error {
	return r.service.store.CompleteWebhookDelivery(ctx, deliveryID, status, result, processedAt)
}

func restoreWorkspaceConnection(
	ctx context.Context,
	store *Store,
	connection *WorkspaceConnection,
	workspaceID string,
) error {
	if connection == nil {
		return store.DeleteWorkspaceConnection(ctx, workspaceID)
	}
	return store.UpsertWorkspaceConnection(ctx, connection)
}

type installationRepositorySettingsUpdater struct {
	service *Service
}

func (u *installationRepositorySettingsUpdater) ApplyInstallationRepositories(
	ctx context.Context,
	change InstallationRepositoriesChange,
) (bool, error) {
	if u == nil || u.service == nil {
		return false, errors.New("GitHub installation repository updater is not configured")
	}
	lock := u.service.workspaceConnectionMutationLock(change.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()

	connection, err := u.service.store.GetWorkspaceConnection(ctx, change.WorkspaceID)
	if err != nil {
		return false, err
	}
	expectedInstallationID := change.InstallationID
	if !matchesWorkspaceConnectionExpectation(connection, WorkspaceConnectionExpectation{
		Source:               change.ConnectionSource,
		CredentialGeneration: change.CredentialGeneration,
		InstallationID:       &expectedInstallationID,
	}) || connection.Status != ConnectionStatusActive {
		return false, nil
	}
	u.service.InvalidateInstallationCredentials(change.InstallationID)
	u.service.invalidateWorkspaceCredential(change.WorkspaceID)
	if len(change.Removed) == 0 {
		return true, nil
	}
	settings, err := u.service.store.GetWorkspaceSettings(ctx, change.WorkspaceID)
	if err != nil || settings == nil || settings.RepoScopeMode != RepoScopeModeRepos {
		return err == nil, err
	}
	removed := make(map[string]struct{}, len(change.Removed))
	for _, repository := range change.Removed {
		removed[strings.ToLower(repository.FullName)] = struct{}{}
	}
	filtered := settings.RepoScopeRepos[:0]
	for _, repository := range settings.RepoScopeRepos {
		if _, ok := removed[strings.ToLower(repository.Owner+"/"+repository.Name)]; !ok {
			filtered = append(filtered, repository)
		}
	}
	if len(filtered) == len(settings.RepoScopeRepos) {
		return true, nil
	}
	settings.RepoScopeRepos = filtered
	if err := u.service.store.UpsertWorkspaceSettings(ctx, settings); err != nil {
		return false, err
	}
	return true, nil
}
