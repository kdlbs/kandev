package github

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"go.uber.org/zap"
)

var (
	ErrDeploymentAppOperatorRequired    = errors.New("deployment GitHub App operator access is required")
	ErrDeploymentAppEnvironmentReadOnly = errors.New(
		"environment-managed GitHub App configuration is read-only",
	)
	ErrDeploymentAppIdentityMismatch      = errors.New("GitHub App manifest result did not match the requested owner")
	ErrDeploymentAppPolicyMismatch        = errors.New("GitHub App manifest result did not match the required policy")
	ErrDeploymentAppRegistrationCancelled = errors.New("GitHub App registration was cancelled")
)

const (
	defaultGitHubAppHost           = "github.com"
	deploymentAppNotConfiguredCode = "github_app_not_configured"
)

type DeploymentAppManifestConverter interface {
	Convert(context.Context, string) (ManifestConversionResult, error)
}

type DeploymentAppRegistrationConfig struct {
	Environment config.GitHubAppConfig
	Repository  *DeploymentAppRepository
	Store       *Store
	Runtime     *Service
	Converter   DeploymentAppManifestConverter
	Resolver    PublicGitHubBaseURLResolver
	Now         func() time.Time
	Random      io.Reader
}

type DeploymentAppRegistrationService struct {
	environment config.GitHubAppConfig
	repository  *DeploymentAppRepository
	store       *Store
	runtime     *Service
	converter   DeploymentAppManifestConverter
	resolver    PublicGitHubBaseURLResolver
	now         func() time.Time
	random      io.Reader
}

type DeploymentAppRegistrationStatus struct {
	Source            DeploymentAppSource        `json:"source"`
	State             string                     `json:"state"`
	Ready             bool                       `json:"ready"`
	ReadOnly          bool                       `json:"read_only"`
	Registration      *DeploymentAppRegistration `json:"registration,omitempty"`
	CallbackURL       string                     `json:"callback_url,omitempty"`
	WebhookURL        string                     `json:"webhook_url,omitempty"`
	UnavailableCode   string                     `json:"unavailable_code,omitempty"`
	UnavailableReason string                     `json:"unavailable_reason,omitempty"`
}

type DeploymentAppRegistrationStartRequest struct {
	OwnerType     ManifestOwnerType `json:"owner_type"`
	OwnerLogin    string            `json:"owner_login"`
	PublicBaseURL string            `json:"public_base_url"`
}

type DeploymentAppRegistrationStart struct {
	State           string                `json:"state"`
	ExpiresAt       time.Time             `json:"expires_at"`
	Revision        int                   `json:"revision"`
	RegistrationURL string                `json:"registration_url"`
	Manifest        DeploymentAppManifest `json:"manifest"`
}

type DeploymentAppRegistrationCallback struct {
	State string
	Code  string
	Error string
}

type DeploymentAppRegistrationResult struct {
	Source DeploymentAppSource `json:"source"`
	AppID  int64               `json:"app_id"`
	Slug   string              `json:"slug"`
}

func NewDeploymentAppRegistrationService(cfg DeploymentAppRegistrationConfig) *DeploymentAppRegistrationService {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}
	return &DeploymentAppRegistrationService{
		environment: cfg.Environment, repository: cfg.Repository, store: cfg.Store,
		runtime: cfg.Runtime, converter: cfg.Converter, resolver: cfg.Resolver,
		now: now, random: random,
	}
}

func (s *DeploymentAppRegistrationService) Boot(ctx context.Context) error {
	if err := s.ready(); err != nil {
		return err
	}
	resolved, err := ResolveDeploymentAppConfig(ctx, s.environment, s.repository)
	if err != nil {
		return err
	}
	s.runtime.deploymentAppMutationMu.Lock()
	err = s.runtime.ApplyDeploymentAppRuntime(resolved)
	s.runtime.deploymentAppMutationMu.Unlock()
	if err != nil {
		return err
	}
	if cleanupErr := s.repository.CleanupOrphanedCredentialBundles(ctx); cleanupErr != nil &&
		s.runtime.logger != nil {
		s.runtime.logger.Warn(
			"GitHub deployment App orphan cleanup failed",
			zap.Error(cleanupErr),
		)
	}
	return nil
}

