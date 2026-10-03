package cursorcloud

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/profiles"
)

type ModelLister interface {
	ListModels(context.Context) (ModelCatalog, error)
}

const (
	ExecutorConfigSecretID    = "cursor_cloud_api_key_secret_id"
	ExecutorConfigCallbackURL = "cursor_cloud_callback_url"
	ManagedCallbackPath       = "/api/v1/managed-agent-mcp"
	MockSelectorEnv           = "KANDEV_MOCK_CURSOR_CLOUD"
	MockOriginEnv             = "KANDEV_MOCK_CURSOR_CLOUD_BASE_URL"
)

// E2EMockEnabled is true only when the resolved runtime profile is e2e and its
// Cursor Cloud mock selector is enabled. Environment overrides alone cannot
// enable the mock in production or development.
func E2EMockEnabled() bool {
	return profiles.DetectEnvironment() == profiles.EnvE2E && isTruthy(os.Getenv(MockSelectorEnv))
}

// RuntimeAPIOrigin selects the provider origin for composition. A fixture URL
// is accepted only for an enabled feature in an explicitly selected e2e mock
// profile. Every other runtime retains the fixed production API origin.
func RuntimeAPIOrigin(featureEnabled bool) (string, error) {
	if !featureEnabled || !E2EMockEnabled() {
		return APIOrigin, nil
	}
	origin := strings.TrimSpace(os.Getenv(MockOriginEnv))
	if origin == "" {
		return "", fmt.Errorf("cursor cloud e2e mock origin is required")
	}
	if err := validateLoopbackHTTPOrigin(origin); err != nil {
		return "", fmt.Errorf("invalid cursor cloud e2e mock origin")
	}
	return origin, nil
}

// NewRuntimeClient composes the configured provider client without permitting
// development or production to redirect requests to an arbitrary origin.
func NewRuntimeClient(apiKey string, featureEnabled bool) (*Client, error) {
	origin, err := RuntimeAPIOrigin(featureEnabled)
	if err != nil {
		return nil, err
	}
	return NewClientWithConfig(ClientConfig{APIKey: apiKey, APIOrigin: origin})
}

// ValidateModel confirms that the selected model still exists in the current
// server-side provider catalog. It never sends a mutation to Cursor.
func ValidateModel(ctx context.Context, client ModelLister, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("cursor cloud model is required")
	}
	models, err := client.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("cursor cloud model catalog is unavailable")
	}
	for _, model := range models.Items {
		if model.ID == modelID {
			return nil
		}
		for _, alias := range model.Aliases {
			if alias == modelID {
				return nil
			}
		}
	}
	return fmt.Errorf("cursor cloud model is not in the current catalog")
}

// ValidateCallbackURL accepts HTTPS callback bases, plus loopback HTTP only
// for the explicitly selected Cursor Cloud e2e mock runtime.
func ValidateCallbackURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("callback URL must be an absolute HTTPS URL")
	}
	if parsed.Path != "" && parsed.Path != "/" && strings.TrimRight(parsed.Path, "/") != ManagedCallbackPath {
		return fmt.Errorf("callback URL path must use the managed MCP route")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == schemeHTTP && E2EMockEnabled() && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("callback URL must use HTTPS")
}

// ManagedCallbackURL appends a per-operation grant ID to a configured public
// callback base or to the managed MCP route prefix.
func ManagedCallbackURL(raw, grantID string) (string, error) {
	if grantID == "" || ValidateCallbackURL(raw) != nil {
		return "", fmt.Errorf("managed MCP callback URL is invalid")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("managed MCP callback URL is invalid")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = ManagedCallbackPath
	}
	parsed.Path = path + "/" + grantID
	return parsed.String(), nil
}

func validateLoopbackHTTPOrigin(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != schemeHTTP || !isLoopbackHost(parsed.Hostname()) || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("origin must be a loopback HTTP origin")
	}
	return nil
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
