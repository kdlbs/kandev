package github

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SecretProvider is the interface the factory uses to look up a GitHub PAT.
type SecretProvider interface {
	List(ctx context.Context) ([]*SecretListItem, error)
	Reveal(ctx context.Context, id string) (string, error)
}

// SecretListItem mirrors secrets.SecretListItem to avoid a direct import cycle.
type SecretListItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	HasValue bool   `json:"has_value"`
}

const legacyMockAuthMethod = "mock"

// NewClient creates a GitHub client using the best available auth method.
// It tries the gh CLI first, then falls back to a PAT from the secret store.
func NewClient(ctx context.Context, secrets SecretProvider, log *logger.Logger) (Client, string, error) {
	client, method, _, err := newLegacyCredential(ctx, secrets, log, false)
	return client, method, err
}

// newLegacyGitTransportCredential resolves the same ambient credential used by
// migrated legacy_shared API access and additionally returns its raw token for
// in-memory Git transport use. The token is never exposed through Client.
func newLegacyGitTransportCredential(
	ctx context.Context,
	secrets SecretProvider,
	log *logger.Logger,
) (Client, string, string, error) {
	return newLegacyCredential(ctx, secrets, log, true)
}

func newLegacyCredential(
	ctx context.Context,
	secrets SecretProvider,
	log *logger.Logger,
	includeTransportCredential bool,
) (Client, string, string, error) {
	// Mock client for E2E testing
	if os.Getenv("KANDEV_MOCK_GITHUB") == "true" {
		log.Info("using mock client for GitHub integration")
		credential := ""
		if includeTransportCredential {
			credential = legacyMockAuthMethod
		}
		return NewMockClient(), legacyMockAuthMethod, credential, nil
	}

	// gh inherits GH_TOKEN/GITHUB_TOKEN for API calls, while exact-account
	// token resolution deliberately strips them. For Git transport, resolve
	// an ambient token directly so the API and clone identities cannot diverge.
	// GH_TOKEN follows gh's documented precedence over GITHUB_TOKEN.
	if includeTransportCredential {
		if token := legacyEnvironmentToken(); token != "" {
			log.Info("using environment PAT for legacy GitHub Git transport")
			return NewPATClient(token), AuthMethodPAT, token, nil
		}
	}

	if client, method, credential, found, err := resolveLegacyGHCredential(
		ctx, log, includeTransportCredential,
	); found || err != nil {
		return client, method, credential, err
	}

	// Fall back to PAT from environment variable
	if token := legacyEnvironmentToken(); token != "" {
		log.Info("using environment PAT for GitHub integration")
		return NewPATClient(token), AuthMethodPAT, transportCredential(token, includeTransportCredential), nil
	}

	// Fall back to PAT from secrets store
	if secrets != nil {
		token, err := findGitHubPAT(ctx, secrets)
		if err == nil && token != "" {
			log.Info("using PAT from secrets store for GitHub integration")
			return NewPATClient(token), AuthMethodPAT, transportCredential(token, includeTransportCredential), nil
		}
		if err != nil {
			log.Debug("failed to find GitHub PAT in secrets", zap.Error(err))
		}
	}

	return &NoopClient{}, AuthMethodNone, "", nil
}

func resolveLegacyGHCredential(
	ctx context.Context,
	log *logger.Logger,
	includeTransportCredential bool,
) (Client, string, string, bool, error) {
	if !GHAvailable() {
		return nil, "", "", false, nil
	}

	ghClient := NewGHClient()
	ok, err := ghClient.IsAuthenticated(ctx)
	if err != nil || !ok {
		log.Debug("gh CLI available but not authenticated", zap.Error(err))
		return nil, "", "", false, nil
	}

	log.Info("using gh CLI for GitHub integration")
	if !includeTransportCredential {
		return ghClient, string(ConnectionSourceGHCLI), "", true, nil
	}
	login, err := ghClient.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, AuthMethodNone, "", false, err
	}
	token, err := ResolveGHAccountToken(ctx, defaultGitHubHost, login)
	if err != nil {
		return nil, AuthMethodNone, "", false, err
	}
	return ghClient, string(ConnectionSourceGHCLI), token, true, nil
}

func legacyEnvironmentToken() string {
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GITHUB_TOKEN")
}

func transportCredential(token string, include bool) string {
	if include {
		return token
	}
	return ""
}

// findGitHubPAT looks for a secret named "GITHUB_TOKEN" or "github_token".
func findGitHubPAT(ctx context.Context, secrets SecretProvider) (string, error) {
	items, err := secrets.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list secrets: %w", err)
	}
	for _, item := range items {
		if !item.HasValue {
			continue
		}
		name := item.Name
		if name == "GITHUB_TOKEN" || name == "github_token" {
			return secrets.Reveal(ctx, item.ID)
		}
	}
	return "", nil
}
