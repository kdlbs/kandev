// Package credentials manages secure injection of API keys and secrets into agent containers.
package credentials

import (
	"context"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Credential represents a stored credential
type Credential struct {
	Key         string // Environment variable name (e.g., ANTHROPIC_API_KEY)
	Value       string // The secret value (never logged)
	Source      string // Where it came from (env, vault, file)
	Description string
}

// CredentialProvider interface for different secret sources
type CredentialProvider interface {
	// GetCredential retrieves a credential by key
	GetCredential(ctx context.Context, key string) (*Credential, error)

	// ListAvailable returns list of available credential keys
	ListAvailable(ctx context.Context) ([]string, error)

	// Name returns the provider name
	Name() string
}

// Manager manages credentials for agent containers
type Manager struct {
	providers []CredentialProvider
	cache     map[string]*Credential
	mu        sync.RWMutex
	logger    *logger.Logger
}

// NewManager creates a new credentials manager
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		providers: make([]CredentialProvider, 0),
		cache:     make(map[string]*Credential),
		logger:    log.WithFields(zap.String("component", "credentials-manager")),
	}
}

// AddProvider adds a credential provider
func (m *Manager) AddProvider(provider CredentialProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.providers = append(m.providers, provider)
	m.logger.Info("added credential provider", zap.String("provider", provider.Name()))
}

// GetCredentialValue retrieves just the value of a credential (implements lifecycle.CredentialsManager)
func (m *Manager) GetCredentialValue(ctx context.Context, key string) (string, error) {
	cred, err := m.GetCredential(ctx, key)
	if err != nil {
		return "", err
	}
	return cred.Value, nil
}

// GetCredential retrieves a credential from providers
func (m *Manager) GetCredential(ctx context.Context, key string) (*Credential, error) {
	// Check cache first
	m.mu.RLock()
	if cred, ok := m.cache[key]; ok {
		m.mu.RUnlock()
		return cred, nil
	}
	m.mu.RUnlock()

	// Try each provider
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, provider := range m.providers {
		cred, err := provider.GetCredential(ctx, key)
		if err == nil {
			m.cache[key] = cred
			m.logger.Debug("credential retrieved",
				zap.String("key", key),
				zap.String("source", cred.Source))
			return cred, nil
		}
	}

	return nil, fmt.Errorf("credential not found: %s", key)
}

// GetCredentials retrieves multiple credentials
func (m *Manager) GetCredentials(ctx context.Context, keys []string) (map[string]*Credential, error) {
	result := make(map[string]*Credential)
	var errs []string

	for _, key := range keys {
		cred, err := m.GetCredential(ctx, key)
		if err != nil {
			errs = append(errs, key)
			continue
		}
		result[key] = cred
	}

	if len(errs) > 0 {
		return result, fmt.Errorf("missing credentials: %v", errs)
	}

	return result, nil
}

// BuildEnvVars builds environment variables for required credentials
// Returns error if any required credential is missing
func (m *Manager) BuildEnvVars(ctx context.Context, required []string, additional map[string]string) ([]string, error) {
	envVars := make([]string, 0)

	// Add required credentials
	for _, key := range required {
		cred, err := m.GetCredential(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("required credential missing: %s", key)
		}
		envVars = append(envVars, fmt.Sprintf("%s=%s", cred.Key, cred.Value))
	}

	// Add additional environment variables
	for key, value := range additional {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	m.logger.Debug("built environment variables",
		zap.Int("required_count", len(required)),
		zap.Int("additional_count", len(additional)))

	return envVars, nil
}

// HasCredential checks if a credential is available
func (m *Manager) HasCredential(ctx context.Context, key string) bool {
	_, err := m.GetCredential(ctx, key)
	return err == nil
}

// ListAvailable lists all available credentials (keys only, not values)
func (m *Manager) ListAvailable(ctx context.Context) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keySet := make(map[string]struct{})

	for _, provider := range m.providers {
		keys, err := provider.ListAvailable(ctx)
		if err != nil {
			m.logger.Warn("failed to list credentials from provider",
				zap.String("provider", provider.Name()),
				zap.Error(err))
			continue
		}
		for _, key := range keys {
			keySet[key] = struct{}{}
		}
	}

	result := make([]string, 0, len(keySet))
	for key := range keySet {
		result = append(result, key)
	}

	return result
}

// ClearCache clears the credential cache
func (m *Manager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache = make(map[string]*Credential)
	m.logger.Debug("credential cache cleared")
}
