package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	personalTokenRefreshMargin  = 5 * time.Minute
	personalTokenRefreshTimeout = 30 * time.Second
)

var (
	ErrPersonalIdentityChanged = errors.New("refreshed GitHub identity does not match the connected user")
	ErrPersonalTokenInvalid    = errors.New("personal GitHub token is missing or expired")
	ErrPersonalConnectionStale = errors.New("personal GitHub connection changed")
)

type personalOAuth interface {
	githubAppOAuth
	RefreshUserToken(context.Context, string) (GitHubOAuthTokens, error)
}

// PersonalConnectionRepository owns the atomic boundary between encrypted
// personal secrets and user-connection metadata. Replace must either commit
// all values or preserve the previous row and both previous secrets.
type PersonalConnectionRepository interface {
	GetWorkspaceConnection(context.Context, string) (*WorkspaceConnection, error)
	GetUserConnection(context.Context, string, string) (*UserConnection, error)
	GetPersonalConnectionGeneration(context.Context, string, string) (int64, error)
	GetPersonalTokens(context.Context, string, string) (GitHubOAuthTokens, error)
	ReplacePersonalConnection(context.Context, *UserConnection, GitHubOAuthTokens, int64) error
	ReplacePersonalConnectionForFlow(
		context.Context,
		*UserConnection,
		GitHubOAuthTokens,
		int64,
		WorkspaceConnectionExpectation,
	) error
	MarkPersonalConnectionInvalid(context.Context, string, string, int64, error) error
	RevokePersonalConnection(context.Context, string, string) error
	RevokePersonalConnectionIfUnchanged(context.Context, *UserConnection) (bool, error)
	RevokeWorkspacePersonalConnections(context.Context, string) error
}

type PersonalAuthConfig struct {
	ClientID    string
	CallbackURL string
}

