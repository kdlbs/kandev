package github

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrAppRegistrationNotFound = errors.New("GitHub App registration was not found")

const appRegistrationImportPreparationTTL = 10 * time.Minute

type AppRegistrationInUseError struct {
	BindingCount int `json:"binding_count"`
}

func (e *AppRegistrationInUseError) Error() string {
	return "GitHub App registration is in use"
}

func (e *AppRegistrationInUseError) Unwrap() error { return ErrDeploymentAppInUse }

type AppRegistrationLifecycleConfig struct {
	Repository *AppRegistrationRepository
	Store      *Store
	Runtime    appRegistrationRuntimeManager
	Converter  DeploymentAppManifestConverter
	Resolver   PublicGitHubBaseURLResolver
	Importer   *AppRegistrationImporter
	Now        func() time.Time
	Random     io.Reader
}

type AppRegistrationLifecycleService struct {
	repository *AppRegistrationRepository
	store      *Store
	runtime    appRegistrationRuntimeManager
	converter  DeploymentAppManifestConverter
	resolver   PublicGitHubBaseURLResolver
	importer   *AppRegistrationImporter
	now        func() time.Time
	random     io.Reader
}

type appRegistrationRuntimeManager interface {
	ValidateAppRegistrationRuntime(ResolvedDeploymentAppConfig) error
	ApplyAppRegistrationRuntime(ResolvedDeploymentAppConfig) error
	InvalidateAppRegistrationRuntime(string, int64)
}

type AppRegistrationCatalogItem struct {
	*AppRegistration
	Selected              bool   `json:"selected"`
	BindingCount          int    `json:"binding_count"`
	WorkspaceBindingCount int    `json:"workspace_binding_count"`
	Shared                bool   `json:"shared"`
	ManifestCallbackURL   string `json:"manifest_callback_url"`
	InstallCallbackURL    string `json:"install_callback_url"`
	PersonalCallbackURL   string `json:"personal_callback_url"`
	WebhookURL            string `json:"webhook_url"`
}

type AppRegistrationCatalog struct {
	WorkspaceID   string                       `json:"workspace_id"`
	Registrations []AppRegistrationCatalogItem `json:"registrations"`
}

type AppRegistrationManifestStartRequest struct {
	WorkspaceID   string                    `json:"workspace_id"`
	DisplayName   string                    `json:"display_name"`
	OwnerType     ManifestOwnerType         `json:"owner_type"`
	OwnerLogin    string                    `json:"owner_login"`
	Visibility    AppRegistrationVisibility `json:"visibility"`
	PublicBaseURL string                    `json:"public_base_url"`
}

type AppRegistrationManifestStart struct {
	RegistrationID  string                `json:"registration_id"`
	WorkspaceID     string                `json:"workspace_id"`
	State           string                `json:"state"`
	ExpiresAt       time.Time             `json:"expires_at"`
	Revision        int                   `json:"revision"`
	RegistrationURL string                `json:"registration_url"`
	Manifest        DeploymentAppManifest `json:"manifest"`
}

type AppRegistrationManifestCallback struct {
	State string
	Code  string
	Error string
}

type AppRegistrationManifestResult struct {
	WorkspaceID    string           `json:"workspace_id"`
	RegistrationID string           `json:"registration_id"`
	Registration   *AppRegistration `json:"registration"`
}

type AppRegistrationImportPrepareRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	PublicBaseURL string `json:"public_base_url"`
}

type AppRegistrationImportPreparationResult struct {
	RegistrationID      string            `json:"registration_id"`
	PublicBaseURL       string            `json:"public_base_url"`
	ManifestCallbackURL string            `json:"manifest_callback_url"`
	InstallCallbackURL  string            `json:"install_callback_url"`
	PersonalCallbackURL string            `json:"personal_callback_url"`
	WebhookURL          string            `json:"webhook_url"`
	SetupURL            string            `json:"setup_url"`
	Permissions         map[string]string `json:"permissions"`
	Events              []string          `json:"events"`
	ExpiresAt           time.Time         `json:"expires_at"`
}

func NewAppRegistrationLifecycleService(
	cfg AppRegistrationLifecycleConfig,
) *AppRegistrationLifecycleService {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}
	importer := cfg.Importer
	if importer == nil && cfg.Repository != nil {
		importer = NewAppRegistrationImporter(cfg.Repository, cfg.Resolver, nil)
	}
	return &AppRegistrationLifecycleService{
		repository: cfg.Repository, store: cfg.Store, runtime: cfg.Runtime,
		converter: cfg.Converter, resolver: cfg.Resolver, importer: importer,
		now: now, random: random,
	}
}

