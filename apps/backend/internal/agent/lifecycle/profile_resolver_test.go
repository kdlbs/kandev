package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

// MockRepository implements store.Repository for testing
type MockRepository struct {
	GetAgentFn          func(ctx context.Context, id string) (*models.Agent, error)
	GetAgentByNameFn    func(ctx context.Context, name string) (*models.Agent, error)
	GetAgentProfileFn   func(ctx context.Context, id string) (*models.AgentProfile, error)
	ListAgentsFn        func(ctx context.Context) ([]*models.Agent, error)
	ListAgentProfilesFn func(ctx context.Context, agentID string) ([]*models.AgentProfile, error)
}

var _ store.Repository = (*MockRepository)(nil)

func (m *MockRepository) CreateAgent(ctx context.Context, agent *models.Agent) error {
	return nil
}

func (m *MockRepository) GetAgent(ctx context.Context, id string) (*models.Agent, error) {
	if m.GetAgentFn != nil {
		return m.GetAgentFn(ctx, id)
	}
	return nil, errors.New("agent not found")
}

func (m *MockRepository) GetAgentByName(ctx context.Context, name string) (*models.Agent, error) {
	if m.GetAgentByNameFn != nil {
		return m.GetAgentByNameFn(ctx, name)
	}
	return nil, errors.New("agent not found")
}

func (m *MockRepository) UpdateAgent(ctx context.Context, agent *models.Agent) error {
	return nil
}

func (m *MockRepository) DeleteAgent(ctx context.Context, id string) error {
	return nil
}

func (m *MockRepository) ListAgents(ctx context.Context) ([]*models.Agent, error) {
	if m.ListAgentsFn != nil {
		return m.ListAgentsFn(ctx)
	}
	return []*models.Agent{}, nil
}

func (m *MockRepository) GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*models.AgentProfileMcpConfig, error) {
	return nil, errors.New("not implemented")
}

func (m *MockRepository) UpsertAgentProfileMcpConfig(ctx context.Context, config *models.AgentProfileMcpConfig) error {
	return nil
}

func (m *MockRepository) CreateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	return nil
}

func (m *MockRepository) UpdateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	return nil
}

func (m *MockRepository) DeleteAgentProfile(ctx context.Context, id string) error {
	return nil
}

func (m *MockRepository) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	if m.GetAgentProfileFn != nil {
		return m.GetAgentProfileFn(ctx, id)
	}
	return nil, errors.New("profile not found")
}

func (m *MockRepository) ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error) {
	if m.ListAgentProfilesFn != nil {
		return m.ListAgentProfilesFn(ctx, agentID)
	}
	return []*models.AgentProfile{}, nil
}

func (m *MockRepository) Close() error {
	return nil
}

func TestNewStoreProfileResolver(t *testing.T) {
	mockRepo := &MockRepository{}

	resolver := NewStoreProfileResolver(mockRepo, nil)

	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	if resolver.store != mockRepo {
		t.Error("expected resolver to use the provided store")
	}
}

func TestStoreProfileResolver_ResolveProfile_Success(t *testing.T) {
	mockRepo := &MockRepository{
		GetAgentProfileFn: func(ctx context.Context, id string) (*models.AgentProfile, error) {
			return &models.AgentProfile{
				ID:                         "profile-123",
				AgentID:                    "agent-456",
				Name:                       "My Profile",
				Model:                      "claude-3.5-sonnet",
				AutoApprove:                true,
				DangerouslySkipPermissions: false,
			}, nil
		},
		GetAgentFn: func(ctx context.Context, id string) (*models.Agent, error) {
			return &models.Agent{
				ID:   "agent-456",
				Name: "claude",
			}, nil
		},
	}

	resolver := NewStoreProfileResolver(mockRepo, nil)
	ctx := context.Background()

	info, err := resolver.ResolveProfile(ctx, "profile-123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil profile info")
	}
	if info.ProfileID != "profile-123" {
		t.Errorf("expected ProfileID 'profile-123', got '%s'", info.ProfileID)
	}
	if info.ProfileName != "My Profile" {
		t.Errorf("expected ProfileName 'My Profile', got '%s'", info.ProfileName)
	}
	if info.AgentID != "agent-456" {
		t.Errorf("expected AgentID 'agent-456', got '%s'", info.AgentID)
	}
	if info.AgentName != "claude" {
		t.Errorf("expected AgentName 'claude', got '%s'", info.AgentName)
	}
	if info.Model != "claude-3.5-sonnet" {
		t.Errorf("expected Model 'claude-3.5-sonnet', got '%s'", info.Model)
	}
	if info.AutoApprove != true {
		t.Error("expected AutoApprove to be true")
	}
	if info.DangerouslySkipPermissions != false {
		t.Error("expected DangerouslySkipPermissions to be false")
	}
}

