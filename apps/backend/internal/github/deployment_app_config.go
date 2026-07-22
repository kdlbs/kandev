package github

import (
	"context"
	"errors"
	"strings"
)

type DeploymentAppSource string

const (
	DeploymentAppSourceNone    DeploymentAppSource = "none"
	DeploymentAppSourceManaged DeploymentAppSource = "managed"
)

type AppRegistrationRuntimeConfig struct {
	AppID         int64
	ClientID      string
	ClientSecret  string
	PrivateKey    string
	WebhookSecret string
	Slug          string
	PublicBaseURL string
}

func (c AppRegistrationRuntimeConfig) Validate() error {
	if c.AppID <= 0 || strings.TrimSpace(c.ClientID) == "" || c.ClientSecret == "" ||
		strings.TrimSpace(c.PrivateKey) == "" || c.WebhookSecret == "" ||
		strings.TrimSpace(c.Slug) == "" || strings.TrimSpace(c.PublicBaseURL) == "" {
		return errors.New("GitHub App registration runtime configuration is incomplete")
	}
	return nil
}

func (c AppRegistrationRuntimeConfig) PrivateKeyPEM() ([]byte, error) {
	if strings.TrimSpace(c.PrivateKey) == "" {
		return nil, errors.New("GitHub App private key is required")
	}
	return []byte(c.PrivateKey), nil
}

type ResolvedDeploymentAppConfig struct {
	Source       DeploymentAppSource
	Config       AppRegistrationRuntimeConfig
	Registration *AppRegistration
}

func ResolveAppRegistrationConfig(
	ctx context.Context,
	registrationID string,
	repository *AppRegistrationRepository,
) (ResolvedDeploymentAppConfig, error) {
	if repository == nil {
		return ResolvedDeploymentAppConfig{}, errors.New("GitHub App registration repository is not configured")
	}
	registration, credentials, err := repository.LoadRegistration(ctx, registrationID)
	resolved := ResolvedDeploymentAppConfig{
		Source: DeploymentAppSourceManaged, Registration: registration,
	}
	if err != nil || registration == nil {
		return resolved, err
	}
	if registration.Status != AppRegistrationStatusActive {
		return resolved, errors.New("GitHub App registration is not active")
	}
	resolved.Config = appRegistrationRuntimeConfig(registration, credentials)
	if err := resolved.Config.Validate(); err != nil {
		return resolved, err
	}
	return resolved, nil
}

func appRegistrationRuntimeConfig(
	registration *AppRegistration,
	credentials DeploymentAppCredentials,
) AppRegistrationRuntimeConfig {
	return AppRegistrationRuntimeConfig{
		AppID: registration.AppID, ClientID: registration.ClientID,
		ClientSecret: credentials.ClientSecret, PrivateKey: credentials.PrivateKey,
		WebhookSecret: credentials.WebhookSecret, Slug: registration.Slug,
		PublicBaseURL: registration.PublicBaseURL,
	}
}