func (s *AppRegistrationLifecycleService) ready() error {
	if s == nil || s.repository == nil || s.store == nil || s.runtime == nil {
		return errors.New("GitHub App registration lifecycle is not configured")
	}
	return nil
}

func (s *AppRegistrationLifecycleService) List(
	ctx context.Context,
	userID, workspaceID string,
) (AppRegistrationCatalog, error) {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return AppRegistrationCatalog{}, err
	}
	if err := s.ready(); err != nil {
		return AppRegistrationCatalog{}, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return AppRegistrationCatalog{}, ErrGitHubWorkspaceRequired
	}
	selected, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return AppRegistrationCatalog{}, err
	}
	registrations, err := s.store.ListAppRegistrations(ctx)
	if err != nil {
		return AppRegistrationCatalog{}, err
	}
	result := AppRegistrationCatalog{WorkspaceID: workspaceID, Registrations: make([]AppRegistrationCatalogItem, 0, len(registrations))}
	for _, registration := range registrations {
		bindings, countErr := s.store.CountAppRegistrationBindings(ctx, registration.ID)
		if countErr != nil {
			return AppRegistrationCatalog{}, countErr
		}
		workspaceBindings, workspaceCountErr := s.store.CountAppRegistrationWorkspaceBindings(
			ctx, registration.ID,
		)
		if workspaceCountErr != nil {
			return AppRegistrationCatalog{}, workspaceCountErr
		}
		item := appRegistrationCatalogItem(registration, bindings, workspaceBindings)
		item.Selected = selected != nil && selected.AppRegistrationID == registration.ID
		result.Registrations = append(result.Registrations, item)
	}
	return result, nil
}

func appRegistrationCatalogItem(
	registration *AppRegistration,
	bindingCount, workspaceBindingCount int,
) AppRegistrationCatalogItem {
	base := strings.TrimRight(registration.PublicBaseURL, "/") +
		"/api/v1/github/app/registrations/" + registration.ID
	return AppRegistrationCatalogItem{
		AppRegistration: cloneDeploymentAppRegistration(registration), BindingCount: bindingCount,
		WorkspaceBindingCount: workspaceBindingCount, Shared: workspaceBindingCount > 1,
		ManifestCallbackURL: base + "/manifest/callback",
		InstallCallbackURL:  base + "/install/callback", PersonalCallbackURL: base + "/personal/callback",
		WebhookURL: base + "/webhook",
	}
}

func (s *AppRegistrationLifecycleService) StartManifest(
	ctx context.Context,
	userID string,
	request AppRegistrationManifestStartRequest,
) (AppRegistrationManifestStart, error) {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return AppRegistrationManifestStart{}, err
	}
	if err := s.ready(); err != nil {
		return AppRegistrationManifestStart{}, err
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if request.WorkspaceID == "" || request.DisplayName == "" ||
		len(request.DisplayName) > maxAppRegistrationDisplayName {
		return AppRegistrationManifestStart{}, errors.New("workspace and display name are required")
	}
	baseURL, err := ValidatePublicGitHubBaseURL(ctx, request.PublicBaseURL, s.resolver)
	if err != nil {
		return AppRegistrationManifestStart{}, err
	}
	registrationID := uuid.NewString()
	submission, err := BuildAppRegistrationManifest(AppRegistrationManifestRequest{
		RegistrationID: registrationID, OwnerType: request.OwnerType, OwnerLogin: request.OwnerLogin,
		PublicBaseURL: baseURL, Visibility: request.Visibility,
	})
	if err != nil {
		return AppRegistrationManifestStart{}, err
	}
	state, err := randomBase64URL(s.random)
	if err != nil {
		return AppRegistrationManifestStart{}, errors.New("generate GitHub App manifest state")
	}
	now := s.now().UTC()
	digest := sha256.Sum256([]byte(state))
	flow := &DeploymentAppRegistrationFlow{
		StateHash: stateDigestString(digest), RegistrationID: registrationID,
		WorkspaceID: request.WorkspaceID, UserID: userID, OperatorUserID: userID,
		OwnerType: deploymentOwnerType(request.OwnerType), OwnerLogin: request.OwnerLogin,
		DisplayName: request.DisplayName, Visibility: normalizedAppVisibility(request.Visibility),
		PublicBaseURL: baseURL, ManifestRevision: submission.Revision,
		ExpiresAt: DeploymentAppManifestFlowExpiresAt(now), CreatedAt: now,
	}
	if err := s.store.CreateDeploymentAppRegistrationFlow(ctx, flow); err != nil {
		return AppRegistrationManifestStart{}, fmt.Errorf("persist GitHub App manifest state: %w", err)
	}
	callback, err := url.Parse(submission.Manifest.RedirectURL)
	if err != nil {
		return AppRegistrationManifestStart{}, errors.New("build GitHub App manifest callback")
	}
	query := callback.Query()
	query.Set("state", state)
	callback.RawQuery = query.Encode()
	submission.Manifest.RedirectURL = callback.String()
	return AppRegistrationManifestStart{
		RegistrationID: registrationID, WorkspaceID: request.WorkspaceID,
		State: state, ExpiresAt: flow.ExpiresAt, Revision: submission.Revision,
		RegistrationURL: submission.RegistrationURL, Manifest: submission.Manifest,
	}, nil
}

