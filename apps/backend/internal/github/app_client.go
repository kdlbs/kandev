package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxGitHubAppResponseSize = 4 * 1024 * 1024

var ErrGitHubAppResponseTooLarge = errors.New("GitHub App response exceeds the allowed size")

// PermissionLevel is a GitHub App repository permission access level.
type PermissionLevel string

const (
	PermissionRead  PermissionLevel = "read"
	PermissionWrite PermissionLevel = "write"
)

// InstallationPermissions is the permission subset requested for an
// installation access token.
type InstallationPermissions map[string]PermissionLevel

// GitHubAppCapability is an operation-level capability derived from the
// permissions GitHub reports for an installation or token.
type GitHubAppCapability string

const (
	CapabilityRepositoryRead       GitHubAppCapability = "repository_read"
	CapabilityGitRead              GitHubAppCapability = "git_read"
	CapabilityGitWrite             GitHubAppCapability = "git_write"
	CapabilityPullRequestRead      GitHubAppCapability = "pull_request_read"
	CapabilityPullRequestWrite     GitHubAppCapability = "pull_request_write"
	CapabilityIssueRead            GitHubAppCapability = "issue_read"
	CapabilityIssueWrite           GitHubAppCapability = "issue_write"
	CapabilityChecksRead           GitHubAppCapability = "checks_read"
	CapabilityStatusesRead         GitHubAppCapability = "statuses_read"
	CapabilityActionsRead          GitHubAppCapability = "actions_read"
	CapabilityBranchProtectionRead GitHubAppCapability = "branch_protection_read"
	CapabilityMembersRead          GitHubAppCapability = "members_read"
	CapabilityWorkflowsWrite       GitHubAppCapability = "workflows_write"
)

// InstallationToken is a short-lived token minted for one App installation.
type InstallationToken struct {
	Token       string
	ExpiresAt   time.Time
	Permissions InstallationPermissions
	Principal   TokenPrincipal
}

// AppInstallation is verified installation metadata returned by GitHub.
type AppInstallation struct {
	ID           int64
	AccountID    int64
	AccountLogin string
	AccountType  string
	Permissions  InstallationPermissions
	SuspendedAt  *time.Time
}

type AuthenticatedApp struct {
	ID          int64
	ClientID    string
	Slug        string
	Name        string
	OwnerLogin  string
	OwnerType   string
	ExternalURL string
	Permissions map[string]string
	Events      []string
}

type AppWebhookConfig struct {
	URL         string
	ContentType string
	InsecureSSL string
}

// AppClient authenticates as the deployment GitHub App.
type AppClient struct {
	appID      int64
	privateKey *rsa.PrivateKey
	httpClient *http.Client
	baseURL    string
	now        func() time.Time
}

// NewAppClient parses an RSA private key and creates a GitHub App client.
func NewAppClient(appID int64, privateKeyPEM []byte) (*AppClient, error) {
	if appID <= 0 {
		return nil, fmt.Errorf("GitHub App ID must be positive")
	}
	privateKey, err := parseAppPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	return &AppClient{
		appID:      appID,
		privateKey: privateKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    githubAPIBase,
		now:        time.Now,
	}, nil
}

func parseAppPrivateKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("PEM block not found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return key, nil
}

// AppJWT creates a GitHub App JWT with the recommended one-minute clock skew
// and a lifetime below GitHub's ten-minute maximum.
func (c *AppClient) AppJWT() (string, error) {
	now := c.now().UTC()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(c.appID, 10),
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// GetInstallation returns App-authenticated installation metadata.
func (c *AppClient) GetInstallation(ctx context.Context, installationID int64) (AppInstallation, error) {
	var response struct {
		ID      int64 `json:"id"`
		Account struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
		Permissions map[string]string `json:"permissions"`
		SuspendedAt *time.Time        `json:"suspended_at"`
	}
	if err := c.appRequest(ctx, http.MethodGet, fmt.Sprintf("/app/installations/%d", installationID), nil, &response); err != nil {
		return AppInstallation{}, err
	}
	return AppInstallation{
		ID:           response.ID,
		AccountID:    response.Account.ID,
		AccountLogin: response.Account.Login,
		AccountType:  response.Account.Type,
		Permissions:  permissionLevels(response.Permissions),
		SuspendedAt:  response.SuspendedAt,
	}, nil
}

func (c *AppClient) GetAuthenticatedApp(ctx context.Context) (AuthenticatedApp, error) {
	var response struct {
		ID       int64  `json:"id"`
		ClientID string `json:"client_id"`
		Slug     string `json:"slug"`
		Name     string `json:"name"`
		Owner    struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
		ExternalURL string            `json:"external_url"`
		Permissions map[string]string `json:"permissions"`
		Events      []string          `json:"events"`
	}
	if err := c.appRequest(ctx, http.MethodGet, "/app", nil, &response); err != nil {
		return AuthenticatedApp{}, err
	}
	return AuthenticatedApp{
		ID: response.ID, ClientID: response.ClientID, Slug: response.Slug, Name: response.Name,
		OwnerLogin: response.Owner.Login, OwnerType: response.Owner.Type,
		ExternalURL: response.ExternalURL,
		Permissions: response.Permissions, Events: response.Events,
	}, nil
}

func (c *AppClient) GetWebhookConfig(ctx context.Context) (AppWebhookConfig, error) {
	var response struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
		InsecureSSL any    `json:"insecure_ssl"`
	}
	if err := c.appRequest(ctx, http.MethodGet, "/app/hook/config", nil, &response); err != nil {
		return AppWebhookConfig{}, err
	}
	return AppWebhookConfig{
		URL: response.URL, ContentType: response.ContentType,
		InsecureSSL: fmt.Sprint(response.InsecureSSL),
	}, nil
}

