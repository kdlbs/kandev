package controller

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestControllerWithRepo(t *testing.T) (*Controller, *registry.Registry) {
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
	return ctrl, reg
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