func normalizedAppVisibility(value AppRegistrationVisibility) AppRegistrationVisibility {
	if value == "" {
		return AppRegistrationVisibilityPrivate
	}
	return value
}

func (s *AppRegistrationLifecycleService) CompleteManifest(
	ctx context.Context,
	routeRegistrationID string,
	callback AppRegistrationManifestCallback,
) (AppRegistrationManifestResult, error) {
	if err := s.ready(); err != nil {
		return AppRegistrationManifestResult{}, err
	}
	flow, err := s.consumeManifestFlow(ctx, callback.State, routeRegistrationID)
	if err != nil {
		return AppRegistrationManifestResult{}, err
	}
	if strings.TrimSpace(callback.Error) != "" || strings.TrimSpace(callback.Code) == "" {
		return AppRegistrationManifestResult{}, ErrDeploymentAppRegistrationCancelled
	}
	if s.converter == nil {
		return AppRegistrationManifestResult{}, errors.New("GitHub App manifest conversion is unavailable")
	}
	converted, err := s.converter.Convert(ctx, strings.TrimSpace(callback.Code))
	if err != nil {
		return AppRegistrationManifestResult{}, err
	}
	if !matchesDeploymentAppOwner(flow, converted) {
		return AppRegistrationManifestResult{}, ErrDeploymentAppIdentityMismatch
	}
	if !matchesDeploymentAppPolicy(converted) {
		return AppRegistrationManifestResult{}, ErrDeploymentAppPolicyMismatch
	}
	registration := registrationFromManifestFlow(flow, converted, s.now().UTC())
	credentials := DeploymentAppCredentials{
		PrivateKey: converted.PrivateKeyPEM, ClientSecret: converted.ClientSecret,
		WebhookSecret: converted.WebhookSecret,
	}
	resolved := resolvedDeploymentAppRegistration(registration, credentials)
	if err := s.runtime.ValidateAppRegistrationRuntime(resolved); err != nil {
		return AppRegistrationManifestResult{}, sanitizeDeploymentAppActivationError(err)
	}
	unlock := s.store.lockAppRegistrationLifecycle(registration.ID)
	defer unlock()
	if err := s.repository.SaveRegistration(ctx, registration, credentials); err != nil {
		return AppRegistrationManifestResult{}, sanitizeDeploymentAppPersistenceError(err)
	}
	if err := s.runtime.ApplyAppRegistrationRuntime(resolved); err != nil {
		cleanupErr := s.repository.DeleteRegistration(context.WithoutCancel(ctx), registration.ID)
		return AppRegistrationManifestResult{}, sanitizeDeploymentAppActivationAndCleanupError(err, cleanupErr)
	}
	return AppRegistrationManifestResult{
		WorkspaceID: flow.WorkspaceID, RegistrationID: registration.ID,
		Registration: cloneDeploymentAppRegistration(registration),
	}, nil
}

