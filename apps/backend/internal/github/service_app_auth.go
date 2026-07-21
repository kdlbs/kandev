package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type githubAppRuntime struct {
	registrationID       string
	source               DeploymentAppSource
	generation           int64
	appID                int64
	appClient            *AppClient
	installationTokens   *InstallationTokenCache
	installationProvider *CachedInstallationCredentialProvider
	appInstallationAuth  *AppInstallationService
	personalAuth         *PersonalAuthService
	webhookAuth          *GitHubWebhookService
}

type DeploymentAppRuntimeSnapshot struct {
	RegistrationID string
	Source         DeploymentAppSource
	Ready          bool
	AppID          int64
	Generation     int64
}

func (s *Service) ValidateAppRegistrationRuntime(resolved ResolvedDeploymentAppConfig) error {
	_, err := s.buildDeploymentAppRuntime(resolved)
	return err
}

// ApplyAppRegistrationRuntime builds a complete App runtime before publishing
// it under its registration ID. A failure leaves every existing registration
// generation untouched.
func (s *Service) ApplyAppRegistrationRuntime(resolved ResolvedDeploymentAppConfig) error {
	if s == nil {
		return errors.New("GitHub service is not configured")
	}
	if resolved.Registration == nil || strings.TrimSpace(resolved.Registration.ID) == "" {
		return errors.New("GitHub App registration ID is required")
	}
	runtime, err := s.buildDeploymentAppRuntime(resolved)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.appRegistrationRuntimes == nil {
		s.appRegistrationRuntimes = make(map[string]*githubAppRuntime)
	}
	s.appRegistrationRuntimes[runtime.registrationID] = runtime
	s.mu.Unlock()
	if s.resolver != nil {
		s.resolver.InvalidateAppRegistration(runtime.registrationID)
	}
	return nil
}

// InvalidateAppRegistrationRuntime removes only the expected generation. A
// delayed invalidation cannot tear down a newer key rotation.
func (s *Service) InvalidateAppRegistrationRuntime(registrationID string, generation int64) {
	if s == nil || strings.TrimSpace(registrationID) == "" {
		return
	}
	s.mu.Lock()
	runtime := s.appRegistrationRuntimes[registrationID]
	if runtime != nil && (generation <= 0 || runtime.generation == generation) {
		delete(s.appRegistrationRuntimes, registrationID)
	}
	s.mu.Unlock()
	if runtime != nil && (generation <= 0 || runtime.generation == generation) && s.resolver != nil {
		s.resolver.InvalidateAppRegistration(registrationID)
	}
}

func (s *Service) AppRegistrationRuntimeSnapshot(registrationID string) DeploymentAppRuntimeSnapshot {
	runtime := s.currentAppRegistrationRuntime(registrationID)
	if runtime == nil {
		return DeploymentAppRuntimeSnapshot{RegistrationID: registrationID, Source: DeploymentAppSourceNone}
	}
	return DeploymentAppRuntimeSnapshot{
		RegistrationID: registrationID, Source: runtime.source, Ready: true,
		AppID: runtime.appID, Generation: runtime.generation,
	}
}

func (s *Service) currentAppRegistrationRuntime(registrationID string) *githubAppRuntime {
	if s == nil || strings.TrimSpace(registrationID) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appRegistrationRuntimes[registrationID]
}

// InitializeAppRegistrationRuntimes independently loads every catalog entry.
// Invalid entries are reported together after all valid entries are active.
func (s *Service) InitializeAppRegistrationRuntimes(ctx context.Context) error {
	if s == nil || s.store == nil || s.connectionSecrets == nil {
		return errors.New("GitHub App registration dependencies are not configured")
	}
	repository := NewAppRegistrationRepository(s.store, s.connectionSecrets)
	registrations, err := s.store.ListAppRegistrations(ctx)
	if err != nil {
		return err
	}
	var loadErrors []error
	for _, registration := range registrations {
		if registration == nil {
			continue
		}
		resolved, resolveErr := ResolveAppRegistrationConfig(ctx, registration.ID, repository)
		if resolveErr == nil {
			resolveErr = s.ApplyAppRegistrationRuntime(resolved)
		}
		if resolveErr != nil {
			loadErrors = append(loadErrors, fmt.Errorf("load GitHub App registration %s: %w", registration.ID, resolveErr))
		}
	}
	if cleanupErr := repository.CleanupOrphanedCredentialBundles(ctx); cleanupErr != nil {
		loadErrors = append(loadErrors, cleanupErr)
	}
	return errors.Join(loadErrors...)
}

