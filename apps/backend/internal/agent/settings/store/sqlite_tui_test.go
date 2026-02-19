package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func newTestRepo(t *testing.T) Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestListTUIAgents_ReturnsOnlyTUIAgents(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create a regular agent (no tui_config)
	regular := &models.Agent{Name: "regular-agent"}
	if err := repo.CreateAgent(ctx, regular); err != nil {
		t.Fatal(err)
	}

	// Create a TUI agent (with tui_config)
	tui := &models.Agent{
		Name: "tui-agent",
		TUIConfig: &models.TUIConfigJSON{
			Command:     "my-cli",
			DisplayName: "TUI Agent",
		},
	}
	if err := repo.CreateAgent(ctx, tui); err != nil {
		t.Fatal(err)
	}

	// ListTUIAgents should only return the TUI agent
	tuiAgents, err := repo.ListTUIAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tuiAgents) != 1 {
		t.Fatalf("expected 1 TUI agent, got %d", len(tuiAgents))
	}
	if tuiAgents[0].Name != "tui-agent" {
		t.Errorf("expected name %q, got %q", "tui-agent", tuiAgents[0].Name)
	}
	if tuiAgents[0].TUIConfig == nil {
		t.Fatal("expected tui_config to be set")
	}
	if tuiAgents[0].TUIConfig.Command != "my-cli" {
		t.Errorf("expected command %q, got %q", "my-cli", tuiAgents[0].TUIConfig.Command)
	}

	// ListAgents should return both
	allAgents, err := repo.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(allAgents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(allAgents))
	}
}

func TestListTUIAgents_EmptyWhenNone(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	regular := &models.Agent{Name: "only-regular"}
	if err := repo.CreateAgent(ctx, regular); err != nil {
		t.Fatal(err)
	}

	tuiAgents, err := repo.ListTUIAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tuiAgents) != 0 {
		t.Errorf("expected 0 TUI agents, got %d", len(tuiAgents))
	}
}

func TestCreateAgent_TUIConfigRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.Agent{
		Name: "roundtrip-agent",
		TUIConfig: &models.TUIConfigJSON{
			Command:         "my-cli --flag",
			DisplayName:     "Round Trip",
			Model:           "best",
			Description:     "A round trip test",
			CommandArgs:     []string{"--extra"},
			WaitForTerminal: true,
		},
	}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TUIConfig == nil {
		t.Fatal("expected tui_config to be set after round-trip")
	}
	cfg := got.TUIConfig
	if cfg.Command != "my-cli --flag" {
		t.Errorf("command = %q, want %q", cfg.Command, "my-cli --flag")
	}
	if cfg.DisplayName != "Round Trip" {
		t.Errorf("display_name = %q, want %q", cfg.DisplayName, "Round Trip")
	}
	if cfg.Model != "best" {
		t.Errorf("model = %q, want %q", cfg.Model, "best")
	}
	if cfg.Description != "A round trip test" {
		t.Errorf("description = %q, want %q", cfg.Description, "A round trip test")
	}
	if len(cfg.CommandArgs) != 1 || cfg.CommandArgs[0] != "--extra" {
		t.Errorf("command_args = %v, want [--extra]", cfg.CommandArgs)
	}
	if !cfg.WaitForTerminal {
		t.Error("wait_for_terminal should be true")
	}
}

func TestGetAgentByName_TUIConfig(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.Agent{
		Name: "by-name-tui",
		TUIConfig: &models.TUIConfigJSON{
			Command:     "test-cli",
			DisplayName: "By Name",
		},
	}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetAgentByName(ctx, "by-name-tui")
	if err != nil {
		t.Fatal(err)
	}
	if got.TUIConfig == nil {
		t.Fatal("expected tui_config via GetAgentByName")
	}
	if got.TUIConfig.Command != "test-cli" {
		t.Errorf("command = %q, want %q", got.TUIConfig.Command, "test-cli")
	}
}
