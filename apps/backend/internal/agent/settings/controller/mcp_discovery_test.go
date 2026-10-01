package controller

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestDiscoverAgentMCPReturnsSanitizedCursorCatalog(t *testing.T) {
	st := newFakeStore()
	st.agents["agent-row-1"] = &models.Agent{ID: "agent-row-1", Name: "cursor-acp", SupportsMCP: true}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewCursorACP()); err != nil {
		t.Fatal(err)
	}
	ctrl := NewController(st, nil, reg, nil, log)
	cursorHome := filepath.Join(t.TempDir(), ".cursor")
	candidates := []mcpconfig.DiscoveredServerCandidate{
		{
			Name: "search", SourceKind: mcpconfig.SourceKindPlugin, PluginName: "fixture-plugin",
			Server: agentctltypes.McpServer{
				Name: "unsafe-name", Command: "secret-command", Args: []string{"--token", "secret-token"},
				Env: map[string]string{"API_TOKEN": "secret-token"}, Headers: map[string]string{"Authorization": "secret-token"},
			},
		},
		{Name: "calendar", SourceKind: mcpconfig.SourceKindGlobal, Server: agentctltypes.McpServer{URL: "https://secret.example/mcp"}},
	}
	lookups := 0
	ctrl.cursorMCPDiscoverySource = &cursorMCPDiscoverySource{
		homeDir: func() (string, error) { return cursorHome, nil },
		loadInventory: func(context.Context) (mcpconfig.CursorNativeInventory, error) {
			return mcpconfig.CursorNativeInventory{}, nil
		},
		discoverCandidates: func(home string, _ mcpconfig.CursorNativeInventory) ([]mcpconfig.DiscoveredServerCandidate, error) {
			if home != cursorHome {
				t.Fatalf("Cursor home = %q, want %q", home, cursorHome)
			}
			return candidates, nil
		},
		credentialStatus: func(_ string, ids []string) map[string]bool {
			lookups++
			want := []string{"plugin-fixture-plugin-search", "calendar"}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("credential IDs = %#v, want %#v", ids, want)
			}
			return map[string]bool{ids[0]: true}
		},
	}

	got, err := ctrl.DiscoverAgentMCP(context.Background(), "agent-row-1")
	if err != nil {
		t.Fatalf("DiscoverAgentMCP: %v", err)
	}
	if got.AgentID != "agent-row-1" || got.ProviderID != "cursor" || got.Status != "ready" || len(got.Servers) != 2 || lookups != 1 {
		t.Fatalf("discovery response = %#v, credential scans = %d", got, lookups)
	}
	if got.Servers[0].ID != "plugin-fixture-plugin-search" || got.Servers[0].SourceKind != "plugin" || got.Servers[0].PluginName != "fixture-plugin" || !got.Servers[0].CredentialsAvailable {
		t.Fatalf("plugin preview = %#v", got.Servers[0])
	}
	if got.Servers[1].ID != "calendar" || got.Servers[1].SourceKind != "global" || got.Servers[1].CredentialsAvailable {
		t.Fatalf("global preview = %#v", got.Servers[1])
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-command", "secret-token", "secret.example", "Authorization", "command", "args", "env", "headers", "SourcePath"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("discovery response exposed %q: %s", forbidden, encoded)
		}
	}
}

func TestDiscoverAgentMCPReturnsUnavailableWithClosedReasonAndLocalCandidates(t *testing.T) {
	st := newFakeStore()
	st.agents["agent-row-1"] = &models.Agent{ID: "agent-row-1", Name: "cursor-acp", SupportsMCP: true}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewCursorACP()); err != nil {
		t.Fatal(err)
	}
	ctrl := NewController(st, nil, reg, nil, log)
	ctrl.cursorMCPDiscoverySource = &cursorMCPDiscoverySource{
		homeDir: func() (string, error) { return "/safe/.cursor", nil },
		loadInventory: func(context.Context) (mcpconfig.CursorNativeInventory, error) {
			return mcpconfig.CursorNativeInventory{}, &mcpconfig.CursorInventoryError{Reason: "credentials_unavailable"}
		},
		discoverCandidates: func(_ string, inventory mcpconfig.CursorNativeInventory) ([]mcpconfig.DiscoveredServerCandidate, error) {
			if len(inventory.Plugins) != 0 {
				t.Fatalf("failed inventory unexpectedly retained %d plugins", len(inventory.Plugins))
			}
			return []mcpconfig.DiscoveredServerCandidate{{Name: "local", SourceKind: mcpconfig.SourceKindGlobal}}, nil
		},
		credentialStatus: func(string, []string) map[string]bool { return map[string]bool{"local": true} },
	}
	got, err := ctrl.DiscoverAgentMCP(context.Background(), "agent-row-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "unavailable" || got.Reason != "credentials_unavailable" || len(got.Servers) != 1 || got.Servers[0].ID != "local" {
		t.Fatalf("unavailable preview = %#v", got)
	}
}

func TestMCPDiscoveryProviderRequiresRegisteredCursorStrategy(t *testing.T) {
	st := newFakeStore()
	st.agents["cursor"] = &models.Agent{ID: "cursor", Name: "cursor-acp", SupportsMCP: true}
	st.agents["cursor-tui"] = &models.Agent{ID: "cursor-tui", Name: "custom-cursor-tui", SupportsMCP: true}
	st.agents["other-tui"] = &models.Agent{ID: "other-tui", Name: "custom-other-tui", SupportsMCP: true}
	st.agents["other"] = &models.Agent{ID: "other", Name: "claude-acp", SupportsMCP: true}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewCursorACP()); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []agents.TUIAgentConfig{
		{AgentID: "custom-cursor-tui", AgentName: "Cursor Strategy TUI", Command: "cursor-wrapper", MCPStrategy: mcpconfig.CursorStrategy{}},
		{AgentID: "custom-other-tui", AgentName: "Other Strategy TUI", Command: "other-wrapper", MCPStrategy: mcpconfig.ClaudeStrategy{}},
	} {
		if err := reg.Register(agents.NewTUIAgent(spec)); err != nil {
			t.Fatal(err)
		}
	}
	ctrl := NewController(st, nil, reg, nil, log)
	for _, id := range []string{"cursor", "cursor-tui"} {
		providerID, supported, err := ctrl.MCPDiscoveryProvider(context.Background(), id)
		if err != nil || !supported || providerID != "cursor" {
			t.Errorf("Cursor provider for %q = %q, supported=%v err=%v", id, providerID, supported, err)
		}
	}
	for _, id := range []string{"other", "other-tui"} {
		providerID, supported, err := ctrl.MCPDiscoveryProvider(context.Background(), id)
		if err != nil || supported || providerID != "" {
			t.Errorf("provider for %q = %q, supported=%v err=%v", id, providerID, supported, err)
		}
	}
	if _, _, err := ctrl.MCPDiscoveryProvider(context.Background(), "missing"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("missing agent error = %v, want ErrAgentNotFound", err)
	}
}
