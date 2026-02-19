package credentials

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AugmentSessionProvider provides AUGMENT_SESSION_AUTH from ~/.augment/session.json
type AugmentSessionProvider struct {
	sessionPath string
	sessionData string
	loaded      bool
	mu          sync.RWMutex
}

// NewAugmentSessionProvider creates a provider that reads Augment session credentials
func NewAugmentSessionProvider() *AugmentSessionProvider {
	// Default to ~/.augment/session.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp"
	}
	sessionPath := filepath.Join(homeDir, ".augment", "session.json")

	return &AugmentSessionProvider{
		sessionPath: sessionPath,
	}
}

// Name returns the provider name
func (p *AugmentSessionProvider) Name() string {
	return "augment-session"
}

// load reads the session.json file content
func (p *AugmentSessionProvider) load() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.loaded {
		return nil
	}

	data, err := os.ReadFile(p.sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			p.loaded = true
			return nil
		}
		return fmt.Errorf("failed to read augment session file: %w", err)
	}

	// Store the entire JSON content as the credential value
	p.sessionData = string(data)
	p.loaded = true
	return nil
}

// GetCredential retrieves the AUGMENT_SESSION_AUTH credential
func (p *AugmentSessionProvider) GetCredential(ctx context.Context, key string) (*Credential, error) {
	if key != "AUGMENT_SESSION_AUTH" {
		return nil, fmt.Errorf("credential not found: %s", key)
	}

	if err := p.load(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.sessionData == "" {
		return nil, fmt.Errorf("augment session not found at %s", p.sessionPath)
	}

	return &Credential{
		Key:         "AUGMENT_SESSION_AUTH",
		Value:       p.sessionData,
		Source:      "augment-session",
		Description: "Augment session credentials from ~/.augment/session.json",
	}, nil
}

// ListAvailable returns AUGMENT_SESSION_AUTH if the session file exists
func (p *AugmentSessionProvider) ListAvailable(ctx context.Context) ([]string, error) {
	if err := p.load(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.sessionData != "" {
		return []string{"AUGMENT_SESSION_AUTH"}, nil
	}

	return []string{}, nil
}
