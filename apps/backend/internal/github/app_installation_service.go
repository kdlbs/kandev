package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var ErrInstallationAssociationUnverified = errors.New("GitHub App installation association could not be verified")

type GitHubOAuthTokens struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt *time.Time
}

type GitHubOAuthUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type githubAppOAuth interface {
	ExchangeUserCode(ctx context.Context, code, pkceVerifier, redirectURI string) (GitHubOAuthTokens, error)
	GetOAuthUser(ctx context.Context, accessToken string) (GitHubOAuthUser, error)
	UserCanAccessInstallation(ctx context.Context, accessToken string, installationID int64) (bool, error)
}

type appInstallationVerifier interface {
	GetInstallation(context.Context, int64) (AppInstallation, error)
}

type appInstallationStore interface {
	GetWorkspaceConnection(context.Context, string) (*WorkspaceConnection, error)
	ReplaceWorkspaceConnection(
		context.Context,
		*WorkspaceConnection,
		WorkspaceConnectionExpectation,
	) error
}

type AppInstallationConfig struct {
	RegistrationID string
	Slug           string
	CallbackURL    string
}

type AppInstallationStart struct {
	URL       string    `json:"url"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AppInstallationCallback struct {
	WorkspaceID    string
	UserID         string
	State          string
	Code           string
	SetupAction    string
	InstallationID int64
}

type AppInstallationResult struct {
	WorkspaceID     string
	Installation    AppInstallation
	AuthorizingUser GitHubOAuthUser
}

type AppInstallationService struct {
	config AppInstallationConfig
	flows  *OAuthFlowManager
	store  appInstallationStore
	app    appInstallationVerifier
	oauth  githubAppOAuth
}

func NewAppInstallationService(
	config AppInstallationConfig,
	flows *OAuthFlowManager,
	store appInstallationStore,
	app appInstallationVerifier,
	oauth githubAppOAuth,
) *AppInstallationService {
	return &AppInstallationService{config: config, flows: flows, store: store, app: app, oauth: oauth}
}

func (s *AppInstallationService) Start(
	ctx context.Context,
	workspaceID, userID string,
) (AppInstallationStart, error) {
	if s == nil || s.flows == nil || s.store == nil {
		return AppInstallationStart{}, errors.New("GitHub App installation service is not configured")
	}
	if err := validateAppInstallationConfig(s.config); err != nil {
		return AppInstallationStart{}, err
	}
	existing, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return AppInstallationStart{}, fmt.Errorf("load GitHub workspace connection: %w", err)
	}
	expected := workspaceConnectionExpectation(existing)
	flow, err := s.flows.Start(ctx, OAuthFlowRequest{
		WorkspaceID:                        workspaceID,
		UserID:                             userID,
		AppRegistrationID:                  s.config.RegistrationID,
		Kind:                               AuthFlowKindAppInstallation,
		ExpectedWorkspaceSource:            expected.Source,
		ExpectedWorkspaceGeneration:        expected.CredentialGeneration,
		ExpectedInstallationID:             expected.InstallationID,
		ExpectedWorkspaceAppRegistrationID: expected.AppRegistrationID,
	})
	if err != nil {
		return AppInstallationStart{}, err
	}
	installURL := &url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/apps/" + s.config.Slug + "/installations/new",
	}
	query := installURL.Query()
	query.Set("state", flow.State)
	installURL.RawQuery = query.Encode()
	return AppInstallationStart{URL: installURL.String(), State: flow.State, ExpiresAt: flow.ExpiresAt}, nil
}

func (s *AppInstallationService) Complete(
	ctx context.Context,
	callback AppInstallationCallback,
) (AppInstallationResult, error) {
	if s == nil || s.flows == nil || s.store == nil || s.app == nil || s.oauth == nil {
		return AppInstallationResult{}, errors.New("GitHub App installation service is not configured")
	}
	flow, err := consumeCallbackFlow(
		ctx, s.flows, callback.State, callback.WorkspaceID, callback.UserID,
		s.config.RegistrationID, AuthFlowKindAppInstallation,
	)
	if err != nil {
		return AppInstallationResult{}, err
	}
	installation, user, err := s.verifyInstallationCallback(ctx, callback, flow)
	if err != nil {
		return AppInstallationResult{}, err
	}
	connection := s.newInstallationConnection(flow, installation)
	if err := s.store.ReplaceWorkspaceConnection(
		ctx, connection, authFlowWorkspaceExpectation(flow),
	); err != nil {
		return AppInstallationResult{}, fmt.Errorf("save GitHub App installation: %w", err)
	}
	return AppInstallationResult{
		WorkspaceID: flow.WorkspaceID, Installation: installation, AuthorizingUser: user,
	}, nil
}

func (s *AppInstallationService) verifyInstallationCallback(
	ctx context.Context,
	callback AppInstallationCallback,
	flow *AuthFlow,
) (AppInstallation, GitHubOAuthUser, error) {
	if callback.InstallationID <= 0 || callback.Code == "" ||
		(callback.SetupAction != "install" && callback.SetupAction != "update") {
		return AppInstallation{}, GitHubOAuthUser{}, ErrInstallationAssociationUnverified
	}

	tokens, err := s.oauth.ExchangeUserCode(ctx, callback.Code, flow.PKCEVerifier, s.config.CallbackURL)
	if err != nil || tokens.AccessToken == "" {
		return AppInstallation{}, GitHubOAuthUser{}, fmt.Errorf(
			"exchange GitHub App authorizer code: %w", associationError(err),
		)
	}
	user, err := s.oauth.GetOAuthUser(ctx, tokens.AccessToken)
	if err != nil || user.ID <= 0 || strings.TrimSpace(user.Login) == "" {
		return AppInstallation{}, GitHubOAuthUser{}, fmt.Errorf(
			"verify GitHub App authorizing user: %w", associationError(err),
		)
	}
	accessible, err := s.oauth.UserCanAccessInstallation(ctx, tokens.AccessToken, callback.InstallationID)
	if err != nil || !accessible {
		return AppInstallation{}, GitHubOAuthUser{}, fmt.Errorf(
			"verify GitHub App installation access: %w", associationError(err),
		)
	}
	installation, err := s.app.GetInstallation(ctx, callback.InstallationID)
	if err != nil || installation.ID != callback.InstallationID {
		return AppInstallation{}, GitHubOAuthUser{}, fmt.Errorf(
			"verify GitHub App installation: %w", associationError(err),
		)
	}
	return installation, user, nil
}

func (s *AppInstallationService) newInstallationConnection(
	flow *AuthFlow,
	installation AppInstallation,
) *WorkspaceConnection {
	status := ConnectionStatusActive
	if installation.SuspendedAt != nil {
		status = ConnectionStatusSuspended
	}
	return &WorkspaceConnection{
		WorkspaceID:              flow.WorkspaceID,
		Source:                   ConnectionSourceGitHubAppInstallation,
		GitHubHost:               "github.com",
		InstallationID:           &installation.ID,
		InstallationAccountLogin: installation.AccountLogin,
		InstallationAccountType:  installation.AccountType,
		AppRegistrationID:        flow.AppRegistrationID,
		Status:                   status,
		CredentialGeneration:     flow.ExpectedWorkspaceGeneration + 1,
	}
}

func validateAppInstallationConfig(config AppInstallationConfig) error {
	if strings.TrimSpace(config.RegistrationID) == "" {
		return errors.New("GitHub App registration ID is required")
	}
	if !validGitHubAppSlug(config.Slug) {
		return errors.New("GitHub App slug is required")
	}
	callback, err := url.Parse(config.CallbackURL)
	if err != nil || !callback.IsAbs() || callback.Host == "" {
		return errors.New("GitHub App callback URL must be absolute")
	}
	return nil
}

func validGitHubAppSlug(slug string) bool {
	if slug == "" {
		return false
	}
	for _, character := range slug {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func associationError(err error) error {
	if err == nil {
		return ErrInstallationAssociationUnverified
	}
	return fmt.Errorf("%w: %v", ErrInstallationAssociationUnverified, err)
}

func consumeCallbackFlow(
	ctx context.Context,
	flows *OAuthFlowManager,
	state, workspaceID, userID string,
	registrationID string,
	kind AuthFlowKind,
) (*AuthFlow, error) {
	if workspaceID == "" && userID == "" {
		return flows.ConsumeBound(ctx, state, registrationID, kind)
	}
	return flows.Consume(ctx, state, OAuthFlowExpectation{
		WorkspaceID:       workspaceID,
		UserID:            userID,
		AppRegistrationID: registrationID,
		Kind:              kind,
	})
}
