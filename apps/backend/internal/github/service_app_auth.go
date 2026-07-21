package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

type GitHubAppRuntimeConfig struct {
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	Slug          string
	PublicBaseURL string
}

type githubAppRuntime struct {
	source               DeploymentAppSource
	generation           int64
	appID                int64
	appClient            *AppClient
	installationTokens   *InstallationTokenCache
	installationProvider *CachedInstallationCredentialProvider
	appInstallationAuth  *AppInstallationService
	personalAuth         *PersonalAuthService
	webhookAuth          *GitHubWebhookService
	initialWebhookHealth deploymentAppWebhookHealth
}

type deploymentAppWebhookHealth struct {
	status        DeploymentAppWebhookStatus
	lastWebhookAt *time.Time
	lastError     string
}

type DeploymentAppRuntimeSnapshot struct {
	Source     DeploymentAppSource
	Ready      bool
	AppID      int64
	Generation int64
}

// ApplyDeploymentAppRuntime builds every App-dependent service before making
// the generation visible. Readers observe either the old generation or the
// complete replacement, never a mixture of keys, OAuth clients, and webhooks.
func (s *Service) ApplyDeploymentAppRuntime(resolved ResolvedDeploymentAppConfig) error {
	if s == nil {
		return errors.New("GitHub service is not configured")
	}
	if resolved.Source == DeploymentAppSourceNone {
		s.swapDeploymentAppRuntime(nil)
		return nil
	}
	runtime, err := s.buildDeploymentAppRuntime(resolved)
	if err != nil {
		return err
	}
	s.swapDeploymentAppRuntime(runtime)
	return nil
}

func (s *Service) buildDeploymentAppRuntime(
	resolved ResolvedDeploymentAppConfig,
) (*githubAppRuntime, error) {
	if s.store == nil || s.connectionSecrets == nil {
		return nil, errors.New("GitHub App authentication dependencies are not configured")
	}
	if resolved.Source != DeploymentAppSourceEnvironment && resolved.Source != DeploymentAppSourceManaged {
		return nil, errors.New("GitHub App runtime source is invalid")
	}
	if err := resolved.Config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid GitHub App runtime configuration: %w", err)
	}
	privateKey, err := resolved.Config.PrivateKeyPEM()
	if err != nil {
		return nil, fmt.Errorf("load GitHub App private key: %w", err)
	}
	appClient, err := NewAppClient(resolved.Config.AppID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("initialize GitHub App client: %w", err)
	}
	return s.buildDeploymentAppRuntimeWithClient(resolved, appClient)
}

func (s *Service) buildDeploymentAppRuntimeWithClient(
	resolved ResolvedDeploymentAppConfig,
	appClient *AppClient,
) (*githubAppRuntime, error) {
	config := resolved.Config
	baseURL := strings.TrimRight(config.PublicBaseURL, "/")
	if appClient == nil || baseURL == "" || config.ClientID == "" || config.ClientSecret == "" ||
		config.WebhookSecret == "" || config.Slug == "" {
		return nil, errors.New("GitHub App runtime configuration is incomplete")
	}
	flows := NewOAuthFlowManager(s.store)
	oauth := NewGitHubOAuthClient(config.ClientID, config.ClientSecret)
	personalRepo := s.personalConnections
	if personalRepo == nil {
		personalRepo = NewStorePersonalConnectionRepository(s.store, s.connectionSecrets)
	}
	connectionStore := &serviceAppConnectionStore{service: s}
	personal := NewPersonalAuthService(PersonalAuthConfig{
		ClientID:    config.ClientID,
		CallbackURL: baseURL + "/api/v1/github/personal-connection/callback",
	}, flows, personalRepo, oauth)
	personal.SetWorkspaceMutationLock(s.workspaceConnectionMutationLock)
	tokens := NewInstallationTokenCache(appClient)
	generation := int64(0)
	health := deploymentAppWebhookHealth{status: DeploymentAppWebhookUnverified}
	if resolved.Registration != nil {
		generation = resolved.Registration.CredentialGeneration
		health.status = resolved.Registration.WebhookStatus
		health.lastWebhookAt = resolved.Registration.LastWebhookAt
		health.lastError = resolved.Registration.LastError
		if health.status == "" {
			health.status = DeploymentAppWebhookUnverified
		}
	}
	return &githubAppRuntime{
		source: resolved.Source, generation: generation, appID: config.AppID,
		appClient: appClient, installationTokens: tokens,
		installationProvider: NewCachedInstallationCredentialProvider(tokens),
		appInstallationAuth: NewAppInstallationService(AppInstallationConfig{
			Slug: config.Slug, CallbackURL: baseURL + "/api/v1/github/app/install/callback",
		}, flows, connectionStore, appClient, oauth),
		personalAuth: personal,
		webhookAuth: NewGitHubWebhookService(
			config.WebhookSecret, connectionStore,
			&installationRepositorySettingsUpdater{service: s}, personalRepo,
			GitHubWebhookReconciliation{Installations: appClient, Personal: personal},
		),
		initialWebhookHealth: health,
	}, nil
}