func (s *DeploymentAppRegistrationService) Status(
	ctx context.Context,
	operatorUserID string,
) (DeploymentAppRegistrationStatus, error) {
	if err := requireDeploymentAppOperator(operatorUserID); err != nil {
		return DeploymentAppRegistrationStatus{}, err
	}
	if err := s.ready(); err != nil {
		return DeploymentAppRegistrationStatus{}, err
	}
	resolved, err := ResolveDeploymentAppConfig(ctx, s.environment, s.repository)
	status := deploymentAppStatusFromResolution(resolved, err)
	if err == nil {
		registering, flowErr := s.hasActiveRegistrationFlow(ctx)
		if flowErr != nil {
			return DeploymentAppRegistrationStatus{}, flowErr
		}
		if registering {
			status.State = "registering"
		}
	}
	if status.Registration != nil {
		if status.Ready {
			health := s.runtime.currentDeploymentAppWebhookHealth()
			status.Registration.WebhookStatus = health.status
			status.Registration.LastWebhookAt = health.lastWebhookAt
			status.Registration.LastError = health.lastError
		}
		status.CallbackURL = strings.TrimRight(status.Registration.PublicBaseURL, "/") +
			"/api/v1/github/app/registration/callback"
		status.WebhookURL = strings.TrimRight(status.Registration.PublicBaseURL, "/") +
			"/api/v1/github/app/webhook"
	}
	return status, nil
}

func deploymentAppStatusFromResolution(
	resolved ResolvedDeploymentAppConfig,
	resolveErr error,
) DeploymentAppRegistrationStatus {
	status := DeploymentAppRegistrationStatus{
		Source: resolved.Source, ReadOnly: resolved.Source == DeploymentAppSourceEnvironment,
	}
	if resolved.Source == DeploymentAppSourceManaged {
		status.Registration = cloneDeploymentAppRegistration(resolved.Registration)
	}
	if resolveErr != nil {
		status.State = "invalid"
		status.UnavailableCode = deploymentAppUnavailableCode(resolved.Source, resolveErr)
		status.UnavailableReason = deploymentAppUnavailableReason(status.UnavailableCode)
		return status
	}
	if resolved.Source == DeploymentAppSourceEnvironment {
		status.Registration = &DeploymentAppRegistration{
			GitHubHost: defaultGitHubAppHost, AppID: resolved.Config.AppID,
			ClientID: resolved.Config.ClientID, Slug: resolved.Config.Slug,
			PublicBaseURL: resolved.Config.PublicBaseURL,
		}
	}
	status.Ready = resolved.Source != DeploymentAppSourceNone
	if !status.Ready {
		status.State = "unconfigured"
		status.UnavailableCode = deploymentAppNotConfiguredCode
		status.UnavailableReason = deploymentAppUnavailableReason(status.UnavailableCode)
	}
	if status.Ready {
		status.State = "ready"
	}
	return status
}

func (s *DeploymentAppRegistrationService) hasActiveRegistrationFlow(ctx context.Context) (bool, error) {
	var count int
	err := s.store.ro.GetContext(ctx, &count, s.store.ro.Rebind(`
		SELECT COUNT(*) FROM github_app_registration_flows
		WHERE consumed_at IS NULL AND expires_at > ?`), s.now().UTC())
	if err != nil {
		return false, fmt.Errorf("load deployment App manifest state: %w", err)
	}
	return count > 0, nil
}

