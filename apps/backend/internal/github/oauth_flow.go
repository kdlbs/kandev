package github

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	oauthRandomBytes        = 32
	appInstallationStateTTL = 10 * time.Minute
	personalOAuthStateTTL   = 10 * time.Minute
)

var (
	ErrOAuthStateInvalid  = errors.New("GitHub OAuth state is expired, consumed, or invalid")
	ErrOAuthStateMismatch = errors.New("GitHub OAuth state does not match the current request")
	ErrOAuthFlowStale     = errors.New("GitHub authentication changed after authorization started")
)

func workspaceConnectionExpectation(connection *WorkspaceConnection) WorkspaceConnectionExpectation {
	if connection == nil {
		return WorkspaceConnectionExpectation{}
	}
	return WorkspaceConnectionExpectation{
		Source:               connection.Source,
		CredentialGeneration: connection.CredentialGeneration,
		InstallationID:       cloneInt64Pointer(connection.InstallationID),
	}
}

func authFlowWorkspaceExpectation(flow *AuthFlow) WorkspaceConnectionExpectation {
	if flow == nil {
		return WorkspaceConnectionExpectation{}
	}
	return WorkspaceConnectionExpectation{
		Source:               flow.ExpectedWorkspaceSource,
		CredentialGeneration: flow.ExpectedWorkspaceGeneration,
		InstallationID:       cloneInt64Pointer(flow.ExpectedInstallationID),
	}
}

func matchesWorkspaceConnectionExpectation(
	connection *WorkspaceConnection,
	expected WorkspaceConnectionExpectation,
) bool {
	if connection == nil {
		return expected.Source == "" && expected.CredentialGeneration == 0 && expected.InstallationID == nil
	}
	return connection.Source == expected.Source &&
		connection.CredentialGeneration == expected.CredentialGeneration &&
		equalInt64Pointers(connection.InstallationID, expected.InstallationID)
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func equalInt64Pointers(left, right *int64) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && *left == *right)
}

// GitHubOAuthError preserves the machine-readable OAuth error code so token
// lifecycle decisions do not depend on parsing user-facing text.
type GitHubOAuthError struct {
	StatusCode  int
	Code        string
	Description string
}

func (e *GitHubOAuthError) Error() string {
	return "GitHub OAuth exchange failed: " + strings.TrimSpace(e.Code+" "+e.Description)
}

type oauthFlowStore interface {
	CreateAuthFlow(context.Context, *AuthFlow) error
	ConsumeAuthFlow(context.Context, string, time.Time) (*AuthFlow, error)
}

type OAuthFlowRequest struct {
	WorkspaceID                 string
	UserID                      string
	Kind                        AuthFlowKind
	ExpectedWorkspaceSource     ConnectionSource
	ExpectedWorkspaceGeneration int64
	ExpectedInstallationID      *int64
	ExpectedPersonalGeneration  int64
}

type OAuthFlowExpectation = OAuthFlowRequest

type OAuthFlowStart struct {
	State         string
	PKCEChallenge string
	ExpiresAt     time.Time
}

type OAuthFlowManager struct {
	store  oauthFlowStore
	now    func() time.Time
	random io.Reader
}

func NewOAuthFlowManager(store oauthFlowStore) *OAuthFlowManager {
	return &OAuthFlowManager{store: store, now: time.Now, random: rand.Reader}
}

