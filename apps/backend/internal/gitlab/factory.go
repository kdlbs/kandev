package gitlab

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SecretProvider is the interface the factory uses to look up a GitLab PAT.
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

// NewClient builds a GitLab client using the best available auth method.
// Selection order: mock env var → glab CLI (if authenticated for host) →
// PAT (env var GITLAB_TOKEN, then secrets store name GITLAB_TOKEN /
// gitlab_token) → noop.
//
// host configures the GitLab base URL; pass "" for the public default.
func NewClient(ctx context.Context, host string, secrets SecretProvider, log *logger.Logger) (Client, string, error) {
	if host == "" {
		host = DefaultHost
	}

	if os.Getenv("KANDEV_MOCK_GITLAB") == "true" {
		log.Info("using mock client for GitLab integration")
		return NewMockClient(host), "mock", nil
	}

	if GLabAvailable() {
		client, err := NewGLabClient(ctx, host)
		if err == nil {
			log.Info("using glab CLI for GitLab integration", zap.String("host", host))
			return client, AuthMethodGLab, nil
		}
		log.Debug("glab CLI available but not authenticated", zap.Error(err))
	}

	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		log.Info("using GITLAB_TOKEN from environment for GitLab integration")
		return NewPATClient(host, token), AuthMethodPAT, nil
	}

	if secrets != nil {
		token, err := findPAT(ctx, secrets)
		if err == nil && token != "" {
			log.Info("using PAT from secrets store for GitLab integration")
			return NewPATClient(host, token), AuthMethodPAT, nil
		}
		if err != nil {
			log.Debug("failed to find GitLab PAT in secrets", zap.Error(err))
		}
	}

	return NewNoopClient(host), AuthMethodNone, nil
}

// findPAT looks for a secret named "GITLAB_TOKEN" or "gitlab_token".
func findPAT(ctx context.Context, secrets SecretProvider) (string, error) {
	items, err := secrets.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list secrets: %w", err)
	}
	for _, item := range items {
		if !item.HasValue {
			continue
		}
		if item.Name == "GITLAB_TOKEN" || item.Name == "gitlab_token" {
			return secrets.Reveal(ctx, item.ID)
		}
	}
	return "", nil
}