func (s *Service) buildDeploymentAppRuntime(
	resolved ResolvedDeploymentAppConfig,
) (*githubAppRuntime, error) {
	if s.store == nil || s.connectionSecrets == nil {
		return nil, errors.New("GitHub App authentication dependencies are not configured")
	}
	if resolved.Source != DeploymentAppSourceManaged {
		return nil, errors.New("GitHub App runtime source is invalid")
	}
	if resolved.Registration == nil || strings.TrimSpace(resolved.Registration.ID) == "" {
		return nil, errors.New("GitHub App registration ID is required")
	}
	if resolved.Registration.Status != AppRegistrationStatusActive {
		return nil, errors.New("GitHub App registration is not active")
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
		RegistrationID: resolved.Registration.ID,
		ClientID:       config.ClientID,
		CallbackURL:    baseURL + "/api/v1/github/app/registrations/" + resolved.Registration.ID + "/personal/callback",
	}, flows, personalRepo, oauth)
	personal.SetWorkspaceMutationLock(s.workspaceConnectionMutationLock)
	registrationID := resolved.Registration.ID
	generation := resolved.Registration.CredentialGeneration
	tokens := NewAppInstallationTokenCache(registrationID, generation, appClient)
	return &githubAppRuntime{
		registrationID: registrationID, source: resolved.Source, generation: generation, appID: config.AppID,
		appClient: appClient, installationTokens: tokens,
		installationProvider: NewCachedInstallationCredentialProvider(tokens),
		appInstallationAuth: NewAppInstallationService(AppInstallationConfig{
			RegistrationID: registrationID, Slug: config.Slug,
			CallbackURL: baseURL + "/api/v1/github/app/registrations/" + registrationID + "/install/callback",
		}, flows, connectionStore, appClient, oauth),
		personalAuth: personal,
		webhookAuth: NewAppRegistrationWebhookService(
			registrationID, config.WebhookSecret, connectionStore,
			&installationRepositorySettingsUpdater{service: s}, personalRepo,
			GitHubWebhookReconciliation{Installations: appClient, Personal: personal},
		),
	}, nil
}

func (s *Service) InitializeAppRegistrationLifecycle() error {
	if s == nil || s.store == nil || s.connectionSecrets == nil {
		return errors.New("GitHub App registration dependencies are not configured")
	}
	repository := NewAppRegistrationRepository(s.store, s.connectionSecrets)
	s.mu.Lock()
	s.appRegistrationLifecycle = NewAppRegistrationLifecycleService(AppRegistrationLifecycleConfig{
		Repository: repository, Store: s.store, Runtime: s,
		Converter: NewManifestConversionClient(),
	})
	s.mu.Unlock()
	return nil
}

func (s *Service) currentAppRegistrationLifecycle() *AppRegistrationLifecycleService {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appRegistrationLifecycle
}

func (s *Service) ListAppRegistrationCatalog(
	ctx context.Context,
	userID, workspaceID string,
) (AppRegistrationCatalog, error) {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return AppRegistrationCatalog{}, ErrGitHubNotConfigured
	}
	return lifecycle.List(ctx, userID, workspaceID)
}

func (s *Service) StartAppRegistrationManifest(
	ctx context.Context,
	userID string,
	request AppRegistrationManifestStartRequest,
) (AppRegistrationManifestStart, error) {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return AppRegistrationManifestStart{}, ErrGitHubNotConfigured
	}
	return lifecycle.StartManifest(ctx, userID, request)
}

func (s *Service) CompleteAppRegistrationManifest(
	ctx context.Context,
	registrationID string,
	callback AppRegistrationManifestCallback,
) (AppRegistrationManifestResult, error) {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return AppRegistrationManifestResult{}, ErrGitHubNotConfigured
	}
	return lifecycle.CompleteManifest(ctx, registrationID, callback)
}