func (s *AppRegistrationLifecycleService) consumeManifestFlow(
	ctx context.Context,
	state, registrationID string,
) (*DeploymentAppRegistrationFlow, error) {
	digest := sha256.Sum256([]byte(strings.TrimSpace(state)))
	flow, err := s.store.ConsumeDeploymentAppRegistrationFlow(
		ctx, stateDigestString(digest), registrationID, s.now().UTC(),
	)
	if err != nil {
		if errors.Is(err, ErrDeploymentAppFlowUnavailable) {
			return nil, ErrDeploymentAppManifestStateUnavailable
		}
		return nil, fmt.Errorf("consume GitHub App manifest state: %w", err)
	}
	if flow == nil || flow.RegistrationID != registrationID || flow.UserID != DefaultUserID ||
		flow.ManifestRevision != DeploymentAppManifestRevision {
		return nil, ErrDeploymentAppManifestStateUnavailable
	}
	return flow, nil
}

func registrationFromManifestFlow(
	flow *DeploymentAppRegistrationFlow,
	converted ManifestConversionResult,
	now time.Time,
) *AppRegistration {
	return &AppRegistration{
		ID: flow.RegistrationID, Source: AppRegistrationSourceManaged,
		DisplayName: flow.DisplayName, GitHubHost: defaultGitHubAppHost,
		AppID: converted.AppID, ClientID: converted.ClientID, Slug: converted.Slug,
		OwnerLogin: converted.Owner.Login, OwnerType: deploymentOwnerTypeFromGitHub(converted.Owner.Type),
		Visibility: flow.Visibility, PublicBaseURL: flow.PublicBaseURL,
		CreatedForWorkspaceID: flow.WorkspaceID, CredentialGeneration: 1,
		Status: AppRegistrationStatusActive, WebhookStatus: DeploymentAppWebhookUnverified,
		CreatedAt: now, UpdatedAt: now,
	}
}

func (s *AppRegistrationLifecycleService) Import(
	ctx context.Context,
	userID string,
	request AppRegistrationImportRequest,
) (*AppRegistration, error) {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	if s.importer == nil {
		return nil, errors.New("GitHub App importer is not configured")
	}
	request.RegistrationID = strings.TrimSpace(request.RegistrationID)
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	parsedID, parseErr := uuid.Parse(request.RegistrationID)
	if parseErr != nil || parsedID.String() != request.RegistrationID || request.WorkspaceID == "" {
		return nil, ErrAppRegistrationImportPreparationUnavailable
	}
	baseURL, err := ValidatePublicGitHubBaseURL(ctx, request.PublicBaseURL, s.resolver)
	if err != nil {
		return nil, err
	}
	request.PublicBaseURL = baseURL
	if _, err := s.store.ConsumeAppRegistrationImportPreparation(
		ctx, request.RegistrationID, request.WorkspaceID, userID, baseURL, s.now().UTC(),
	); err != nil {
		return nil, err
	}
	unlock := s.store.lockAppRegistrationLifecycle(request.RegistrationID)
	defer unlock()
	registration, err := s.importer.Import(ctx, request)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveAppRegistrationConfig(ctx, registration.ID, s.repository)
	if err == nil {
		err = s.runtime.ApplyAppRegistrationRuntime(resolved)
	}
	if err != nil {
		cleanupErr := s.repository.DeleteRegistration(context.WithoutCancel(ctx), registration.ID)
		return nil, sanitizeDeploymentAppActivationAndCleanupError(err, cleanupErr)
	}
	return cloneDeploymentAppRegistration(registration), nil
}

func (s *AppRegistrationLifecycleService) PrepareImport(
	ctx context.Context,
	userID string,
	request AppRegistrationImportPrepareRequest,
) (AppRegistrationImportPreparationResult, error) {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return AppRegistrationImportPreparationResult{}, err
	}
	if err := s.ready(); err != nil {
		return AppRegistrationImportPreparationResult{}, err
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || len(request.WorkspaceID) > maxAppRegistrationWorkspaceID {
		return AppRegistrationImportPreparationResult{}, ErrGitHubWorkspaceRequired
	}
	baseURL, err := ValidatePublicGitHubBaseURL(ctx, request.PublicBaseURL, s.resolver)
	if err != nil {
		return AppRegistrationImportPreparationResult{}, err
	}
	registrationID := uuid.NewString()
	now := s.now().UTC()
	preparation := &AppRegistrationImportPreparation{
		RegistrationID: registrationID, WorkspaceID: request.WorkspaceID, UserID: userID,
		PublicBaseURL: baseURL, ExpiresAt: now.Add(appRegistrationImportPreparationTTL), CreatedAt: now,
	}
	if err := s.store.CreateAppRegistrationImportPreparation(ctx, preparation); err != nil {
		return AppRegistrationImportPreparationResult{}, fmt.Errorf("persist GitHub App import preparation: %w", err)
	}
	policy, err := BuildDeploymentAppManifest(ManifestOwnerUser, "policy", baseURL)
	if err != nil {
		return AppRegistrationImportPreparationResult{}, err
	}
	registrationBaseURL := strings.TrimRight(baseURL, "/") +
		"/api/v1/github/app/registrations/" + registrationID
	installCallbackURL := registrationBaseURL + "/install/callback"
	return AppRegistrationImportPreparationResult{
		RegistrationID: registrationID, PublicBaseURL: baseURL,
		ManifestCallbackURL: registrationBaseURL + "/manifest/callback",
		InstallCallbackURL:  installCallbackURL,
		PersonalCallbackURL: registrationBaseURL + "/personal/callback",
		WebhookURL:          registrationBaseURL + "/webhook", SetupURL: installCallbackURL,
		Permissions: policy.Manifest.DefaultPermissions, Events: policy.Manifest.DefaultEvents,
		ExpiresAt: preparation.ExpiresAt,
	}, nil
}

