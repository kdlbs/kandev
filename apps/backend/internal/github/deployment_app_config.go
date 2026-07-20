package github

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/config"
)

type DeploymentAppSource string

const (
	DeploymentAppSourceNone        DeploymentAppSource = "none"
	DeploymentAppSourceEnvironment DeploymentAppSource = "environment"
	DeploymentAppSourceManaged     DeploymentAppSource = "managed"
)

// ResolvedDeploymentAppConfig identifies the authoritative deployment source
// and the complete runtime configuration when one is usable.
type ResolvedDeploymentAppConfig struct {
	Source       DeploymentAppSource
	Config       config.GitHubAppConfig
	Registration *DeploymentAppRegistration
}

// ResolveDeploymentAppConfig applies strict environment > managed > none
// precedence. Any partial environment configuration remains authoritative.
func ResolveDeploymentAppConfig(
	ctx context.Context,
	environment config.GitHubAppConfig,
	repository *DeploymentAppRepository,
) (ResolvedDeploymentAppConfig, error) {
	if environment.Configured() {
		resolved := ResolvedDeploymentAppConfig{
			Source: DeploymentAppSourceEnvironment,
			Config: environment,
		}
		if err := environment.Validate(); err != nil {
			return resolved, fmt.Errorf("invalid environment GitHub App configuration: %w", err)
		}
		return resolved, nil
	}
	if repository == nil {
		return ResolvedDeploymentAppConfig{}, fmt.Errorf("deployment App repository is not configured")
	}
	registration, credentials, err := repository.LoadManagedRegistration(ctx)
	resolved := ResolvedDeploymentAppConfig{
		Source:       DeploymentAppSourceManaged,
		Registration: registration,
	}
	if registration == nil {
		if err != nil {
			return resolved, err
		}
		resolved.Source = DeploymentAppSourceNone
		return resolved, nil
	}
	if err != nil {
		return resolved, err
	}
	resolved.Config = config.GitHubAppConfig{
		AppID: registration.AppID, ClientID: registration.ClientID,
		ClientSecret: credentials.ClientSecret, PrivateKey: credentials.PrivateKey,
		WebhookSecret: credentials.WebhookSecret, Slug: registration.Slug,
		PublicBaseURL: registration.PublicBaseURL,
	}
	if err := resolved.Config.Validate(); err != nil {
		return resolved, fmt.Errorf("invalid managed GitHub App configuration: %w", err)
	}
	return resolved, nil
}
