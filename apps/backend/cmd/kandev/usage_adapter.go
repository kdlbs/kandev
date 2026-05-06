package main

import (
	"context"
	"os"
	"path/filepath"

	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// usageProviderAdapter implements officeagents.UsageProvider by:
//  1. Looking up the agent profile by ID from the settings store.
//  2. Looking up the agent type from the registry to get its billing type.
//  3. Delegating to the UsageService with the appropriate client registered.
type usageProviderAdapter struct {
	svc           *agentusage.UsageService
	settingsStore settingsstore.Repository
	agentRegistry *agentregistry.Registry
}

// GetUsage implements officeagents.UsageProvider.
func (a *usageProviderAdapter) GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil {
		return nil, nil //nolint:nilerr // profile missing or settings unavailable — fail-open
	}
	ag, ok := a.agentRegistry.Get(profile.AgentID)
	if !ok {
		return nil, nil
	}
	if ag.BillingType() != agentusage.BillingTypeSubscription {
		return nil, nil
	}
	// Ensure client is registered for this profile.
	a.ensureRegistered(profileID, profile.AgentID)
	return a.svc.GetUsage(ctx, profileID)
}

// ensureRegistered creates and registers a usage client for the profile if not already registered.
func (a *usageProviderAdapter) ensureRegistered(profileID, agentName string) {
	// We rely on the fact that GetUsage returns nil,nil for unregistered profiles,
	// so we can call Register without causing duplicate cache entries — the cache key
	// is credential-path-based, not profileID-based, so two profiles with the same
	// credentials share one cache entry.
	//
	// `home` is required for both branches — bail rather than registering a
	// client pointed at a relative ".claude/.credentials.json" / ".codex/auth.json"
	// that silently misses the real file (common in containers or when HOME is
	// unset). Without this guard the consumer sees BillingTypeAPIKey forever.
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	switch agentName {
	case "claude-acp":
		credPath := filepath.Join(home, ".claude", ".credentials.json")
		client := agentusage.NewClaudeUsageClientWithPath(credPath)
		key := agentusage.CacheKey("anthropic", credPath)
		a.svc.Register(profileID, client, key)
	case "codex-acp":
		// Path must match codex_acp.go's SourceFiles / Runtime mounts —
		// the real Codex CLI persists OAuth tokens at ~/.codex/auth.json,
		// not the earlier XDG-style ~/.config/codex/ guess.
		authPath := filepath.Join(home, ".codex", "auth.json")
		client := agentusage.NewCodexUsageClientWithPath(authPath)
		key := agentusage.CacheKey("openai", authPath)
		a.svc.Register(profileID, client, key)
	}
}

// newUsageProviderAdapter creates an adapter and returns it.
func newUsageProviderAdapter(
	settingsStore settingsstore.Repository,
	agentRegistry *agentregistry.Registry,
) *usageProviderAdapter {
	return &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: settingsStore,
		agentRegistry: agentRegistry,
	}
}