func (s *AppRegistrationLifecycleService) Rename(
	ctx context.Context,
	userID, registrationID, displayName string,
) (*AppRegistration, error) {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > maxAppRegistrationDisplayName {
		return nil, errors.New("GitHub App display name is invalid")
	}
	unlock := s.store.lockAppRegistrationLifecycle(registrationID)
	defer unlock()
	registration, err := s.store.RenameAppRegistration(ctx, registrationID, displayName)
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, ErrAppRegistrationNotFound
	}
	return cloneDeploymentAppRegistration(registration), nil
}

func (s *AppRegistrationLifecycleService) Delete(
	ctx context.Context,
	userID, registrationID string,
) error {
	if err := requireDeploymentAppOperator(userID); err != nil {
		return err
	}
	unlock := s.store.lockAppRegistrationLifecycle(registrationID)
	defer unlock()
	registration, err := s.store.GetAppRegistration(ctx, registrationID)
	if err != nil {
		return err
	}
	if registration == nil {
		return ErrAppRegistrationNotFound
	}
	bindings, err := s.store.CountAppRegistrationBindings(ctx, registrationID)
	if err != nil {
		return err
	}
	if bindings > 0 {
		return &AppRegistrationInUseError{BindingCount: bindings}
	}
	if err := s.repository.DeleteRegistration(ctx, registrationID); err != nil {
		s.invalidateRuntimeForUnusableRegistration(ctx, registration)
		return err
	}
	s.runtime.InvalidateAppRegistrationRuntime(registrationID, registration.CredentialGeneration)
	return nil
}

func (s *AppRegistrationLifecycleService) ResetForE2E(
	ctx context.Context,
	workspaceID string,
) error {
	if err := s.ready(); err != nil {
		return err
	}
	if workspaceID != "" {
		if err := s.store.DeleteAppRegistrationFlowsByWorkspace(ctx, workspaceID); err != nil {
			return err
		}
	}
	if err := s.store.DeleteAppRegistrationImportPreparationsByWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	registrations, err := s.store.ListAppRegistrations(ctx)
	if err != nil {
		return err
	}
	for _, registration := range registrations {
		if workspaceID != "" && registration.CreatedForWorkspaceID != workspaceID {
			continue
		}
		bindings, countErr := s.store.CountAppRegistrationBindings(ctx, registration.ID)
		if countErr != nil {
			return countErr
		}
		if bindings != 0 {
			continue
		}
		unlock := s.store.lockAppRegistrationLifecycle(registration.ID)
		if err := s.repository.DeleteRegistration(ctx, registration.ID); err != nil {
			unlock()
			s.invalidateRuntimeForUnusableRegistration(ctx, registration)
			return err
		}
		s.runtime.InvalidateAppRegistrationRuntime(registration.ID, registration.CredentialGeneration)
		unlock()
	}
	return s.repository.CleanupOrphanedCredentialBundles(ctx)
}

func (s *AppRegistrationLifecycleService) invalidateRuntimeForUnusableRegistration(
	ctx context.Context,
	previous *AppRegistration,
) {
	current, _, err := s.repository.LoadRegistration(context.WithoutCancel(ctx), previous.ID)
	if err != nil || current == nil || current.Status != AppRegistrationStatusActive ||
		current.CredentialGeneration != previous.CredentialGeneration {
		s.runtime.InvalidateAppRegistrationRuntime(previous.ID, previous.CredentialGeneration)
	}
}
