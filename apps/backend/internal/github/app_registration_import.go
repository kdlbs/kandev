package github

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxAppRegistrationDisplayName   = 100
	maxAppRegistrationWorkspaceID   = 255
	maxAppRegistrationClientID      = 255
	maxAppRegistrationClientSecret  = 4096
	maxAppRegistrationPrivateKey    = 64 * 1024
	maxAppRegistrationWebhookSecret = 4096
)

type AppRegistrationImportErrorCode string

const (
	AppRegistrationImportInvalidRequest     AppRegistrationImportErrorCode = "invalid_request"
	AppRegistrationImportVerificationFailed AppRegistrationImportErrorCode = "verification_failed"
	AppRegistrationImportIdentityMismatch   AppRegistrationImportErrorCode = "identity_mismatch"
	AppRegistrationImportPolicyMismatch     AppRegistrationImportErrorCode = "policy_mismatch"
	AppRegistrationImportAlreadyRegistered  AppRegistrationImportErrorCode = "already_registered"
	AppRegistrationImportPersistenceFailed  AppRegistrationImportErrorCode = "persistence_failed"
)

type AppRegistrationImportError struct {
	Code                   AppRegistrationImportErrorCode
	ExistingRegistrationID string
	Problems               []string
}

func (e *AppRegistrationImportError) Error() string {
	if e == nil {
		return "GitHub App import failed"
	}
	return "GitHub App import failed: " + string(e.Code)
}

type AppRegistrationImportRequest struct {
	RegistrationID string                    `json:"registration_id"`
	WorkspaceID    string                    `json:"workspace_id"`
	DisplayName    string                    `json:"display_name"`
	GitHubHost     string                    `json:"github_host"`
	AppID          int64                     `json:"app_id"`
	ClientID       string                    `json:"client_id"`
	ClientSecret   string                    `json:"client_secret"`
	PrivateKey     string                    `json:"private_key"`
	WebhookSecret  string                    `json:"webhook_secret"`
	Slug           string                    `json:"slug"`
	OwnerLogin     string                    `json:"owner_login"`
	OwnerType      AppRegistrationOwnerType  `json:"owner_type"`
	Visibility     AppRegistrationVisibility `json:"visibility"`
	PublicBaseURL  string                    `json:"public_base_url"`
}

type AppRegistrationVerifier interface {
	GetAuthenticatedApp(context.Context) (AuthenticatedApp, error)
}

type AppRegistrationWebhookVerifier interface {
	GetWebhookConfig(context.Context) (AppWebhookConfig, error)
}

type AppRegistrationVerifierFactory func(int64, []byte) (AppRegistrationVerifier, error)

type AppRegistrationImporter struct {
	repository *AppRegistrationRepository
	resolver   PublicGitHubBaseURLResolver
	verifier   AppRegistrationVerifierFactory
}

func NewAppRegistrationImporter(
	repository *AppRegistrationRepository,
	resolver PublicGitHubBaseURLResolver,
	verifier AppRegistrationVerifierFactory,
) *AppRegistrationImporter {
	return &AppRegistrationImporter{repository: repository, resolver: resolver, verifier: verifier}
}

