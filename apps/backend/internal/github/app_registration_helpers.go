package github

import (
	"context"
	"errors"
	"slices"
	"strings"
)

var (
	ErrDeploymentAppOperatorRequired      = errors.New("deployment GitHub App operator access is required")
	ErrDeploymentAppIdentityMismatch      = errors.New("GitHub App manifest result did not match the requested owner")
	ErrDeploymentAppPolicyMismatch        = errors.New("GitHub App manifest result did not match the required policy")
	ErrDeploymentAppRegistrationCancelled = errors.New("GitHub App registration was cancelled")
)

const defaultGitHubAppHost = "github.com"

type DeploymentAppManifestConverter interface {
	Convert(context.Context, string) (ManifestConversionResult, error)
}

func requireDeploymentAppOperator(userID string) error {
	if userID != DefaultUserID {
		return ErrDeploymentAppOperatorRequired
	}
	return nil
}

func deploymentOwnerType(ownerType ManifestOwnerType) DeploymentAppOwnerType {
	if ownerType == ManifestOwnerOrganization {
		return DeploymentAppOwnerOrganization
	}
	return DeploymentAppOwnerUser
}

func deploymentOwnerTypeFromGitHub(ownerType string) DeploymentAppOwnerType {
	if strings.EqualFold(ownerType, string(DeploymentAppOwnerOrganization)) {
		return DeploymentAppOwnerOrganization
	}
	return DeploymentAppOwnerUser
}

func matchesDeploymentAppOwner(
	flow *DeploymentAppRegistrationFlow,
	converted ManifestConversionResult,
) bool {
	return flow != nil && strings.EqualFold(flow.OwnerLogin, converted.Owner.Login) &&
		strings.EqualFold(string(flow.OwnerType), converted.Owner.Type)
}

func matchesDeploymentAppPolicy(converted ManifestConversionResult) bool {
	submission, err := BuildDeploymentAppManifest(
		ManifestOwnerUser, "policy", "https://kandev.example",
	)
	if err != nil {
		return false
	}
	if len(converted.Permissions) != len(submission.Manifest.DefaultPermissions) ||
		len(converted.Events) != len(submission.Manifest.DefaultEvents) {
		return false
	}
	for permission, level := range submission.Manifest.DefaultPermissions {
		if converted.Permissions[permission] != level {
			return false
		}
	}
	for _, event := range submission.Manifest.DefaultEvents {
		if !slices.Contains(converted.Events, event) {
			return false
		}
	}
	return true
}

func resolvedDeploymentAppRegistration(
	registration *DeploymentAppRegistration,
	credentials DeploymentAppCredentials,
) ResolvedDeploymentAppConfig {
	return ResolvedDeploymentAppConfig{
		Source: DeploymentAppSourceManaged, Registration: registration,
		Config: appRegistrationRuntimeConfig(registration, credentials),
	}
}

func cloneDeploymentAppRegistration(value *DeploymentAppRegistration) *DeploymentAppRegistration {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sanitizeDeploymentAppActivationError(_ error) error {
	return errors.New("generated GitHub App credentials could not be activated")
}

func sanitizeDeploymentAppActivationAndCleanupError(activationErr, cleanupErr error) error {
	activationErr = sanitizeDeploymentAppActivationError(activationErr)
	if cleanupErr == nil {
		return activationErr
	}
	return errors.Join(
		activationErr,
		errors.New("generated GitHub App credentials could not be cleaned up; retry deletion"),
	)
}

func sanitizeDeploymentAppPersistenceError(err error) error {
	if errors.Is(err, ErrDeploymentAppInUse) {
		return ErrDeploymentAppInUse
	}
	return errors.New("generated GitHub App credentials could not be stored")
}
