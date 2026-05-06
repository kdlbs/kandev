package github

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// mockSecretManager implements SecretManager for testing.
type mockSecretManager struct {
	secrets map[string]string // id -> value
	names   map[string]string // id -> name
	nextID  int
	mu      sync.Mutex
}

func newMockSecretManager() *mockSecretManager {
	return &mockSecretManager{
		secrets: make(map[string]string),
		names:   make(map[string]string),
	}
}

func (m *mockSecretManager) Create(_ context.Context, name, value string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("secret-%d", m.nextID)
	m.secrets[id] = value
	m.names[id] = name
	return id, nil
}

func (m *mockSecretManager) Update(_ context.Context, id, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.secrets[id]; !ok {
		return fmt.Errorf("secret not found: %s", id)
	}
	m.secrets[id] = value
	return nil
}

func (m *mockSecretManager) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.secrets, id)
	delete(m.names, id)
	return nil
}

// mockSecretProvider implements SecretProvider for testing.
type mockSecretProvider struct {
	mgr *mockSecretManager
}

func (m *mockSecretProvider) List(_ context.Context) ([]*SecretListItem, error) {
	m.mgr.mu.Lock()
	defer m.mgr.mu.Unlock()
	var items []*SecretListItem
	for id, name := range m.mgr.names {
		items = append(items, &SecretListItem{
			ID:       id,
			Name:     name,
			HasValue: m.mgr.secrets[id] != "",
		})
	}
	return items, nil
}

func (m *mockSecretProvider) Reveal(_ context.Context, id string) (string, error) {
	m.mgr.mu.Lock()
	defer m.mgr.mu.Unlock()
	if val, ok := m.mgr.secrets[id]; ok {
		return val, nil
	}
	return "", fmt.Errorf("secret not found: %s", id)
}

func TestFindGitHubTokenSecret(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}

	svc := &Service{
		secrets: provider,
		logger:  log,
	}

	// No secrets - should return empty
	id, found := svc.findGitHubTokenSecret(context.Background())
	if found {
		t.Error("expected no token to be found when secrets are empty")
	}
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}

	// Add a GITHUB_TOKEN secret
	secretID, _ := mgr.Create(context.Background(), "GITHUB_TOKEN", "ghp_test123")

	id, found = svc.findGitHubTokenSecret(context.Background())
	if !found {
		t.Error("expected token to be found")
	}
	if id != secretID {
		t.Errorf("expected id %q, got %q", secretID, id)
	}
}

func TestFindGitHubTokenSecret_LowerCase(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}

	svc := &Service{
		secrets: provider,
		logger:  log,
	}

	// Add a github_token (lowercase) secret
	secretID, _ := mgr.Create(context.Background(), "github_token", "ghp_test456")

	id, found := svc.findGitHubTokenSecret(context.Background())
	if !found {
		t.Error("expected token to be found with lowercase name")
	}
	if id != secretID {
		t.Errorf("expected id %q, got %q", secretID, id)
	}
}

func TestGetStatus_WithTokenConfigured(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}

	// Add a GITHUB_TOKEN secret
	secretID, _ := mgr.Create(context.Background(), "GITHUB_TOKEN", "ghp_test789")

	svc := &Service{
		secrets:    provider,
		logger:     log,
		authMethod: AuthMethodNone,
	}

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if !status.TokenConfigured {
		t.Error("expected TokenConfigured to be true")
	}
	if status.TokenSecretID != secretID {
		t.Errorf("expected TokenSecretID %q, got %q", secretID, status.TokenSecretID)
	}
	if len(status.RequiredScopes) == 0 {
		t.Error("expected RequiredScopes to be populated")
	}
}

func TestGetStatus_WithoutToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}

	svc := &Service{
		secrets:    provider,
		logger:     log,
		authMethod: AuthMethodNone,
	}

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.TokenConfigured {
		t.Error("expected TokenConfigured to be false")
	}
	if status.TokenSecretID != "" {
		t.Errorf("expected empty TokenSecretID, got %q", status.TokenSecretID)
	}
}

func TestClearToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}

	// Add a GITHUB_TOKEN secret
	_, err := mgr.Create(context.Background(), "GITHUB_TOKEN", "ghp_toberemoved")
	if err != nil {
		t.Fatalf("failed to create secret: %v", err)
	}

	svc := &Service{
		secrets:       provider,
		secretManager: mgr,
		logger:        log,
		authMethod:    AuthMethodPAT,
	}

	// Verify token exists
	_, found := svc.findGitHubTokenSecret(context.Background())
	if !found {
		t.Fatal("expected token to exist before clearing")
	}

	// Clear the token
	err = svc.ClearToken(context.Background())
	if err != nil {
		t.Fatalf("ClearToken failed: %v", err)
	}

	// Verify token is gone
	_, found = svc.findGitHubTokenSecret(context.Background())
	if found {
		t.Error("expected token to be removed after clearing")
	}

	// After ClearToken, authMethod should no longer be "pat" (it resets to "none"
	// then retryClientCreation may set it to "gh_cli" if gh CLI is available).
	if svc.authMethod == AuthMethodPAT {
		t.Error("expected authMethod to no longer be PAT after clearing token")
	}
}

func TestClearToken_NoToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}

	svc := &Service{
		secrets:       provider,
		secretManager: mgr,
		logger:        log,
	}

	// Clearing when no token exists should not error
	err := svc.ClearToken(context.Background())
	if err != nil {
		t.Fatalf("ClearToken should not error when no token exists: %v", err)
	}
}

func TestClearToken_NoSecretManager(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})

	svc := &Service{
		secrets:       nil,
		secretManager: nil,
		logger:        log,
	}

	err := svc.ClearToken(context.Background())
	if err == nil {
		t.Error("expected error when secretManager is nil")
	}
}

// Regression: PAT clients minted via the service must share the rate tracker.
// ConfigureToken previously created a bare NewPATClient and dropped quota
// signals on the floor, so the UI / health / poller throttling all went stale
// after a token reconfigure.
func TestService_NewPATClient_AttachesRateTracker(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := &Service{
		logger:      log,
		rateTracker: NewRateTracker(nil, nil),
	}

	c := svc.newPATClient("ghp_x")
	if c == nil {
		t.Fatalf("newPATClient returned nil")
	}
	if c.rateTracker != svc.rateTracker {
		t.Fatalf("expected rate tracker to be wired onto the new PAT client")
	}
}
