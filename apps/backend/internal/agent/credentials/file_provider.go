package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileProvider provides credentials from a JSON file
type FileProvider struct {
	path        string
	credentials map[string]*Credential
	mu          sync.RWMutex
	loaded      bool
}

// NewFileProvider creates a new file provider
func NewFileProvider(path string) *FileProvider {
	return &FileProvider{
		path:        path,
		credentials: make(map[string]*Credential),
	}
}

// Name returns the provider name
func (p *FileProvider) Name() string {
	return "file"
}

// load loads credentials from the JSON file
// File format: {"ANTHROPIC_API_KEY": "sk-...", ...}
func (p *FileProvider) load() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.loaded {
		return nil
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, not an error - just no credentials
			p.loaded = true
			return nil
		}
		return fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Parse JSON file
	var rawCredentials map[string]string
	if err := json.Unmarshal(data, &rawCredentials); err != nil {
		return fmt.Errorf("failed to parse credentials file: %w", err)
	}

	// Convert to Credential objects
	for key, value := range rawCredentials {
		p.credentials[key] = &Credential{
			Key:    key,
			Value:  value,
			Source: "file",
		}
	}

	p.loaded = true
	return nil
}

// GetCredential retrieves a credential from the file
func (p *FileProvider) GetCredential(ctx context.Context, key string) (*Credential, error) {
	if err := p.load(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	cred, ok := p.credentials[key]
	if !ok {
		return nil, fmt.Errorf("credential not found: %s", key)
	}

	return cred, nil
}

// ListAvailable returns list of available credential keys from the file
func (p *FileProvider) ListAvailable(ctx context.Context) ([]string, error) {
	if err := p.load(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	keys := make([]string, 0, len(p.credentials))
	for key := range p.credentials {
		keys = append(keys, key)
	}

	return keys, nil
}

// Reload forces a reload of credentials from the file
func (p *FileProvider) Reload() error {
	p.mu.Lock()
	p.loaded = false
	p.credentials = make(map[string]*Credential)
	p.mu.Unlock()

	return p.load()
}
