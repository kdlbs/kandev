package controller

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

const (
	agentMCPDiscoveryReady       = "ready"
	agentMCPDiscoveryUnavailable = "unavailable"
	agentMCPDiscoveryUnsupported = "unsupported"
	cursorMCPDiscoveryProvider   = "cursor"
	cursorMCPDiscoveryMaxID      = 512
	cursorMCPSourcePlugin        = "plugin"
)

type cursorMCPDiscoverySource struct {
	homeDir            func() (string, error)
	loadInventory      func(context.Context) (mcpconfig.CursorNativeInventory, error)
	discoverCandidates func(string, mcpconfig.CursorNativeInventory) ([]mcpconfig.DiscoveredServerCandidate, error)
	credentialStatus   func(string, []string) map[string]bool
}

func newCursorMCPDiscoverySource() *cursorMCPDiscoverySource {
	return &cursorMCPDiscoverySource{
		homeDir:            cursorMCPHomeDir,
		loadInventory:      mcpconfig.LoadCursorNativeInventory,
		discoverCandidates: mcpconfig.DiscoverCursorPluginCandidatesWithInventory,
		credentialStatus:   mcpconfig.CursorMCPAuthCredentialAvailability,
	}
}

func cursorMCPHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cursor"), nil
}

// MCPDiscoveryProvider resolves the source catalog from the persisted agent
// row and its currently registered runtime strategy.
func (c *Controller) MCPDiscoveryProvider(ctx context.Context, agentID string) (providerID string, supported bool, err error) {
	stored, err := c.repo.GetAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, ErrAgentNotFound
		}
		return "", false, err
	}
	if stored == nil {
		return "", false, ErrAgentNotFound
	}
	if !stored.SupportsMCP || c.agentRegistry == nil {
		return "", false, nil
	}
	registered, exists := c.agentRegistry.Get(stored.Name)
	if !exists || registered == nil {
		return "", false, nil
	}
	if registeredSupportsCursorMCPDiscovery(registered) {
		return cursorMCPDiscoveryProvider, true, nil
	}
	return "", false, nil
}

func registeredSupportsCursorMCPDiscovery(registered agents.Agent) bool {
	if runtime := registered.Runtime(); runtime != nil && isCursorMCPDiscoveryStrategy(runtime.ProjectMCPStrategy) {
		return true
	}
	passthrough, ok := registered.(agents.PassthroughAgent)
	return ok && isCursorMCPDiscoveryStrategy(passthrough.PassthroughConfig().MCPStrategy)
}

func isCursorMCPDiscoveryStrategy(strategy mcpconfig.PassthroughMCPStrategy) bool {
	switch strategy.(type) {
	case mcpconfig.CursorStrategy, *mcpconfig.CursorStrategy:
		return true
	default:
		return false
	}
}

// DiscoverAgentMCP returns a safe host preview resolved for one persisted agent.
// The preview never writes native configuration or changes saved selection.
func (c *Controller) DiscoverAgentMCP(ctx context.Context, agentID string) (*dto.AgentMCPDiscoveryDTO, error) {
	providerID, supported, err := c.MCPDiscoveryProvider(ctx, agentID)
	if err != nil {
		return nil, err
	}
	result := &dto.AgentMCPDiscoveryDTO{
		AgentID: agentID, ProviderID: providerID, Status: agentMCPDiscoveryUnsupported,
		Reason: "unsupported_agent", Servers: []dto.AgentMCPDiscoveryServerDTO{},
	}
	if !supported {
		return result, nil
	}
	source := c.cursorMCPDiscoverySource
	if source == nil {
		source = newCursorMCPDiscoverySource()
	}
	return source.discover(ctx, result), nil
}

func (s *cursorMCPDiscoverySource) discover(ctx context.Context, result *dto.AgentMCPDiscoveryDTO) *dto.AgentMCPDiscoveryDTO {
	cursorHome, err := s.homeDir()
	if err != nil {
		result.Status = agentMCPDiscoveryUnavailable
		result.Reason = "host_home_unavailable"
		return result
	}
	inventory, inventoryErr := s.loadInventory(ctx)
	if inventoryErr != nil {
		inventory = mcpconfig.CursorNativeInventory{}
		result.Status = agentMCPDiscoveryUnavailable
		result.Reason = cursorInventoryReason(inventoryErr)
	} else {
		result.Status = agentMCPDiscoveryReady
		result.Reason = ""
	}
	candidates, err := s.discoverCandidates(cursorHome, inventory)
	if err != nil {
		result.Status = agentMCPDiscoveryUnavailable
		result.Reason = "cursor_discovery_failed"
		return result
	}
	ids := cursorMCPDiscoveryIDs(candidates)
	availability := s.credentialStatus(cursorHome, ids)
	result.Servers = cursorMCPDiscoveryServers(candidates, availability)
	return result
}

func cursorInventoryReason(err error) string {
	var inventoryErr *mcpconfig.CursorInventoryError
	if !errors.As(err, &inventoryErr) {
		return "cursor_inventory_unavailable"
	}
	switch inventoryErr.Reason {
	case "credential_reader_unavailable", "credentials_unavailable", "timeout", "canceled",
		"request_invalid", "request_failed", "service_status", "response_unreadable", "response_too_large", "response_invalid":
		return inventoryErr.Reason
	default:
		return "cursor_inventory_unavailable"
	}
}

func cursorMCPDiscoveryIDs(candidates []mcpconfig.DiscoveredServerCandidate) []string {
	ids := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		id := cursorMCPDiscoveryServerID(candidate)
		if id == "" || len(id) > cursorMCPDiscoveryMaxID {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func cursorMCPDiscoveryServers(candidates []mcpconfig.DiscoveredServerCandidate, credentials map[string]bool) []dto.AgentMCPDiscoveryServerDTO {
	servers := make([]dto.AgentMCPDiscoveryServerDTO, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		id := cursorMCPDiscoveryServerID(candidate)
		name := strings.TrimSpace(candidate.Name)
		sourceKind, ok := cursorMCPDiscoverySourceKind(candidate.SourceKind)
		if !ok || id == "" || len(id) > cursorMCPDiscoveryMaxID || name == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		server := dto.AgentMCPDiscoveryServerDTO{
			ID: id, Name: name, SourceKind: sourceKind,
			CredentialsAvailable: credentials[id],
		}
		if sourceKind == cursorMCPSourcePlugin {
			server.PluginName = strings.TrimSpace(candidate.PluginName)
		}
		servers = append(servers, server)
	}
	return servers
}

func cursorMCPDiscoveryServerID(candidate mcpconfig.DiscoveredServerCandidate) string {
	name := strings.TrimSpace(candidate.Name)
	if candidate.SourceKind == mcpconfig.SourceKindPlugin && strings.TrimSpace(candidate.PluginName) != "" {
		return "plugin-" + strings.TrimSpace(candidate.PluginName) + "-" + name
	}
	return name
}

func cursorMCPDiscoverySourceKind(kind mcpconfig.SourceKind) (string, bool) {
	switch kind {
	case mcpconfig.SourceKindPlugin:
		return cursorMCPSourcePlugin, true
	case mcpconfig.SourceKindGlobal:
		return "global", true
	case mcpconfig.SourceKindProject:
		return "project", true
	case mcpconfig.SourceKindProfile:
		return "profile", true
	default:
		return "", false
	}
}