func TestStoreProfileResolver_ResolveProfile_ProfileNotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetAgentProfileFn: func(ctx context.Context, id string) (*models.AgentProfile, error) {
			return nil, errors.New("profile not found")
		},
	}

	resolver := NewStoreProfileResolver(mockRepo, nil)
	ctx := context.Background()

	info, err := resolver.ResolveProfile(ctx, "non-existent-profile")

	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
	if info != nil {
		t.Error("expected nil profile info on error")
	}
	if !errors.Is(err, errors.Unwrap(err)) && err.Error() == "" {
		t.Error("expected error to contain message")
	}
	// Verify the error message contains "profile not found"
	expectedMsg := "profile not found"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("expected error message to start with '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestStoreProfileResolver_ResolveProfile_AgentNotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetAgentProfileFn: func(ctx context.Context, id string) (*models.AgentProfile, error) {
			return &models.AgentProfile{
				ID:      "profile-123",
				AgentID: "non-existent-agent",
				Name:    "My Profile",
				Model:   "gpt-4",
			}, nil
		},
		GetAgentFn: func(ctx context.Context, id string) (*models.Agent, error) {
			return nil, errors.New("agent not found")
		},
	}

	resolver := NewStoreProfileResolver(mockRepo, nil)
	ctx := context.Background()

	info, err := resolver.ResolveProfile(ctx, "profile-123")

	if err == nil {
		t.Fatal("expected error when agent not found")
	}
	if info != nil {
		t.Error("expected nil profile info on error")
	}
	// Verify the error message contains "agent not found for profile"
	expectedMsg := "agent not found for profile"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("expected error message to start with '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestStoreProfileResolver_ResolveProfile_FallbackToRegistryDefaultModel(t *testing.T) {
	mockRepo := &MockRepository{
		GetAgentProfileFn: func(ctx context.Context, id string) (*models.AgentProfile, error) {
			return &models.AgentProfile{
				ID:          "profile-123",
				AgentID:     "agent-456",
				Name:        "Default Profile",
				Model:       "", // Empty model - should fallback to registry
				AutoApprove: false,
			}, nil
		},
		GetAgentFn: func(ctx context.Context, id string) (*models.Agent, error) {
			return &models.Agent{
				ID:   "agent-456",
				Name: "claude-code", // Agent name matches registry key
			}, nil
		},
	}

	// Create a registry with a default model for claude-code
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	reg := registry.NewRegistry(log)
	err := reg.Register(&registry.AgentTypeConfig{
		ID:   "claude-code",
		Name: "claude-code",
		Cmd:  []string{"claude"}, // Standalone agent uses Cmd
		ResourceLimits: registry.ResourceLimits{
			MemoryMB:       1024,
			CPUCores:       1,
			TimeoutSeconds: 3600,
		},
		ModelConfig: registry.ModelConfig{
			DefaultModel: "claude-sonnet-4-20250514",
		},
	})
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	resolver := NewStoreProfileResolver(mockRepo, reg)
	ctx := context.Background()

	info, err := resolver.ResolveProfile(ctx, "profile-123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil profile info")
	}
	// Should have fallback to registry's default model
	if info.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected Model 'claude-sonnet-4-20250514' (from registry), got '%s'", info.Model)
	}
}

func TestStoreProfileResolver_ResolveProfile_EmptyModelNoRegistry(t *testing.T) {
	mockRepo := &MockRepository{
		GetAgentProfileFn: func(ctx context.Context, id string) (*models.AgentProfile, error) {
			return &models.AgentProfile{
				ID:      "profile-123",
				AgentID: "agent-456",
				Name:    "Default Profile",
				Model:   "", // Empty model
			}, nil
		},
		GetAgentFn: func(ctx context.Context, id string) (*models.Agent, error) {
			return &models.Agent{
				ID:   "agent-456",
				Name: "custom-agent",
			}, nil
		},
	}

	// No registry provided
	resolver := NewStoreProfileResolver(mockRepo, nil)
	ctx := context.Background()

	info, err := resolver.ResolveProfile(ctx, "profile-123")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil profile info")
	}
	// Model should remain empty since no registry fallback available
	if info.Model != "" {
		t.Errorf("expected empty Model when no registry fallback, got '%s'", info.Model)
	}
}