func (s *Service) ImportAppRegistration(
	ctx context.Context,
	userID string,
	request AppRegistrationImportRequest,
) (*AppRegistration, error) {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return nil, ErrGitHubNotConfigured
	}
	return lifecycle.Import(ctx, userID, request)
}

func (s *Service) PrepareAppRegistrationImport(
	ctx context.Context,
	userID string,
	request AppRegistrationImportPrepareRequest,
) (AppRegistrationImportPreparationResult, error) {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return AppRegistrationImportPreparationResult{}, ErrGitHubNotConfigured
	}
	return lifecycle.PrepareImport(ctx, userID, request)
}

func (s *Service) RenameAppRegistration(
	ctx context.Context,
	userID, registrationID, displayName string,
) (*AppRegistration, error) {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return nil, ErrGitHubNotConfigured
	}
	return lifecycle.Rename(ctx, userID, registrationID, displayName)
}

func (s *Service) DeleteAppRegistration(
	ctx context.Context,
	userID, registrationID string,
) error {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return ErrGitHubNotConfigured
	}
	return lifecycle.Delete(ctx, userID, registrationID)
}

func (s *Service) ResetAppRegistrationsForE2E(ctx context.Context, workspaceID string) error {
	lifecycle := s.currentAppRegistrationLifecycle()
	if lifecycle == nil {
		return nil
	}
	return lifecycle.ResetForE2E(ctx, workspaceID)
}

type runtimeInstallationCredentialProvider struct{ service *Service }

func (p *runtimeInstallationCredentialProvider) ResolveInstallation(
	ctx context.Context,
	connection *WorkspaceConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if connection == nil || strings.TrimSpace(connection.AppRegistrationID) == "" {
		return nil, ErrGitHubNotConfigured
	}
	runtime := p.service.currentAppRegistrationRuntime(connection.AppRegistrationID)
	if runtime == nil || runtime.installationProvider == nil {
		return nil, ErrGitHubNotConfigured
	}
	return runtime.installationProvider.ResolveInstallation(ctx, connection, req)
}

func (p *runtimeInstallationCredentialProvider) AppCredentialGeneration(
	registrationID string,
) (int64, bool) {
	runtime := p.service.currentAppRegistrationRuntime(registrationID)
	if runtime == nil {
		return 0, false
	}
	return runtime.generation, true
}

type runtimeUserCredentialProvider struct{ service *Service }