type PersonalAuthStart struct {
	URL       string    `json:"url"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PersonalAuthCallback struct {
	WorkspaceID string
	UserID      string
	State       string
	Code        string
}

type PersonalAuthResult struct {
	WorkspaceID string
	Connection  UserConnection
}

type PersonalAuthService struct {
	config                PersonalAuthConfig
	flows                 *OAuthFlowManager
	repo                  PersonalConnectionRepository
	oauth                 personalOAuth
	now                   func() time.Time
	refresh               singleflight.Group
	workspaceMutationLock func(string) *sync.Mutex
}

func NewPersonalAuthService(
	config PersonalAuthConfig,
	flows *OAuthFlowManager,
	repo PersonalConnectionRepository,
	oauth personalOAuth,
) *PersonalAuthService {
	return &PersonalAuthService{config: config, flows: flows, repo: repo, oauth: oauth, now: time.Now}
}

func (s *PersonalAuthService) SetWorkspaceMutationLock(lock func(string) *sync.Mutex) {
	if s != nil {
		s.workspaceMutationLock = lock
	}
}

func (s *PersonalAuthService) Start(
	ctx context.Context,
	workspaceID, userID string,
) (PersonalAuthStart, error) {
	if s == nil || s.flows == nil || s.repo == nil {
		return PersonalAuthStart{}, errors.New("personal GitHub authentication is not configured")
	}
	if err := validatePersonalAuthConfig(s.config); err != nil {
		return PersonalAuthStart{}, err
	}
	workspace, err := s.requireActiveAppWorkspace(ctx, workspaceID)
	if err != nil {
		return PersonalAuthStart{}, err
	}
	personalGeneration, err := s.repo.GetPersonalConnectionGeneration(ctx, workspaceID, userID)
	if err != nil {
		return PersonalAuthStart{}, fmt.Errorf("load personal GitHub connection: %w", err)
	}
	expected := workspaceConnectionExpectation(workspace)
	flow, err := s.flows.Start(ctx, OAuthFlowRequest{
		WorkspaceID:                 workspaceID,
		UserID:                      userID,
		Kind:                        AuthFlowKindPersonal,
		ExpectedWorkspaceSource:     expected.Source,
		ExpectedWorkspaceGeneration: expected.CredentialGeneration,
		ExpectedInstallationID:      expected.InstallationID,
		ExpectedPersonalGeneration:  personalGeneration,
	})
	if err != nil {
		return PersonalAuthStart{}, err
	}
	authorizeURL := &url.URL{Scheme: "https", Host: "github.com", Path: "/login/oauth/authorize"}
	query := authorizeURL.Query()
	query.Set("client_id", s.config.ClientID)
	query.Set("redirect_uri", s.config.CallbackURL)
	query.Set("state", flow.State)
	query.Set("code_challenge", flow.PKCEChallenge)
	query.Set("code_challenge_method", "S256")
	authorizeURL.RawQuery = query.Encode()
	return PersonalAuthStart{URL: authorizeURL.String(), State: flow.State, ExpiresAt: flow.ExpiresAt}, nil
}

func (s *PersonalAuthService) Complete(
	ctx context.Context,
	callback PersonalAuthCallback,
) (PersonalAuthResult, error) {
	if s == nil || s.flows == nil || s.repo == nil || s.oauth == nil {
		return PersonalAuthResult{}, errors.New("personal GitHub authentication is not configured")
	}
	flow, err := consumeCallbackFlow(
		ctx, s.flows, callback.State, callback.WorkspaceID, callback.UserID, AuthFlowKindPersonal,
	)
	if err != nil {
		return PersonalAuthResult{}, err
	}
	unlock := s.lockWorkspace(flow.WorkspaceID)
	defer unlock()
	workspace, err := s.requireActiveAppWorkspace(ctx, flow.WorkspaceID)
	if err != nil {
		return PersonalAuthResult{}, err
	}
	if !matchesWorkspaceConnectionExpectation(workspace, authFlowWorkspaceExpectation(flow)) {
		return PersonalAuthResult{}, ErrOAuthFlowStale
	}
	tokens, user, err := s.verifyPersonalCallback(ctx, callback, flow, *workspace.InstallationID)
	if err != nil {
		return PersonalAuthResult{}, err
	}
	connection, err := s.savePersonalConnection(ctx, flow, user, tokens)
	if err != nil {
		return PersonalAuthResult{}, err
	}
	return PersonalAuthResult{WorkspaceID: flow.WorkspaceID, Connection: connection}, nil
}

func (s *PersonalAuthService) verifyPersonalCallback(
	ctx context.Context,
	callback PersonalAuthCallback,
	flow *AuthFlow,
	installationID int64,
) (GitHubOAuthTokens, GitHubOAuthUser, error) {
	if callback.Code == "" || flow.PKCEVerifier == "" {
		return GitHubOAuthTokens{}, GitHubOAuthUser{}, ErrPersonalTokenInvalid
	}
	tokens, err := s.oauth.ExchangeUserCode(ctx, callback.Code, flow.PKCEVerifier, s.config.CallbackURL)
	if err != nil {
		return GitHubOAuthTokens{}, GitHubOAuthUser{}, fmt.Errorf(
			"exchange personal GitHub authorization: %w", err,
		)
	}
	if err := validatePersonalTokens(tokens, s.now()); err != nil {
		return GitHubOAuthTokens{}, GitHubOAuthUser{}, err
	}
	user, err := s.oauth.GetOAuthUser(ctx, tokens.AccessToken)
	if err != nil || user.ID <= 0 || strings.TrimSpace(user.Login) == "" {
		return GitHubOAuthTokens{}, GitHubOAuthUser{}, fmt.Errorf(
			"verify personal GitHub user: %w", associationError(err),
		)
	}
	accessible, err := s.oauth.UserCanAccessInstallation(ctx, tokens.AccessToken, installationID)
	if err != nil || !accessible {
		return GitHubOAuthTokens{}, GitHubOAuthUser{}, fmt.Errorf(
			"verify personal GitHub installation access: %w", associationError(err),
		)
	}
	return tokens, user, nil
}

func (s *PersonalAuthService) savePersonalConnection(
	ctx context.Context,
	flow *AuthFlow,
	user GitHubOAuthUser,
	tokens GitHubOAuthTokens,
) (UserConnection, error) {
	existing, err := s.repo.GetUserConnection(ctx, flow.WorkspaceID, flow.UserID)
	if err != nil {
		return UserConnection{}, fmt.Errorf("load personal GitHub connection: %w", err)
	}
	currentGeneration, err := s.repo.GetPersonalConnectionGeneration(ctx, flow.WorkspaceID, flow.UserID)
	if err != nil {
		return UserConnection{}, fmt.Errorf("load personal GitHub generation: %w", err)
	}
	connection := newUserConnection(flow.WorkspaceID, flow.UserID, user, tokens, existing)
	connection.CredentialGeneration = flow.ExpectedPersonalGeneration + 1
	if currentGeneration != flow.ExpectedPersonalGeneration {
		return UserConnection{}, ErrOAuthFlowStale
	}
	if err := s.repo.ReplacePersonalConnectionForFlow(
		ctx,
		&connection,
		tokens,
		flow.ExpectedPersonalGeneration,
		authFlowWorkspaceExpectation(flow),
	); err != nil {
		return UserConnection{}, fmt.Errorf("atomically save personal GitHub connection: %w", err)
	}
	return connection, nil
}

func (s *PersonalAuthService) ResolvePersonalToken(
	ctx context.Context,
	workspaceID, userID string,
) (string, *UserConnection, error) {
	if s == nil || s.repo == nil || s.oauth == nil {
		return "", nil, ErrGitHubPersonalRequired
	}
	workspace, err := s.requireActiveAppWorkspace(ctx, workspaceID)
	if err != nil {
		return "", nil, err
	}
	connection, tokens, err := s.loadPersonalConnection(ctx, workspaceID, userID)
	if err != nil {
		return "", nil, err
	}
	now := s.now().UTC()
	if tokens.AccessToken != "" && tokens.AccessExpiresAt.After(now.Add(personalTokenRefreshMargin)) {
		return tokens.AccessToken, connection, nil
	}
	return s.coalescedPersonalTokenRefresh(ctx, workspaceID, userID, *workspace.InstallationID)
}

type personalRefreshResult struct {
	token      string
	connection *UserConnection
}

func (s *PersonalAuthService) coalescedPersonalTokenRefresh(
	ctx context.Context,
	workspaceID, userID string,
	installationID int64,
) (string, *UserConnection, error) {
	key := fmt.Sprintf("%d:%s|%d:%s", len(workspaceID), workspaceID, len(userID), userID)
	resultChannel := s.refresh.DoChan(key, func() (any, error) {
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), personalTokenRefreshTimeout)
		defer cancel()
		connection, tokens, loadErr := s.loadPersonalConnection(refreshCtx, workspaceID, userID)
		if loadErr != nil {
			return nil, loadErr
		}
		now := s.now().UTC()
		if tokens.AccessToken != "" && tokens.AccessExpiresAt.After(now.Add(personalTokenRefreshMargin)) {
			return personalRefreshResult{token: tokens.AccessToken, connection: connection}, nil
		}
		token, refreshedConnection, refreshErr := s.refreshPersonalToken(
			refreshCtx, workspaceID, userID, installationID, connection, tokens, now,
		)
		if refreshErr != nil {
			return nil, refreshErr
		}
		return personalRefreshResult{token: token, connection: refreshedConnection}, nil
	})
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			return "", nil, result.Err
		}
		value := result.Val.(personalRefreshResult)
		return value.token, value.connection, nil
	}
}

func (s *PersonalAuthService) loadPersonalConnection(
	ctx context.Context,
	workspaceID, userID string,
) (*UserConnection, GitHubOAuthTokens, error) {
	connection, err := s.repo.GetUserConnection(ctx, workspaceID, userID)
	if err != nil {
		return nil, GitHubOAuthTokens{}, fmt.Errorf("load personal GitHub connection: %w", err)
	}
	if connection == nil || connection.Status != ConnectionStatusActive {
		return nil, GitHubOAuthTokens{}, ErrGitHubPersonalRequired
	}
	tokens, err := s.repo.GetPersonalTokens(ctx, workspaceID, userID)
	if err != nil {
		return nil, GitHubOAuthTokens{}, fmt.Errorf("load personal GitHub tokens: %w", err)
	}
	return connection, tokens, nil
}

func (s *PersonalAuthService) refreshPersonalToken(
	ctx context.Context,
	workspaceID, userID string,
	installationID int64,
	connection *UserConnection,
	tokens GitHubOAuthTokens,
	now time.Time,
) (string, *UserConnection, error) {
	if tokens.RefreshToken == "" || (tokens.RefreshExpiresAt != nil && !tokens.RefreshExpiresAt.After(now)) {
		s.markInvalid(ctx, workspaceID, userID, connection.CredentialGeneration, ErrPersonalTokenInvalid)
		return "", nil, ErrPersonalTokenInvalid
	}

	refreshed, err := s.oauth.RefreshUserToken(ctx, tokens.RefreshToken)
	if err != nil {
		if isDefinitivePersonalAuthFailure(err) {
			s.markInvalid(ctx, workspaceID, userID, connection.CredentialGeneration, err)
		}
		return "", nil, fmt.Errorf("refresh personal GitHub token: %w", err)
	}
	if err := validatePersonalTokens(refreshed, now); err != nil {
		s.markInvalid(ctx, workspaceID, userID, connection.CredentialGeneration, err)
		return "", nil, err
	}
	user, err := s.verifyRefreshedPersonalUser(ctx, workspaceID, userID, installationID, connection, refreshed)
	if err != nil {
		return "", nil, err
	}
	replacement := newUserConnection(workspaceID, userID, user, refreshed, connection)
	if err := s.repo.ReplacePersonalConnection(
		ctx, &replacement, refreshed, connection.CredentialGeneration,
	); err != nil {
		return "", nil, fmt.Errorf("atomically rotate personal GitHub tokens: %w", err)
	}
	return refreshed.AccessToken, &replacement, nil
}

func (s *PersonalAuthService) verifyRefreshedPersonalUser(
	ctx context.Context,
	workspaceID, userID string,
	installationID int64,
	connection *UserConnection,
	refreshed GitHubOAuthTokens,
) (GitHubOAuthUser, error) {
	user, err := s.oauth.GetOAuthUser(ctx, refreshed.AccessToken)
	if err != nil {
		if isDefinitivePersonalAuthFailure(err) {
			s.markInvalid(ctx, workspaceID, userID, connection.CredentialGeneration, err)
		}
		return GitHubOAuthUser{}, fmt.Errorf("verify refreshed personal GitHub user: %w", err)
	}
	if user.ID != connection.GitHubUserID {
		s.markInvalid(ctx, workspaceID, userID, connection.CredentialGeneration, ErrPersonalIdentityChanged)
		return GitHubOAuthUser{}, ErrPersonalIdentityChanged
	}
	accessible, err := s.oauth.UserCanAccessInstallation(ctx, refreshed.AccessToken, installationID)
	if err != nil || !accessible {
		associationErr := associationError(err)
		if err == nil || isDefinitivePersonalAuthFailure(err) {
			s.markInvalid(ctx, workspaceID, userID, connection.CredentialGeneration, associationErr)
		}
		return GitHubOAuthUser{}, associationErr
	}
	return user, nil
}

func (s *PersonalAuthService) Revoke(ctx context.Context, workspaceID, userID string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	unlock := s.lockWorkspace(workspaceID)
	defer unlock()
	return s.repo.RevokePersonalConnection(ctx, workspaceID, userID)
}

func (s *PersonalAuthService) lockWorkspace(workspaceID string) func() {
	if s.workspaceMutationLock == nil {
		return func() {}
	}
	lock := s.workspaceMutationLock(workspaceID)
	lock.Lock()
	return lock.Unlock
}

func (s *PersonalAuthService) RevokeWorkspace(ctx context.Context, workspaceID string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.RevokeWorkspacePersonalConnections(ctx, workspaceID)
}

func (s *PersonalAuthService) ReconcileAuthorizationRevocation(
	ctx context.Context,
	snapshot *UserConnection,
) (bool, error) {
	if s == nil || s.repo == nil || s.oauth == nil || snapshot == nil {
		return false, nil
	}
	unlock := s.lockWorkspace(snapshot.WorkspaceID)
	defer unlock()
	current, err := s.repo.GetUserConnection(ctx, snapshot.WorkspaceID, snapshot.UserID)
	if err != nil || !sameUserConnectionVersion(current, snapshot) {
		return false, err
	}
	tokens, err := s.repo.GetPersonalTokens(ctx, snapshot.WorkspaceID, snapshot.UserID)
	if err != nil {
		return false, err
	}
	user, err := s.oauth.GetOAuthUser(ctx, tokens.AccessToken)
	if err == nil && user.ID == current.GitHubUserID {
		return false, nil
	}
	if err != nil && !isDefinitivePersonalAuthFailure(err) {
		return false, err
	}
	return s.repo.RevokePersonalConnectionIfUnchanged(ctx, current)
}

func isDefinitivePersonalAuthFailure(err error) bool {
	var oauthErr *GitHubOAuthError
	if errors.As(err, &oauthErr) {
		return oauthErr.Code == "invalid_grant" || oauthErr.StatusCode == http.StatusUnauthorized
	}
	var apiErr *GitHubAPIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}

func (s *PersonalAuthService) requireActiveAppWorkspace(
	ctx context.Context,
	workspaceID string,
) (*WorkspaceConnection, error) {
	connection, err := s.repo.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load GitHub workspace connection: %w", err)
	}
	if connection == nil || connection.Source != ConnectionSourceGitHubAppInstallation ||
		connection.Status != ConnectionStatusActive || connection.InstallationID == nil {
		return nil, ErrGitHubPersonalRequired
	}
	return connection, nil
}

func (s *PersonalAuthService) markInvalid(
	ctx context.Context,
	workspaceID, userID string,
	expectedGeneration int64,
	reason error,
) {
	_ = s.repo.MarkPersonalConnectionInvalid(ctx, workspaceID, userID, expectedGeneration, reason)
}

func personalConnectionGeneration(connection *UserConnection) int64 {
	if connection == nil {
		return 0
	}
	return connection.CredentialGeneration
}

func sameUserConnectionVersion(current, expected *UserConnection) bool {
	return current != nil && expected != nil &&
		current.WorkspaceID == expected.WorkspaceID && current.UserID == expected.UserID &&
		current.GitHubUserID == expected.GitHubUserID &&
		current.CredentialGeneration == expected.CredentialGeneration &&
		current.UpdatedAt.Equal(expected.UpdatedAt)
}

func newUserConnection(
	workspaceID, userID string,
	user GitHubOAuthUser,
	tokens GitHubOAuthTokens,
	existing *UserConnection,
) UserConnection {
	generation := int64(1)
	var createdAt time.Time
	if existing != nil {
		generation = existing.CredentialGeneration + 1
		createdAt = existing.CreatedAt
	}
	return UserConnection{
		WorkspaceID:          workspaceID,
		UserID:               userID,
		GitHubUserID:         user.ID,
		Login:                user.Login,
		Status:               ConnectionStatusActive,
		AccessExpiresAt:      tokens.AccessExpiresAt,
		RefreshExpiresAt:     tokens.RefreshExpiresAt,
		CredentialGeneration: generation,
		CreatedAt:            createdAt,
	}
}

func validatePersonalTokens(tokens GitHubOAuthTokens, now time.Time) error {
	if tokens.AccessToken == "" || !tokens.AccessExpiresAt.After(now) {
		return ErrPersonalTokenInvalid
	}
	if tokens.RefreshExpiresAt != nil && !tokens.RefreshExpiresAt.After(now) {
		return ErrPersonalTokenInvalid
	}
	return nil
}

func validatePersonalAuthConfig(config PersonalAuthConfig) error {
	if strings.TrimSpace(config.ClientID) == "" {
		return errors.New("GitHub App client ID is required")
	}
	callback, err := url.Parse(config.CallbackURL)
	if err != nil || !callback.IsAbs() || callback.Host == "" {
		return errors.New("personal GitHub callback URL must be absolute")
	}
	return nil
}