func (s *AppRegistrationImporter) Import(
	ctx context.Context,
	request AppRegistrationImportRequest,
) (*AppRegistration, error) {
	validated, manifest, err := s.validateRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	if duplicate, err := s.findDuplicate(ctx, validated); err != nil || duplicate != nil {
		return nil, duplicateOrPersistenceError(duplicate, err)
	}
	verifier, err := s.verifierFactory()(validated.AppID, []byte(validated.PrivateKey))
	if err != nil {
		return nil, importError(AppRegistrationImportVerificationFailed)
	}
	identity, err := verifier.GetAuthenticatedApp(ctx)
	if err != nil {
		return nil, importError(AppRegistrationImportVerificationFailed)
	}
	if problems := importedAppIdentityProblems(validated, identity); len(problems) > 0 {
		return nil, &AppRegistrationImportError{
			Code: AppRegistrationImportIdentityMismatch, Problems: problems,
		}
	}
	var webhookConfig *AppWebhookConfig
	if webhookVerifier, ok := verifier.(AppRegistrationWebhookVerifier); ok {
		config, err := webhookVerifier.GetWebhookConfig(ctx)
		if err != nil {
			return nil, importError(AppRegistrationImportVerificationFailed)
		}
		webhookConfig = &config
	}
	if problems := importedAppPolicyProblems(manifest.Manifest, identity, webhookConfig); len(problems) > 0 {
		return nil, &AppRegistrationImportError{
			Code: AppRegistrationImportPolicyMismatch, Problems: problems,
		}
	}
	registration := importedAppRegistration(validated, identity)
	credentials := DeploymentAppCredentials{
		PrivateKey: validated.PrivateKey, ClientSecret: validated.ClientSecret,
		WebhookSecret: validated.WebhookSecret,
	}
	if err := s.repository.CreateRegistration(ctx, registration, credentials); err != nil {
		duplicate, lookupErr := s.findDuplicate(ctx, validated)
		if lookupErr == nil && duplicate != nil {
			return nil, duplicateImportError(duplicate.ID)
		}
		return nil, importError(AppRegistrationImportPersistenceFailed)
	}
	return registration, nil
}

func (s *AppRegistrationImporter) validateRequest(
	ctx context.Context,
	request AppRegistrationImportRequest,
) (AppRegistrationImportRequest, DeploymentAppManifestSubmission, error) {
	if s == nil || s.repository == nil || s.repository.store == nil || s.repository.secrets == nil {
		return request, DeploymentAppManifestSubmission{}, importError(AppRegistrationImportPersistenceFailed)
	}
	if !validImportScalarFields(request) {
		return request, DeploymentAppManifestSubmission{}, importError(AppRegistrationImportInvalidRequest)
	}
	parsedID, err := uuid.Parse(request.RegistrationID)
	if err != nil || parsedID.String() != request.RegistrationID {
		return request, DeploymentAppManifestSubmission{}, importError(AppRegistrationImportInvalidRequest)
	}
	request.PublicBaseURL, err = ValidatePublicGitHubBaseURL(ctx, request.PublicBaseURL, s.resolver)
	if err != nil {
		return request, DeploymentAppManifestSubmission{}, importError(AppRegistrationImportInvalidRequest)
	}
	manifest, err := BuildAppRegistrationManifest(AppRegistrationManifestRequest{
		RegistrationID: request.RegistrationID,
		OwnerType:      importManifestOwnerType(request.OwnerType), OwnerLogin: request.OwnerLogin,
		PublicBaseURL: request.PublicBaseURL, Visibility: request.Visibility,
	})
	if err != nil {
		return request, DeploymentAppManifestSubmission{}, importError(AppRegistrationImportInvalidRequest)
	}
	return request, manifest, nil
}

func validImportScalarFields(request AppRegistrationImportRequest) bool {
	return request.GitHubHost == defaultGitHubAppHost && request.AppID > 0 &&
		boundedTrimmed(request.WorkspaceID, maxAppRegistrationWorkspaceID) &&
		boundedTrimmed(request.DisplayName, maxAppRegistrationDisplayName) &&
		boundedTrimmed(request.ClientID, maxAppRegistrationClientID) &&
		boundedSecret(request.ClientSecret, maxAppRegistrationClientSecret) &&
		boundedSecret(request.PrivateKey, maxAppRegistrationPrivateKey) &&
		boundedSecret(request.WebhookSecret, maxAppRegistrationWebhookSecret) &&
		boundedTrimmed(request.Slug, maxAppRegistrationDisplayName) &&
		validManifestOwner(importManifestOwnerType(request.OwnerType), request.OwnerLogin) &&
		validImportVisibility(request.Visibility)
}

func boundedTrimmed(value string, maxLength int) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maxLength
}

func boundedSecret(value string, maxLength int) bool {
	return value != "" && len(value) <= maxLength
}

func validImportVisibility(visibility AppRegistrationVisibility) bool {
	return visibility == "" || visibility == AppRegistrationVisibilityPrivate ||
		visibility == AppRegistrationVisibilityPublic
}

func importManifestOwnerType(ownerType AppRegistrationOwnerType) ManifestOwnerType {
	if ownerType == AppRegistrationOwnerOrganization {
		return ManifestOwnerOrganization
	}
	if ownerType == AppRegistrationOwnerUser {
		return ManifestOwnerUser
	}
	return ""
}