func (p *runtimeUserCredentialProvider) ResolveUser(
	ctx context.Context,
	connection *UserConnection,
	req ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if connection == nil || strings.TrimSpace(connection.AppRegistrationID) == "" {
		return nil, ErrGitHubPersonalRequired
	}
	runtime := p.service.currentAppRegistrationRuntime(connection.AppRegistrationID)
	if runtime == nil || runtime.personalAuth == nil {
		return nil, ErrGitHubPersonalRequired
	}
	return (&personalAuthCredentialProvider{service: runtime.personalAuth}).ResolveUser(ctx, connection, req)
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
	AppRegistration              *AppRegistration           `json:"app_registration,omitempty"`
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
	if connection == nil {
		return status, nil
	}
	status.AuthMethod = string(connection.Source)
	status.TokenConfigured = connection.Source == ConnectionSourcePAT
	status.Automation = &WorkspaceAutomationStatus{
		WorkspaceConnection: connection,
		LegacyMigration:     connection.Source == ConnectionSourceLegacyShared,
	}
	if connection.Source == ConnectionSourceGitHubAppInstallation && connection.AppRegistrationID != "" {
		registration, registrationErr := s.store.GetAppRegistration(ctx, connection.AppRegistrationID)
		if registrationErr != nil {
			return nil, registrationErr
		}
		if registration != nil {
			status.AppRegistration = cloneDeploymentAppRegistration(registration)
			runtime := s.AppRegistrationRuntimeSnapshot(registration.ID)
			status.GitHubAppAvailable = registration.Status == AppRegistrationStatusActive && runtime.Ready
			status.AppAvailable = status.GitHubAppAvailable
		}
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
	workspaceID, userID, registrationID string,
) (AppInstallationStart, error) {
	runtime := s.currentAppRegistrationRuntime(registrationID)
	if runtime == nil || runtime.appInstallationAuth == nil {
		return AppInstallationStart{}, ErrGitHubNotConfigured
	}
	return runtime.appInstallationAuth.Start(ctx, workspaceID, userID)
}

func (s *Service) CompleteAppInstallation(
	ctx context.Context,
	registrationID string,
	callback AppInstallationCallback,
) (AppInstallationResult, error) {
	runtime := s.currentAppRegistrationRuntime(registrationID)
	if runtime == nil || runtime.appInstallationAuth == nil {
		return AppInstallationResult{}, ErrGitHubNotConfigured
	}
	return runtime.appInstallationAuth.Complete(ctx, callback)
}

func (s *Service) StartPersonalAuth(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalAuthStart, error) {
	connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return PersonalAuthStart{}, err
	}
	if connection == nil || connection.Source != ConnectionSourceGitHubAppInstallation ||
		strings.TrimSpace(connection.AppRegistrationID) == "" {
		return PersonalAuthStart{}, ErrGitHubPersonalRequired
	}
	runtime := s.currentAppRegistrationRuntime(connection.AppRegistrationID)
	if runtime == nil || runtime.personalAuth == nil {
		return PersonalAuthStart{}, ErrGitHubNotConfigured
	}
	return runtime.personalAuth.Start(ctx, workspaceID, userID)
}

func (s *Service) CompletePersonalAuth(
	ctx context.Context,
	registrationID string,
	callback PersonalAuthCallback,
) (PersonalAuthResult, error) {
	runtime := s.currentAppRegistrationRuntime(registrationID)
	if runtime == nil || runtime.personalAuth == nil {
		return PersonalAuthResult{}, ErrGitHubNotConfigured
	}
	return runtime.personalAuth.Complete(ctx, callback)
}

func (s *Service) DisconnectPersonalAuth(ctx context.Context, workspaceID, userID string) error {
	if s == nil || s.personalConnections == nil {
		return nil
	}
	return s.personalConnections.RevokePersonalConnection(ctx, workspaceID, userID)
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
	if err := r.service.applyAutomationTransition(ctx, existing, connection, func() error {
		return r.service.store.UpsertWorkspaceConnection(ctx, connection)
	}); err != nil {
		return fmt.Errorf("replace workspace GitHub connection: %w", err)
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
		r.service.InvalidateAppInstallationCredentials(existing.AppRegistrationID, *existing.InstallationID)
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

func (r *serviceAppConnectionStore) ListWorkspaceConnectionsByAppInstallation(
	ctx context.Context,
	registrationID string,
	installationID int64,
) ([]*WorkspaceConnection, error) {
	return r.service.store.ListWorkspaceConnectionsByAppInstallation(
		ctx, registrationID, installationID,
	)
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
		r.service.InvalidateAppInstallationCredentials(expected.AppRegistrationID, *expected.InstallationID)
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

func (r *serviceAppConnectionStore) ListUserConnectionsByAppGitHubUser(
	ctx context.Context,
	registrationID string,
	githubUserID int64,
) ([]*UserConnection, error) {
	return r.service.store.ListUserConnectionsByAppGitHubUser(ctx, registrationID, githubUserID)
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

func (r *serviceAppConnectionStore) CompleteAppRegistrationWebhookDelivery(
	ctx context.Context,
	registrationID, deliveryID string,
	status WebhookDeliveryStatus,
	result string,
	processedAt time.Time,
) error {
	return r.service.store.CompleteAppRegistrationWebhookDelivery(
		ctx, registrationID, deliveryID, status, result, processedAt,
	)
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
		AppRegistrationID:    change.AppRegistrationID,
	}) || connection.Status != ConnectionStatusActive {
		return false, nil
	}
	u.service.InvalidateAppInstallationCredentials(change.AppRegistrationID, change.InstallationID)
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
