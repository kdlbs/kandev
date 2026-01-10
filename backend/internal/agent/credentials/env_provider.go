package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// knownAPIKeyPatterns contains patterns for known API key environment variables
var knownAPIKeyPatterns = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"GEMINI_API_KEY",
	"GOOGLE_API_KEY",
	"AZURE_OPENAI_API_KEY",
	"COHERE_API_KEY",
	"HUGGINGFACE_API_KEY",
	"MISTRAL_API_KEY",
	"TOGETHER_API_KEY",
	"REPLICATE_API_TOKEN",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"GCP_SERVICE_ACCOUNT_KEY",
	"GITHUB_TOKEN",
	"GITLAB_TOKEN",
	"BITBUCKET_TOKEN",
	"NPM_TOKEN",
	"DOCKER_PASSWORD",
	"DOCKER_TOKEN",
}

// EnvProvider provides credentials from environment variables
type EnvProvider struct {
	prefix string // Optional prefix filter (e.g., "KANDEV_")
}

// NewEnvProvider creates a new environment provider
func NewEnvProvider(prefix string) *EnvProvider {
	return &EnvProvider{
		prefix: prefix,
	}
}

// Name returns the provider name
func (p *EnvProvider) Name() string {
	return "environment"
}

// GetCredential retrieves a credential from environment variables
func (p *EnvProvider) GetCredential(ctx context.Context, key string) (*Credential, error) {
	// First try exact key
	value := os.Getenv(key)
	if value != "" {
		return &Credential{
			Key:    key,
			Value:  value,
			Source: "environment",
		}, nil
	}

	// Try with prefix
	if p.prefix != "" {
		prefixedKey := p.prefix + key
		value = os.Getenv(prefixedKey)
		if value != "" {
			return &Credential{
				Key:    key,
				Value:  value,
				Source: "environment",
			}, nil
		}
	}

	return nil, fmt.Errorf("credential not found: %s", key)
}

// ListAvailable returns list of available credential keys from environment
func (p *EnvProvider) ListAvailable(ctx context.Context) ([]string, error) {
	available := make([]string, 0)

	// Check known API key patterns
	for _, pattern := range knownAPIKeyPatterns {
		// Check exact match
		if os.Getenv(pattern) != "" {
			available = append(available, pattern)
			continue
		}

		// Check with prefix
		if p.prefix != "" && os.Getenv(p.prefix+pattern) != "" {
			available = append(available, pattern)
		}
	}

	// Also scan environment for any variables ending in common patterns
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}

		key := parts[0]

		// Skip if already in list
		found := false
		for _, existing := range available {
			if existing == key {
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Check for API key patterns in environment variable names
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "api_key") ||
			strings.Contains(lowerKey, "apikey") ||
			strings.Contains(lowerKey, "api-key") ||
			strings.Contains(lowerKey, "_token") ||
			strings.Contains(lowerKey, "_secret") {

			// Strip prefix if present
			if p.prefix != "" && strings.HasPrefix(key, p.prefix) {
				key = strings.TrimPrefix(key, p.prefix)
			}
			available = append(available, key)
		}
	}

	return available, nil
}