// MintInstallationToken requests a permission-scoped installation token.
func (c *AppClient) MintInstallationToken(
	ctx context.Context,
	installationID int64,
	permissions InstallationPermissions,
	repositories []string,
) (InstallationToken, error) {
	if installationID <= 0 {
		return InstallationToken{}, fmt.Errorf("installation ID must be positive")
	}
	if err := validateInstallationPermissions(permissions); err != nil {
		return InstallationToken{}, err
	}
	body := struct {
		Permissions  InstallationPermissions `json:"permissions,omitempty"`
		Repositories []string                `json:"repositories,omitempty"`
	}{Permissions: permissions, Repositories: repositories}
	var response struct {
		Token       string            `json:"token"`
		ExpiresAt   time.Time         `json:"expires_at"`
		Permissions map[string]string `json:"permissions"`
	}
	if err := c.appRequest(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/app/installations/%d/access_tokens", installationID),
		body,
		&response,
	); err != nil {
		return InstallationToken{}, err
	}
	if response.Token == "" {
		return InstallationToken{}, errors.New("GitHub returned an empty installation token")
	}
	return InstallationToken{
		Token:       response.Token,
		ExpiresAt:   response.ExpiresAt,
		Permissions: permissionLevels(response.Permissions),
		Principal: TokenPrincipal{
			Kind:           TokenCredentialInstallation,
			PrincipalID:    fmt.Sprintf("installation:%d", installationID),
			InstallationID: installationID,
		},
	}, nil
}

func (c *AppClient) appRequest(ctx context.Context, method, path string, body, out any) error {
	jwt, err := c.AppJWT()
	if err != nil {
		return err
	}
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal GitHub App request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+path, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub App request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxGitHubAppResponseSize+1))
	if readErr != nil {
		return fmt.Errorf("read GitHub App response: %w", readErr)
	}
	if len(responseBody) > maxGitHubAppResponseSize {
		return ErrGitHubAppResponseTooLarge
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return &GitHubAPIError{StatusCode: resp.StatusCode, Endpoint: path, Body: string(responseBody)}
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode GitHub App response: %w", err)
	}
	return nil
}

func validateInstallationPermissions(permissions InstallationPermissions) error {
	for name, level := range permissions {
		if strings.TrimSpace(name) == "" {
			return errors.New("installation permission name must not be empty")
		}
		if level != PermissionRead && level != PermissionWrite {
			return fmt.Errorf("invalid access level for installation permission %q", name)
		}
	}
	return nil
}

func permissionLevels(input map[string]string) InstallationPermissions {
	permissions := make(InstallationPermissions, len(input))
	for name, level := range input {
		permissions[name] = PermissionLevel(level)
	}
	return permissions
}

// CapabilitiesForPermissions maps GitHub's repository permissions to Kandev
// operations. Write permission implies read permission.
func CapabilitiesForPermissions(permissions InstallationPermissions) map[GitHubAppCapability]bool {
	capabilities := make(map[GitHubAppCapability]bool)
	grant := func(permission string, readCapability, writeCapability GitHubAppCapability) {
		level := permissions[permission]
		if level == PermissionRead || level == PermissionWrite {
			capabilities[readCapability] = true
		}
		if writeCapability != "" && level == PermissionWrite {
			capabilities[writeCapability] = true
		}
	}
	grant("metadata", CapabilityRepositoryRead, "")
	grant("contents", CapabilityGitRead, CapabilityGitWrite)
	grant("pull_requests", CapabilityPullRequestRead, CapabilityPullRequestWrite)
	grant("issues", CapabilityIssueRead, CapabilityIssueWrite)
	grant("checks", CapabilityChecksRead, "")
	grant("statuses", CapabilityStatusesRead, "")
	grant("actions", CapabilityActionsRead, "")
	grant("administration", CapabilityBranchProtectionRead, "")
	grant("members", CapabilityMembersRead, "")
	if permissions["workflows"] == PermissionWrite {
		capabilities[CapabilityWorkflowsWrite] = true
	}
	return capabilities
}
