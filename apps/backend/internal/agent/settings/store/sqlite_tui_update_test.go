package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestUpdateAgent_PersistsTUIConfig pins the column that UpdateAgent used to
// omit. A custom TUI agent's MCP strategy is editable after creation, and the
// UPDATE statement listed only workspace_id/supports_mcp/mcp_config_path — so
// the edit was accepted, supports_mcp was written, and the strategy it is
// derived from silently reverted on the next read. Round-tripping through the
// real store is the only thing that catches this: a fake repository that
// returns the same pointer it was handed reports success either way.
func TestUpdateAgent_PersistsTUIConfig(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.Agent{
		Name: "fuel-claude",
		TUIConfig: &models.TUIConfigJSON{
			Command:         "fuelclaude",
			DisplayName:     "Fuel Claude",
			WaitForTerminal: true,
		},
	}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	agent.TUIConfig.MCPStrategy = "claude"
	agent.SupportsMCP = true
	if err := repo.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	// Re-read from the database rather than inspecting the struct we just
	// mutated — that distinction is the entire point of this test.
	stored, err := repo.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if stored.TUIConfig == nil {
		t.Fatal("TUIConfig = nil after update")
	}
	if stored.TUIConfig.MCPStrategy != "claude" {
		t.Errorf("MCPStrategy = %q, want %q", stored.TUIConfig.MCPStrategy, "claude")
	}
	if !stored.SupportsMCP {
		t.Error("SupportsMCP = false, want true")
	}
	// The rest of the config must survive the write: UpdateAgent rewrites the
	// whole JSON blob, so a dropped field here would quietly change the command
	// the user launches.
	if stored.TUIConfig.Command != "fuelclaude" {
		t.Errorf("Command = %q, want %q", stored.TUIConfig.Command, "fuelclaude")
	}
	if stored.TUIConfig.DisplayName != "Fuel Claude" {
		t.Errorf("DisplayName = %q, want %q", stored.TUIConfig.DisplayName, "Fuel Claude")
	}
	if !stored.TUIConfig.WaitForTerminal {
		t.Error("WaitForTerminal = false, want true")
	}
}

// TestUpdateAgent_ClearsTUIStrategy pins the off direction, which shares the
// same write path.
func TestUpdateAgent_ClearsTUIStrategy(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.Agent{
		Name:        "toggle-agent",
		SupportsMCP: true,
		TUIConfig: &models.TUIConfigJSON{
			Command:     "toggle-cli",
			DisplayName: "Toggle",
			MCPStrategy: "codex",
		},
	}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	agent.TUIConfig.MCPStrategy = ""
	agent.SupportsMCP = false
	if err := repo.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	stored, err := repo.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if stored.TUIConfig.MCPStrategy != "" {
		t.Errorf("MCPStrategy = %q, want empty", stored.TUIConfig.MCPStrategy)
	}
	if stored.SupportsMCP {
		t.Error("SupportsMCP = true, want false")
	}
}

// TestUpdateAgent_NonTUIAgentKeepsNullConfig guards the nil path: a built-in
// agent has no tui_config and must not gain one.
func TestUpdateAgent_NonTUIAgentKeepsNullConfig(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.Agent{Name: "claude-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	agent.SupportsMCP = true
	if err := repo.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	stored, err := repo.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if stored.TUIConfig != nil {
		t.Errorf("TUIConfig = %+v, want nil", stored.TUIConfig)
	}
}
