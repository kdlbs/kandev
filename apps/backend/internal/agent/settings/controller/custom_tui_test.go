package controller

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestControllerWithRepo(t *testing.T) (*Controller, *registry.Registry) {
	ctrl, reg, _ := newTestControllerWithRepoAndStore(t)
	return ctrl, reg
}

func newTestControllerWithRepoAndStore(t *testing.T) (*Controller, *registry.Registry, store.Repository) {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, _, err := store.Provide(db, db)
	if err != nil {
		t.Fatal(err)
	}

	reg := registry.NewRegistry(log)
	ctrl := &Controller{
		repo:          repo,
		agentRegistry: reg,
		modelCache:    modelfetcher.NewCache(),
		logger:        log,
	}
	return ctrl, reg, repo
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Agent", "my-agent"},
		{"  hello  world  ", "hello-world"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "special-chars"},
		{"---leading-trailing---", "leading-trailing"},
		{"", ""},
		{"   ", ""},
		{"already-slug", "already-slug"},
		{"múltiple àccénts", "m-ltiple-cc-nts"},
		{"a", "a"},
		{"123", "123"},
		{"a--b", "a-b"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.expected {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCreateCustomTUIAgent_Success(t *testing.T) {
	ctrl, reg := newTestControllerWithRepo(t)
	ctx := context.Background()

	result, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "My Custom Agent",
		Model:       "best",
		Command:     "my-agent --yolo",
		Description: "A custom agent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Name != "my-custom-agent" {
		t.Errorf("expected name %q, got %q", "my-custom-agent", result.Name)
	}
	if result.TUIConfig == nil {
		t.Fatal("expected tui_config to be set")
	}
	if result.TUIConfig.Command != "my-agent --yolo" {
		t.Errorf("expected command %q, got %q", "my-agent --yolo", result.TUIConfig.Command)
	}
	if result.TUIConfig.DisplayName != "My Custom Agent" {
		t.Errorf("expected display name %q, got %q", "My Custom Agent", result.TUIConfig.DisplayName)
	}
	if len(result.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(result.Profiles))
	}
	if result.Profiles[0].Name != "best" {
		t.Errorf("expected profile name %q, got %q", "best", result.Profiles[0].Name)
	}
	if result.Profiles[0].Model != "passthrough" {
		t.Errorf("expected profile model %q, got %q", "passthrough", result.Profiles[0].Model)
	}

	// Verify registered in registry
	if !reg.Exists("my-custom-agent") {
		t.Error("expected agent to be registered in registry")
	}
}

func TestCreateCustomTUIAgent_ProfileNameFallsBackToDisplayName(t *testing.T) {
	ctrl, _ := newTestControllerWithRepo(t)
	ctx := context.Background()

	result, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "No Model Agent",
		Command:     "no-model-cli",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(result.Profiles))
	}
	if result.Profiles[0].Name != "No Model Agent" {
		t.Errorf("expected profile name %q, got %q", "No Model Agent", result.Profiles[0].Name)
	}
}

func TestCreateCustomTUIAgent_InvalidSlug(t *testing.T) {
	ctrl, _ := newTestControllerWithRepo(t)
	ctx := context.Background()

	_, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "!!!",
		Command:     "some-cli",
	})
	if err != ErrInvalidSlug {
		t.Errorf("expected ErrInvalidSlug, got %v", err)
	}
}

func TestCreateCustomTUIAgent_EmptyCommand(t *testing.T) {
	ctrl, _ := newTestControllerWithRepo(t)
	ctx := context.Background()

	_, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Valid Name",
		Command:     "",
	})
	if err != ErrCommandRequired {
		t.Errorf("expected ErrCommandRequired, got %v", err)
	}
}

func TestCreateCustomTUIAgent_RegistryConflict(t *testing.T) {
	ctrl, reg := newTestControllerWithRepo(t)
	ctx := context.Background()

	// Pre-register an agent in the registry
	_ = reg.RegisterCustomTUIAgent("my-agent", "My Agent", "my-cli", "", "", nil)

	_, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "My Agent",
		Command:     "other-cli",
	})
	if err != ErrAgentAlreadyExists {
		t.Errorf("expected ErrAgentAlreadyExists, got %v", err)
	}
}

func TestCreateCustomTUIAgent_DBConflict(t *testing.T) {
	ctrl, _ := newTestControllerWithRepo(t)
	ctx := context.Background()

	// Create first agent
	_, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Unique Agent",
		Command:     "unique-cli",
	})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Try to create with same name — should conflict
	_, err = ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Unique Agent",
		Command:     "other-cli",
	})
	if err != ErrAgentAlreadyExists {
		t.Errorf("expected ErrAgentAlreadyExists, got %v", err)
	}
}

func TestDeleteAgent_UnregistersFromRegistry(t *testing.T) {
	ctrl, reg := newTestControllerWithRepo(t)
	ctx := context.Background()

	// Create a custom TUI agent
	result, err := ctrl.CreateCustomTUIAgent(ctx, CreateCustomTUIAgentRequest{
		DisplayName: "Delete Me",
		Command:     "delete-cli",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if !reg.Exists("delete-me") {
		t.Fatal("expected agent to exist in registry before delete")
	}

	// Delete it
	if err := ctrl.DeleteAgent(ctx, result.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// Verify unregistered
	if reg.Exists("delete-me") {
		t.Error("expected agent to be unregistered from registry after delete")
	}
}

func TestUpdateProfile_AutoUpdatesNameWhenModelChanges(t *testing.T) {
	ctrl, repo := setupProfileUpdateTest(t)
	ctx := context.Background()

	agent := &models.Agent{Name: "test-agent"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	profile, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agent.ID,
		Name:    "Model A Display",
		Model:   "model-a",
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	// Update only the model (not the name)
	newModel := "model-b"
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:    profile.ID,
		Model: &newModel,
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}

	if updated.Name != "Model B Display" {
		t.Errorf("profile name = %q, want %q", updated.Name, "Model B Display")
	}
	if updated.Model != "model-b" {
		t.Errorf("profile model = %q, want %q", updated.Model, "model-b")
	}
}

func TestUpdateProfile_KeepsExplicitNameWhenProvided(t *testing.T) {
	ctrl, repo := setupProfileUpdateTest(t)
	ctx := context.Background()

	agent := &models.Agent{Name: "test-agent"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	profile, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agent.ID,
		Name:    "Original Name",
		Model:   "model-a",
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	// Update both model and name explicitly
	newModel := "model-b"
	newName := "My Custom Name"
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:    profile.ID,
		Model: &newModel,
		Name:  &newName,
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}

	if updated.Name != "My Custom Name" {
		t.Errorf("profile name = %q, want %q", updated.Name, "My Custom Name")
	}
}

// setupProfileUpdateTest creates a controller with a test agent that has multiple models.
func setupProfileUpdateTest(t *testing.T) (*Controller, store.Repository) {
	t.Helper()
	ctrl, reg, repo := newTestControllerWithRepoAndStore(t)
	ta := &testAgent{
		id:   "test-agent",
		name: "test-agent",
		modelList: &agents.ModelList{
			Models: []agents.Model{
				{ID: "model-a", Name: "Model A Display"},
				{ID: "model-b", Name: "Model B Display"},
			},
		},
	}
	if err := reg.Register(ta); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	return ctrl, repo
}