func (m *OAuthFlowManager) Start(ctx context.Context, request OAuthFlowRequest) (OAuthFlowStart, error) {
	if m == nil || m.store == nil {
		return OAuthFlowStart{}, errors.New("GitHub OAuth flow store is not configured")
	}
	if request.WorkspaceID == "" || request.UserID == "" {
		return OAuthFlowStart{}, errors.New("workspace and user are required for GitHub OAuth")
	}
	ttl, err := oauthFlowTTL(request.Kind)
	if err != nil {
		return OAuthFlowStart{}, err
	}
	state, err := randomBase64URL(m.random)
	if err != nil {
		return OAuthFlowStart{}, fmt.Errorf("generate GitHub OAuth state: %w", err)
	}

	var verifier, challenge string
	if request.Kind == AuthFlowKindPersonal {
		verifier, err = randomBase64URL(m.random)
		if err != nil {
			return OAuthFlowStart{}, fmt.Errorf("generate GitHub OAuth PKCE verifier: %w", err)
		}
		challengeDigest := sha256.Sum256([]byte(verifier))
		challenge = base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	}

	now := m.now().UTC()
	stateDigest := sha256.Sum256([]byte(state))
	flow := &AuthFlow{
		StateHash:                   stateDigestString(stateDigest),
		WorkspaceID:                 request.WorkspaceID,
		UserID:                      request.UserID,
		Kind:                        request.Kind,
		PKCEVerifier:                verifier,
		ExpectedWorkspaceSource:     request.ExpectedWorkspaceSource,
		ExpectedWorkspaceGeneration: request.ExpectedWorkspaceGeneration,
		ExpectedInstallationID:      request.ExpectedInstallationID,
		ExpectedPersonalGeneration:  request.ExpectedPersonalGeneration,
		ExpiresAt:                   now.Add(ttl),
		CreatedAt:                   now,
	}
	if err := m.store.CreateAuthFlow(ctx, flow); err != nil {
		return OAuthFlowStart{}, fmt.Errorf("persist GitHub OAuth flow: %w", err)
	}
	return OAuthFlowStart{State: state, PKCEChallenge: challenge, ExpiresAt: flow.ExpiresAt}, nil
}

func (m *OAuthFlowManager) Consume(
	ctx context.Context,
	state string,
	expected OAuthFlowExpectation,
) (*AuthFlow, error) {
	flow, err := m.ConsumeBound(ctx, state, expected.Kind)
	if err != nil {
		return nil, err
	}
	if flow.WorkspaceID != expected.WorkspaceID || flow.UserID != expected.UserID {
		return nil, ErrOAuthStateMismatch
	}
	return flow, nil
}

// ConsumeBound is for public callbacks that receive only the opaque state.
// The workspace and user are recovered from the consumed server-side row,
// never from untrusted callback query parameters.
func (m *OAuthFlowManager) ConsumeBound(
	ctx context.Context,
	state string,
	expectedKind AuthFlowKind,
) (*AuthFlow, error) {
	if m == nil || m.store == nil || state == "" {
		return nil, ErrOAuthStateInvalid
	}
	digest := sha256.Sum256([]byte(state))
	flow, err := m.store.ConsumeAuthFlow(ctx, stateDigestString(digest), m.now().UTC())
	if err != nil {
		if errors.Is(err, ErrAuthFlowUnavailable) {
			return nil, ErrOAuthStateInvalid
		}
		return nil, fmt.Errorf("consume GitHub OAuth flow: %w", err)
	}
	if flow == nil || flow.Kind != expectedKind {
		return nil, ErrOAuthStateMismatch
	}
	return flow, nil
}

func oauthFlowTTL(kind AuthFlowKind) (time.Duration, error) {
	switch kind {
	case AuthFlowKindAppInstallation:
		return appInstallationStateTTL, nil
	case AuthFlowKindPersonal:
		return personalOAuthStateTTL, nil
	default:
		return 0, fmt.Errorf("unsupported GitHub OAuth flow kind %q", kind)
	}
}