func (s *Service) swapDeploymentAppRuntime(runtime *githubAppRuntime) {
	s.mu.Lock()
	s.deploymentAppRuntime = runtime
	s.appAvailable = runtime != nil
	if runtime == nil {
		s.deploymentAppWebhookHealth = deploymentAppWebhookHealth{}
		s.appClient = nil
		s.installationTokens = nil
		s.appInstallationAuth = nil
		s.personalAuth = nil
		s.webhookAuth = nil
	} else {
		s.deploymentAppWebhookHealth = runtime.initialWebhookHealth
		s.appClient = runtime.appClient
		s.installationTokens = runtime.installationTokens
		s.appInstallationAuth = runtime.appInstallationAuth
		s.personalAuth = runtime.personalAuth
		s.webhookAuth = runtime.webhookAuth
	}
	s.mu.Unlock()
	if s.resolver != nil {
		s.resolver.InvalidateAll()
	}
}

func (s *Service) currentDeploymentAppWebhookHealth() deploymentAppWebhookHealth {
	if s == nil {
		return deploymentAppWebhookHealth{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	health := s.deploymentAppWebhookHealth
	if health.lastWebhookAt != nil {
		lastWebhookAt := *health.lastWebhookAt
		health.lastWebhookAt = &lastWebhookAt
	}
	return health
}

func (s *Service) updateDeploymentAppWebhookHealth(
	runtime *githubAppRuntime,
	status DeploymentAppWebhookStatus,
	lastWebhookAt time.Time,
	lastError string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deploymentAppRuntime != runtime {
		return
	}
	s.deploymentAppWebhookHealth = deploymentAppWebhookHealth{
		status: status, lastWebhookAt: &lastWebhookAt, lastError: lastError,
	}
}

func (s *Service) DeploymentAppRuntimeSnapshot() DeploymentAppRuntimeSnapshot {
	if s == nil {
		return DeploymentAppRuntimeSnapshot{Source: DeploymentAppSourceNone}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deploymentAppRuntime == nil {
		return DeploymentAppRuntimeSnapshot{Source: DeploymentAppSourceNone}
	}
	runtime := s.deploymentAppRuntime
	return DeploymentAppRuntimeSnapshot{
		Source: runtime.source, Ready: true, AppID: runtime.appID, Generation: runtime.generation,
	}
}

func (s *Service) SetDeploymentAppRegistrationService(
	registration *DeploymentAppRegistrationService,
) {
	s.mu.Lock()
	s.deploymentAppRegistration = registration
	s.mu.Unlock()
}

// InitializeDeploymentAppRegistration resolves the deployment source at boot
// and installs the registration API service even when configuration is absent
// or invalid, so System Settings can report and repair the state.
func (s *Service) InitializeDeploymentAppRegistration(
	ctx context.Context,
	environment config.GitHubAppConfig,
) error {
	if s == nil || s.store == nil || s.connectionSecrets == nil {
		return errors.New("deployment App registration dependencies are not configured")
	}
	registration := NewDeploymentAppRegistrationService(DeploymentAppRegistrationConfig{
		Environment: environment,
		Repository:  NewDeploymentAppRepository(s.store, s.connectionSecrets),
		Store:       s.store,
		Runtime:     s,
		Converter:   NewManifestConversionClient(),
	})
	s.SetDeploymentAppRegistrationService(registration)
	return registration.Boot(ctx)
}

func (s *Service) DeploymentAppRegistrationStatus(
	ctx context.Context,
	userID string,
) (DeploymentAppRegistrationStatus, error) {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return DeploymentAppRegistrationStatus{}, err
	}
	s.mu.Lock()
	if s.mockDeploymentAppStatus != nil {
		status := cloneDeploymentAppRegistrationStatus(*s.mockDeploymentAppStatus)
		s.mu.Unlock()
		return status, nil
	}
	s.mu.Unlock()
	registration := s.currentDeploymentAppRegistrationService()
	if registration == nil {
		return DeploymentAppRegistrationStatus{}, ErrGitHubNotConfigured
	}
	return registration.Status(ctx, userID)
}

func cloneDeploymentAppRegistrationStatus(
	status DeploymentAppRegistrationStatus,
) DeploymentAppRegistrationStatus {
	status.Registration = cloneDeploymentAppRegistration(status.Registration)
	return status
}

func (s *Service) StartDeploymentAppRegistration(
	ctx context.Context,
	userID string,
	request DeploymentAppRegistrationStartRequest,
) (DeploymentAppRegistrationStart, error) {
	registration := s.currentDeploymentAppRegistrationService()
	if registration == nil {
		return DeploymentAppRegistrationStart{}, ErrGitHubNotConfigured
	}
	return registration.Start(ctx, userID, request)
}

func (s *Service) CompleteDeploymentAppRegistration(
	ctx context.Context,
	callback DeploymentAppRegistrationCallback,
) (DeploymentAppRegistrationResult, error) {
	registration := s.currentDeploymentAppRegistrationService()
	if registration == nil {
		return DeploymentAppRegistrationResult{}, ErrGitHubNotConfigured
	}
	return registration.Complete(ctx, callback)
}

func (s *Service) DeleteDeploymentAppRegistration(ctx context.Context, userID string) error {
	registration := s.currentDeploymentAppRegistrationService()
	if registration == nil {
		return ErrGitHubNotConfigured
	}
	return registration.Delete(ctx, userID)
}

// ResetDeploymentAppForE2E clears mock-controlled deployment registration
// persistence. It is called only from endpoints gated by KANDEV_MOCK_AGENT.
func (s *Service) ResetDeploymentAppForE2E(ctx context.Context) error {
	registration := s.currentDeploymentAppRegistrationService()
	if registration == nil {
		return nil
	}
	return registration.resetForE2E(ctx)
}

func (s *Service) currentDeploymentAppRegistrationService() *DeploymentAppRegistrationService {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deploymentAppRegistration
}

func (s *Service) ConfigureGitHubAppAuth(config GitHubAppRuntimeConfig) error {
	if s == nil || s.store == nil || s.connectionSecrets == nil || s.appClient == nil {
		return errors.New("GitHub App authentication dependencies are not configured")
	}
	s.mu.Lock()
	appClient := s.appClient
	s.mu.Unlock()
	resolved := ResolvedDeploymentAppConfig{
		Source: DeploymentAppSourceEnvironment,
		Config: legacyGitHubAppConfig(config, appClient),
	}
	runtime, err := s.buildDeploymentAppRuntimeWithClient(resolved, appClient)
	if err != nil {
		return err
	}
	s.swapDeploymentAppRuntime(runtime)
	return nil
}

func legacyGitHubAppConfig(runtime GitHubAppRuntimeConfig, appClient *AppClient) config.GitHubAppConfig {
	appID := int64(1)
	if appClient != nil {
		appID = appClient.appID
	}
	return config.GitHubAppConfig{
		AppID: appID, ClientID: runtime.ClientID, ClientSecret: runtime.ClientSecret,
		WebhookSecret: runtime.WebhookSecret, Slug: runtime.Slug, PublicBaseURL: runtime.PublicBaseURL,
	}
}

type runtimeInstallationCredentialProvider struct{ service *Service }

func (p *runtimeInstallationCredentialProvider) ResolveInstallation(
	ctx context.Context,
	connection *WorkspaceConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	runtime := p.service.currentDeploymentAppRuntime()
	if runtime == nil || runtime.installationProvider == nil {
		return nil, ErrGitHubNotConfigured
	}
	return runtime.installationProvider.ResolveInstallation(ctx, connection, req)
}

type runtimeUserCredentialProvider struct{ service *Service }

func (p *runtimeUserCredentialProvider) ResolveUser(
	ctx context.Context,
	connection *UserConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	runtime := p.service.currentDeploymentAppRuntime()
	if runtime == nil || runtime.personalAuth == nil {
		return nil, ErrGitHubPersonalRequired
	}
	return (&personalAuthCredentialProvider{service: runtime.personalAuth}).ResolveUser(ctx, connection, req)
}

func (s *Service) currentDeploymentAppRuntime() *githubAppRuntime {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deploymentAppRuntime
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
	s.deploymentAppMutationMu.Lock()
	defer s.deploymentAppMutationMu.Unlock()
	runtime := s.currentDeploymentAppRuntime()
	if runtime == nil {
		return AppInstallationResult{}, ErrGitHubNotConfigured
	}
	service := runtime.appInstallationAuth
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