func (s *AppRegistrationImporter) findDuplicate(
	ctx context.Context,
	request AppRegistrationImportRequest,
) (*AppRegistration, error) {
	return s.repository.store.GetAppRegistrationByGitHubApp(ctx, request.GitHubHost, request.AppID)
}

func (s *AppRegistrationImporter) verifierFactory() AppRegistrationVerifierFactory {
	if s.verifier != nil {
		return s.verifier
	}
	return func(appID int64, privateKey []byte) (AppRegistrationVerifier, error) {
		return NewAppClient(appID, privateKey)
	}
}

func importedAppIdentityProblems(
	request AppRegistrationImportRequest,
	identity AuthenticatedApp,
) []string {
	var problems []string
	if identity.ID != request.AppID {
		problems = append(problems, "app_id")
	}
	if identity.ClientID != request.ClientID {
		problems = append(problems, "client_id")
	}
	if !strings.EqualFold(identity.Slug, request.Slug) {
		problems = append(problems, "slug")
	}
	if !strings.EqualFold(identity.OwnerLogin, request.OwnerLogin) {
		problems = append(problems, "owner_login")
	}
	if !strings.EqualFold(identity.OwnerType, string(request.OwnerType)) {
		problems = append(problems, "owner_type")
	}
	return problems
}

func importedAppPolicyProblems(
	manifest DeploymentAppManifest,
	identity AuthenticatedApp,
	webhookConfig *AppWebhookConfig,
) []string {
	var problems []string
	if identity.ExternalURL != manifest.URL {
		problems = append(problems, "external_url")
	}
	if webhookConfig != nil && (webhookConfig.URL != manifest.HookAttributes.URL ||
		webhookConfig.ContentType != "json" || webhookConfig.InsecureSSL != "0") {
		problems = append(problems, "webhook")
	}
	for name, required := range manifest.DefaultPermissions {
		if !permissionSatisfies(identity.Permissions[name], required) {
			problems = append(problems, "permission:"+name)
		}
	}
	for _, event := range manifest.DefaultEvents {
		if !containsString(identity.Events, event) {
			problems = append(problems, "event:"+event)
		}
	}
	return problems
}

func permissionSatisfies(actual, required string) bool {
	return actual == required ||
		(required == string(PermissionRead) && actual == string(PermissionWrite))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func importedAppRegistration(
	request AppRegistrationImportRequest,
	identity AuthenticatedApp,
) *AppRegistration {
	now := time.Now().UTC()
	visibility := request.Visibility
	if visibility == "" {
		visibility = AppRegistrationVisibilityPrivate
	}
	return &AppRegistration{
		ID: request.RegistrationID, Source: AppRegistrationSourceImported,
		DisplayName: request.DisplayName, GitHubHost: request.GitHubHost,
		AppID: identity.ID, ClientID: identity.ClientID, Slug: identity.Slug,
		OwnerLogin: identity.OwnerLogin, OwnerType: AppRegistrationOwnerType(identity.OwnerType),
		Visibility: visibility, PublicBaseURL: request.PublicBaseURL,
		CreatedForWorkspaceID: request.WorkspaceID, CredentialGeneration: 1,
		Status: AppRegistrationStatusActive, WebhookStatus: DeploymentAppWebhookUnverified,
		CreatedAt: now, UpdatedAt: now,
	}
}

func duplicateOrPersistenceError(duplicate *AppRegistration, err error) error {
	if duplicate != nil {
		return duplicateImportError(duplicate.ID)
	}
	if err != nil {
		return importError(AppRegistrationImportPersistenceFailed)
	}
	return nil
}

func duplicateImportError(registrationID string) error {
	return &AppRegistrationImportError{
		Code: AppRegistrationImportAlreadyRegistered, ExistingRegistrationID: registrationID,
	}
}

func importError(code AppRegistrationImportErrorCode) error {
	return &AppRegistrationImportError{Code: code}
}

func IsAppRegistrationImportError(err error, code AppRegistrationImportErrorCode) bool {
	var importErr *AppRegistrationImportError
	return errors.As(err, &importErr) && importErr.Code == code
}