func randomBase64URL(source io.Reader) (string, error) {
	value := make([]byte, oauthRandomBytes)
	if _, err := io.ReadFull(source, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func stateDigestString(digest [sha256.Size]byte) string {
	return hex.EncodeToString(digest[:])
}

// GitHubOAuthClient implements GitHub App user authorization and refresh. It
// never persists tokens; callers hand successful results to the atomic
// personal-connection repository.
type GitHubOAuthClient struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	webBaseURL   string
	apiBaseURL   string
	now          func() time.Time
}

func NewGitHubOAuthClient(clientID, clientSecret string) *GitHubOAuthClient {
	return &GitHubOAuthClient{
		clientID: clientID, clientSecret: clientSecret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		webBaseURL: "https://github.com", apiBaseURL: githubAPIBase, now: time.Now,
	}
}

func (c *GitHubOAuthClient) ExchangeUserCode(
	ctx context.Context,
	code, pkceVerifier, redirectURI string,
) (GitHubOAuthTokens, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	if pkceVerifier != "" {
		form.Set("code_verifier", pkceVerifier)
	}
	return c.exchangeTokens(ctx, form)
}

func (c *GitHubOAuthClient) RefreshUserToken(
	ctx context.Context,
	refreshToken string,
) (GitHubOAuthTokens, error) {
	return c.exchangeTokens(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (c *GitHubOAuthClient) GetOAuthUser(ctx context.Context, accessToken string) (GitHubOAuthUser, error) {
	request, err := c.apiRequest(ctx, http.MethodGet, "/user", accessToken)
	if err != nil {
		return GitHubOAuthUser{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return GitHubOAuthUser{}, fmt.Errorf("request GitHub OAuth user: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= http.StatusBadRequest {
		return GitHubOAuthUser{}, &GitHubAPIError{StatusCode: response.StatusCode, Endpoint: "/user"}
	}
	var user GitHubOAuthUser
	if err := json.NewDecoder(io.LimitReader(response.Body, maxGitHubAppResponseSize)).Decode(&user); err != nil {
		return GitHubOAuthUser{}, fmt.Errorf("decode GitHub OAuth user: %w", err)
	}
	return user, nil
}

func (c *GitHubOAuthClient) UserCanAccessInstallation(
	ctx context.Context,
	accessToken string,
	installationID int64,
) (bool, error) {
	endpoint := fmt.Sprintf("/user/installations/%d", installationID)
	request, err := c.apiRequest(ctx, http.MethodGet, endpoint, accessToken)
	if err != nil {
		return false, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return false, fmt.Errorf("request GitHub user installation: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode >= http.StatusBadRequest {
		return false, &GitHubAPIError{StatusCode: response.StatusCode, Endpoint: endpoint}
	}
	return true, nil
}

func (c *GitHubOAuthClient) exchangeTokens(ctx context.Context, form url.Values) (GitHubOAuthTokens, error) {
	if c == nil || c.clientID == "" || c.clientSecret == "" {
		return GitHubOAuthTokens{}, errors.New("GitHub App OAuth client is not configured")
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.webBaseURL, "/")+"/login/oauth/access_token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return GitHubOAuthTokens{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return GitHubOAuthTokens{}, fmt.Errorf("exchange GitHub OAuth token: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	var body struct {
		AccessToken           string `json:"access_token"`
		RefreshToken          string `json:"refresh_token"`
		ExpiresIn             int64  `json:"expires_in"`
		RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
		Error                 string `json:"error"`
		ErrorDescription      string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxGitHubAppResponseSize)).Decode(&body); err != nil {
		return GitHubOAuthTokens{}, fmt.Errorf("decode GitHub OAuth token response: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest || body.Error != "" {
		return GitHubOAuthTokens{}, &GitHubOAuthError{
			StatusCode: response.StatusCode,
			Code:       body.Error, Description: body.ErrorDescription,
		}
	}
	now := c.now().UTC()
	tokens := GitHubOAuthTokens{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
	}
	if body.ExpiresIn > 0 {
		tokens.AccessExpiresAt = now.Add(time.Duration(body.ExpiresIn) * time.Second)
	}
	if body.RefreshTokenExpiresIn > 0 {
		expiresAt := now.Add(time.Duration(body.RefreshTokenExpiresIn) * time.Second)
		tokens.RefreshExpiresAt = &expiresAt
	}
	return tokens, nil
}

func (c *GitHubOAuthClient) apiRequest(
	ctx context.Context,
	method, endpoint, accessToken string,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(
		ctx, method, strings.TrimRight(c.apiBaseURL, "/")+endpoint, nil,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", githubAccept)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	return request, nil
}
