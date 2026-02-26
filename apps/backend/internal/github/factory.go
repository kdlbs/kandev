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

// NewClient creates a GitHub client using the best available auth method.
// It tries the gh CLI first, then falls back to a PAT from the secret store.
func NewClient(ctx context.Context, secrets SecretProvider, log *logger.Logger) (Client, string, error) {
	// Mock client for E2E testing
	if os.Getenv("KANDEV_MOCK_GITHUB") == "true" {
		log.Info("using mock client for GitHub integration")
		return NewMockClient(), "mock", nil
	}

	// Try gh CLI first
	if GHAvailable() {
		ghClient := NewGHClient()
		ok, err := ghClient.IsAuthenticated(ctx)
		if err == nil && ok {
			log.Info("using gh CLI for GitHub integration")
			return ghClient, "gh_cli", nil
		}
		log.Debug("gh CLI available but not authenticated", zap.Error(err))
	}

	// Fall back to PAT from secrets store
	if secrets != nil {
		token, err := findGitHubPAT(ctx, secrets)
		if err == nil && token != "" {
			log.Info("using PAT from secrets store for GitHub integration")
			return NewPATClient(token), "pat", nil
		}
		if err != nil {
			log.Debug("failed to find GitHub PAT in secrets", zap.Error(err))
		}
	}

	return &NoopClient{}, "none", nil
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