func (s *Store) updateDeploymentAppWebhookHealth(
	ctx context.Context,
	generation int64,
	status DeploymentAppWebhookStatus,
	lastWebhookAt time.Time,
	lastError string,
) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE github_app_registration
		SET webhook_status = ?, last_webhook_at = ?, last_error = ?, updated_at = ?
		WHERE singleton_id = ? AND credential_generation = ?`),
		status, lastWebhookAt, nullString(lastError), time.Now().UTC(),
		deploymentAppSingletonID, generation,
	)
	return err
}

func (s *DeploymentAppRegistrationService) Start(
	ctx context.Context,
	operatorUserID string,
	request DeploymentAppRegistrationStartRequest,
) (DeploymentAppRegistrationStart, error) {
	if err := requireDeploymentAppOperator(operatorUserID); err != nil {
		return DeploymentAppRegistrationStart{}, err
	}
	if err := s.ready(); err != nil {
		return DeploymentAppRegistrationStart{}, err
	}
	if s.environment.Configured() {
		return DeploymentAppRegistrationStart{}, ErrDeploymentAppEnvironmentReadOnly
	}
	bindings, err := s.store.CountDeploymentAppWorkspaceBindings(ctx)
	if err != nil {
		return DeploymentAppRegistrationStart{}, fmt.Errorf("check deployment App bindings: %w", err)
	}
	if bindings > 0 {
		return DeploymentAppRegistrationStart{}, ErrDeploymentAppInUse
	}
	baseURL, err := ValidatePublicGitHubBaseURL(ctx, request.PublicBaseURL, s.resolver)
	if err != nil {
		return DeploymentAppRegistrationStart{}, err
	}
	submission, err := BuildDeploymentAppManifest(request.OwnerType, request.OwnerLogin, baseURL)
	if err != nil {
		return DeploymentAppRegistrationStart{}, err
	}
	state, err := randomBase64URL(s.random)
	if err != nil {
		return DeploymentAppRegistrationStart{}, errors.New("generate deployment App manifest state")
	}
	now := s.now().UTC()
	digest := sha256.Sum256([]byte(state))
	flow := &DeploymentAppRegistrationFlow{
		StateHash: stateDigestString(digest), OperatorUserID: operatorUserID,
		OwnerType: deploymentOwnerType(request.OwnerType), OwnerLogin: request.OwnerLogin,
		PublicBaseURL: baseURL, ManifestRevision: submission.Revision,
		ExpiresAt: DeploymentAppManifestFlowExpiresAt(now), CreatedAt: now,
	}
	s.runtime.deploymentAppMutationMu.Lock()
	err = s.store.CreateDeploymentAppRegistrationFlow(ctx, flow)
	s.runtime.deploymentAppMutationMu.Unlock()
	if err != nil {
		return DeploymentAppRegistrationStart{}, fmt.Errorf("persist deployment App manifest state: %w", err)
	}
	callback, err := url.Parse(submission.Manifest.RedirectURL)
	if err != nil {
		return DeploymentAppRegistrationStart{}, errors.New("build deployment App callback")
	}
	query := callback.Query()
	query.Set("state", state)
	callback.RawQuery = query.Encode()
	submission.Manifest.RedirectURL = callback.String()
	return DeploymentAppRegistrationStart{
		State: state, ExpiresAt: flow.ExpiresAt, Revision: submission.Revision,
		RegistrationURL: submission.RegistrationURL, Manifest: submission.Manifest,
	}, nil
}

func (s *DeploymentAppRegistrationService) Complete(
	ctx context.Context,
	callback DeploymentAppRegistrationCallback,
) (DeploymentAppRegistrationResult, error) {
	if err := s.ready(); err != nil {
		return DeploymentAppRegistrationResult{}, err
	}
	if s.environment.Configured() {
		return DeploymentAppRegistrationResult{}, ErrDeploymentAppEnvironmentReadOnly
	}
	flow, err := s.consumeRegistrationFlow(ctx, callback.State)
	if err != nil {
		return DeploymentAppRegistrationResult{}, err
	}
	if strings.TrimSpace(callback.Error) != "" || strings.TrimSpace(callback.Code) == "" {
		return DeploymentAppRegistrationResult{}, ErrDeploymentAppRegistrationCancelled
	}
	if s.converter == nil {
		return DeploymentAppRegistrationResult{}, errors.New("deployment App manifest conversion is unavailable")
	}
	converted, err := s.convertAndValidate(ctx, flow, callback.Code)
	if err != nil {
		return DeploymentAppRegistrationResult{}, err
	}
	return s.activateConvertedRegistration(ctx, flow, converted)
}

func (s *DeploymentAppRegistrationService) consumeRegistrationFlow(
	ctx context.Context,
	state string,
) (*DeploymentAppRegistrationFlow, error) {
	digest := sha256.Sum256([]byte(strings.TrimSpace(state)))
	flow, err := s.store.ConsumeDeploymentAppRegistrationFlow(
		ctx, stateDigestString(digest), s.now().UTC(),
	)
	if err != nil {
		if errors.Is(err, ErrDeploymentAppFlowUnavailable) {
			return nil, ErrDeploymentAppManifestStateUnavailable
		}
		return nil, fmt.Errorf("consume deployment App manifest state: %w", err)
	}
	if flow == nil || flow.OperatorUserID != DefaultUserID ||
		flow.ManifestRevision != DeploymentAppManifestRevision {
		return nil, ErrDeploymentAppManifestStateUnavailable
	}
	return flow, nil
}

func (s *DeploymentAppRegistrationService) convertAndValidate(
	ctx context.Context,
	flow *DeploymentAppRegistrationFlow,
	code string,
) (ManifestConversionResult, error) {
	converted, err := s.converter.Convert(ctx, strings.TrimSpace(code))
	if err != nil {
		return ManifestConversionResult{}, err
	}
	if !matchesDeploymentAppOwner(flow, converted) {
		return ManifestConversionResult{}, ErrDeploymentAppIdentityMismatch
	}
	if !matchesDeploymentAppPolicy(converted) {
		return ManifestConversionResult{}, ErrDeploymentAppPolicyMismatch
	}
	return converted, nil
}

func (s *DeploymentAppRegistrationService) activateConvertedRegistration(
	ctx context.Context,
	flow *DeploymentAppRegistrationFlow,
	converted ManifestConversionResult,
) (DeploymentAppRegistrationResult, error) {
	s.runtime.deploymentAppMutationMu.Lock()
	defer s.runtime.deploymentAppMutationMu.Unlock()
	latest, err := s.store.IsLatestDeploymentAppRegistrationFlow(ctx, flow.StateHash)
	if err != nil {
		return DeploymentAppRegistrationResult{}, fmt.Errorf("verify deployment App manifest state: %w", err)
	}
	if !latest {
		return DeploymentAppRegistrationResult{}, ErrDeploymentAppManifestStateUnavailable
	}
	previous, err := s.store.GetDeploymentAppRegistration(ctx)
	if err != nil {
		return DeploymentAppRegistrationResult{}, fmt.Errorf("load deployment App registration: %w", err)
	}
	generation := int64(1)
	if previous != nil {
		generation = previous.CredentialGeneration + 1
	}
	registration := &DeploymentAppRegistration{
		GitHubHost: defaultGitHubAppHost, AppID: converted.AppID, ClientID: converted.ClientID,
		Slug: converted.Slug, OwnerLogin: converted.Owner.Login,
		OwnerType:     deploymentOwnerTypeFromGitHub(converted.Owner.Type),
		PublicBaseURL: flow.PublicBaseURL, CredentialGeneration: generation,
		WebhookStatus: DeploymentAppWebhookUnverified,
	}
	credentials := DeploymentAppCredentials{
		PrivateKey: converted.PrivateKeyPEM, ClientSecret: converted.ClientSecret,
		WebhookSecret: converted.WebhookSecret,
	}
	resolved := resolvedDeploymentAppRegistration(registration, credentials)
	runtime, err := s.runtime.buildDeploymentAppRuntime(resolved)
	if err != nil {
		return DeploymentAppRegistrationResult{}, sanitizeDeploymentAppActivationError(err)
	}
	if err := s.repository.SaveManagedRegistration(ctx, registration, credentials); err != nil {
		return DeploymentAppRegistrationResult{}, sanitizeDeploymentAppPersistenceError(err)
	}
	s.runtime.swapDeploymentAppRuntime(runtime)
	return DeploymentAppRegistrationResult{
		Source: DeploymentAppSourceManaged, AppID: registration.AppID, Slug: registration.Slug,
	}, nil
}

func (s *DeploymentAppRegistrationService) Delete(ctx context.Context, operatorUserID string) error {
	if err := requireDeploymentAppOperator(operatorUserID); err != nil {
		return err
	}
	if err := s.ready(); err != nil {
		return err
	}
	if s.environment.Configured() {
		return ErrDeploymentAppEnvironmentReadOnly
	}
	s.runtime.deploymentAppMutationMu.Lock()
	defer s.runtime.deploymentAppMutationMu.Unlock()
	bindings, err := s.store.CountDeploymentAppWorkspaceBindings(ctx)
	if err != nil {
		return fmt.Errorf("check deployment App bindings: %w", err)
	}
	if bindings > 0 {
		return ErrDeploymentAppInUse
	}
	if err := s.store.DeleteDeploymentAppRegistrationFlows(ctx); err != nil {
		return fmt.Errorf("delete deployment App registration flows: %w", err)
	}
	if err := s.repository.DeleteManagedRegistration(ctx); err != nil {
		return err
	}
	s.runtime.swapDeploymentAppRuntime(nil)
	return nil
}

func (s *DeploymentAppRegistrationService) resetForE2E(ctx context.Context) error {
	if err := s.ready(); err != nil {
		return err
	}
	s.runtime.deploymentAppMutationMu.Lock()
	defer s.runtime.deploymentAppMutationMu.Unlock()
	if err := s.store.DeleteDeploymentAppRegistrationFlows(ctx); err != nil {
		return fmt.Errorf("delete deployment App registration flows: %w", err)
	}
	if !s.environment.Configured() {
		if err := s.repository.DeleteManagedRegistration(ctx); err != nil {
			return err
		}
		if err := s.repository.CleanupOrphanedCredentialBundles(ctx); err != nil {
			return err
		}
		s.runtime.swapDeploymentAppRuntime(nil)
	}
	return nil
}

func (s *DeploymentAppRegistrationService) ready() error {
	if s == nil || s.repository == nil || s.store == nil || s.runtime == nil {
		return errors.New("deployment App registration service is not configured")
	}
	return nil
}

func requireDeploymentAppOperator(userID string) error {
	if userID != DefaultUserID {
		return ErrDeploymentAppOperatorRequired
	}
	return nil
}

func deploymentOwnerType(ownerType ManifestOwnerType) DeploymentAppOwnerType {
	if ownerType == ManifestOwnerOrganization {
		return DeploymentAppOwnerOrganization
	}
	return DeploymentAppOwnerUser
}

func deploymentOwnerTypeFromGitHub(ownerType string) DeploymentAppOwnerType {
	if strings.EqualFold(ownerType, string(DeploymentAppOwnerOrganization)) {
		return DeploymentAppOwnerOrganization
	}
	return DeploymentAppOwnerUser
}

func matchesDeploymentAppOwner(
	flow *DeploymentAppRegistrationFlow,
	converted ManifestConversionResult,
) bool {
	return flow != nil && strings.EqualFold(flow.OwnerLogin, converted.Owner.Login) &&
		strings.EqualFold(string(flow.OwnerType), converted.Owner.Type)
}

func matchesDeploymentAppPolicy(converted ManifestConversionResult) bool {
	submission, err := BuildDeploymentAppManifest(
		ManifestOwnerUser, "policy", "https://kandev.example",
	)
	if err != nil {
		return false
	}
	if len(converted.Permissions) != len(submission.Manifest.DefaultPermissions) ||
		len(converted.Events) != len(submission.Manifest.DefaultEvents) {
		return false
	}
	for permission, level := range submission.Manifest.DefaultPermissions {
		if converted.Permissions[permission] != level {
			return false
		}
	}
	for _, event := range submission.Manifest.DefaultEvents {
		if !slices.Contains(converted.Events, event) {
			return false
		}
	}
	return true
}

func resolvedDeploymentAppRegistration(
	registration *DeploymentAppRegistration,
	credentials DeploymentAppCredentials,
) ResolvedDeploymentAppConfig {
	return ResolvedDeploymentAppConfig{
		Source: DeploymentAppSourceManaged, Registration: registration,
		Config: config.GitHubAppConfig{
			AppID: registration.AppID, ClientID: registration.ClientID,
			ClientSecret: credentials.ClientSecret, PrivateKey: credentials.PrivateKey,
			WebhookSecret: credentials.WebhookSecret, Slug: registration.Slug,
			PublicBaseURL: registration.PublicBaseURL,
		},
	}
}

func cloneDeploymentAppRegistration(value *DeploymentAppRegistration) *DeploymentAppRegistration {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func deploymentAppUnavailableCode(source DeploymentAppSource, err error) string {
	if source == DeploymentAppSourceEnvironment {
		return "github_app_environment_invalid"
	}
	if source == DeploymentAppSourceManaged {
		return "github_app_managed_invalid"
	}
	if err != nil {
		return "github_app_status_unavailable"
	}
	return deploymentAppNotConfiguredCode
}

func deploymentAppUnavailableReason(code string) string {
	switch code {
	case "github_app_environment_invalid":
		return "The externally managed GitHub App configuration is incomplete or invalid."
	case "github_app_managed_invalid":
		return "The managed GitHub App credentials could not be loaded."
	case deploymentAppNotConfiguredCode:
		return "No deployment GitHub App is configured."
	default:
		return "GitHub App registration status is unavailable."
	}
}

func sanitizeDeploymentAppActivationError(_ error) error {
	return errors.New("generated GitHub App credentials could not be activated")
}

func sanitizeDeploymentAppPersistenceError(err error) error {
	if errors.Is(err, ErrDeploymentAppInUse) {
		return ErrDeploymentAppInUse
	}
	return errors.New("generated GitHub App credentials could not be stored")
}
